# Performance measurements

All numbers were measured on one laptop (Intel Core i3-1215U, 6 cores / 8 threads, 7.7 GB RAM, Windows 11, Docker
Desktop for Postgres and the Linux runs). The load generator shares the CPU with the server, so these are
conservative. Single runs on a laptop vary by tens of percent; read them as orders of magnitude.

Each measurement is a test or benchmark in the repository that only runs on request.

## Live progress streams (Server-Sent Events)

Real HTTP sockets, the real SSE handler and event broker, a stubbed review pipeline. N clients connect, a review
finishes, and each client must receive the final status. Run in a Linux container.

| Clients | Delivered | Failed | Connect p95 | Final status p50 / p95 / max |
|---|---|---|---|---|
| 1,000 | 1,000 | 0 | 28 ms | 22 / 45 / 59 ms |
| 5,000 | 5,000 | 0 | 293 ms | 258 / 319 / 343 ms |

In-process (service plus broker, no sockets): 20,000 watchers, none dropped, final status p95 21 to 50 ms over three runs.

`LOAD=1 go test -run LoadSSE -v ./internal/httpapi/` (Linux needed above about 1,000 connections: Windows refuses
bursts of loopback connections)

## Read endpoints, requests per second

Real sockets, the review store stubbed in memory, so this is the HTTP layer: routing, middleware, JSON.
Linux container, 4 seconds per cell, zero errors in every cell.

| Endpoint | 16 connections | 64 connections | 256 connections |
|---|---|---|---|
| `GET /healthz` | 46,600 req/s, p95 1.5 ms | 48,700 req/s, p95 4.7 ms | 17,100 req/s, p95 48 ms |
| `GET /api/reviews` (20 reviews) | 8,600 req/s, p95 7.7 ms | 14,500 req/s, p95 15 ms | 26,400 req/s, p95 25 ms |
| `GET /api/reviews/{id}` | 38,900 req/s, p95 1.6 ms | 17,900 req/s, p95 15 ms | 26,300 req/s, p95 28 ms |

`LOAD=1 go test -run LoadHTTP -v ./internal/httpapi/`

## Load shedding

500 review requests arrive at once with 2 workers and a queue of 50, while every review is held open.
52 are accepted (2 running, 50 queued), 448 are refused immediately with `ErrBusy` (HTTP 503), none fail any other
way, and all 52 accepted reviews finish once released. Create latency stays under 1 ms.

`LOAD=1 go test -run LoadShedding -v ./internal/service/`

## Analyzer speed

One large real file each (Go: `net/http/server.go`, TypeScript: zod `schemas.ts`, Python: click `core.py`).

| Analyzer | File | Time | Throughput |
|---|---|---|---|
| Go (`go/ast`) | 4,293 lines | 4.0 ms | about 1,075,000 lines/s |
| Python (tree-sitter) | 4,039 lines | 35 ms | about 114,000 lines/s |
| TypeScript (tree-sitter) | 5,271 lines | 53 ms | about 100,000 lines/s |

`BENCH_GO=... BENCH_TS=... BENCH_PY=... go test -run xxx -bench . ./internal/analyzer/bench/`

## Repository search (knowledge service)

Exact cosine search over 1024-dimension vectors (no approximate index, so every query reads every chunk of that
repository), with keyword search merged in. Random vectors, PostgreSQL 18 with pgvector in Docker.
A real repository such as go-chi/chi is about 500 chunks.

| Chunks | Vectors only, p50 / p95 | Hybrid, p50 / p95 | Hybrid, 8 searches at once, p50 / p95 |
|---|---|---|---|
| 5,000 | 28 / 35 ms | 32 / 41 ms | 98 / 150 ms |
| 20,000 | 141 / 216 ms | 112 / 148 ms | 318 / 441 ms |

Cost grows linearly with the size of one repository's index. A common keyword that matches most chunks makes the
keyword half rank all of them, which is slower; the benchmark uses rare words, like most identifiers.

The query now cuts each list to its best candidates before numbering them, instead of numbering every row of the
repository first. Run-to-run variation on this machine (vector-only search itself varied by 50 percent between
runs) is larger than that change's effect, so no speedup is claimed from it.

`LOAD=1 go test -run LoadSearchScaling -v ./internal/store/` in pullwise-knowledge, with TEST_DATABASE_URL set.

## What dominates in real use

A review of a real pull request takes 6 to 87 seconds (median 43 s) on the deployed site, and the first question
about a new repository takes about 77 seconds. Almost all of that is waiting for GitHub, the embedding API and the
language model, which is why the server's own work above is small by comparison.
