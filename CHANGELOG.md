## [0.5.0] - 2026-04-02

### Added
- Ingest backpressure: returns 503 with Retry-After when queue is full instead of silently dropping events
- `internal/observe` package for self-monitoring (slog + Sentry best-effort, panic recovery, throttled alerts)
- `internal/notify` package with dedicated NtfySender service and HTTP client
- Shared ntfy configurations model (`ntfy_configurations` table) replacing per-project inline config
- Admin CRUD API and UI page for managing ntfy configurations (`#/ntfy`)
- Project form uses ntfy config selector instead of inline URL/topic/token fields
- Test endpoint for ntfy configurations (`POST /api/admin/ntfy-configs/{id}/test`)
- Worker and ntfy goroutine panic recovery with full stack traces
- Processor shutdown with 15s timeout and diagnostic logging
- Active workers/ntfy/queue drop atomic counters for observability
- HTTP request logging middleware (Debug < 400, Info >= 400, /health excluded)
- HTTP panic observer middleware (captures to Sentry before chi Recoverer)
- Lazy eviction cleanup for login rate limiter memory

### Changed
- Migration 009: auto-migrates existing inline ntfy config to shared model with deduplication
- Processor.Enqueue() returns bool for backpressure signaling
- ntfy goroutines tracked in WaitGroup for clean shutdown
- Queue drops reported to Sentry via throttled aggregation (max 1/minute)

## [0.4.0] - 2026-04-02

### Added
- Sentry Browser SDK integration for admin UI error and performance tracking
- New `SENTRY_FRONTEND_DSN` env var to configure frontend self-monitoring
- `/api/version` endpoint now includes `sentryDsn` when configured

### Fixed
- `extractSentryClient` now reads `sentry_client` from query params (Browser SDKs use this instead of `X-Sentry-Auth` header)

## [0.3.0] - 2026-03-24

### Removed
- Reverted environment-based issue separation (migration 007 removes environment columns)
- Environment is still visible per-event in the Event Details tab from JSONB payload
- Recommended approach: create separate projects per environment (e.g. myapp-prod, myapp-dev)

## [0.2.0] - 2026-03-24

### Added
- Enhanced issue detail view with structured stacktrace, event details, breadcrumbs tabs
- Issue header with status badges, first/last seen, event count, resolve/reopen controls
- Event navigation (prev/next) within an issue
- Collapsible stack frames with in-app vs library distinction

### Changed
- Refactored admin UI from monolithic index.html into ES6 modules (14 files)
- Static assets served from embedded static/ directory

## [0.1.0] - 2026-03-18

### Added
- Initial versioned release
- Sentry-compatible error and performance tracking
- Admin UI with issue detail, transaction list, performance stats
- Per-project ntfy notifications on new issues and regressions
- Self-monitoring via Sentry Go SDK
- 30-day retention for transactions/spans
