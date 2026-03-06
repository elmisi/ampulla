package grouping

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// Compute generates a fingerprint for an error event payload.
// It extracts the exception type, value, and top stack frame,
// matching Sentry's default grouping behavior.
func Compute(payload json.RawMessage) string {
	var data struct {
		Exception struct {
			Values []struct {
				Type       string `json:"type"`
				Value      string `json:"value"`
				Stacktrace struct {
					Frames []struct {
						Filename string `json:"filename"`
						Function string `json:"function"`
						Module   string `json:"module"`
					} `json:"frames"`
				} `json:"stacktrace"`
			} `json:"values"`
		} `json:"exception"`
		Message string `json:"message"`
		LogEntry struct {
			Message string `json:"message"`
		} `json:"logentry"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		return hashString(string(payload))
	}

	// Try exception-based fingerprinting
	if len(data.Exception.Values) > 0 {
		exc := data.Exception.Values[len(data.Exception.Values)-1] // last = most relevant
		parts := exc.Type + "|" + exc.Value

		// Add top frame (last frame in Sentry's convention = most recent)
		if frames := exc.Stacktrace.Frames; len(frames) > 0 {
			top := frames[len(frames)-1]
			loc := top.Filename
			if loc == "" {
				loc = top.Module
			}
			parts += "|" + loc + "|" + top.Function
		}

		return hashString(parts)
	}

	// Fallback: message-based fingerprinting
	msg := data.Message
	if msg == "" {
		msg = data.LogEntry.Message
	}
	if msg != "" {
		return hashString(msg)
	}

	// Last resort: hash the entire payload
	return hashString(string(payload))
}

// Title extracts a human-readable title from the event payload.
func Title(payload json.RawMessage) string {
	var data struct {
		Exception struct {
			Values []struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"values"`
		} `json:"exception"`
		Message string `json:"message"`
		LogEntry struct {
			Message string `json:"message"`
		} `json:"logentry"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		return "Unknown Error"
	}

	if len(data.Exception.Values) > 0 {
		exc := data.Exception.Values[len(data.Exception.Values)-1]
		if exc.Type != "" && exc.Value != "" {
			return exc.Type + ": " + exc.Value
		}
		if exc.Type != "" {
			return exc.Type
		}
		if exc.Value != "" {
			return exc.Value
		}
	}

	if data.Message != "" {
		return truncate(data.Message, 200)
	}
	if data.LogEntry.Message != "" {
		return truncate(data.LogEntry.Message, 200)
	}

	return "Unknown Error"
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
