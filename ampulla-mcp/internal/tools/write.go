package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elmisi/ampulla-mcp/internal/client"
)

// --- resolve_issue ---

type resolveIssueArgs struct {
	IssueID int64 `json:"issueId" jsonschema:"issue ID to resolve"`
}

type updateIssueOutput struct {
	IssueID int64  `json:"issueId"`
	Status  string `json:"status"`
}

func registerResolveIssue(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "resolve_issue",
		Description: "Mark an issue as resolved",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args resolveIssueArgs) (*mcp.CallToolResult, updateIssueOutput, error) {
		if err := c.UpdateIssueStatus(ctx, args.IssueID, "resolved"); err != nil {
			return errResult(err), updateIssueOutput{}, nil
		}
		return nil, updateIssueOutput{IssueID: args.IssueID, Status: "resolved"}, nil
	})
}

// --- reopen_issue ---

type reopenIssueArgs struct {
	IssueID int64 `json:"issueId" jsonschema:"issue ID to reopen"`
}

func registerReopenIssue(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "reopen_issue",
		Description: "Reopen a resolved or ignored issue (set status to unresolved)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args reopenIssueArgs) (*mcp.CallToolResult, updateIssueOutput, error) {
		if err := c.UpdateIssueStatus(ctx, args.IssueID, "unresolved"); err != nil {
			return errResult(err), updateIssueOutput{}, nil
		}
		return nil, updateIssueOutput{IssueID: args.IssueID, Status: "unresolved"}, nil
	})
}
