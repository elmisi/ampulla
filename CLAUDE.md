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

Tests exist for `internal/envelope/`, `internal/grouping/`, `internal/cursor/`, and `internal/admin/` (token generation/hashing/Bearer extraction) — all pure unit tests with no external dependencies. Test style: `TestFunctionName` or `TestFunctionName_Scenario`, table-driven where appropriate, stdlib `testing` only (no assertion libraries).

## Architecture

### Request Flow

1. Sentry SDK sends event to `POST /api/{projectID}/envelope/` (or `/store/`)
2. `auth.Middleware` extracts DSN public key from `X-Sentry-Auth` header or `sentry_key` query param, validates against DB (cached 5 min)
3. Handler decompresses (gzip/deflate) and parses the Sentry envelope wire format (newline-delimited JSON)
4. `event.Processor` worker pool (4 goroutines, channel queue of 1000) processes async: computes fingerprint, upserts issue, stores event/transaction/spans. If the queue is full, handler returns `503 Service Unavailable` with `Retry-After: 60` (backpressure).
5. On new issue or regression (resolved issue receiving new event), sends ntfy notification if configured on the project (via shared `ntfy_configurations`).

### API Surfaces

- **Utility** (`/health`, `/api/version`) — unauthenticated, always available
- **Ingestion** (`/api/{projectID}/envelope/`, `/store/`) — Sentry-compatible, DSN key auth via middleware
- **Web API** (`/api/0/...`) — read-only Sentry-compatible endpoints, session-authenticated
- **Admin API** (`/api/admin/...`) — CRUD for orgs/projects/keys/issues, performance stats. Auth via `CombinedAuthMiddleware`: accepts `Authorization: Bearer ampt_...` (API token) OR HMAC-SHA256 session cookie

Both Web API and Admin API are only mounted when `ADMIN_USER` + `ADMIN_PASSWORD` are set (`cfg.AdminEnabled()`).

### Key Packages

- `cmd/ampulla/main.go` — entrypoint, router wiring, Sentry self-monitoring, graceful shutdown
- `internal/admin/` — session auth (`auth.go`, HMAC-SHA256 cookies, login rate limiting), API token auth (`tokens.go`, sha256-hashed Bearer tokens, `CombinedAuthMiddleware`), admin UI (`ui.go` embeds `index.html` + `static/` directory including `pages/tokens.js`)
- `internal/api/admin/` — admin CRUD handlers + performance stats endpoint + API token endpoints (`tokens.go`)
- `internal/api/ingest/` — ingestion handlers (envelope + legacy store), gzip/deflate decompression
- `internal/api/web/` — read-only Sentry-compatible API
- `internal/auth/` — DSN public key extraction and validation middleware (in-memory cache with TTL)
- `internal/envelope/` — Sentry envelope wire format parser
- `internal/event/` — domain models (`model.go`), `Envelope`/`EnvelopeItem` types, worker pool processor (`processor.go`), ntfy notifications, cleanup goroutine
- `internal/grouping/` — fingerprinting: exception type + value + top frame → SHA-256
- `internal/notify/` — ntfy sender service (`NtfySender` interface, `HTTPNtfySender` with dedicated client)
- `internal/observe/` — self-monitoring: `Error`, `Message`, `RecoverPanic`, `Throttled` (slog + Sentry best-effort)
- `internal/store/` — PostgreSQL repository (single `postgres.go`) + embedded migrations

### Key Interfaces

`event.Store` (in `processor.go`) defines the DB contract used by the processor: `UpsertIssue`, `InsertEvent`, `InsertTransaction`, `InsertSpans`, `DeleteOldTransactions`, `GetProjectNtfyConfig`. The `store.DB` struct implements this plus all admin/web query methods.

### Design Decisions

- Worker pool processing (4 workers, buffered channel) — adequate for 1k-5k events/month
- JSONB columns preserve full Sentry event payloads
- Migrations embedded in binary via `embed.FS` from `internal/store/migrations/`
- Admin UI is split into ES6 modules (`index.html` shell + `static/` directory with CSS, JS, page modules) embedded in the Go binary
- Multi-stage Dockerfile: `golang:1.23-alpine` builder → `alpine:3.21` runtime
- `docker-compose.yml` is the production compose (includes Traefik labels)
- Batch span insert (single INSERT with N rows instead of N queries)
- 30-day retention for transactions/spans via hourly cleanup goroutine (errors kept indefinitely)
- Self-monitoring: `internal/observe` package provides slog + Sentry best-effort capture; panic recovery in workers, ntfy goroutines, and HTTP handler; throttled queue drop alerts
- Sentry tracing middleware on admin/web API routes only (ingestion excluded to avoid loops)
- Shared ntfy configurations (`ntfy_configurations` table) with admin CRUD + test endpoint; projects link via `ntfy_config_id` FK
- API tokens (`api_tokens` table): plaintext shown only at creation, storage is sha256 hash, prefix kept for UI display, `last_used_at` updated best-effort on every request
- Keyset pagination via `internal/cursor/` (opaque base64url JSON tokens with timestamp+id); backward compatible with plain numeric cursors
- Processor shutdown with 15s timeout and diagnostic logging; ntfy goroutines tracked in WaitGroup
- HTTP request logging middleware (Debug < 400, Info >= 400, /health excluded)
- Environment separation: use separate projects per environment (e.g. `myapp-prod`, `myapp-dev`) rather than environment-level filtering within a project
- Project filter persisted in localStorage across Issues, Transactions, and Performance pages

## Database

PostgreSQL 16. Tables: `organizations`, `projects` (with `ntfy_config_id` FK), `project_keys`, `issues`, `events`, `transactions`, `spans`, `ntfy_configurations`, `api_tokens`. Migrations in `internal/store/migrations/` (001 through 010). Migrations use `golang-migrate/migrate` with embedded `iofs` source.

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
| `SENTRY_DSN` | | Optional — self-monitoring DSN (Go backend) |
| `SENTRY_FRONTEND_DSN` | | Optional — self-monitoring DSN for admin UI (Browser SDK) |
| `SENTRY_ENVIRONMENT` | | Optional — environment tag for self-monitoring |
| `POSTGRES_DB` | | Used by docker-compose db service |
| `POSTGRES_USER` | | Used by docker-compose db service |
| `POSTGRES_PASSWORD` | | Used by docker-compose db service |

## Notifications

Shared ntfy configurations managed via admin UI (`#/ntfy` page). Each project optionally links to one configuration via `ntfy_config_id` (set in project edit form). Configurations include server URL, topic, and optional Bearer token.

Triggers: new issue (first time fingerprint seen), regression (resolved issue receives new event, auto-reopened to unresolved).

## API Tokens

Manage via admin UI (`#/tokens`). Tokens have format `ampt_<64 hex>`, plaintext is shown only once at creation time. Storage is sha256 hash + 12-char prefix (`ampt_xxxxxxx`). Used by machine clients (e.g. ampulla-mcp) to call the admin API without sharing the admin password.

Endpoints (require existing auth):
- `GET /api/admin/tokens` — list (no plaintext)
- `POST /api/admin/tokens` — create, returns plaintext once
- `DELETE /api/admin/tokens/{id}` — revoke

## ampulla-mcp

Sibling project at `ampulla-mcp/` (separate Go module, `go 1.25`). MCP (Model Context Protocol) server that exposes Ampulla data to AI agents via the SDK `github.com/modelcontextprotocol/go-sdk` v1.4.1.

- **Auth:** prefers `AMPULLA_TOKEN` (Bearer); falls back to `AMPULLA_USER`/`AMPULLA_PASSWORD` (cookie session with retry on 401).
- **Transport:** stdio (default) or HTTP via `-transport http -http-addr 127.0.0.1:8765`.
- **Tools:** read (`list_projects`, `list_issues`, `get_issue`, `get_issue_events`, `list_transactions`, `get_transaction_spans`, `get_performance_stats`) + write (`resolve_issue`, `reopen_issue`).
- **Build/test:** `docker run --rm -v $(pwd)/ampulla-mcp:/app -w /app golang:1.25-alpine go build ./cmd/ampulla-mcp` (note: 1.25, not 1.23).
- **Safety:** stacktraces capped at 30 frames, breadcrumbs at 20, tags at 50, strings truncated at 1000 bytes; `Authorization`/`Cookie`/`Set-Cookie` headers redacted from request blobs; logs only on stderr.
- **MCP client cookie jar:** wraps stdlib `cookiejar` to strip the `Secure` flag, otherwise Ampulla session cookies (set with `Secure: true`) are dropped over `http://localhost`. Transport security is enforced at construction time by `client.New`.
