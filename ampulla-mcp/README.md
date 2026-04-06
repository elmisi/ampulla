# ampulla-mcp

MCP (Model Context Protocol) server for [Ampulla](https://github.com/elmisi/ampulla). Lets AI agents read issues, events, transactions, and performance stats from Ampulla without interpreting raw JSON or copying data from the admin UI.

## Quick start

```bash
# Build
docker run --rm -v $(pwd)/ampulla-mcp:/app -w /app golang:1.25-alpine go build ./cmd/ampulla-mcp

# Run
AMPULLA_URL=http://localhost:8090 \
AMPULLA_USER=admin \
AMPULLA_PASSWORD=secret \
./ampulla-mcp
```

## Configuration

The MCP server supports two authentication modes. **Bearer token is preferred**.

### Bearer token (preferred)

Create an API token in the Ampulla admin UI (`#/tokens` page) and set:

| Variable | Required | Description |
|----------|----------|-------------|
| `AMPULLA_URL` | yes | Ampulla instance URL (https required, except localhost) |
| `AMPULLA_TOKEN` | yes | API token (`ampt_...`) |

### Session cookie (legacy)

If `AMPULLA_TOKEN` is not set, the MCP server falls back to admin credentials:

| Variable | Required | Description |
|----------|----------|-------------|
| `AMPULLA_URL` | yes | Ampulla instance URL (https required, except localhost) |
| `AMPULLA_USER` | yes | Admin username |
| `AMPULLA_PASSWORD` | yes | Admin password |

## `.mcp.json` example

```json
{
  "mcpServers": {
    "ampulla": {
      "command": "/path/to/ampulla-mcp",
      "env": {
        "AMPULLA_URL": "https://ampulla.example.com",
        "AMPULLA_TOKEN": "ampt_..."
      }
    }
  }
}
```

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
