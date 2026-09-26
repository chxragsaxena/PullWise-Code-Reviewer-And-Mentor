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
- [~] Install toolchain (see Environment): Go done; Node, Postgres, Semgrep pending
- [ ] `go.mod`, project layout (`cmd/server`, `internal/...`)
- [ ] Interfaces: `StaticAnalyzer`, `LLMProvider`
- [ ] Skill loader (parse `skills/*/skill.yaml`, sync to `skills` table)
- [ ] GitHub client: parse PR URL, fetch PR metadata + diff + changed files
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
Machine: Windows 11, PowerShell. Workspace: `C:\Users\jyoti\OneDrive\Desktop\LearnCode` (not a git repo).
As of 2026-09-26:
- **Installed:** Go 1.27.1 at `C:\Program Files\Go\bin` (on system PATH; a session started before the install won't see it, so use the full path or `$env:Path += ";C:\Program Files\Go\bin"`).
- **Installed:** Node 24.21.0 / npm 11.19.0.
- **PostgreSQL 18.6 binaries** at `C:\Program Files\PostgreSQL\18\bin` (not on PATH), but **no data directory and no Windows service, nothing listening on 5432** (checked 2026-09-26). The install looks incomplete; server is not usable yet. Docker was being installed as the alternative.
- **Not installed:** Docker, Semgrep (Python CLI).
- Not yet decided: local Postgres vs Docker.

## Open questions
- Local Postgres install or Docker?
- Should the workspace become a git repo?
- Go module path / project name?
- Claude model/pricing choice for the AI pass (decide when we reach it; use claude-api skill)

## Known risks
- Large PR diffs: GitHub truncates big patches and LLM context is finite. Need a chunking / file-by-file strategy before the AI pass.
- Semgrep on Windows can be awkward; may need WSL or Docker.
- Nothing written so far has been executed or compiled.

## Repo layout (current)
```
docs/architecture.md, docs/TRACKER.md
migrations/0001_init.sql
skills/go/skill.yaml
skills/go/lessons/{error-handling,context,concurrency,interfaces}.md
```

## Session log
- 2026-09-26: Defined project and scope through discussion; locked the 4 decisions above. Wrote schema, Go skill + lessons, architecture doc, this tracker. No code executed (no toolchain installed).
