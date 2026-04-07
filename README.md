# Ampulla

Self-hosted error and performance tracking service. Implements a subset of [Sentry's](https://sentry.io) ingestion API, so standard Sentry SDKs work without modification.

Built with Go and PostgreSQL. No external dependencies, no queue, no workers — just a single binary and a database.

## Features

- **Sentry SDK compatible** — drop-in DSN replacement, SDKs send events as usual
- **Error tracking** — exceptions grouped by fingerprint, with full event payloads stored as JSONB
- **Performance monitoring** — transactions and spans ingestion, with latency percentiles (P50/P75/P95/P99)
- **Retention policy** — automatic cleanup of transactions older than 30 days
- **ntfy notifications** — per-project push notifications on new issues and regressions
- **Admin UI** — dark monospace dashboard at `/admin/` for managing organizations, projects, DSN keys, browsing issues with structured stacktraces, and viewing performance stats
- **API tokens** — Bearer token auth (`ampt_...`) for machine clients, manageable via the admin UI
- **MCP server** — companion `ampulla-mcp/` exposes issues, events, transactions, and performance stats to AI agents via the Model Context Protocol
- **Self-monitoring** — Ampulla reports its own errors to itself via Sentry Go SDK

## Quick Start

```bash
cp .env.example .env
# Edit .env with your values
docker compose up -d
```

The service starts on port 8090. Admin UI is at `/admin/` (set `ADMIN_USER` and `ADMIN_PASSWORD` to enable it).

### Get a DSN

1. Open the admin UI
2. Create an organization and a project
3. Generate a DSN key
4. Use the DSN in any Sentry SDK:

```python
import sentry_sdk
sentry_sdk.init(dsn="https://<public_key>@your-domain.com/<project_id>")
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | *required* | PostgreSQL connection string |
| `AMPULLA_HOST` | `0.0.0.0` | Listen host |
| `AMPULLA_PORT` | `8090` | Listen port |
| `AMPULLA_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `ADMIN_USER` | | Admin username (enables admin UI) |
| `ADMIN_PASSWORD` | | Admin password |
| `SESSION_SECRET` | *auto-generated* | HMAC key for session cookies |
| `AMPULLA_DOMAIN` | `ampulla.elmisi.com` | Domain used in generated DSN strings |
| `SENTRY_DSN` | | Optional — Ampulla's own error reporting DSN |
| `SENTRY_ENVIRONMENT` | | Optional — environment tag for self-monitoring |

## Notifications

Each project can be configured with [ntfy](https://ntfy.sh) push notifications via the admin UI (project edit form):

- **Server URL** — e.g. `https://ntfy.sh` or self-hosted
- **Topic** — the ntfy topic to publish to
- **Token** — optional Bearer token for authenticated servers

Notifications are sent for:
- **New issues** — first time a fingerprint is seen
- **Regressions** — a resolved issue receives a new event (auto-reopened to unresolved)

### Environment Separation

Use separate projects per environment (e.g. `myapp-prod`, `myapp-staging`, `myapp-dev`), each with its own DSN. This keeps issues, transactions, and performance metrics cleanly separated. The project filter is persisted across pages.

## Architecture

```
cmd/ampulla/          Entrypoint, router, Sentry tracing, graceful shutdown
internal/
  admin/              Session auth, admin UI (index.html + static/ ES6 modules)
  api/admin/          Admin CRUD + performance stats API handlers
  api/ingest/         POST /api/{projectID}/envelope/ and /store/ (gzip/deflate)
  api/web/            Read-only Sentry-compatible API (/api/0/...)
  auth/               DSN public key validation middleware
  config/             Environment variable configuration
  envelope/           Sentry envelope wire format parser
  event/              Models, worker pool processor, ntfy notifications, cleanup
  grouping/           Fingerprinting (exception type + value + top frame -> SHA-256)
  store/              PostgreSQL repositories + embedded migrations
```

Worker pool processing (4 goroutines, buffered channel of 1000) — designed for low-to-medium volume (1k-5k events/month).

## Performance Percentiles

The Performance page in the admin UI shows latency percentiles for each endpoint, filterable by project and time range (24h, 7d, 30d):

| Metric | Meaning |
|--------|---------|
| **P50** | Median — 50% of requests complete within this time. The "typical" experience. |
| **P75** | 75th percentile — 25% of requests are slower than this. Shows where slowdowns begin. |
| **P95** | 95th percentile — only 5% of requests are slower. The standard metric for SLAs. |
| **P99** | 99th percentile — only 1% is slower. Captures outliers and worst-case latency. |

Values above 1s are highlighted in red, above 500ms in yellow.

## Issue Detail View

Clicking an issue shows a structured event display with:
- **Issue header** — title, level/status badges, first/last seen, event count, resolve/reopen controls
- **Event navigation** — prev/next buttons to browse all events in the issue
- **Stacktrace tab** — collapsible stack frames with in-app frame highlighting
- **Event Details tab** — structured tags, user, request, contexts, SDK, release info
- **Breadcrumbs tab** — chronological trail of actions before the error
- **Raw JSON tab** — full Sentry event payload as fallback

## API

### Ingestion (Sentry-compatible)

- `POST /api/{projectID}/envelope/` — Sentry envelope format (gzip/deflate supported)
- `POST /api/{projectID}/store/` — Sentry store format (legacy)

### Admin

All under `/api/admin/`. Authentication accepts either:

- **Session cookie** — `POST /api/admin/login` with admin credentials, returns HMAC-SHA256 signed cookie
- **API token** — `Authorization: Bearer ampt_...` (created via `#/tokens` admin page or `POST /api/admin/tokens`)

CRUD for organizations, projects, DSN keys, issues, ntfy configurations, and API tokens. Performance stats at `GET /api/admin/performance?project=ID&days=7`. List endpoints support keyset pagination via opaque `cursor` tokens.

### MCP server

`ampulla-mcp/` is a companion Go module that runs as a Model Context Protocol server, letting AI agents read and write Ampulla data without parsing raw JSON. Tools include `list_projects`, `get_issue`, `get_issue_events`, `list_transactions`, `get_transaction_spans`, `get_performance_stats`, `resolve_issue`, `reopen_issue`.

When deployed alongside Ampulla via the bundled compose, the MCP server is reachable at `https://<your-domain>/mcp/`. Each MCP client uses its own API token from `/admin/#/tokens` — the MCP server validates and forwards on the caller's behalf, with no shared credentials of its own. Token revocation takes effect on the next request.

See [`ampulla-mcp/README.md`](ampulla-mcp/README.md) for setup, transports (stdio + HTTP), and `.mcp.json` examples.

## Development

Go is not required locally — builds and tests run via Docker:

```bash
# Build
docker run --rm -v $(pwd):/app -w /app golang:1.23-alpine go build ./cmd/ampulla

# Test
docker run --rm -v $(pwd):/app -w /app golang:1.23-alpine go test ./...
```

## License

MIT
