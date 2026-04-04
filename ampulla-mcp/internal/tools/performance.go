package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elmisi/ampulla-mcp/internal/client"
)

type getPerformanceStatsArgs struct {
	ProjectID int64 `json:"projectId" jsonschema:"project ID"`
	Days      int   `json:"days,omitempty" jsonschema:"lookback window in days (1-90, default 7)"`
}

type getPerformanceStatsOutput struct {
	TotalCount int64           `json:"totalCount"`
	Endpoints  []endpointEntry `json:"endpoints"`
}

type endpointEntry struct {
	Name  string  `json:"name"`
	Op    string  `json:"op,omitempty"`
	Count int64   `json:"count"`
	AvgMs float64 `json:"avgMs"`
	P50   float64 `json:"p50"`
	P75   float64 `json:"p75"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
}

func registerGetPerformanceStats(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_performance_stats",
		Description: "Get aggregate performance statistics for a project",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getPerformanceStatsArgs) (*mcp.CallToolResult, getPerformanceStatsOutput, error) {
		days := clampInt(args.Days, 1, 90, 7)

		stats, err := c.GetPerformanceStats(ctx, args.ProjectID, days)
		if err != nil {
			return errResult(err), getPerformanceStatsOutput{}, nil
		}

		out := getPerformanceStatsOutput{
			TotalCount: stats.TotalCount,
			Endpoints:  make([]endpointEntry, len(stats.Endpoints)),
		}
		for i, e := range stats.Endpoints {
			out.Endpoints[i] = endpointEntry{
				Name:  e.Name,
				Op:    e.Op,
				Count: e.Count,
				AvgMs: e.AvgMs,
				P50:   e.P50,
				P75:   e.P75,
				P95:   e.P95,
				P99:   e.P99,
			}
		}

		return nil, out, nil
	})
}
