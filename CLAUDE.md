# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Ampulla is a lightweight, self-hosted Sentry-compatible error and performance tracking service. Standard Sentry SDKs work without modification — just swap the DSN.

**Stack:** Go 1.23 + PostgreSQL 16 + Chi router
**Domain:** `ampulla.elmisi.com` | **Port:** 8090

## Commands

Go is **not installed locally** — all builds and tests run via Docker:

```bash
# Build
docker run --rm -v $(pwd):/app -w /app golang:1.23-alpine go build -ldflags "-X github.com/elmisi/ampulla/internal/version.Value=$(cat VERSION)" ./cmd/ampulla

# Run all tests (pure unit tests, no DB required)
docker run --rm -v $(pwd):/app -w /app golang:1.23-alpine go test ./...

# Run a single package's tests
docker run --rm -v $(pwd):/app -w /app golang:1.23-alpine go test ./internal/envelope/

# Run locally (production compose with Traefik labels)
docker compose up -d

# Manual ingestion test (requires running instance)
./test-event.sh

# Deploy
cd ../traefik.services && ./deploy.sh ampulla
```

Tests exist for `internal/envelope/` and `internal/grouping/` — both are pure unit tests with no external dependencies. Test style: `TestFunctionName` or `TestFunctionName_Scenario`, table-driven where appropriate, stdlib `testing` only (no assertion libraries).

## Architecture

### Request Flow

1. Sentry SDK sends event to `POST /api/{projectID}/envelope/` (or `/store/`)
2. `auth.Middleware` extracts DSN public key from `X-Sentry-Auth` header or `sentry_key` query param, validates against DB (cached 5 min)
3. Handler decompresses (gzip/deflate) and parses the Sentry envelope wire format (newline-delimited JSON), returns 200 immediately
4. `event.Processor` worker pool (4 goroutines, channel queue of 1000) processes async: computes fingerprint, upserts issue, stores event/transaction/spans. Jobs are dropped if the queue is full.
5. On new issue or regression (resolved issue receiving new event), sends ntfy notification if configured on the project.

### API Surfaces

- **Utility** (`/health`, `/api/version`) — unauthenticated, always available
- **Ingestion** (`/api/{projectID}/envelope/`, `/store/`) — Sentry-compatible, DSN key auth via middleware
- **Web API** (`/api/0/...`) — read-only Sentry-compatible endpoints, session-authenticated
- **Admin API** (`/api/admin/...`) — CRUD for orgs/projects/keys/issues, performance stats, session-authenticated (HMAC-SHA256 cookies)

Both Web API and Admin API are only mounted when `ADMIN_USER` + `ADMIN_PASSWORD` are set (`cfg.AdminEnabled()`).

### Key Packages

- `cmd/ampulla/main.go` — entrypoint, router wiring, Sentry self-monitoring, graceful shutdown
- `internal/admin/` — session auth (`auth.go`, HMAC-SHA256 cookies, login rate limiting) + embedded single-file admin UI (`ui.go` embeds `index.html`)
- `internal/api/admin/` — admin CRUD handlers + performance stats endpoint
- `internal/api/ingest/` — ingestion handlers (envelope + legacy store), gzip/deflate decompression
- `internal/api/web/` — read-only Sentry-compatible API
- `internal/auth/` — DSN public key extraction and validation middleware (in-memory cache with TTL)
- `internal/envelope/` — Sentry envelope wire format parser
- `internal/event/` — domain models (`model.go`), `Envelope`/`EnvelopeItem` types, worker pool processor (`processor.go`), ntfy notifications, cleanup goroutine
- `internal/grouping/` — fingerprinting: exception type + value + top frame → SHA-256
- `internal/store/` — PostgreSQL repository (single `postgres.go`) + embedded migrations

### Key Interfaces

`event.Store` (in `processor.go`) defines the DB contract used by the processor: `UpsertIssue`, `InsertEvent`, `InsertTransaction`, `InsertSpans`, `DeleteOldTransactions`, `GetProjectNtfyConfig`. The `store.DB` struct implements this plus all admin/web query methods.

### Design Decisions

- Worker pool processing (4 workers, buffered channel) — adequate for 1k-5k events/month
- JSONB columns preserve full Sentry event payloads
- Migrations embedded in binary via `embed.FS` from `internal/store/migrations/`
- Admin UI is a single `index.html` embedded in the Go binary
- Multi-stage Dockerfile: `golang:1.23-alpine` builder → `alpine:3.21` runtime
- `docker-compose.yml` is the production compose (includes Traefik labels)
- Batch span insert (single INSERT with N rows instead of N queries)
- 30-day retention for transactions/spans via hourly cleanup goroutine (errors kept indefinitely)
- Self-monitoring: Ampulla reports its own errors to itself via Sentry Go SDK (project 3, `SENTRY_DSN` env var)
- Sentry tracing middleware on admin/web API routes only (ingestion excluded to avoid loops)
- Per-project ntfy notifications on new issues and regressions (resolved → new event)

## Database

PostgreSQL 16. Tables: `organizations`, `projects` (with `ntfy_url`, `ntfy_topic`, `ntfy_token`), `project_keys`, `issues`, `events`, `transactions`, `spans`. Migrations in `internal/store/migrations/` (001 through 004). Migrations use `golang-migrate/migrate` with embedded `iofs` source.

Key constraints: `events.issue_id` → `issues(id) ON DELETE CASCADE`, `spans.transaction_id` → `transactions(id) ON DELETE CASCADE`.

## DSN Format

```
https://<public_key>@ampulla.elmisi.com/<project_id>
```

## Configuration

All via environment variables. See `.env.example` for defaults.

**Go module path:** `github.com/elmisi/ampulla` (used in import paths and `-ldflags` version injection).

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | *required* | PostgreSQL connection string |
| `AMPULLA_HOST` | `0.0.0.0` | Listen host |
| `AMPULLA_PORT` | `8090` | Listen port |
| `AMPULLA_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `ADMIN_USER` | | Admin username (enables admin UI + web/admin APIs) |
| `ADMIN_PASSWORD` | | Admin password |
| `SESSION_SECRET` | *auto-generated* | HMAC key for session cookies |
| `AMPULLA_DOMAIN` | `ampulla.elmisi.com` | Domain used in generated DSN strings |
| `SENTRY_DSN` | | Optional — self-monitoring DSN |
| `SENTRY_ENVIRONMENT` | | Optional — environment tag for self-monitoring |
| `POSTGRES_DB` | | Used by docker-compose db service |
| `POSTGRES_USER` | | Used by docker-compose db service |
| `POSTGRES_PASSWORD` | | Used by docker-compose db service |

## Notifications

Per-project ntfy integration configured via admin UI (project edit form):
- **ntfy Server URL** — e.g. `https://n.elmisi.com`
- **Topic** — e.g. `ampulla-errors`
- **Token** — optional Bearer token for authenticated ntfy servers

Triggers: new issue (first time fingerprint seen), regression (resolved issue receives new event, auto-reopened to unresolved).
