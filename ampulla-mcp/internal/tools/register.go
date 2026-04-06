package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elmisi/ampulla-mcp/internal/client"
)

// Register adds all MCP tools to the server.
func Register(s *mcp.Server, c *client.Client) {
	// Read tools
	registerListProjects(s, c)
	registerListIssues(s, c)
	registerGetIssue(s, c)
	registerGetIssueEvents(s, c)
	registerListTransactions(s, c)
	registerGetTransactionSpans(s, c)
	registerGetPerformanceStats(s, c)

	// Write tools
	registerResolveIssue(s, c)
	registerReopenIssue(s, c)
}
