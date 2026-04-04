// Package cursor provides opaque keyset pagination tokens.
//
// A cursor encodes a (timestamp, id) pair as base64url JSON.
// For backward compatibility, a plain numeric string is accepted
// as an id-only cursor with zero timestamp.
package cursor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Token holds the keyset pagination coordinates.
type Token struct {
	Timestamp time.Time
	ID        int64
}

type wire struct {
	T int64 `json:"t"` // unix microseconds
	I int64 `json:"i"` // row id
}

// Encode returns an opaque cursor string from a timestamp and row id.
func Encode(ts time.Time, id int64) string {
	w := wire{T: ts.UnixMicro(), I: id}
	b, _ := json.Marshal(w)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Decode parses a cursor string. It accepts:
//   - an opaque base64url token (standard)
//   - a plain integer (backward compat: treated as id with zero timestamp)
//   - an empty string (returns zero Token)
func Decode(s string) (Token, error) {
	if s == "" {
		return Token{}, nil
	}

	// Backward compatibility: plain integer cursor.
	if id, err := strconv.ParseInt(s, 10, 64); err == nil {
		return Token{ID: id}, nil
	}

	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Token{}, fmt.Errorf("cursor: invalid encoding: %w", err)
	}

	var w wire
	if err := json.Unmarshal(b, &w); err != nil {
		return Token{}, fmt.Errorf("cursor: invalid payload: %w", err)
	}

	return Token{
		Timestamp: time.UnixMicro(w.T).UTC(),
		ID:        w.I,
	}, nil
}
