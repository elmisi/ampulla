package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elmisi/ampulla-mcp/internal/client"
)

type listProjectsArgs struct{}

type listProjectsOutput struct {
	Projects []projectEntry `json:"projects"`
}

type projectEntry struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	Platform          string `json:"platform,omitempty"`
	IssuesTotal       int64  `json:"issuesTotal"`
	IssuesUnresolved  int64  `json:"issuesUnresolved"`
	TransactionsTotal int64  `json:"transactionsTotal"`
}

func registerListProjects(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_projects",
		Description: "List all projects with issue and transaction counts",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ listProjectsArgs) (*mcp.CallToolResult, listProjectsOutput, error) {
		dash, err := c.GetDashboard(ctx)
		if err != nil {
			return errResult(err), listProjectsOutput{}, nil
		}

		out := listProjectsOutput{Projects: make([]projectEntry, len(dash.Projects))}
		for i, p := range dash.Projects {
			out.Projects[i] = projectEntry{
				ID:                p.ID,
				Name:              p.Name,
				Slug:              p.Slug,
				Platform:          p.Platform,
				IssuesTotal:       p.IssuesTotal,
				IssuesUnresolved:  p.IssuesUnresolved,
				TransactionsTotal: p.TransactionsTotal,
			}
		}
		return nil, out, nil
	})
}
