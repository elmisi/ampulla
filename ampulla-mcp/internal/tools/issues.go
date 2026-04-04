package tools

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elmisi/ampulla-mcp/internal/client"
)

// --- list_issues ---

type listIssuesArgs struct {
	ProjectID int64  `json:"projectId" jsonschema:"project ID to filter issues"`
	Status    string `json:"status,omitempty" jsonschema:"filter by status: unresolved, resolved, ignored"`
	Cursor    string `json:"cursor,omitempty" jsonschema:"opaque pagination token"`
	Limit     int    `json:"limit,omitempty" jsonschema:"max results 1-100, default 25"`
}

type listIssuesOutput struct {
	Issues    []issueEntry `json:"issues"`
	NextCursor string      `json:"nextCursor,omitempty"`
	Truncated  bool        `json:"truncated"`
}

type issueEntry struct {
	ID         int64     `json:"id"`
	ProjectID  int64     `json:"projectId"`
	Title      string    `json:"title"`
	Level      string    `json:"level"`
	Status     string    `json:"status"`
	FirstSeen  time.Time `json:"firstSeen"`
	LastSeen   time.Time `json:"lastSeen"`
	EventCount int64     `json:"eventCount"`
}

const maxInternalPages = 5

func registerListIssues(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_issues",
		Description: "List issues for a project, optionally filtered by status",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listIssuesArgs) (*mcp.CallToolResult, listIssuesOutput, error) {
		limit := clampInt(args.Limit, 1, 100, 25)

		var collected []issueEntry
		cursor := args.Cursor
		truncated := false

		for page := 0; page < maxInternalPages; page++ {
			raw, err := c.ListIssues(ctx, args.ProjectID, cursor, limit)
			if err != nil {
				return errResult(err), listIssuesOutput{}, nil
			}
			if len(raw) == 0 {
				break
			}

			for _, r := range raw {
				entry := toIssueEntry(r)
				if args.Status == "" || entry.Status == args.Status {
					collected = append(collected, entry)
				}
			}

			// Build cursor from last raw element for next page
			lastRaw := raw[len(raw)-1]
			cursor = encodeCursor(lastRaw.LastSeen, lastRaw.ID)

			if len(collected) >= limit {
				collected = collected[:limit]
				break
			}

			// If we got fewer than requested, no more data
			if len(raw) < limit {
				break
			}
		}

		// If we exhausted the page budget without filling, mark truncated
		if args.Status != "" && len(collected) < limit && cursor != "" {
			truncated = true
		}

		out := listIssuesOutput{
			Issues:    collected,
			Truncated: truncated,
		}
		if out.Issues == nil {
			out.Issues = []issueEntry{}
		}

		// nextCursor points to end of last scanned page
		if len(collected) == limit || truncated {
			out.NextCursor = cursor
		}

		return nil, out, nil
	})
}

func toIssueEntry(r client.Issue) issueEntry {
	return issueEntry{
		ID:         r.ID,
		ProjectID:  r.ProjectID,
		Title:      r.Title,
		Level:      r.Level,
		Status:     r.Status,
		FirstSeen:  r.FirstSeen,
		LastSeen:   r.LastSeen,
		EventCount: r.EventCount,
	}
}

// --- get_issue ---

type getIssueArgs struct {
	IssueID int64 `json:"issueId" jsonschema:"issue ID"`
}

type getIssueOutput struct {
	ID          int64        `json:"id"`
	ProjectID   int64        `json:"projectId"`
	Title       string       `json:"title"`
	Level       string       `json:"level"`
	Status      string       `json:"status"`
	FirstSeen   time.Time    `json:"firstSeen"`
	LastSeen    time.Time    `json:"lastSeen"`
	EventCount  int64        `json:"eventCount"`
	LatestEvent *eventDetail `json:"latestEvent,omitempty"`
}

type eventDetail struct {
	EventID     string       `json:"eventId"`
	Timestamp   time.Time    `json:"timestamp"`
	Platform    string       `json:"platform,omitempty"`
	Message     string       `json:"message,omitempty"`
	Stacktrace  []StackFrame `json:"stacktrace"`
	Tags        []Tag        `json:"tags"`
	Breadcrumbs []Breadcrumb `json:"breadcrumbs"`
}

func registerGetIssue(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_issue",
		Description: "Get a single issue with its latest event (structured stacktrace, tags, breadcrumbs)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getIssueArgs) (*mcp.CallToolResult, getIssueOutput, error) {
		issue, err := c.GetIssue(ctx, args.IssueID)
		if err != nil {
			return errResult(err), getIssueOutput{}, nil
		}

		out := getIssueOutput{
			ID:         issue.ID,
			ProjectID:  issue.ProjectID,
			Title:      issue.Title,
			Level:      issue.Level,
			Status:     issue.Status,
			FirstSeen:  issue.FirstSeen,
			LastSeen:   issue.LastSeen,
			EventCount: issue.EventCount,
		}

		// Fetch latest event
		events, err := c.ListIssueEvents(ctx, args.IssueID, "", 1)
		if err == nil && len(events) > 0 {
			ev := events[0]
			parsed := ParseEventData(ev.Data)
			out.LatestEvent = &eventDetail{
				EventID:     ev.EventID,
				Timestamp:   ev.Timestamp,
				Platform:    ev.Platform,
				Message:     ev.Message,
				Stacktrace:  parsed.Stacktrace,
				Tags:        parsed.Tags,
				Breadcrumbs: parsed.Breadcrumbs,
			}
		}

		return nil, out, nil
	})
}

// --- get_issue_events ---

type getIssueEventsArgs struct {
	IssueID int64  `json:"issueId" jsonschema:"issue ID"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"opaque pagination token"`
	Limit   int    `json:"limit,omitempty" jsonschema:"max results 1-50, default 10"`
}

type getIssueEventsOutput struct {
	Events     []eventListEntry `json:"events"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type eventListEntry struct {
	EventID     string       `json:"eventId"`
	Timestamp   time.Time    `json:"timestamp"`
	Platform    string       `json:"platform,omitempty"`
	Message     string       `json:"message,omitempty"`
	Stacktrace  []StackFrame `json:"stacktrace"`
	Tags        []Tag        `json:"tags"`
	User        *UserInfo    `json:"user,omitempty"`
}

func registerGetIssueEvents(s *mcp.Server, c *client.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_issue_events",
		Description: "List events for an issue with structured stacktraces and tags",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getIssueEventsArgs) (*mcp.CallToolResult, getIssueEventsOutput, error) {
		limit := clampInt(args.Limit, 1, 50, 10)

		raw, err := c.ListIssueEvents(ctx, args.IssueID, args.Cursor, limit)
		if err != nil {
			return errResult(err), getIssueEventsOutput{}, nil
		}

		out := getIssueEventsOutput{Events: make([]eventListEntry, len(raw))}
		for i, ev := range raw {
			parsed := ParseEventData(ev.Data)
			out.Events[i] = eventListEntry{
				EventID:    ev.EventID,
				Timestamp:  ev.Timestamp,
				Platform:   ev.Platform,
				Message:    ev.Message,
				Stacktrace: parsed.Stacktrace,
				Tags:       parsed.Tags,
				User:       parsed.User,
			}
		}

		if len(raw) == limit {
			last := raw[len(raw)-1]
			out.NextCursor = encodeCursor(last.Timestamp, last.ID)
		}

		return nil, out, nil
	})
}
