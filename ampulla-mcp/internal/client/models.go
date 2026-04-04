package client

import (
	"encoding/json"
	"time"
)

// ProjectStats mirrors the dashboard project entry from Ampulla.
type ProjectStats struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	Platform          string `json:"platform,omitempty"`
	IssuesTotal       int64  `json:"issuesTotal"`
	IssuesUnresolved  int64  `json:"issuesUnresolved"`
	TransactionsTotal int64  `json:"transactionsTotal"`
}

// DashboardResponse is the shape returned by GET /api/admin/dashboard.
type DashboardResponse struct {
	Projects []ProjectStats `json:"projects"`
}

// Issue mirrors an Ampulla issue.
type Issue struct {
	ID         int64     `json:"id"`
	ProjectID  int64     `json:"project"`
	Title      string    `json:"title"`
	Fingerprint string   `json:"culprit"`
	Level      string    `json:"level"`
	Status     string    `json:"status"`
	FirstSeen  time.Time `json:"firstSeen"`
	LastSeen   time.Time `json:"lastSeen"`
	EventCount int64     `json:"count"`
}

// Event mirrors an Ampulla event.
type Event struct {
	ID         int64           `json:"id"`
	EventID    string          `json:"eventID"`
	ProjectID  int64           `json:"projectID"`
	IssueID    int64           `json:"groupID"`
	Timestamp  time.Time       `json:"dateCreated"`
	Platform   string          `json:"platform,omitempty"`
	Level      string          `json:"level,omitempty"`
	Message    string          `json:"message,omitempty"`
	Data       json.RawMessage `json:"context"`
	ReceivedAt time.Time       `json:"dateReceived"`
}

// Transaction mirrors an Ampulla transaction.
type Transaction struct {
	ID         int64           `json:"id"`
	EventID    string          `json:"eventID"`
	ProjectID  int64           `json:"projectID"`
	TraceID    string          `json:"traceID"`
	SpanID     string          `json:"spanID"`
	Op         string          `json:"op,omitempty"`
	Name       string          `json:"transaction"`
	DurationMs float64         `json:"duration"`
	Status     string          `json:"status,omitempty"`
	Timestamp  time.Time       `json:"startTimestamp"`
	Data       json.RawMessage `json:"context"`
	ReceivedAt time.Time       `json:"dateReceived"`
}

// EndpointStats holds per-endpoint performance metrics.
type EndpointStats struct {
	Name  string  `json:"name"`
	Op    string  `json:"op"`
	Count int64   `json:"count"`
	AvgMs float64 `json:"avgMs"`
	P50   float64 `json:"p50"`
	P75   float64 `json:"p75"`
	P95   float64 `json:"p95"`
	P99   float64 `json:"p99"`
}

// PerformanceStats holds aggregate performance data.
type PerformanceStats struct {
	Endpoints       []EndpointStats `json:"endpoints"`
	TotalCount      int64           `json:"totalCount"`
	OldestTimestamp *time.Time      `json:"oldestTransaction"`
}
