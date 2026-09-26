# PullWise: Code Reviewer and Mentor

An AI-powered GitHub pull-request reviewer that combines static analysis and
packaged software-engineering skills (Go, REST, SOLID, security, code quality)
to find issues and teach developers *why* their code violates a principle.

Every finding carries evidence and an explanation: what was found, why it
matters, how to improve it, and a lesson to learn from.

**Status:** early development. See [docs/TRACKER.md](docs/TRACKER.md) for
progress and [docs/architecture.md](docs/architecture.md) for the design.

## Stack

Go (`net/http`) · PostgreSQL · React + TypeScript + Vite · Tree-sitter + Semgrep · Claude · GitHub API

## Layout

```
backend/       Go REST API
  internal/      review pipeline packages
  migrations/    PostgreSQL schema
  skills/        review skills: skill.yaml (what to check) + lessons/*.md (what to teach)
frontend/      React + TypeScript + Vite dashboard
docs/          architecture, API contract and project tracker
```

## Run the frontend (demo mode, no backend needed)

```
cd frontend
npm install
npm run dev
```

The frontend ships with a mock API, so it works before the Go backend has the review endpoints.
Set `VITE_API_MODE=http` (see `frontend/.env.example`) to use the real backend.

## Run the backend

```
cd backend
go run ./cmd/server
```

Serves `GET /healthz` on `:8080`.
