# CLAUDE.md

## Overview

Ampulla is a lightweight, self-hosted Sentry-compatible error and performance tracking service. It implements a subset of Sentry's ingestion API so standard Sentry SDKs can send events without modification.

**Stack:** Go 1.23 + PostgreSQL 16 + Chi router
**Domain:** `ampulla.elmisi.com`
**Internal port:** 8090

## Architecture

- `cmd/ampulla/main.go` — entrypoint, config, router wiring, graceful shutdown
- `internal/config/` — env var configuration
- `internal/api/ingest/` — `POST /api/{projectID}/envelope/` and `/store/` handlers
- `internal/api/web/` — read-only REST API (`/api/0/...`)
- `internal/auth/` — DSN public key extraction and validation middleware
- `internal/envelope/` — Sentry envelope wire format parser
- `internal/event/` — models and event processor (fingerprint → upsert issue → store)
- `internal/grouping/` — default fingerprinting (exception type + value + top frame → SHA-256)
- `internal/store/` — PostgreSQL repositories + embedded migrations

## Common Commands

```bash
# Build and test (Go not installed locally, use Docker)
docker run --rm -v $(pwd):/app -w /app golang:1.23-alpine go build ./cmd/ampulla
docker run --rm -v $(pwd):/app -w /app golang:1.23-alpine go test ./...

# Run locally with PostgreSQL
docker compose up -d

# Deploy to VPS
cd ../traefik.services && ./deploy.sh ampulla
```

## DSN Format

```
https://<public_key>@ampulla.elmisi.com/<project_id>
```

## Key Design Decisions

- Synchronous processing (no queue) — adequate for 1k-5k events/month
- JSONB for raw event payloads — full Sentry event data preserved
- Auth keys cached in memory with 5-min TTL
- Background context used for async event processing after HTTP response
- Migrations embedded in Go binary via `embed.FS`

## Database

PostgreSQL 16. Tables: `organizations`, `projects`, `project_keys`, `issues`, `events`, `transactions`, `spans`. Migrations in `internal/store/migrations/`.

## Editing Guidelines

- Go is not installed locally — use `docker run golang:1.23-alpine` for builds/tests
- Migrations are embedded from `internal/store/migrations/` — new migrations must go there
- The `docker-compose.yml` is the production compose (includes Traefik labels)
