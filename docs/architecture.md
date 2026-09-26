# Architecture

**Brief:** an AI-powered GitHub PR reviewer that combines static analysis and
packaged software-engineering skills to find issues and teach developers *why*
their code violates a principle. Findings must carry evidence, not opinion.

## Responsibilities

| Layer | Owns |
|---|---|
| Go code | the review engine: fetch, select skills, run analyzers, aggregate, serve |
| `skills/*/skill.yaml` | what to check |
| `skills/*/lessons/*.md` | what to teach |
| PostgreSQL | what happened (reviews, findings) |
| Claude | contextual judgment, only for `kind: ai` checks |

## Pipeline (V1, deterministic only)

```
PR URL -> GitHub API -> PR + diff -> detect languages -> select skills
       -> static analyzers -> findings -> PostgreSQL -> SSE progress -> React dashboard
```

The AI pass is added after this works end to end:
`existing pipeline -> AI context builder -> Claude -> structured findings -> aggregator`.

## Interfaces (planned)

```go
type StaticAnalyzer interface {
    Analyze(ctx context.Context, input AnalysisInput) ([]Finding, error)
}
type LLMProvider interface {
    Review(ctx context.Context, input ReviewInput) (ReviewResult, error)
}
```

Semgrep is one `StaticAnalyzer` (shelled out, JSON output). Tree-sitter powers our
own analyzers. Claude is one `LLMProvider`, using structured JSON output.

## Stack

React + TypeScript + Vite / Go `net/http` (1.22+ routing) / PostgreSQL /
Tree-sitter + Semgrep / Claude / GitHub API / REST + SSE.

## Not in scope yet

Private repos, GitHub App and webhooks, Redis, NATS, repository RAG, auto-fixes,
custom team skills, languages beyond Go and TypeScript.

## Roadmap

V1 public PR URL -> dashboard. V2 GitHub OAuth. V3 GitHub App and webhooks.
V4 repository RAG. V5 quizzes and progress. V6 custom team standards.

## Decisions

- Text + CHECK constraints instead of Postgres enums.
- `repositories.user_id` is nullable because V1 has no login.
- Skills live in files; the `skills` table only mirrors them for foreign keys.
- Deterministic findings always have confidence 1.0.
- No pgvector until V4.
