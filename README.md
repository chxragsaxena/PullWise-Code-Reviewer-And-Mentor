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
docs/          architecture and project tracker
```
