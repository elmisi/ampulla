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

- **Tools:** read (`list_projects`, `list_issues`, `get_issue`, `get_issue_events`, `list_transactions`, `get_transaction_spans`, `get_performance_stats`) + write (`resolve_issue`, `reopen_issue`).
- **Build/test:** `docker run --rm -v $(pwd)/ampulla-mcp:/app -w /app golang:1.25-alpine go build ./cmd/ampulla-mcp` (note: 1.25, not 1.23).
- **Safety:** stacktraces capped at 30 frames, breadcrumbs at 20, tags at 50, strings truncated at 1000 bytes; `Authorization`/`Cookie`/`Set-Cookie` headers redacted from request blobs; logs only on stderr.

### Transports and authentication

Two transport modes with different auth models:

1. **stdio** (default, local development): single client constructed at startup using `AMPULLA_TOKEN` (preferred) or `AMPULLA_USER`/`AMPULLA_PASSWORD` (cookie session, retries on 401). The same token serves every request. Used as a child process via `.mcp.json` `command`.

2. **http** (hosted production, `-transport http`): per-request Bearer pass-through. Each incoming HTTP request must carry `Authorization: Bearer ampt_...`. The MCP server validates the token by calling Ampulla `GET /api/admin/tokens/whoami` (via `auth.RequireBearerToken` middleware from the SDK), then constructs a per-session client using that token. The MCP server has no credentials of its own. Session-binding via `auth.TokenInfo.UserID = "token:<id>"` prevents token swap mid-session (SDK rejects with 403). Token revocation in Ampulla takes effect on the very next request because every request triggers re-validation. The verified token is stashed in `auth.TokenInfo.Extra[tokenExtraKey]` and recovered by `getServer` — **never** reparsed from the Authorization header, to avoid drift with the SDK middleware's RFC 6750 parsing rules.

### HTTP error classification (important)

Only a genuine Ampulla 401 response to the whoami probe is mapped to `auth.ErrInvalidToken` (→ SDK returns 401 to the client). Everything else — network failures, 5xx responses, JSON decode errors — propagates as a generic server error (→ SDK returns 500). This distinction prevents transient backend outages from being misreported to MCP clients as credential revocations. The contract is enforced by `client.ErrUnauthorized` (only wrapped on 401) and tested in `http_test.go` (`TestVerifier_Preserves5xxAsServerError`, `TestHandlerChain_UpstreamOutageReturns500`).

Two cmd files: `cmd/ampulla-mcp/main.go` (entrypoint, stdio mode) and `cmd/ampulla-mcp/http.go` (HTTP mode + verifier + factory).

### Production deployment

Both `ampulla` and `ampulla-mcp` ship in the same `docker-compose.yml`. Traefik routes `Host(ampulla.elmisi.com) && PathPrefix(/mcp)` to the MCP container with priority 200, applying a `stripprefix` middleware to remove `/mcp` before forwarding. The MCP container reaches Ampulla via the internal Docker network using `http://ampulla:8090` and the `AMPULLA_INSECURE_HTTP=1` escape hatch (TLS terminates at Traefik on the public side).

The MCP container needs **no secrets at all** in production: it has no token, no admin credentials. Each MCP client carries its own Bearer token.

### MCP client cookie jar quirk

Wraps stdlib `cookiejar` to strip the `Secure` flag, otherwise Ampulla session cookies (set with `Secure: true`) are dropped over `http://localhost`. Transport security is enforced at construction time by `client.New`. Only relevant for cookie-mode (legacy stdio); token-mode bypasses cookies entirely.
