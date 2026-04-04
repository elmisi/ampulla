package tools

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elmisi/ampulla-mcp/internal/client"
)

type listTransactionsArgs struct {
	ProjectID int64  `json:"projectId" jsonschema:"project ID"`
	Cursor    string `json:"cursor,omitempty" jsonschema:"opaque pagination token"`
	Limit     int    `json:"limit,omitempty" jsonschema:"max results 1-100, default 25"`
}

type listTransactionsOutput struct {
	Transactions []transactionEntry `json:"transactions"`
	NextCursor   string             `json:"nextCursor,omitempty"`
}

type transactionEntry struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Op         string    `json:"op,omitempty"`
	DurationMs float64   `json:"durationMs"`
	Status     string    `json:"status,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

func registerListTransactions(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_transactions",
		Description: "List transactions for a project",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listTransactionsArgs) (*mcp.CallToolResult, listTransactionsOutput, error) {
		limit := clampInt(args.Limit, 1, 100, 25)

		raw, err := c.ListTransactions(ctx, args.ProjectID, args.Cursor, limit)
		if err != nil {
			return errResult(err), listTransactionsOutput{}, nil
		}

		out := listTransactionsOutput{Transactions: make([]transactionEntry, len(raw))}
		for i, t := range raw {
			out.Transactions[i] = transactionEntry{
				ID:         t.ID,
				Name:       t.Name,
				Op:         t.Op,
				DurationMs: t.DurationMs,
				Status:     t.Status,
				Timestamp:  t.Timestamp,
			}
		}

		if len(raw) == limit {
			last := raw[len(raw)-1]
			out.NextCursor = encodeCursor(last.Timestamp, last.ID)
		}

		return nil, out, nil
	})
}
