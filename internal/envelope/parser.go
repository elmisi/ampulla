package envelope

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/elmisi/ampulla/internal/event"
)

// Parse reads a Sentry envelope from r and returns the parsed envelope.
// Format: newline-delimited JSON. First line is the envelope header,
// then alternating item headers and payloads.
func Parse(r io.Reader) (*event.Envelope, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 512*1024), 1024*1024)

	// Read envelope header
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty envelope")
	}
	var header event.EnvelopeHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return nil, fmt.Errorf("parse envelope header: %w", err)
	}

	env := &event.Envelope{Header: header}

	// Read items: each item has a header line followed by a payload line
	for scanner.Scan() {
		itemHeaderBytes := scanner.Bytes()
		if len(itemHeaderBytes) == 0 {
			continue
		}

		var itemHeader struct {
			Type   string `json:"type"`
			Length int    `json:"length"`
		}
		if err := json.Unmarshal(itemHeaderBytes, &itemHeader); err != nil {
			// Skip malformed item headers
			continue
		}

		if itemHeader.Type == "" {
			continue
		}

		// Read payload line
		if !scanner.Scan() {
			break
		}
		payload := make([]byte, len(scanner.Bytes()))
		copy(payload, scanner.Bytes())

		if len(payload) == 0 {
			continue
		}

		// Only process supported types
		switch itemHeader.Type {
		case "event", "transaction":
			env.Items = append(env.Items, event.EnvelopeItem{
				Type:    itemHeader.Type,
				Payload: json.RawMessage(payload),
			})
		default:
			// Silently skip unsupported types (session, attachment, etc.)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read envelope: %w", err)
	}

	return env, nil
}

// ParseStoreRequest parses a legacy /store/ request body (single JSON event).
func ParseStoreRequest(r io.Reader) (*event.Envelope, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read store body: %w", err)
	}

	// Extract event_id if present
	var partial struct {
		EventID string `json:"event_id"`
	}
	json.Unmarshal(body, &partial)

	return &event.Envelope{
		Header: event.EnvelopeHeader{EventID: partial.EventID},
		Items: []event.EnvelopeItem{
			{Type: "event", Payload: json.RawMessage(body)},
		},
	}, nil
}
