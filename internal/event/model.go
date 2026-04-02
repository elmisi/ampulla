package event

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Organization struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"dateCreated"`
}

type Project struct {
	ID              int64      `json:"id"`
	OrgID           int64      `json:"organizationId"`
	Name            string     `json:"name"`
	Slug            string     `json:"slug"`
	Platform        string     `json:"platform,omitempty"`
	CreatedAt       time.Time  `json:"dateCreated"`
	LastTransaction *time.Time `json:"lastTransaction,omitempty"`
	NtfyURL         string     `json:"ntfyUrl,omitempty"`
	NtfyTopic       string     `json:"ntfyTopic,omitempty"`
	NtfyToken       string     `json:"ntfyToken,omitempty"`
	KnownSDKVersion string     `json:"knownSdkVersion,omitempty"`
	LastSDKVersion  string     `json:"lastSdkVersion,omitempty"`
}

type ProjectKey struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	PublicKey string `json:"public"`
	SecretKey string `json:"secret"`
	Label     string `json:"label"`
	IsActive  bool   `json:"isActive"`
}

type Issue struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"project"`
	Title       string    `json:"title"`
	Fingerprint string    `json:"culprit"`
	Level       string    `json:"level"`
	Status      string    `json:"status"`
	FirstSeen   time.Time `json:"firstSeen"`
	LastSeen    time.Time `json:"lastSeen"`
	EventCount  int64     `json:"count"`
}

type Event struct {
	ID         int64           `json:"id"`
	EventID    uuid.UUID       `json:"eventID"`
	ProjectID  int64           `json:"projectID"`
	IssueID    int64           `json:"groupID"`
	Timestamp  time.Time       `json:"dateCreated"`
	Platform   string          `json:"platform,omitempty"`
	Level      string          `json:"level,omitempty"`
	Message    string          `json:"message,omitempty"`
	Data       json.RawMessage `json:"context"`
	ReceivedAt time.Time       `json:"dateReceived"`
}

type Transaction struct {
	ID         int64           `json:"id"`
	EventID    uuid.UUID       `json:"eventID"`
	ProjectID  int64           `json:"projectID"`
	TraceID    uuid.UUID       `json:"traceID"`
	SpanID     string          `json:"spanID"`
	Op         string          `json:"op,omitempty"`
	Name       string          `json:"transaction"`
	DurationMs float64         `json:"duration"`
	Status     string          `json:"status,omitempty"`
	Timestamp  time.Time       `json:"startTimestamp"`
	Data       json.RawMessage `json:"context"`
	ReceivedAt time.Time       `json:"dateReceived"`
}

type Span struct {
	ID            int64           `json:"id"`
	TransactionID int64           `json:"-"`
	TraceID       uuid.UUID       `json:"traceID"`
	SpanID        string          `json:"spanID"`
	ParentSpanID  string          `json:"parentSpanID,omitempty"`
	Op            string          `json:"op,omitempty"`
	Description   string          `json:"description,omitempty"`
	DurationMs    float64         `json:"duration"`
	Status        string          `json:"status,omitempty"`
	Timestamp     time.Time       `json:"startTimestamp"`
	Data          json.RawMessage `json:"data,omitempty"`
}

// UpsertResult wraps an issue with notification context.
type UpsertResult struct {
	Issue        *Issue
	IsNew        bool // first time this fingerprint was seen
	IsRegression bool // was resolved, now has a new event
}

// Performance analytics types

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

type PerformanceStats struct {
	Endpoints       []EndpointStats `json:"endpoints"`
	TotalCount      int64           `json:"totalCount"`
	OldestTimestamp *time.Time      `json:"oldestTransaction"`
}

// ProjectStats holds per-project counts for the dashboard.
type ProjectStats struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	Platform           string `json:"platform,omitempty"`
	IssuesTotal        int64  `json:"issuesTotal"`
	IssuesUnresolved   int64  `json:"issuesUnresolved"`
	TransactionsTotal  int64  `json:"transactionsTotal"`
}

// SDKAlert represents a project with mismatched SDK version.
type SDKAlert struct {
	ProjectID    int64  `json:"projectId"`
	ProjectName  string `json:"projectName"`
	KnownVersion string `json:"knownVersion"`
	LastVersion  string `json:"lastVersion"`
}

// Envelope represents a parsed Sentry envelope
type Envelope struct {
	Header EnvelopeHeader
	Items  []EnvelopeItem
}

type EnvelopeHeader struct {
	EventID string `json:"event_id"`
	DSN     string `json:"dsn"`
	SentAt  string `json:"sent_at"`
}

type EnvelopeItem struct {
	Type    string
	Payload json.RawMessage
}
