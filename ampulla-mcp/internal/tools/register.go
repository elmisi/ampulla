package tools

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elmisi/ampulla-mcp/internal/client"
)

// Register adds all MCP tools to the server.
func Register(s *mcp.Server, c *client.Client) {
	registerListProjects(s, c)
	registerListIssues(s, c)
	registerGetIssue(s, c)
	registerGetIssueEvents(s, c)
	registerListTransactions(s, c)
	registerGetPerformanceStats(s, c)
}
