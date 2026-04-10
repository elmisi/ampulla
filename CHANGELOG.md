## [0.8.1] - 2026-04-10

### Fixed
- Add CORS headers for browser SDK ingest: Traefik middleware allows cross-origin `POST` from any origin, unblocking Sentry Browser SDKs on external domains (e.g. powder.elmisi.com)

## [0.8.0] - 2026-04-07

### Added
- `GET /api/admin/tokens/whoami` endpoint: returns metadata for the API token authenticating the current request (used by ampulla-mcp HTTP transport for token validation)
- `admin.TokenFromContext()` helper to retrieve the validated APITokenRow from request context
- ampulla-mcp container shipped in the same docker-compose.yml as Ampulla
- Traefik routing for `https://<domain>/mcp/` → ampulla-mcp container with stripprefix middleware

### Changed
- `CombinedAuthMiddleware` now stores the APITokenRow in the request context on successful Bearer auth (transparent: cookie auth path is unchanged)
- ampulla-mcp HTTP transport now uses Bearer pass-through: each MCP client carries its own API token, the MCP server has no service credentials and no shared MCP_AUTH_TOKEN. Validation happens on every request (revocation is instant), session-binding via SDK `auth.TokenInfo.UserID` prevents token swap mid-session.
- ampulla-mcp client gained `AMPULLA_INSECURE_HTTP=1` opt-in escape hatch for trusted internal Docker networks where TLS terminates at an upstream proxy
- ampulla-mcp client `getJSON`: new exported `client.ErrUnauthorized` sentinel, wrapped only on Ampulla 401 responses. Other errors (5xx, network, decode) are preserved as-is so callers can distinguish token revocation from backend outage.
- ampulla-mcp HTTP verifier: the verified token is stashed in `auth.TokenInfo.Extra` and recovered by `getServer` from the request context, eliminating the risk of drift between the SDK middleware's RFC 6750 header parsing and the session-creation path. Only genuine 401s from Ampulla are mapped to `auth.ErrInvalidToken`; transient backend failures surface as 500 to MCP clients instead of masquerading as credential revocations.

## [0.7.0] - 2026-04-06

### Added
- API token authentication for the admin API: `Authorization: Bearer ampt_...`
- Migration 010: `api_tokens` table (sha256 hash storage, name + prefix, last_used tracking)
- Admin endpoints: `GET/POST /api/admin/tokens`, `DELETE /api/admin/tokens/{id}`
- Admin UI page (`#/tokens`) for creating/listing/revoking tokens
- `internal/admin/tokens.go`: token generation, hashing, and `CombinedAuthMiddleware` (Bearer or session cookie)

### Changed
- Admin API routes use `CombinedAuthMiddleware` instead of `SessionMiddleware`, accepting both session cookies and Bearer tokens

## [0.6.0] - 2026-04-04

### Added
- `internal/cursor` package: opaque keyset pagination tokens (base64url JSON with timestamp + id)
- Backward-compatible cursor decoding: plain numeric cursors still accepted

### Fixed
- Keyset pagination now uses `(timestamp, id) < (cursor_ts, cursor_id)` instead of `id > cursor`, fixing potential gaps and duplicates when ordering by `last_seen DESC` or `timestamp DESC`
- All paginated queries now include secondary sort on `id DESC` for deterministic ordering
- Affected endpoints: AdminListIssues, ListEventsByIssue, AdminListTransactions, ListIssues (web), ListTransactions (web)

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
