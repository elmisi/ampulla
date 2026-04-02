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
