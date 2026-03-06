package envelope

import (
	"strings"
	"testing"
)

func TestParse_ErrorEvent(t *testing.T) {
	raw := `{"event_id":"abc123","dsn":"https://key@sentry.io/1","sent_at":"2024-01-01T00:00:00Z"}
{"type":"event","length":0}
{"exception":{"values":[{"type":"ValueError","value":"bad input"}]},"event_id":"abc123"}`

	env, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if env.Header.EventID != "abc123" {
		t.Errorf("expected event_id abc123, got %s", env.Header.EventID)
	}

	if len(env.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(env.Items))
	}

	if env.Items[0].Type != "event" {
		t.Errorf("expected type event, got %s", env.Items[0].Type)
	}
}

func TestParse_Transaction(t *testing.T) {
	raw := `{"event_id":"tx123"}
{"type":"transaction"}
{"transaction":"GET /api/users","trace_id":"aaa","span_id":"bbb","start_timestamp":1700000000.0,"timestamp":1700000001.5}`

	env, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(env.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(env.Items))
	}

	if env.Items[0].Type != "transaction" {
		t.Errorf("expected type transaction, got %s", env.Items[0].Type)
	}
}

func TestParse_MixedEnvelope(t *testing.T) {
	raw := `{"event_id":"mix1"}
{"type":"session"}
{"sid":"sess1"}
{"type":"event"}
{"message":"something broke","level":"error"}
{"type":"transaction"}
{"transaction":"POST /checkout","start_timestamp":1700000000,"timestamp":1700000002}`

	env, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Session should be skipped
	if len(env.Items) != 2 {
		t.Fatalf("expected 2 items (event + transaction), got %d", len(env.Items))
	}

	if env.Items[0].Type != "event" {
		t.Errorf("first item should be event, got %s", env.Items[0].Type)
	}
	if env.Items[1].Type != "transaction" {
		t.Errorf("second item should be transaction, got %s", env.Items[1].Type)
	}
}

func TestParse_EmptyEnvelope(t *testing.T) {
	_, err := Parse(strings.NewReader(""))
	if err == nil {
		t.Error("expected error for empty envelope")
	}
}

func TestParseStoreRequest(t *testing.T) {
	raw := `{"event_id":"store1","message":"test error","level":"warning"}`

	env, err := ParseStoreRequest(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if env.Header.EventID != "store1" {
		t.Errorf("expected event_id store1, got %s", env.Header.EventID)
	}

	if len(env.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(env.Items))
	}

	if env.Items[0].Type != "event" {
		t.Errorf("expected type event, got %s", env.Items[0].Type)
	}
}
