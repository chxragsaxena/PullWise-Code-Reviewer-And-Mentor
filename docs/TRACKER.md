# Project Tracker

> **For Claude: read this file first in every new session/context, and update it
> at the end of every work session or whenever a decision is made.** It is the
> source of truth for project state. Design detail lives in `architecture.md`.

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
| 2 | LLM: Claude behind `LLMProvider` interface | Structured JSON output only, no prose parsing |
| 3 | Semgrep shelled out via `exec.Command`, JSON output | Behind `StaticAnalyzer` interface. Tree-sitter = our own AST analysis, Semgrep = established patterns |
| 4 | First slice is deterministic-only; AI added after it works end to end | |
| 5 | Backend: Go stdlib `net/http` (1.22+ routing), no Chi | Add deps only when they solve a real problem |
| 6 | DB: PostgreSQL. No pgvector, Redis or NATS until needed | pgvector at V4 (repo RAG); Redis/NATS when async workers are needed |
| 7 | Frontend: React + TypeScript + Vite | |
| 8 | Comms: REST + SSE for review progress | |
| 9 | Skills are files: `skill.yaml` (what to check) + `lessons/*.md` (what to teach) | DB `skills` table only mirrors them for FKs |
| 10 | V1 = public PR URL only, no login | Server-side GitHub token in env var to avoid the 60 req/hr unauth limit |
| 11 | Deterministic findings always confidence 1.0 | |
| 12 | Findings are teaching units: found / why / improvement / lesson link | |

## Responsibility split
Code = review engine. YAML = what to check. Markdown = what to teach.
PostgreSQL = what happened. Claude = contextual reasoning (only `kind: ai` checks).

## Frontend decisions (confirmed by user)
- Plain CSS with design tokens (no Tailwind); light + dark following the system, plus a manual toggle.
- Data source: typed API client with a **mock implementation** for now; switch to the real Go API by config. Contract lives in `docs/api.md` (proposed, pending user approval; backend must follow it).
- V1 screens (all four): Home (PR URL form + recent reviews), Review progress, Results dashboard (summary, severity/skill filters, findings), Finding detail (evidence, explanation, improvement, Markdown lesson).
- React Router; plain typed `fetch`; no state-management or data-fetching library; **SSE for progress from the start (no polling)**; Vitest + React Testing Library; a list-reviews endpoint for recent reviews; Node 24 + npm.
- UI must stay componentized so mock -> real API needs no UI changes (API client injected via React context).

## Out of scope (do not build unless asked)
Private repos, GitHub App/webhooks, Redis, NATS, repo RAG, auto-fix/rewrite,
custom team skills, >2 languages, agentic architecture, microservices, IDE features.

## Roadmap
- **V1** Public PR URL -> review -> educational dashboard (current)
- V2 GitHub OAuth, private repos
- V3 GitHub App, webhook reviews
- V4 Repository RAG (pgvector)
- V5 Learning system: quizzes, practice, progress
- V6 Custom team standards

## V1 vertical slice, in order
Status: `[x]` done, `[~]` written but unverified, `[ ]` not started

- [x] Project definition and architecture agreed
- [~] Postgres schema: `migrations/0001_init.sql` (not run against a DB)
- [~] Go skill: `skills/go/skill.yaml` + 4 lessons (error-handling, context, concurrency, interfaces)
- [~] `docs/architecture.md`
- [x] Git repo created and pushed to GitHub (initial commit b344776); `.gitignore` and README added
- [x] Install toolchain (see Environment): Go, Node, Git, WSL, Docker engine all working (Semgrep to run as a container later)
- [x] `docs/api.md`: API contract v1, **approved by user** with corrections applied: `Review.error` is `{code, message}`; `Finding.line` is nullable (whole-file findings; Go domain uses 0 = whole file, DB column nullable); SSE event type is `ReviewStatusEvent`; no `total` field; `POST /api/reviews` must return 202 immediately and never block on GitHub/analysis (background goroutine). No auth/Redis/NATS/queue in V1.
- [x] Frontend (React + TS + Vite) built against a mock API: Home, Review progress (SSE-style stream), Results dashboard, Finding detail with Markdown lesson; light/dark theme. `npm run typecheck`, `npm test` (47 tests), `npm run build` all pass. Not yet visually reviewed in a browser by Claude (no browser access); user should run `npm run dev` in `frontend/` and review.
- [ ] Frontend: switch to real backend (`VITE_API_MODE=http`) once the Go endpoints exist; verify SSE against the real server
- [~] `docker-compose.yml` (postgres:18, named volume `pgdata` mounted at `/var/lib/postgresql`, localhost-only port, healthcheck, migrations from `backend/migrations` run on first init) + `.env.example`. Compose parses and refuses to start without `POSTGRES_PASSWORD`. **Blocked on the user creating `.env`** (copy `.env.example`, choose a password; keep `DATABASE_URL` in sync). Migration not yet applied. Changing a migration after first init needs `docker compose down -v`.
- [x] `backend/go.mod` (module `github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor`, matches the GitHub repo, confirmed by user); layout `backend/`, `frontend/` per user
- [~] Interfaces: `StaticAnalyzer` done in `internal/review`; `LLMProvider` not yet
- [~] Skill loader done in `internal/skills` (strict validation, tests pass); syncing to the `skills` table not yet
- [x] GitHub client in `internal/github`: `ParsePRURL`; `Client` with `FetchPullRequest`, `ListFiles` (paginated, 3000-file cap), `FileContent` (raw at a commit, 2 MB cap); typed errors (`ErrNotFound`, `ErrRateLimited`, `ErrTooLarge`, `*APIError`); optional token via `GITHUB_TOKEN`. Tested against httptest servers only, **not yet against the live GitHub API**; do that in the end-to-end step.
- [~] `cmd/server` + `internal/httpapi` + `internal/config`: server, graceful shutdown, recover/log middleware, standard JSON error body, `GET /healthz` done and smoke-tested; review endpoints not yet
- [ ] Language detection from changed files
- [ ] Skill selection
- [ ] Analyzers: metric (cyclomatic complexity), tree-sitter (context-not-first), Semgrep (ignored-error, http-call-without-context) + add Semgrep rule files and `rule:` refs to skill.yaml
- [ ] Persist review + findings (transactional: review completes and findings save together)
- [ ] `POST /api/reviews`, `GET /api/reviews/{id}`, SSE progress endpoint
- [ ] React dashboard (PR summary, findings by severity, finding detail with lesson)
- [ ] End-to-end test on a real public Go PR
- [ ] **Then:** Claude pass (AI context builder -> Claude -> structured findings -> aggregator) for `kind: ai` checks
- [ ] Then: TypeScript skill, REST skill, SOLID skill

## Environment
Machine: Windows 11 Home, PowerShell. Workspace: `C:\Users\jyoti\OneDrive\Desktop\LearnCode`.
Git repo: https://github.com/chxragsaxena/PullWise-Code-Reviewer-And-Mentor (branch `main`, remote `origin`; local git identity chxragsaxena / chiragsaxena79@gmail.com). GitHub login is saved via Git Credential Manager. The push must be run by the user from a terminal (Claude's shell cannot show login prompts, but plain `git push` should work now that credentials are saved).
As of 2026-09-26:
- **Installed:** Go 1.27.1, Node 24.21.0 / npm 11.19.0, Git 2.55.0, WSL 2.7.14 (no Ubuntu; Docker backend only, user wants to work on Windows).
- **Docker Desktop 29.8.0: engine running** (client/server 29.8.0, `docker-desktop` WSL2 distro running). It had been stuck because the backend process started before WSL was installed and never rechecked; fixed by fully killing Docker Desktop + `com.docker.backend` and relaunching. If it hangs again on "Starting the Docker Engine", check `%LOCALAPPDATA%\Docker\log\host\com.docker.backend.exe.log` for the first `[E]` line.
- **Decision:** database runs in Docker (`postgres:18`), not a native install. The user manually removed the broken native PostgreSQL install.
- **Not installed:** Semgrep, Python (Semgrep will run as a container instead).
- Terminals/sessions opened before an install don't see new PATH entries; open a fresh one.

## Open questions
- Go-specific analyzers: `go/ast` (stdlib) can do context-not-first and complexity natively; Tree-sitter would then only be needed for TypeScript. Ask the user before deviating from "Tree-sitter for Go".
- DB password: user wants to choose their own (not `dev`); keep it in a git-ignored `.env`, read by compose and the Go server via `DATABASE_URL`.
- Claude model/pricing choice for the AI pass (decide when we reach it; use claude-api skill)

## Known risks
- Large PR diffs: GitHub truncates big patches and LLM context is finite. Need a chunking / file-by-file strategy before the AI pass.
- Semgrep on Windows can be awkward; may need WSL or Docker.
- The SQL migration has not been run against a real database yet.

## Working rules (from the user)
- **Git: never push.** The user pushes after reviewing. Commit locally only when a change is significant, as clean one-line messages with no Co-Authored-By trailer. Don't commit for trivial edits.
- Keep code clean, good quality: layered (handler -> service -> repository), small interfaces, errors wrapped with context, `ctx` first, table-driven tests, `gofmt` + `go vet` + `go test ./...` must pass before calling work done.
- Goal right now: a working fullstack app with correct REST APIs and a React frontend.
- **If in doubt about a spec/decision, stop and ask the user. Do not invent the spec.**
- Run Go commands from `backend/` and npm commands from `frontend/`. Two top-level folders: `backend/` and `frontend/`. (The shell's working directory persists between commands: always `Set-Location` explicitly or npm/go will run in the wrong folder. This already caused one stray `node_modules` in `backend/`, since cleaned.)

## Repo layout (current)
```
README.md, .gitignore
docs/architecture.md, docs/TRACKER.md
backend/go.mod, go.sum
backend/internal/review/     domain types (Finding, Severity), StaticAnalyzer interface
backend/internal/github/     PR URL parsing
backend/internal/skills/     skill loader + validation
backend/internal/config/     env config (ADDR, SKILLS_DIR)
backend/internal/httpapi/    router, middleware, JSON/error helpers
backend/cmd/server/          entrypoint (run from backend/: go run ./cmd/server)
backend/migrations/0001_init.sql
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
- 2026-09-26: Defined project and scope through discussion; locked the 4 decisions above. Wrote schema, Go skill + lessons, architecture doc, this tracker. No code executed (no toolchain installed).
- 2026-09-26 (later): Installed/verified Go, Node, Git, WSL2; Docker Desktop installed but engine not yet running. Project named PullWise. Repo pushed to GitHub. Next: get Docker engine running, write compose file, then Go module + interfaces + skill loader + GitHub client.
- 2026-09-26 (later still): Split into `backend/` and `frontend/`. Wrote `internal/review`, `internal/github` (URL parser), `internal/skills` (loader) with tests; all pass, gofmt/vet clean. A test caught a loader bug (missing lesson file was silently skipping a skill); fixed. Module renamed to match the GitHub repo. Committed locally (not pushed).
