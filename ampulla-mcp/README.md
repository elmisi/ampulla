# ampulla-mcp

MCP (Model Context Protocol) server for [Ampulla](https://github.com/elmisi/ampulla). Lets AI agents read and act on Ampulla data (issues, events, transactions, performance stats) without parsing raw JSON or copying from the admin UI.

## Two ways to run it

### A) Hosted (production)

If your Ampulla instance is deployed alongside ampulla-mcp (as in the bundled `docker-compose.yml`), the MCP server is reachable at the same hostname under the `/mcp/` path:

```
https://ampulla.elmisi.com/mcp/
```

Each MCP client uses its own API token (created at `https://ampulla.elmisi.com/admin/#/tokens`). The MCP server validates the token against Ampulla on every request and forwards calls on behalf of the caller — there is no shared service token.

`.mcp.json` for a remote HTTP MCP server:

```json
{
  "mcpServers": {
    "ampulla": {
      "url": "https://ampulla.elmisi.com/mcp/",
      "headers": {
        "Authorization": "Bearer ampt_your_token_here"
      }
    }
  }
}
```

### B) Local stdio (development)

Build a local binary and run it as a child process of your MCP client. This mode reuses a single token (or admin credentials) across all calls.

```bash
# Build
docker run --rm -v $(pwd)/ampulla-mcp:/app -w /app golang:1.25-alpine go build ./cmd/ampulla-mcp

# Run as stdio (default)
AMPULLA_URL=https://ampulla.elmisi.com \
AMPULLA_TOKEN=ampt_... \
./ampulla-mcp
```

`.mcp.json` for stdio:

```json
{
  "mcpServers": {
    "ampulla": {
      "command": "/path/to/ampulla-mcp",
      "env": {
        "AMPULLA_URL": "https://ampulla.elmisi.com",
        "AMPULLA_TOKEN": "ampt_..."
      }
    }
  }
}
```

## Transport modes

| Flag | Description |
|------|-------------|
| `-transport stdio` (default) | JSON-RPC over stdin/stdout, single shared token, intended for local use |
| `-transport http` | Streamable HTTP + SSE, per-request Bearer pass-through, intended for hosted use |
| `-http-addr` | Listen address for HTTP transport (default `127.0.0.1:8765`) |

## Authentication

| Mode | Required env vars | Auth source |
|------|-------------------|-------------|
| stdio + token | `AMPULLA_URL`, `AMPULLA_TOKEN` | Single Ampulla API token used for all requests |
| stdio + cookie (legacy) | `AMPULLA_URL`, `AMPULLA_USER`, `AMPULLA_PASSWORD` | Admin credentials, cookie session with retry on 401 |
| http (hosted) | `AMPULLA_URL` | Per-request `Authorization: Bearer ampt_...` from the MCP client |

In HTTP mode, every incoming request must carry an `Authorization: Bearer ampt_...` header. The MCP server validates each token against Ampulla via `GET /api/admin/tokens/whoami` and uses the same token to call Ampulla on the caller's behalf. The MCP server has no credentials of its own.

Session-binding: the MCP HTTP transport prevents token swap mid-session. The first request in a session establishes the token identity; subsequent requests with a different token receive `403 session user mismatch`.

Token revocation in Ampulla (`#/tokens` admin page → Revoke) takes effect immediately on the next request.

Error classification: only a genuine 401 from Ampulla's `/api/admin/tokens/whoami` is reported to MCP clients as an auth failure (401 with `WWW-Authenticate`). Network errors, 5xx responses, and decode failures are surfaced as 500 — MCP clients should distinguish "credentials revoked" (401) from "backend unavailable" (500) and handle them differently.

### `AMPULLA_INSECURE_HTTP`

By default the client refuses `http://` URLs that point to non-localhost hosts. Set `AMPULLA_INSECURE_HTTP=1` to allow `http://` for any host. This is intended for trusted internal networks (e.g. service-to-service inside a Docker compose where TLS is terminated at an upstream proxy). Do not enable for public hosts.

## Tools

### Read tools

| Tool | Description |
|------|-------------|
| `list_projects` | List all projects with issue/transaction counts |
| `list_issues` | List issues with optional status filter and pagination |
| `get_issue` | Get issue details with structured latest event (stacktrace, tags, breadcrumbs) |
| `get_issue_events` | List events for an issue with structured data |
| `list_transactions` | List transactions for a project |
| `get_transaction_spans` | Get all spans for a transaction (capped at 200) |
| `get_performance_stats` | Aggregate performance stats (p50/p75/p95/p99) |

### Write tools

| Tool | Description |
|------|-------------|
| `resolve_issue` | Mark an issue as resolved |
| `reopen_issue` | Reopen a resolved or ignored issue (set to unresolved) |

## Safety

- Sensitive headers (`Authorization`, `Cookie`, `Set-Cookie`) are redacted
- Stacktraces capped at 30 frames, breadcrumbs at 20, tags at 50
- Strings truncated at 1000 bytes
- All logs go to stderr only, no credentials in output

## Tests

```bash
docker run --rm -v $(pwd)/ampulla-mcp:/app -w /app golang:1.25-alpine go test ./...
```
