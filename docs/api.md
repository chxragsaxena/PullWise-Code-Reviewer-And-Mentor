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
| 404 | `review_not_found` | unknown review id |
| 404 | `finding_not_found` | unknown finding id for that review |
| 500 | `internal_error` | unexpected failure |

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

## Not in v1
Authentication, private repositories, deleting reviews, pagination beyond `limit`,
a `total` count on lists, Redis, NATS or any job queue. V1 runs reviews in a
background goroutine inside the Go server; a queue comes only when there is a real
need for durable asynchronous workers.
