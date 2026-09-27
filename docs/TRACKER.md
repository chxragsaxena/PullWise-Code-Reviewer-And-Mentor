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
| 2 | LLM: Claude behind `LLMProvider` interface | Structured JSON output only, no prose parsing. **No AI is used anywhere in the product yet.** |
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
- [ ] Frontend against the real backend (`VITE_API_MODE=http`), verify SSE through the Vite proxy, fix any contract mismatches
- [ ] User creates `.env` (copy `.env.example`, choose `POSTGRES_PASSWORD`, matching `DATABASE_URL`, optional `GITHUB_TOKEN`), then `docker compose up -d db` and run the server for real
- [ ] End-to-end test on a real public Go PR with real findings
- [ ] **Then:** Claude pass: `LLMProvider` interface, AI context builder, structured findings, `reviewing` status (currently skipped), needs `ANTHROPIC_API_KEY` in `.env.example`
- [ ] Then: TypeScript, REST and SOLID skills

## Environment
Machine: Windows 11 Home, PowerShell. Workspace: `C:\Users\jyoti\OneDrive\Desktop\LearnCode`.
Git repo: https://github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor (branch `main`, remote `origin`; local git identity chxragsaxena / chiragsaxena79@gmail.com). GitHub login is saved via Git Credential Manager. Only the initial commit is pushed; everything after is local until the user pushes.
- **Installed:** Go 1.27.1, Node 24.21.0 / npm 11.19.0, Git 2.55.0, WSL 2.7.14 (no Ubuntu; Docker backend only, user works on Windows), Docker Desktop 29.8.0.
- **Docker Desktop must be running** for Postgres. If it is closed, relaunch `%LOCALAPPDATA%\Programs\DockerDesktop\Docker Desktop.exe` and wait for the engine. If it hangs on "Starting the Docker Engine", check `%LOCALAPPDATA%\Docker\log\host\com.docker.backend.exe.log` for the first `[E]` line (once it was a stale backend from before WSL was installed: kill Docker Desktop + `com.docker.backend`, relaunch).
- **The user's own database:** `docker-compose.yml` (postgres:18, volume `pgdata`, localhost-only port 5432). Needs the user's `.env` (not created yet; Claude never reads its password).
- **Claude's test database (throwaway):** container `pullwise-test-db`, DB `pullwise_test`, port 55432, random password (get it with `docker inspect pullwise-test-db`). Integration tests run with `TEST_DATABASE_URL`; the store tests wipe the schema and refuse any database whose name does not end in `_test`. Recreate it with `docker run -d --name pullwise-test-db -e POSTGRES_PASSWORD=<random> -e POSTGRES_DB=pullwise_test -p 127.0.0.1:55432:5432 postgres:18` if missing.
- Not installed: Semgrep, Python (Semgrep would run as a container). The Go race detector needs cgo (no C compiler), so `go test -race` is unavailable.
- Terminals/sessions opened before an install don't see new PATH entries; open a fresh one.

## Open questions
- DB password: the user chooses their own; it lives in a git-ignored `.env`.
- Claude model/pricing choice for the AI pass (decide when we reach it; use the claude-api skill).

## Known risks
- Large PR diffs: GitHub truncates big patches and LLM context is finite. Need a chunking / file-by-file strategy before the AI pass. (The pipeline already caps analyzed files at `MaxFilesToAnalyze`=100 and skips files over 2 MB.)
- Unauthenticated GitHub allows 60 requests/hour and each analyzed file costs one request: set `GITHUB_TOKEN` for real use.
- Semgrep on Windows can be awkward; plan to run it in Docker.
- The rate limiter keys on the connection's remote address; behind a reverse proxy all clients would share one bucket.
- Startup `FailInterrupted` assumes a single server instance.
- The `ignored-error` and `http-call-without-context` checks are syntactic heuristics (no `go/types`, since that needs the whole module graph, not one file): `ignored-error` can misfire on a call whose last result genuinely isn't an error (e.g. a `(value, ok)` lookup via a function call); `http-call-without-context` only covers the 4 net/http package functions with no context variant (`Get`/`Post`/`Head`/`PostForm`) and does not attempt a database-call check, which would need type info to avoid false positives on unrelated `Query`/`Exec` methods.
- GitHub API quota used by manual verification during this session: ~40 of 60 unauthenticated requests. Resets hourly. Set `GITHUB_TOKEN` to avoid hitting this during real use.

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
backend/internal/httpapi/    REST + SSE handlers, DTOs, rate limit, middleware
backend/internal/config/     env config + .env loader
backend/migrations/          NNNN_name.sql + embed.go
backend/skills/go/           skill.yaml + lessons/*.md
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
- 2026-09-27: User approved go/ast (stdlib) for all 4 Go checks; Tree-sitter stays reserved for TypeScript, Semgrep for pattern checks; confirmed the async POST contract and partial-PR-metadata behavior. Built `internal/analyzer/goast` (ignored-error, context-not-first, http-call-without-context, high-complexity), import-aware and alias-safe, 19 unit tests all passing. Wired into `cmd/server`. Verified live against real GitHub PRs (go-chi/chi#1, sirupsen/logrus#1): full pipeline works, zero false positives on real code; did not find a live true-positive PR within a reasonable GitHub API budget (true positives are proven by unit tests instead). All Go tests (incl. Postgres integration) and frontend tests/build still green. Not pushed. Next: register the analyzer's output was already done — next real milestone is either a true-positive live demo (optional) or the Claude AI pass / TypeScript+REST+SOLID skills, per the user.
