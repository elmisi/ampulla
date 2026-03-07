package event

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/elmisi/ampulla/internal/grouping"
	"github.com/google/uuid"
)

const (
	workerCount = 4
	queueSize   = 1000
)

// Store defines the database operations needed by the processor.
type Store interface {
	UpsertIssue(ctx context.Context, projectID int64, fingerprint, title, level string, ts time.Time) (*Issue, error)
	InsertEvent(ctx context.Context, e *Event) error
	InsertTransaction(ctx context.Context, t *Transaction) (int64, error)
	InsertSpans(ctx context.Context, txnID int64, traceID uuid.UUID, spans []Span) error
}

type job struct {
	projectID int64
	env       *Envelope
}

type Processor struct {
	store Store
	queue chan job
	wg    sync.WaitGroup
}

func NewProcessor(s Store) *Processor {
	p := &Processor{
		store: s,
		queue: make(chan job, queueSize),
	}
	for i := 0; i < workerCount; i++ {
		p.wg.Add(1)
		go p.worker()
	}
	return p
}

func (p *Processor) worker() {
	defer p.wg.Done()
	for j := range p.queue {
		p.Process(context.Background(), j.projectID, j.env)
	}
}

// Enqueue submits a job for async processing. Drops the job if the queue is full.
func (p *Processor) Enqueue(projectID int64, env *Envelope) {
	select {
	case p.queue <- job{projectID: projectID, env: env}:
	default:
		slog.Warn("event queue full, dropping event", "project", projectID, "event", env.Header.EventID)
	}
}

// Close shuts down the worker pool and waits for in-flight jobs to complete.
func (p *Processor) Close() {
	close(p.queue)
	p.wg.Wait()
}

// Process handles all items in an envelope for the given project.
func (p *Processor) Process(ctx context.Context, projectID int64, env *Envelope) {
	for _, item := range env.Items {
		switch item.Type {
		case "event":
			if err := p.processEvent(ctx, projectID, item.Payload); err != nil {
				slog.Error("process event failed", "project", projectID, "error", err)
			}
		case "transaction":
			if err := p.processTransaction(ctx, projectID, item.Payload); err != nil {
				slog.Error("process transaction failed", "project", projectID, "error", err)
			}
		}
	}
}

func (p *Processor) processEvent(ctx context.Context, projectID int64, payload json.RawMessage) error {
	var raw struct {
		EventID   string `json:"event_id"`
		Timestamp any    `json:"timestamp"`
		Platform  string `json:"platform"`
		Level     string `json:"level"`
		Message   string `json:"message"`
		LogEntry  struct {
			Message string `json:"message"`
		} `json:"logentry"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return err
	}

	eventID, err := parseUUID(raw.EventID)
	if err != nil {
		eventID = uuid.New()
	}

	ts := parseTimestamp(raw.Timestamp)
	level := raw.Level
	if level == "" {
		level = "error"
	}

	message := raw.Message
	if message == "" {
		message = raw.LogEntry.Message
	}

	fingerprint := grouping.Compute(payload)
	title := grouping.Title(payload)

	issue, err := p.store.UpsertIssue(ctx, projectID, fingerprint, title, level, ts)
	if err != nil {
		return err
	}

	e := &Event{
		EventID:   eventID,
		ProjectID: projectID,
		IssueID:   issue.ID,
		Timestamp: ts,
		Platform:  raw.Platform,
		Level:     level,
		Message:   message,
		Data:      payload,
	}

	return p.store.InsertEvent(ctx, e)
}

func (p *Processor) processTransaction(ctx context.Context, projectID int64, payload json.RawMessage) error {
	var raw struct {
		EventID        string  `json:"event_id"`
		Transaction    string  `json:"transaction"`
		Op             string  `json:"op"`
		TraceID        string  `json:"trace_id"`
		SpanID         string  `json:"span_id"`
		Status         string  `json:"status"`
		StartTimestamp float64 `json:"start_timestamp"`
		Timestamp      float64 `json:"timestamp"`
		Contexts       struct {
			Trace struct {
				TraceID string `json:"trace_id"`
				SpanID  string `json:"span_id"`
				Op      string `json:"op"`
				Status  string `json:"status"`
			} `json:"trace"`
		} `json:"contexts"`
		Spans []struct {
			SpanID         string  `json:"span_id"`
			ParentSpanID   string  `json:"parent_span_id"`
			Op             string  `json:"op"`
			Description    string  `json:"description"`
			Status         string  `json:"status"`
			StartTimestamp float64 `json:"start_timestamp"`
			Timestamp      float64 `json:"timestamp"`
			Data           json.RawMessage `json:"data"`
		} `json:"spans"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return err
	}

	eventID, err := parseUUID(raw.EventID)
	if err != nil {
		eventID = uuid.New()
	}

	// Trace context can be at top level or nested in contexts.trace
	traceIDStr := raw.TraceID
	if traceIDStr == "" {
		traceIDStr = raw.Contexts.Trace.TraceID
	}
	traceID, err := parseUUID(traceIDStr)
	if err != nil {
		traceID = uuid.New()
	}

	spanID := raw.SpanID
	if spanID == "" {
		spanID = raw.Contexts.Trace.SpanID
	}

	op := raw.Op
	if op == "" {
		op = raw.Contexts.Trace.Op
	}

	status := raw.Status
	if status == "" {
		status = raw.Contexts.Trace.Status
	}

	startTS := time.Unix(int64(raw.StartTimestamp), int64((raw.StartTimestamp-float64(int64(raw.StartTimestamp)))*1e9))
	endTS := time.Unix(int64(raw.Timestamp), int64((raw.Timestamp-float64(int64(raw.Timestamp)))*1e9))
	durationMs := float64(endTS.Sub(startTS).Milliseconds())

	txn := &Transaction{
		EventID:    eventID,
		ProjectID:  projectID,
		TraceID:    traceID,
		SpanID:     spanID,
		Op:         op,
		Name:       raw.Transaction,
		DurationMs: durationMs,
		Status:     status,
		Timestamp:  startTS,
		Data:       payload,
	}

	txnID, err := p.store.InsertTransaction(ctx, txn)
	if err != nil {
		return err
	}

	if len(raw.Spans) == 0 {
		return nil
	}

	spans := make([]Span, 0, len(raw.Spans))
	for _, s := range raw.Spans {
		sStartTS := time.Unix(int64(s.StartTimestamp), int64((s.StartTimestamp-float64(int64(s.StartTimestamp)))*1e9))
		sEndTS := time.Unix(int64(s.Timestamp), int64((s.Timestamp-float64(int64(s.Timestamp)))*1e9))
		sDuration := float64(sEndTS.Sub(sStartTS).Milliseconds())

		spans = append(spans, Span{
			SpanID:       s.SpanID,
			ParentSpanID: s.ParentSpanID,
			Op:           s.Op,
			Description:  s.Description,
			DurationMs:   sDuration,
			Status:       s.Status,
			Timestamp:    sStartTS,
			Data:         s.Data,
		})
	}

	return p.store.InsertSpans(ctx, txnID, traceID, spans)
}

func parseUUID(s string) (uuid.UUID, error) {
	s = strings.ReplaceAll(s, "-", "")
	if len(s) == 32 {
		// Insert hyphens for standard UUID format
		s = s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
	}
	return uuid.Parse(s)
}

func parseTimestamp(v any) time.Time {
	switch t := v.(type) {
	case float64:
		sec := int64(t)
		nsec := int64((t - float64(sec)) * 1e9)
		return time.Unix(sec, nsec)
	case string:
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05.000000Z",
			"2006-01-02T15:04:05",
		} {
			if parsed, err := time.Parse(layout, t); err == nil {
				return parsed
			}
		}
	}
	return time.Now()
}
