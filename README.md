# Ampulla

Self-hosted error and performance tracking service. Implements a subset of [Sentry's](https://sentry.io) ingestion API, so standard Sentry SDKs work without modification.

Built with Go and PostgreSQL. No external dependencies, no queue, no workers — just a single binary and a database.

## Features

- **Sentry SDK compatible** — drop-in DSN replacement, SDKs send events as usual
- **Error tracking** — exceptions grouped by fingerprint, with full event payloads stored as JSONB
- **Performance monitoring** — transactions and spans ingestion, with latency percentiles (P50/P75/P95/P99)
- **Retention policy** — automatic cleanup of transactions older than 30 days
- **Admin UI** — dark monospace dashboard at `/admin/` for managing organizations, projects, DSN keys, and browsing issues
- **Session auth** — HMAC-SHA256 signed cookies, credentials from environment variables

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

## Architecture

```
cmd/ampulla/          Entrypoint, router, graceful shutdown
internal/
  admin/              Session auth, embedded admin UI (single HTML file)
  api/admin/          Admin CRUD API handlers
  api/ingest/         POST /api/{projectID}/envelope/ and /store/
  api/web/            Read-only Sentry-compatible API (/api/0/...)
  auth/               DSN public key validation middleware
  config/             Environment variable configuration
  envelope/           Sentry envelope wire format parser
  event/              Models and event processor
  grouping/           Fingerprinting (exception type + value + top frame -> SHA-256)
  store/              PostgreSQL repositories + embedded migrations
```

Synchronous processing, no queue — designed for low-to-medium volume (1k-5k events/month).

## Performance Percentiles

The Performance page in the admin UI shows latency percentiles for each endpoint over the last 7 days:

| Metric | Meaning |
|--------|---------|
| **P50** | Median — 50% of requests complete within this time. The "typical" experience. |
| **P75** | 75th percentile — 25% of requests are slower than this. Shows where slowdowns begin. |
| **P95** | 95th percentile — only 5% of requests are slower. The standard metric for SLAs. |
| **P99** | 99th percentile — only 1% is slower. Captures outliers and worst-case latency. |

Values above 1s are highlighted in red, above 500ms in yellow.

## API

### Ingestion (Sentry-compatible)

- `POST /api/{projectID}/envelope/` — Sentry envelope format
- `POST /api/{projectID}/store/` — Sentry store format

### Admin

All under `/api/admin/`, session-authenticated. CRUD for organizations, projects, DSN keys, issues.

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
