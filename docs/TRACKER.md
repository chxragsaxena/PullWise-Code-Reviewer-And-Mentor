# Project Tracker

> **For Claude: read this file first in every new session/context, and update it
> at the end of every work session or whenever a decision is made.** It is the
> source of truth for project state. Design detail lives in `architecture.md`,
> the API contract in `api.md`.

Last updated: 2026-09-26

## Project in one line
AI-powered GitHub PR reviewer that combines static analysis and packaged
software-engineering skills (REST, SOLID, Go, security, code quality) to find
issues and teach developers *why* their code violates a principle. Learning-oriented;
findings must carry evidence, not LLM opinion.

## Locked decisions
| # | Decision | Notes |
|---|---|---|
| 1 | Languages: Go first, then TypeScript | Skill system is language-aware so others can be added later |
| 2 | LLM: AI pass behind the `ai.Provider` interface | Structured JSON output only, no prose parsing. Two implementations exist: `ClaudeProvider` (built first, needs an Anthropic API key the user doesn't have) and `GroqProvider` (model `openai/gpt-oss-120b`, user's choice after comparing Groq vs Hugging Face free tiers — Groq wins on both speed and a usable free quota; see Session log 2026-10-11). **`main.go` wires in Groq.** No AI call has run live yet (no `GROQ_API_KEY` set). |
| 3 | Semgrep shelled out via `exec.Command`, JSON output | Behind `StaticAnalyzer` interface. Tree-sitter = our own AST analysis, Semgrep = established patterns |
| 4 | First slice is deterministic-only; AI added after it works end to end | |
| 5 | Backend: Go stdlib `net/http` (1.22+ routing), no Chi | Add deps only when they solve a real problem. Deps so far: yaml.v3, pgx |
| 6 | DB: PostgreSQL. No pgvector, Redis or NATS until needed | pgvector at V4 (repo RAG); Redis/NATS when durable async workers are needed |
| 7 | Frontend: React + TypeScript + Vite | |
| 8 | Comms: REST + SSE for review progress | |
| 9 | Skills are files: `skill.yaml` (what to check) + `lessons/*.md` (what to teach) | DB `skills` table only mirrors them for FKs |
| 10 | V1 = public PR URL only, no login | Optional `GITHUB_TOKEN` env var raises the 60 req/hr unauthenticated limit |
| 11 | Deterministic findings always confidence 1.0 | |
| 12 | Findings are teaching units: found / why / improvement / lesson link | |
| 13 | Reviews run on a **fixed worker pool over a bounded FIFO queue** (in-process), not a goroutine per review | `MAX_CONCURRENT_REVIEWS` workers, `MAX_QUEUED_REVIEWS` queue; full queue -> 503 `server_busy` |
| 14 | The server applies migrations itself at startup (embedded, `schema_migrations` table, advisory lock); compose does NOT mount them | Migration files are `backend/migrations/NNNN_name.sql` |
| 15 | Analyzers report only what they observed; **severity, source, confidence, titles and lesson come from the skill definition** | Findings for unknown/AI checks or invalid ones are dropped with a warning |
| 16 | Review completion is one DB transaction (status + findings) | Startup marks leftover in-progress reviews `failed`/`interrupted` |
| 17 | Before GitHub is queried, `pullRequest` has only owner/repo/number; title/author/url/headSha are empty strings | Documented in api.md; frontend shows "Pull request #N". Raised with the user: not yet explicitly confirmed |

## Responsibility split
Code = review engine. YAML = what to check. Markdown = what to teach.
PostgreSQL = what happened. Claude = contextual reasoning (only `kind: ai` checks).

## Frontend decisions (confirmed by user)
- Plain CSS with design tokens (no Tailwind); light + dark following the system, plus a manual toggle.
- Data source: typed API client with a **mock implementation** for now; switch to the real Go API with `VITE_API_MODE=http`. Contract lives in `docs/api.md` (approved).
- V1 screens (all four): Home (PR URL form + recent reviews), Review progress, Results dashboard (summary, severity/skill filters, findings), Finding detail (evidence, explanation, improvement, Markdown lesson).
- React Router; plain typed `fetch`; no state-management or data-fetching library; **SSE for progress from the start (no polling)**; Vitest + React Testing Library; a list-reviews endpoint for recent reviews; Node 24 + npm.
- UI must stay componentized so mock -> real API needs no UI changes (API client injected via React context).
- The user will suggest visual/UX changes to the frontend later.

## Out of scope (do not build unless asked)
Private repos, GitHub App/webhooks, Redis, NATS, repo RAG, auto-fix/rewrite,
custom team skills, >2 languages, agentic architecture, microservices, IDE features.
V2-V6 are not started and need the user's go-ahead.

## Roadmap
- **V1** Public PR URL -> review -> educational dashboard (current)
- V2 GitHub OAuth, private repos
- V3 GitHub App, webhook reviews
- V4 Repository RAG (pgvector)
- V5 Learning system: quizzes, practice, progress
- V6 Custom team standards

Rough progress (2026-09-26): V1 core about 60% by effort; V1 + AI pass + extra skills about 40%; full V1-V6 roadmap about 15%. The infrastructure is solid; the **analyzers that produce findings are not started**.

## V1 vertical slice
Status: `[x]` done and verified, `[~]` partly done, `[ ]` not started

- [x] Project definition, architecture, API contract (`docs/api.md`, approved with corrections), repo + GitHub, toolchain (Go, Node, Git, WSL2, Docker)
- [x] Frontend on a mock API: 4 screens, light/dark, 47 tests, typecheck and build pass. Not yet reviewed visually by Claude (no browser); the user reviews via `npm run dev`
- [x] Domain types + validation (`internal/review`), skill loader (`internal/skills`), language detection (`internal/lang`)
- [x] GitHub client (`internal/github`): PR URL parsing, `FetchPullRequest`, `ListFiles`, `FileContent`, typed errors. **Verified against the live GitHub API** (gorilla/mux#1) via the running server
- [x] PostgreSQL store (`internal/store`): schema, embedded migrations, all read/write operations, tested on real Postgres 18 (8 integration tests, skipped unless `TEST_DATABASE_URL` set)
- [x] Review service (`internal/service`): create-and-return-immediately, worker pool + bounded queue, per-review timeout, panic recovery, error-code mapping, in-memory event broker, graceful `Close`
- [x] HTTP layer (`internal/httpapi`): all endpoints from api.md, SSE stream with keep-alive, JSON errors for 404/405, body limit + Content-Type check, per-IP rate limit on POST, security headers, `/healthz` + `/readyz`
- [x] Config + `.env` loader + `cmd/server` wiring with ordered graceful shutdown
- [x] **End to end smoke test passed** (real server + real Postgres + real GitHub): queued -> fetching_pr -> analyzing -> completed over SSE, real PR metadata saved, failed review for a missing PR maps to `pr_not_found`, crash recovery marks leftovers `interrupted`
- [x] **Analyzers**: `internal/analyzer/goast` implements all 4 Go deterministic checks with `go/parser`/`go/ast` (user-approved: stdlib for Go, Tree-sitter reserved for TypeScript, Semgrep for pattern checks later). Import-aware (`context`/`net/http` resolved by actual import path, alias-safe, dot/blank-import-safe). 19 unit tests: true positives with exact line/evidence assertions for all 4 checks, plus explicit false-positive guards (map comma-ok, type-assertion comma-ok, two-value non-call assign, non-stdlib `context` package, local function named `Get`), a broken-syntax file that's skipped without failing the review, cancellation, and a check that the analyzer never sets severity/source/confidence/lesson (the service fills those from the skill).
- [x] Registered in `cmd/server/main.go`; the complexity threshold is read from `skills/go/skill.yaml` so the two cannot drift apart.
- [x] **Live verification against real GitHub PRs** (go-chi/chi#1, sirupsen/logrus#1): pipeline runs end to end (fetch -> analyze -> persist -> SSE -> completed) with **zero false positives** on real idiomatic Go code — confirmed by reading the actual fetched source, not assumed. A genuine **true-positive demo on a real PR was not obtained**: several PRs were scanned via their diffs (using up GitHub API quota, ~20/60 left) without finding one that trips these specific 4 checks in code that survived to be merged (expected — merged code from popular repos is usually clean). True-positive detection is proven instead by the unit tests, which is the appropriate place for it (deterministic, no network flakiness). Revisit with a `GITHUB_TOKEN` set (5000/hr) if a live true-positive demo is wanted later.
- [x] User created `.env` and it now works: `docker compose up -d db` (the user's real Postgres, container `learncode-db-1`) + `go run ./cmd/server` connect, migrate and serve successfully. (Took two rounds: the user's first two attempts had `DATABASE_URL`'s password not exactly matching `POSTGRES_PASSWORD` — Claude's own verification script was too loose the first time, "contains" instead of exact match; fixed and now checks exact equality without ever displaying the value.)
- [x] Frontend against the real backend: `VITE_API_MODE=http` verified through the Vite dev-server proxy (`POST /api/reviews`, `GET /api/reviews/{id}`, `GET /api/reviews?limit=`) all round-tripped correctly to the real server and real database.
- [~] End-to-end test on a real public Go PR: pipeline verified fully working (go-chi/chi#1) but that PR has **zero findings**, correctly — confirmed by reading its actual source (see the analyzer entry above and known risks). Not yet seen with a **non-zero** finding on a live PR; not blocking, see risk note below.
- [x] **AI pass built, two providers**: `internal/ai` has a shared `Provider` interface, prompt builder (per-check lesson text + diff budget) and JSON schema (`schema.go`, shared by both providers). `ClaudeProvider` (official Go SDK, `claude-sonnet-5`, forced strict tool use) built first; `GroqProvider` (plain `net/http`, model `openai/gpt-oss-120b`, `response_format: json_schema` with `strict: true` — constrained decoding, not best-effort) added after the user asked to compare Groq vs Hugging Face free tiers and use whichever is faster. Wired into `internal/service`: `StatusReviewing` runs between analyzing and completed when an `AIProvider` is configured and a selected skill has a `kind: ai` check; AI confidence is kept as the model reported it (not forced to 1.0); a failed AI call is logged and degrades to deterministic-only findings rather than failing the whole review. **`cmd/server/main.go` constructs `ai.NewGroqProvider` from `GROQ_API_KEY`** (added to config + `.env.example`); nil/disabled with a warning if unset, matching the no-analyzers-registered pattern from earlier. 30 `internal/ai` tests (fake HTTP servers for both providers' real wire shapes, no key needed for either) + 9 `internal/service` tests. All green, `gofmt`/`go vet`/build clean. **Blocked on the user's `GROQ_API_KEY`** (free, console.groq.com/keys) — nothing has called a real AI API yet. `ANTHROPIC_API_KEY` still not in `.env.example`; add it if the user ever gets Claude API access and wants to switch `main.go` back.
- [x] **SOLID and REST skills built and live-verified.** `skills/solid/` (5 checks: SRP, OCP, LSP, ISP, DIP) and `skills/rest/` (6 checks: HTTP method mismatch, wrong status code, non-resource-oriented URL, missing idempotency, inconsistent error shape, unnecessary network calls) — all `kind: ai` (these are design judgments, not syntax patterns, so no new analyzer code was needed; the existing per-skill AI pipeline picked them up with zero Go changes). Moved the Go skill's `interface-too-large` check into `solid` as ISP (was duplicating the same concept under two skill IDs); fixed two Go skill checks mislabeled `analyzer: treesitter`/`semgrep` that are actually `goast` (added `AnalyzerGoAST` to `internal/skills`). **Verified live**: a hand-written file with real SRP, DIP and status-code issues got back three correct, specific findings from real Groq calls — confirmed quality, not just plumbing. A Go PR now triggers up to 3 Groq calls (go, solid, rest skills each called separately) instead of 1 — still comfortably inside Groq's free tier (30 RPM), but worth knowing; batching all skills' AI checks into one call is a possible future optimization, not done.
- [x] **TypeScript analyzer and skill built.** `internal/analyzer/tsast` (new dependency: `github.com/smacker/go-tree-sitter` + its `typescript`/`tsx` grammar subpackages; `.tsx` files parsed with the TSX grammar for JSX, `.ts`/`.mts`/`.cts` with plain TypeScript — picked by file extension since `lang.Detect` treats them as one language). Node/field names for every check were verified empirically (a throwaway AST-dump program, not guessed) before writing any check logic — same discipline as `go doc` for the Anthropic SDK. 6 deterministic checks, each deliberately mirroring one of the Go skill's checks in spirit: `loose-equality` (idiomatic-correctness), `any-type` (type-safety escape hatch), `console-log-left-in`, `unhandled-promise` (parallels `ignored-error` — same "syntactic heuristic" caveat, documented), `fetch-without-abort-signal` (parallels `http-call-without-context` — missing cancellation), `high-complexity` (same definition as Go's, ported to TS node types). `skills/typescript/` + 6 lessons. Added `AnalyzerTSAST` to `internal/skills`. Wired into `cmd/server/main.go` alongside `goast`. 17 new tests, all passing (true positives, false-positive guards — e.g. a non-console `.log()`, a `.then().catch()` chain, a non-literal fetch options argument — parse-error and empty-file skip, cancellation, TSX/JSX parsing, and the "analyzer doesn't set severity/source/confidence" invariant). **Live-verified**: a hand-written file with 5 deliberate issues got back exactly 5 correct findings from the real analyzer.

## Environment
Machine: Windows 11 Home, PowerShell. Workspace: `C:\Users\jyoti\OneDrive\Desktop\LearnCode`.
Git repo: https://github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor (branch `main`, remote `origin`; local git identity chxragsaxena / chiragsaxena79@gmail.com). GitHub login is saved via Git Credential Manager. Only the initial commit is pushed; everything after is local until the user pushes.
- **Installed:** Go 1.27.1, Node 24.21.0 / npm 11.19.0, Git 2.55.0, WSL 2.7.14 (no Ubuntu; Docker backend only, user works on Windows), Docker Desktop 29.8.0.
- **Docker Desktop must be running** for Postgres. If it is closed, relaunch `%LOCALAPPDATA%\Programs\DockerDesktop\Docker Desktop.exe` and wait for the engine. If it hangs on "Starting the Docker Engine", check `%LOCALAPPDATA%\Docker\log\host\com.docker.backend.exe.log` for the first `[E]` line (once it was a stale backend from before WSL was installed: kill Docker Desktop + `com.docker.backend`, relaunch).
- **The user's own database: working.** `.env` exists (git-ignored; Claude never reads its password, only validates exact-match/shape via script). `docker compose up -d db` -> container `learncode-db-1`, volume `pgdata`, localhost-only port 5432. The Go server connects, migrates and serves against it.
- **Claude's test database (throwaway):** container `pullwise-test-db`, DB `pullwise_test`, port 55432, random password (get it with `docker inspect pullwise-test-db`). Integration tests run with `TEST_DATABASE_URL`; the store tests wipe the schema and refuse any database whose name does not end in `_test`. Recreate it with `docker run -d --name pullwise-test-db -e POSTGRES_PASSWORD=<random> -e POSTGRES_DB=pullwise_test -p 127.0.0.1:55432:5432 postgres:18` if missing.
- **C compiler: installed** (2026-10-11, WinLibs GCC 16.2.0 via winget, `BrechtSanders.WinLibs.POSIX.UCRT`). `CGO_ENABLED=1` is now Go's default on this machine; `go test -race` is available if ever needed.
- Not installed: Semgrep, Python (Semgrep would run as a container).
- Terminals/sessions opened before an install don't see new PATH entries; open a fresh one.

## Open questions
- **AI pass verified live** (2026-10-11): real review of go-chi/chi#1 went queued -> fetching_pr -> analyzing -> **reviewing** -> completed over SSE, a real Groq call (~2s latency, no error/warning logged) correctly found nothing wrong with the one goroutine in that PR's diff (it has a proper `ctx.Done()` case, confirmed by reading the source last session). Have not yet seen a real **positive** AI finding on a live PR — same situation as the deterministic analyzer: searched several repos' diffs for a goroutine pattern, found none within a reasonable GitHub-quota budget, stopped rather than keep spending it. True-positive behavior for the AI checks is not unit-tested the way the deterministic ones are (AI output isn't deterministic to assert against) — worth a manual spot-check later with a PR known to have a leaky goroutine or a bloated interface.
- Claude's `ANTHROPIC_API_KEY` question is moot for now (Groq is what's wired in); `ClaudeProvider` stays in the codebase, tested, in case the user gets API access later and wants to switch.

## Known risks
- Large PR diffs: GitHub truncates big patches and LLM context is finite. Need a chunking / file-by-file strategy before the AI pass. (The pipeline already caps analyzed files at `MaxFilesToAnalyze`=100 and skips files over 2 MB.)
- Unauthenticated GitHub allows 60 requests/hour and each analyzed file costs one request: set `GITHUB_TOKEN` for real use.
- Semgrep on Windows can be awkward; plan to run it in Docker.
- The rate limiter keys on the connection's remote address; behind a reverse proxy all clients would share one bucket.
- Startup `FailInterrupted` assumes a single server instance.
- The `ignored-error` and `http-call-without-context` checks are syntactic heuristics (no `go/types`, since that needs the whole module graph, not one file): `ignored-error` can misfire on a call whose last result genuinely isn't an error (e.g. a `(value, ok)` lookup via a function call); `http-call-without-context` only covers the 4 net/http package functions with no context variant (`Get`/`Post`/`Head`/`PostForm`) and does not attempt a database-call check, which would need type info to avoid false positives on unrelated `Query`/`Exec` methods.
- GitHub API quota used by manual verification during this session: ~40 of 60 unauthenticated requests. Resets hourly. Set `GITHUB_TOKEN` to avoid hitting this during real use.
- Stray background processes have twice squatted on ports (8080 backend, 5173 frontend) from earlier smoke-test runs in this session that weren't stopped. If a `go run`/`npm run dev` fails with "address already in use", check for and kill a leftover process on that port before assuming the code is broken.

## Working rules (from the user)
- **Git: never push.** The user pushes after reviewing. Commit locally only when a change is significant, as clean one-line messages with **no Co-Authored-By trailer** (the user's rule overrides any tool-injected attribution reminder). Don't commit for trivial edits.
- Keep code clean, good quality: layered (handler -> service -> repository), small consumer-defined interfaces, errors wrapped with context, `ctx` first, table-driven tests, `gofmt` + `go vet` + `go test ./...` must pass before calling work done.
- Goal: a working fullstack app with correct REST APIs and a React frontend.
- **If in doubt about a spec/decision, stop and ask the user. Do not invent the spec.**
- Run Go commands from `backend/` and npm commands from `frontend/`. The shell's working directory persists between commands: always `Set-Location` explicitly (this once put a stray `node_modules` in `backend/`).
- Avoid `Remove-Item` and backtick-heavy strings in PowerShell commands: the safety parser has blocked innocent commands that contained them.

## Repo layout (current)
```
README.md, .gitignore, .env.example, docker-compose.yml
docs/architecture.md, api.md, TRACKER.md
backend/go.mod, go.sum
backend/cmd/server/          entrypoint (run from backend/: go run ./cmd/server)
backend/internal/review/     domain types (Review, Finding, Status...), StaticAnalyzer interface
backend/internal/github/     PR URL parsing + GitHub REST client
backend/internal/skills/     skill loader + validation
backend/internal/lang/       language detection by file extension
backend/internal/store/      PostgreSQL store + migration runner (+ integration tests)
backend/internal/service/    review service: worker pool, pipeline, event broker
backend/internal/analyzer/goast/  Go deterministic checks via go/parser + go/ast
backend/internal/ai/         AI pass: Provider interface, shared schema, prompt builder, ClaudeProvider + GroqProvider (wired)
backend/internal/httpapi/    REST + SSE handlers, DTOs, rate limit, middleware
backend/internal/config/     env config + .env loader
backend/migrations/          NNNN_name.sql + embed.go
backend/internal/analyzer/tsast/  TypeScript deterministic checks via tree-sitter (go-tree-sitter)
backend/skills/go/           skill.yaml + lessons/*.md (error handling, context, concurrency)
backend/skills/typescript/   skill.yaml + lessons/*.md (loose-equality, any-type, console-log, unhandled-promise, abort-signal, high-complexity)
backend/skills/solid/        skill.yaml + lessons/*.md (srp, ocp, lsp, isp, dip)
backend/skills/rest/         skill.yaml + lessons/*.md (http-methods, status-codes, resource-urls, idempotency, error-responses, network-calls)
backend/skills/system_prompt.md  the AI reviewer's editable system prompt (falls back to a built-in default if missing)
frontend/                    React + TS + Vite app (run from frontend/: npm run dev | test | build)
frontend/src/api/            ApiClient interface, mock + http clients, ApiProvider (context)
frontend/src/lib/            pure logic: prUrl parsing, findings filter/group, formatting, theme
frontend/src/hooks/          useAsync, useReviewProgress (SSE)
frontend/src/components/     UI pieces, each with its own .css
frontend/src/pages/          Home, Review (progress + results), Finding, NotFound
frontend/src/styles/         design tokens (light/dark) and base styles
```

## Session log
- 2026-09-26: Defined the project and scope; locked decisions; schema, Go skill + lessons, architecture doc, this tracker.
- 2026-09-26: Installed/verified Go, Node, Git, WSL2, Docker (fixed a stuck Docker engine). Repo on GitHub, first commit pushed. Split into `backend/` and `frontend/`.
- 2026-09-26: Built `internal/review`, `github` (URL parser + client), `skills`; HTTP skeleton; API contract approved with corrections; full React frontend on a mock API.
- 2026-09-26: Built the rest of the backend: PostgreSQL store with embedded migrations (revised schema), review service with worker pool and event broker, all REST + SSE endpoints, config/.env loader, wiring with graceful shutdown. A test found a real design weakness (unordered goroutines) -> worker pool. Another found a routing gap (POST /healthz -> 404). End-to-end smoke test on live GitHub + real Postgres passed. All Go and frontend tests, vet, gofmt, build green. No analyzers yet, so reviews have zero findings. Not pushed.
- 2026-09-27: User approved go/ast (stdlib) for all 4 Go checks; Tree-sitter stays reserved for TypeScript, Semgrep for pattern checks; confirmed the async POST contract and partial-PR-metadata behavior. Built `internal/analyzer/goast` (ignored-error, context-not-first, http-call-without-context, high-complexity), import-aware and alias-safe, 19 unit tests all passing. Wired into `cmd/server`. Verified live against real GitHub PRs (go-chi/chi#1, sirupsen/logrus#1): full pipeline works, zero false positives on real code; did not find a live true-positive PR within a reasonable GitHub API budget (true positives are proven by unit tests instead). All Go tests (incl. Postgres integration) and frontend tests/build still green. Not pushed.
- 2026-09-27 (later): User filled in `.env` and brought up the real database. Two password-mismatch round trips before it worked (Claude's exact-match check needed fixing). Killed stale leftover server/frontend processes from earlier in the session that were squatting on ports 8080/5173. Verified the full stack live against the user's real Postgres: `docker compose up -d db` + `go run ./cmd/server` + `npm run dev` (VITE_API_MODE=http) all working together, a review round-tripped through the Vite proxy to the real backend and database. User asked about Claude integration: none exists yet (confirmed). User wants Claude Sonnet 5 specifically for the AI pass; clarified a Claude Max subscription doesn't cover API billing — a separate API key is needed. Not pushed.
- 2026-09-27/28 (user said "do the most you can without my help"): Built the full Claude AI pass (`internal/ai` + wiring into `internal/service`) using the real Go SDK — read exact type shapes via `go doc` against the installed module rather than guessing, which paid off (compiled clean first try). 19 new tests, all passing, no API key needed (fake HTTP server standing in for Anthropic). Committed locally, not pushed.
- 2026-10-11 (session resumed after a gap; Docker Desktop was closed, old background processes from the previous session had died): Re-verified everything still builds and all tests still pass (backend incl. the AI pass work, frontend). Committed the AI pass work that was sitting uncommitted. Updated this tracker. Still blocked on the user's `ANTHROPIC_API_KEY`; also still need to wire `ai.NewClaudeProvider` into `cmd/server/main.go` (the package was built but never connected to the entrypoint). Not pushed — 15 commits ahead of origin/main.
- 2026-10-11 (later): User asked whether Groq or Hugging Face open models could replace Claude, for cost reasons (no Anthropic API access). Researched both live (WebSearch/WebFetch against official docs, not SEO blogs): Groq's free tier (14,400 req/day, no card, ~470-480 tok/s on `openai/gpt-oss-120b`, strict JSON-schema structured output via constrained decoding) clearly beats Hugging Face Inference Providers' free tier ($0.10/month credit) — and HF can itself route to Groq or Cerebras as a backend, so the two aren't really separate options. Built `internal/ai/groq.go` (plain net/http, no new SDK dependency) alongside the existing Claude provider — refactored the shared JSON schema into `schema.go` so both reuse it. Wired `ai.NewGroqProvider` into `cmd/server/main.go` via a new `GROQ_API_KEY` config value (also wired Claude's analogous gap in passing). 11 new Groq tests + schema refactor, all green. Not pushed.
- 2026-10-11 (even later): User pasted a real Groq key directly in chat — treated as compromised per the same rule as the Postgres password; told them to revoke it, generate a new one, and put it in `.env` themselves. Once confirmed present (safely, by shape/length only), relaunched Docker Desktop (closed again after the gap), brought the existing `learncode-db-1` container back up, ran the real server. **First live Groq call succeeded**: a real review went through `reviewing` for the first time ever, no errors, correctly found nothing wrong with a goroutine already known (from reading its source) to be safe. Did not find a live true-positive AI finding within a reasonable GitHub-quota budget; stopped rather than keep hunting, same call as for the deterministic analyzer earlier. To actually demonstrate a positive finding without publishing anything to GitHub (an outward-facing action), ran the real `goast` analyzer and real Groq provider directly against a hand-written file with deliberate issues (outside the repo, via a temporary `cmd/scratchdemo`, deleted after) — got back correct, well-reasoned findings for `interface-too-large` and `goroutine-leak-risk`, proving the mechanism and the quality both work. Server stopped; database left running. Not pushed.
- 2026-10-11 (still later): User asked to be able to write their own AI prompt. Moved the system prompt out of a Go constant into `skills/system_prompt.md` (editable like any skill/lesson file, falls back to a built-in default if missing/blank) — `LoadSystemPrompt` + both providers updated, 4 new tests, re-verified live that the file-loaded prompt reaches a real Groq call correctly. Then built the SOLID and REST skills (content only, zero new Go code — the AI pipeline was already skill-agnostic), moved the Go skill's overlapping ISP-shaped check into `solid`, fixed two mislabeled analyzer tags. Verified live with a hand-written file exercising SRP, DIP and a REST status-code issue — three real, correct, specific findings came back from real Groq calls. All temporary demo code deleted, nothing left uncommitted.
- 2026-10-11 (final, before pushing): User asked where to get a C compiler (to unblock TypeScript/Tree-sitter). Found `winget` was already present; installed WinLibs GCC in one command. Verified with a real cgo build-and-run (not just `CGO_ENABLED`). User asked to complete TypeScript too before pushing everything. Built `internal/analyzer/tsast` + `skills/typescript/` as described above — verified every tree-sitter node/field name empirically first, which paid off (clean build on the first try, same as the Anthropic SDK work). All 3 analyzers (goast, tsast) + 3 AI skills (go, solid, rest) + typescript skill now coexist; full backend test suite green. **User's plan: push all 21 (now 23) local commits once this is done.** Not yet pushed at the time of this entry.
