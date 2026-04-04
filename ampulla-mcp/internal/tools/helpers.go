package tools

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func errResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("error: %v", err)}},
		IsError: true,
	}
}

func clampInt(v, min, max, def int) int {
	if v < min || v > max {
		return def
	}
	return v
}
