package grouping

import (
	"encoding/json"
	"testing"
)

func TestCompute_ExceptionWithStacktrace(t *testing.T) {
	payload := json.RawMessage(`{
		"exception": {
			"values": [{
				"type": "ValueError",
				"value": "invalid input",
				"stacktrace": {
					"frames": [
						{"filename": "base.py", "function": "main"},
						{"filename": "handler.py", "function": "process"}
					]
				}
			}]
		}
	}`)

	fp := Compute(payload)
	if fp == "" {
		t.Error("expected non-empty fingerprint")
	}

	// Same event should produce same fingerprint
	fp2 := Compute(payload)
	if fp != fp2 {
		t.Error("expected deterministic fingerprint")
	}
}

func TestCompute_ExceptionWithoutStacktrace(t *testing.T) {
	payload := json.RawMessage(`{
		"exception": {
			"values": [{"type": "RuntimeError", "value": "something went wrong"}]
		}
	}`)

	fp := Compute(payload)
	if fp == "" {
		t.Error("expected non-empty fingerprint")
	}
}

func TestCompute_MessageOnly(t *testing.T) {
	payload := json.RawMessage(`{"message": "disk full"}`)

	fp := Compute(payload)
	if fp == "" {
		t.Error("expected non-empty fingerprint")
	}
}

func TestCompute_DifferentEventsProduceDifferentFingerprints(t *testing.T) {
	p1 := json.RawMessage(`{"exception":{"values":[{"type":"TypeError","value":"x is not a function"}]}}`)
	p2 := json.RawMessage(`{"exception":{"values":[{"type":"ValueError","value":"invalid literal"}]}}`)

	fp1 := Compute(p1)
	fp2 := Compute(p2)

	if fp1 == fp2 {
		t.Error("different exceptions should produce different fingerprints")
	}
}

func TestTitle_Exception(t *testing.T) {
	payload := json.RawMessage(`{
		"exception": {
			"values": [{"type": "KeyError", "value": "'missing_key'"}]
		}
	}`)

	title := Title(payload)
	expected := "KeyError: 'missing_key'"
	if title != expected {
		t.Errorf("expected %q, got %q", expected, title)
	}
}

func TestTitle_MessageFallback(t *testing.T) {
	payload := json.RawMessage(`{"message": "something happened"}`)

	title := Title(payload)
	if title != "something happened" {
		t.Errorf("expected 'something happened', got %q", title)
	}
}

func TestTitle_UnknownError(t *testing.T) {
	payload := json.RawMessage(`{}`)

	title := Title(payload)
	if title != "Unknown Error" {
		t.Errorf("expected 'Unknown Error', got %q", title)
	}
}
