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
