## [0.3.0] - 2026-03-24

### Added
- Environment-based issue separation: same error in dev and prod creates separate issues
- Environment column on issues and events tables (migration 006)
- Environment filter input on issues list page
- Environment badge on issue detail header

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
