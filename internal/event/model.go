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
	ID        int64     `json:"id"`
	OrgID     int64     `json:"organizationId"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Platform  string    `json:"platform,omitempty"`
	CreatedAt time.Time `json:"dateCreated"`
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
