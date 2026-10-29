# API contract (v1, proposed)

The frontend is built against this contract using a mock client. The Go backend
must implement the same shapes. **Status: approved for V1 (with the corrections below).**
This is the source of truth for frontend and backend: change it here first, then in
`frontend/src/api/types.ts` and the backend.

Conventions:
- JSON everywhere except the SSE stream. Field names are `camelCase`.
- IDs are opaque strings (the backend may stringify database integers).
- Timestamps are RFC 3339 in UTC, e.g. `2026-09-26T10:15:00Z`.
- Errors use one shape for every endpoint:

```json
{ "error": { "code": "invalid_pr_url", "message": "Not a valid GitHub pull request URL." } }
```

| Status | Code | When |
|---|---|---|
| 400 | `invalid_pr_url` | `prUrl` is missing or not a GitHub PR link |
| 400 | `invalid_request` | the body is not valid JSON, or `limit` is not a number |
| 404 | `review_not_found` | unknown review id |
| 404 | `finding_not_found` | unknown finding id for that review |
| 404 | `not_found` | no such endpoint |
| 405 | `method_not_allowed` | wrong HTTP method; the `Allow` header lists the right ones |
| 413 | `request_too_large` | request body over 4 KB |
| 415 | `unsupported_media_type` | POST body is not `application/json` |
| 429 | `rate_limited` | too many reviews started from one address; see `Retry-After` |
| 500 | `internal_error` | unexpected failure |
| 503 | `server_busy` | the review queue is full; see `Retry-After` |
| 503 | `shutting_down` | the server is restarting |
| 503 | `not_ready` | `GET /readyz` only: the database is unreachable |

Other endpoints: `GET /healthz` (liveness) and `GET /readyz` (readiness, checks the
database), both returning `{"status":"ok"}`.

**Review failure codes** (`Review.error.code`, and the last SSE message): `pr_not_found`
(missing or private), `github_rate_limited`, `timeout`, `interrupted` (the server
restarted mid-review), `analysis_failed`, `internal_error`.

**Before the PR has been fetched** (status `queued`, and early in `fetching_pr`),
`pullRequest` holds only `owner`, `repo` and `number`; `title`, `author`, `url` and
`headSha` are empty strings and the counts and stats are 0. Clients should show a
placeholder such as "Pull request #7" until the title arrives. The response to `POST`
is in this state.

## Types

```ts
type ReviewStatus = "queued" | "fetching_pr" | "analyzing" | "reviewing" | "completed" | "failed";
type Severity = "critical" | "high" | "medium" | "suggestion";
type FindingSource = "deterministic" | "ai";

// The same shape as the error body of every failed request, and as Review.error.
// `code` is stable and machine-readable (e.g. "pr_not_found"); `message` is for people.
// (Named ApiErrorBody in the frontend code, where ApiError is the thrown class.)
interface ApiError { code: string; message: string }

interface PullRequest {
  owner: string; repo: string; number: number;
  title: string; author: string; url: string; headSha: string;
  filesChanged: number; additions: number; deletions: number;
}

interface SeverityCounts { critical: number; high: number; medium: number; suggestion: number }

interface ReviewSummary {
  id: string;
  status: ReviewStatus;
  pullRequest: PullRequest;
  counts: SeverityCounts;          // all zero until the review completes
  createdAt: string;
  completedAt: string | null;
}

interface Finding {
  id: string;
  skillId: string; skillName: string;
  checkId: string; checkTitle: string;
  source: FindingSource;
  severity: Severity;
  confidence: number;              // 0..1, always 1 for deterministic findings
  file: string;
  line: number | null;             // null when the finding applies to the whole file
  endLine: number | null;
  evidence: string;                // code excerpt or measurement
  found: string;                   // "What we found"
  why: string;                     // "Why this matters"
  improvement: string;             // "Possible improvement"
  lessonId: string | null;
}

interface Review extends ReviewSummary {
  error: ApiError | null;          // set when status is "failed"
  findings: Finding[];             // empty until completed
}

interface FindingDetail extends Finding {
  lesson: { id: string; markdown: string } | null;   // Markdown is served by the backend
}

// One message on the SSE stream (see the events endpoint).
interface ReviewStatusEvent {
  status: ReviewStatus;
  message: string;
  at: string;
}
```

## Endpoints

### `POST /api/reviews`
Start a review. Body: `{ "prUrl": "https://github.com/owner/repo/pull/123" }`.
Returns **202 Accepted** with a `ReviewSummary` whose status is `queued`.

The handler must only validate the link, create the review record and return. It
must **not** wait for GitHub or the analysis: those run in a background goroutine,
and the client follows them with the events stream.

### `GET /api/reviews`
Recent reviews, newest first. Query: `limit` (default 20, max 50).
Returns `{ "reviews": ReviewSummary[] }`.

### `GET /api/reviews/{id}`
Returns a `Review`. `findings` is empty until the status is `completed`.

### `GET /api/reviews/{id}/findings/{findingId}`
Returns a `FindingDetail`, including the lesson Markdown from the skill's
`lessons/` folder.

### `GET /api/reviews/{id}/events` (Server-Sent Events)
`Content-Type: text/event-stream`. The server sends the current status immediately
on connect, then one event per change:

```
event: status
data: {"status":"analyzing","message":"Running static analysis","at":"2026-09-26T10:15:02Z"}
```

Statuses arrive in this order: `queued`, `fetching_pr`, `analyzing`, `reviewing`,
then `completed`. `failed` can replace any step and carries the reason in `message`.
`reviewing` is the AI pass and is skipped until the AI pass exists.
The server closes the stream after a terminal status (`completed` or `failed`),
including for reviews that are already finished. Clients close their `EventSource`
on a terminal status so the browser does not auto-reconnect.

## Signed-in reviews and privacy (V2)
When GitHub login is configured (see the `/auth/*` routes), a request carrying a
valid session cookie is "signed in":

- `POST /api/reviews` records the signed-in user as the review's **owner** and
  reviews the pull request with **that user's own GitHub token**, so private
  repositories they can access work and their rate limit applies. If their token has
  expired the review still runs, as the owner, with the server's anonymous access
  (public PRs only) until they sign in again.
- A review with an owner is **private**: `GET /api/reviews` lists only the caller's
  own reviews plus anonymous ones, and `GET /api/reviews/{id}`, `/findings/{findingId}`
  and `/events` answer **404** (`review_not_found` / `finding_not_found`, identical
  to a missing one) to anyone else, including anonymous callers.
- Reviews created while signed out have no owner and stay public, exactly as in V1.
- The GitHub token is never stored on the review or returned by any endpoint.

## Your standards (rules a user writes)

Signed-in users can write review rules in plain English; the AI checks their reviews
against them. Every route needs a session (`401 unauthorized` otherwise).

```json
{ "id": "7", "title": "Wrap errors", "instructions": "Return errors with fmt.Errorf and %w.", "severity": "high", "enabled": true }
```

- `GET /api/rules`: `{ "rules": Rule[] }`, oldest first.
- `POST /api/rules`: body is a rule without `id` (`enabled` defaults to true). `201` with the rule.
- `PUT /api/rules/{id}`: replaces the rule. `DELETE /api/rules/{id}`: `204`.
- Limits: title 1 to 120 characters, instructions 1 to 1,000, severity one of
  `critical`, `high`, `medium`, `suggestion`, at most 20 rules per user (`409 limit_reached`).
  Invalid input is `400 invalid_rule`. Someone else's rule is `404 rule_not_found`.
- A review copies the owner's enabled rules when it starts, so later edits never change it.
  Findings from them use skill id `custom` ("Your standards"), take the rule's severity, and
  show the rule's instructions as their lesson.

## Not in v1
Private-repository access (added in V2, above), deleting reviews, sharing a private
review, pagination beyond `limit`,
a `total` count on lists, Redis, NATS or any job queue. V1 runs reviews in a
background goroutine inside the Go server; a queue comes only when there is a real
need for durable asynchronous workers.
