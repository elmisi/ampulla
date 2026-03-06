# Ampulla

A lightweight, self-hosted Sentry-compatible error and performance tracking service.

## Overview

Ampulla implements a subset of the Sentry ingestion and web API with 100% SDK compatibility. Any application using a standard Sentry SDK can point its DSN to `ampulla.elmisi.com` and have errors and transactions ingested without modification.

**Domain:** `ampulla.elmisi.com`
**Stack:** Go + PostgreSQL
**Target scale:** Multi-user, 1-20 projects, 1k-5k monthly events

## Sentry Protocol Compatibility

### DSN Format

```
https://<public_key>@ampulla.elmisi.com/<project_id>
```

### Ingestion Endpoints

| Endpoint | Method | Description |
|---|---|---|
| `/api/{project_id}/envelope/` | POST | Modern envelope ingestion (all recent SDKs) |
| `/api/{project_id}/store/` | POST | Legacy single-event ingestion |

### Authentication

SDKs authenticate via the `X-Sentry-Auth` header or `sentry_key` query parameter, both containing the DSN public key. The server validates the key against the target project.

### Envelope Format

The Sentry envelope is a newline-delimited format:

```
{"event_id":"...","dsn":"...","sdk":{"name":"...","version":"..."}}\n
{"type":"event","length":N}\n
{...event JSON payload...}\n
{"type":"transaction","length":N}\n
{...transaction JSON payload...}\n
```

Each envelope contains a header line followed by one or more items. Each item has its own header (with `type` and optional `length`) followed by the payload.

### Supported Item Types

| Type | Description | Stored In |
|---|---|---|
| `event` | Error/exception events | `events` table |
| `transaction` | Performance transactions with spans | `transactions` + `spans` tables |

Unsupported item types (attachment, session, replay, etc.) are silently discarded.

### Web API (Read-Only)

Minimal REST API for querying ingested data:

| Endpoint | Description |
|---|---|
| `GET /api/0/organizations/` | List organizations |
| `GET /api/0/organizations/{org}/projects/` | List projects in org |
| `GET /api/0/projects/{org}/{project}/issues/` | List issues |
| `GET /api/0/issues/{issue_id}/events/` | List events for an issue |
| `GET /api/0/organizations/{org}/events/` | List transactions/events |

## Architecture

### Event Flow

```
Sentry SDK
    |
    | POST /api/{project_id}/envelope/
    v
+-------------------+
|   Auth Middleware  |  Validate public_key against project
+-------------------+
    |
    v
+-------------------+
|  Envelope Parser  |  Parse newline-delimited envelope format
+-------------------+
    |
    v
+-------------------+
|  Event Processor  |  For each item in envelope:
|                   |    error  -> fingerprint -> upsert issue -> store event
|                   |    txn    -> store transaction + spans
+-------------------+
    |
    v
+-------------------+
|    PostgreSQL     |  All persistent storage
+-------------------+
```

Processing is synchronous — at the target scale (1k-5k events/month) no queue is needed. The server returns `202 Accepted` immediately after validation and enqueues processing in a goroutine with a bounded worker pool.

### Fingerprinting (Issue Grouping)

Default grouping strategy matching Sentry's default behavior:

1. Extract exception type + value + top stack frame (filename + function)
2. Compute SHA-256 hash of the concatenation
3. Use hash as fingerprint to find or create an issue

Events with the same fingerprint are grouped into the same issue. The `issues` table tracks `first_seen`, `last_seen`, and `event_count`.

### Database Schema

```sql
-- Organizations
CREATE TABLE organizations (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Projects
CREATE TABLE projects (
    id          BIGSERIAL PRIMARY KEY,
    org_id      BIGINT NOT NULL REFERENCES organizations(id),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL,
    platform    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, slug)
);

-- DSN Keys
CREATE TABLE project_keys (
    id          BIGSERIAL PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id),
    public_key  TEXT NOT NULL UNIQUE,
    secret_key  TEXT NOT NULL UNIQUE,
    label       TEXT NOT NULL DEFAULT 'Default',
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Issues (grouped errors)
CREATE TABLE issues (
    id          BIGSERIAL PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id),
    title       TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    level       TEXT NOT NULL DEFAULT 'error',
    status      TEXT NOT NULL DEFAULT 'unresolved',
    first_seen  TIMESTAMPTZ NOT NULL,
    last_seen   TIMESTAMPTZ NOT NULL,
    event_count BIGINT NOT NULL DEFAULT 1,
    UNIQUE(project_id, fingerprint)
);

-- Error Events
CREATE TABLE events (
    id          BIGSERIAL PRIMARY KEY,
    event_id    UUID NOT NULL,
    project_id  BIGINT NOT NULL REFERENCES projects(id),
    issue_id    BIGINT NOT NULL REFERENCES issues(id),
    timestamp   TIMESTAMPTZ NOT NULL,
    platform    TEXT,
    level       TEXT,
    message     TEXT,
    data        JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, event_id)
);

-- Performance Transactions
CREATE TABLE transactions (
    id          BIGSERIAL PRIMARY KEY,
    event_id    UUID NOT NULL,
    project_id  BIGINT NOT NULL REFERENCES projects(id),
    trace_id    UUID NOT NULL,
    span_id     TEXT NOT NULL,
    op          TEXT,
    name        TEXT NOT NULL,
    duration_ms DOUBLE PRECISION,
    status      TEXT,
    timestamp   TIMESTAMPTZ NOT NULL,
    data        JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, event_id)
);

-- Spans (children of transactions)
CREATE TABLE spans (
    id              BIGSERIAL PRIMARY KEY,
    transaction_id  BIGINT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    trace_id        UUID NOT NULL,
    span_id         TEXT NOT NULL,
    parent_span_id  TEXT,
    op              TEXT,
    description     TEXT,
    duration_ms     DOUBLE PRECISION,
    status          TEXT,
    timestamp       TIMESTAMPTZ NOT NULL,
    data            JSONB
);

-- Indexes
CREATE INDEX idx_events_project_timestamp ON events(project_id, timestamp DESC);
CREATE INDEX idx_events_issue ON events(issue_id, timestamp DESC);
CREATE INDEX idx_issues_project_status ON issues(project_id, status);
CREATE INDEX idx_transactions_project_timestamp ON transactions(project_id, timestamp DESC);
CREATE INDEX idx_transactions_trace ON transactions(trace_id);
CREATE INDEX idx_spans_transaction ON spans(transaction_id);
CREATE INDEX idx_spans_trace ON spans(trace_id);
```

### Project Structure

```
ampulla/
├── cmd/
│   └── ampulla/
│       └── main.go              # Entrypoint, config loading, server startup
├── internal/
│   ├── api/
│   │   ├── ingest/
│   │   │   ├── handler.go       # POST /api/{project_id}/envelope/ and /store/
│   │   │   └── handler_test.go
│   │   └── web/
│   │       ├── handler.go       # REST API handlers (issues, events, projects)
│   │       └── handler_test.go
│   ├── auth/
│   │   ├── middleware.go        # DSN public key extraction and validation
│   │   └── middleware_test.go
│   ├── envelope/
│   │   ├── parser.go           # Sentry envelope format parser
│   │   └── parser_test.go
│   ├── event/
│   │   ├── model.go            # Event, Transaction, Span structs
│   │   └── processor.go        # Event processing: fingerprint, store
│   ├── grouping/
│   │   ├── fingerprint.go      # Default fingerprinting algorithm
│   │   └── fingerprint_test.go
│   └── store/
│       ├── postgres.go         # PostgreSQL connection and repositories
│       └── postgres_test.go
├── migrations/
│   ├── 001_initial.up.sql
│   └── 001_initial.down.sql
├── docker-compose.yml           # Production compose (app + postgres)
├── Dockerfile
├── go.mod
└── go.sum
```

## Configuration

Environment variables:

| Variable | Description | Default |
|---|---|---|
| `AMPULLA_HOST` | Listen address | `0.0.0.0` |
| `AMPULLA_PORT` | Listen port | `8090` |
| `DATABASE_URL` | PostgreSQL connection string | required |
| `AMPULLA_LOG_LEVEL` | Log level (debug, info, warn, error) | `info` |

## Deployment

Deployed on `box.milleguide.it` alongside other services, behind Traefik reverse proxy.

### Docker Compose (Production)

```yaml
services:
  ampulla:
    build: .
    container_name: ampulla
    restart: always
    depends_on:
      ampulla-db:
        condition: service_healthy
    env_file: .env
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.ampulla.rule=Host(`ampulla.elmisi.com`)"
      - "traefik.http.routers.ampulla.entrypoints=websecure"
      - "traefik.http.routers.ampulla.tls.certresolver=letsencrypt"
      - "traefik.http.routers.ampulla.middlewares=security-headers@file,gzip@file"
      - "traefik.http.services.ampulla.loadbalancer.server.port=8090"
    networks:
      - web_proxy_net
      - ampulla_internal

  ampulla-db:
    image: postgres:16-alpine
    container_name: ampulla-db
    restart: always
    volumes:
      - ampulla_pgdata:/var/lib/postgresql/data
    environment:
      POSTGRES_DB: ${POSTGRES_DB}
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]
      interval: 5s
      timeout: 5s
      retries: 5
    networks:
      - ampulla_internal

volumes:
  ampulla_pgdata:

networks:
  web_proxy_net:
    external: true
  ampulla_internal:
    driver: bridge
```

### Deploy Command

```bash
# From traefik.services/
./deploy.sh ampulla
```

## Out of Scope (MVP)

- Source maps / symbol resolution
- Release tracking / deploys
- Alerting / notifications
- Session replay
- Cron monitors
- Web UI (API-only)
- User authentication for the web API (internal use only)

## Dependencies (Go)

| Package | Purpose |
|---|---|
| `github.com/go-chi/chi/v5` | HTTP router |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `github.com/golang-migrate/migrate/v4` | Database migrations |
| `github.com/google/uuid` | UUID generation for keys |
