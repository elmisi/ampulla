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
docker run --rm -v $(pwd):/app -w /app golang:1.23-alpine go build ./cmd/ampulla

# Run all tests
docker run --rm -v $(pwd):/app -w /app golang:1.23-alpine go test ./...

# Run a single package's tests
docker run --rm -v $(pwd):/app -w /app golang:1.23-alpine go test ./internal/envelope/

# Run locally (production compose with Traefik labels)
docker compose up -d

# Deploy
cd ../traefik.services && ./deploy.sh ampulla
```

## Architecture

### Request Flow

1. Sentry SDK sends event to `POST /api/{projectID}/envelope/` (or `/store/`)
2. `auth.Middleware` extracts DSN public key from `X-Sentry-Auth` header or `sentry_key` query param, validates against DB (cached 5 min)
3. Handler parses the Sentry envelope wire format (newline-delimited JSON), returns 200 immediately
4. `event.Processor` runs async (`go` + `context.Background()`): computes fingerprint, upserts issue, stores event/transaction/spans

### Three API Surfaces

- **Ingestion** (`/api/{projectID}/envelope/`, `/store/`) — Sentry-compatible, DSN key auth via middleware
- **Web API** (`/api/0/...`) — read-only Sentry-compatible endpoints, no auth (MVP)
- **Admin API** (`/api/admin/...`) — CRUD for orgs/projects/keys/issues, session-authenticated (HMAC-SHA256 cookies)

### Key Packages

- `cmd/ampulla/main.go` — entrypoint, router wiring, graceful shutdown
- `internal/admin/` — session auth (`auth.go`) + embedded single-file admin UI (`ui.go` embeds `index.html`)
- `internal/api/admin/` — admin CRUD handlers
- `internal/api/ingest/` — ingestion handlers (envelope + legacy store)
- `internal/api/web/` — read-only Sentry-compatible API
- `internal/auth/` — DSN public key extraction and validation middleware (in-memory cache with TTL)
- `internal/envelope/` — Sentry envelope wire format parser
- `internal/event/` — domain models (`model.go`) and event processor (`processor.go`)
- `internal/grouping/` — fingerprinting: exception type + value + top frame → SHA-256
- `internal/store/` — PostgreSQL repository (single `postgres.go`) + embedded migrations

### Design Decisions

- Synchronous processing, no queue — adequate for 1k-5k events/month
- JSONB columns preserve full Sentry event payloads
- Migrations embedded in binary via `embed.FS` from `internal/store/migrations/`
- Admin UI is a single `index.html` embedded in the Go binary; enabled when `ADMIN_USER` + `ADMIN_PASSWORD` are set
- Multi-stage Dockerfile: `golang:1.23-alpine` builder → `alpine:3.21` runtime
- `docker-compose.yml` is the production compose (includes Traefik labels)

## Database

PostgreSQL 16. Tables: `organizations`, `projects`, `project_keys`, `issues`, `events`, `transactions`, `spans`. All migrations in `internal/store/migrations/` (currently single `001_initial`).

## DSN Format

```
https://<public_key>@ampulla.elmisi.com/<project_id>
```

## Configuration

All via environment variables. See `.env.example` for defaults. Key vars: `DATABASE_URL` (required), `ADMIN_USER`/`ADMIN_PASSWORD` (enable admin UI), `SESSION_SECRET` (auto-generated if unset), `AMPULLA_DOMAIN`.
