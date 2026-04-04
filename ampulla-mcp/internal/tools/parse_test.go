package tools

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile("../testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return json.RawMessage(data)
}

func TestParsePythonException(t *testing.T) {
	raw := readFixture(t, "event_python_exception.json")
	p := ParseEventData(raw)

	// Stacktrace
	if len(p.Stacktrace) != 2 {
		t.Fatalf("stacktrace: got %d frames, want 2", len(p.Stacktrace))
	}
	if p.Stacktrace[0].Function != "validate" {
		t.Errorf("frame[0].Function = %q, want validate", p.Stacktrace[0].Function)
	}
	if p.Stacktrace[1].Lineno != 100 {
		t.Errorf("frame[1].Lineno = %d, want 100", p.Stacktrace[1].Lineno)
	}

	// Tags
	if len(p.Tags) != 2 {
		t.Fatalf("tags: got %d, want 2", len(p.Tags))
	}
	if p.Tags[0].Key != "environment" || p.Tags[0].Value != "production" {
		t.Errorf("tag[0] = %+v", p.Tags[0])
	}

	// User
	if p.User == nil {
		t.Fatal("user: nil")
	}
	if p.User.ID != "42" || p.User.Email != "alice@example.com" {
		t.Errorf("user = %+v", p.User)
	}

	// Request — Authorization must be redacted
	if p.Request == nil {
		t.Fatal("request: nil")
	}
	if p.Request.URL != "https://example.com/api/submit" {
		t.Errorf("request.URL = %q", p.Request.URL)
	}
	if _, ok := p.Request.Headers["Authorization"]; ok {
		t.Error("Authorization header should be redacted")
	}
	if v, ok := p.Request.Headers["X-Request-ID"]; !ok || v != "abc-123" {
		t.Error("X-Request-ID header should be present")
	}

	// Breadcrumbs
	if len(p.Breadcrumbs) != 2 {
		t.Fatalf("breadcrumbs: got %d, want 2", len(p.Breadcrumbs))
	}
}

func TestParseJSNoStacktrace(t *testing.T) {
	raw := readFixture(t, "event_js_no_stacktrace.json")
	p := ParseEventData(raw)

	if len(p.Stacktrace) != 0 {
		t.Errorf("stacktrace: got %d frames, want 0", len(p.Stacktrace))
	}

	// Tags from object format
	if len(p.Tags) < 2 {
		t.Fatalf("tags: got %d, want >= 2", len(p.Tags))
	}

	// User with ip_address
	if p.User == nil {
		t.Fatal("user: nil")
	}
	if p.User.IPAddr != "192.168.1.1" {
		t.Errorf("user.IPAddr = %q, want 192.168.1.1", p.User.IPAddr)
	}

	// Breadcrumbs from direct array
	if len(p.Breadcrumbs) != 1 {
		t.Fatalf("breadcrumbs: got %d, want 1", len(p.Breadcrumbs))
	}
}

func TestParseNodeTransaction(t *testing.T) {
	raw := readFixture(t, "transaction_node.json")
	p := ParseEventData(raw)

	// Tags from array format
	if len(p.Tags) != 2 {
		t.Fatalf("tags: got %d, want 2", len(p.Tags))
	}

	// Request — Cookie must be redacted
	if p.Request == nil {
		t.Fatal("request: nil")
	}
	if _, ok := p.Request.Headers["Cookie"]; ok {
		t.Error("Cookie header should be redacted")
	}
	if _, ok := p.Request.Headers["Accept"]; !ok {
		t.Error("Accept header should be present")
	}
}

func TestParseEmptyData(t *testing.T) {
	p := ParseEventData(nil)
	if len(p.Stacktrace) != 0 || len(p.Tags) != 0 || len(p.Breadcrumbs) != 0 {
		t.Errorf("expected empty parsed event, got %+v", p)
	}
}

func TestTruncation_LongString(t *testing.T) {
	long := strings.Repeat("a", 2000)
	result := truncStr(long)
	if len(result) != maxStringBytes {
		t.Errorf("truncStr: got %d bytes, want %d", len(result), maxStringBytes)
	}
}

func TestTruncation_ManyFrames(t *testing.T) {
	frames := make([]StackFrame, 50)
	for i := range frames {
		frames[i] = StackFrame{Function: "fn"}
	}
	result := truncateFrames(frames)
	if len(result) != maxStackFrames {
		t.Errorf("truncateFrames: got %d, want %d", len(result), maxStackFrames)
	}
	// Should keep the last N frames
	if result[0].Function != "fn" {
		t.Error("unexpected frame content")
	}
}

func TestTruncation_ManyBreadcrumbs(t *testing.T) {
	bcs := make([]Breadcrumb, 40)
	for i := range bcs {
		bcs[i] = Breadcrumb{Message: "msg"}
	}
	result := truncBreadcrumbs(bcs)
	if len(result) != maxBreadcrumbs {
		t.Errorf("truncBreadcrumbs: got %d, want %d", len(result), maxBreadcrumbs)
	}
}
