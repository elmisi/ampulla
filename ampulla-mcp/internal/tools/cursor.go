package tools

import (
	"encoding/base64"
	"encoding/json"
	"time"
)

type cursorWire struct {
	T int64 `json:"t"` // unix microseconds
	I int64 `json:"i"` // row id
}

// encodeCursor creates an opaque cursor from a timestamp and ID.
func encodeCursor(ts time.Time, id int64) string {
	b, _ := json.Marshal(cursorWire{T: ts.UnixMicro(), I: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

// issuesCursor returns a cursor for the last issue in the slice (keyed on LastSeen).
func issuesCursor(issues []issueEntry) string {
	if len(issues) == 0 {
		return ""
	}
	last := issues[len(issues)-1]
	return encodeCursor(last.LastSeen, last.ID)
}
