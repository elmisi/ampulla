package cursor

import (
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	ts := time.Date(2026, 4, 4, 12, 30, 0, 123000, time.UTC) // has microsecond precision
	id := int64(42)

	encoded := Encode(ts, id)
	tok, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode(%q): %v", encoded, err)
	}
	if tok.ID != id {
		t.Errorf("ID = %d, want %d", tok.ID, id)
	}
	if !tok.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", tok.Timestamp, ts)
	}
}

func TestDecode_Empty(t *testing.T) {
	tok, err := Decode("")
	if err != nil {
		t.Fatalf("Decode(''): %v", err)
	}
	if tok.ID != 0 || !tok.Timestamp.IsZero() {
		t.Errorf("expected zero Token, got %+v", tok)
	}
}

func TestDecode_NumericBackwardCompat(t *testing.T) {
	tok, err := Decode("99")
	if err != nil {
		t.Fatalf("Decode('99'): %v", err)
	}
	if tok.ID != 99 {
		t.Errorf("ID = %d, want 99", tok.ID)
	}
	if !tok.Timestamp.IsZero() {
		t.Errorf("Timestamp should be zero for numeric cursor, got %v", tok.Timestamp)
	}
}

func TestDecode_Invalid(t *testing.T) {
	for _, input := range []string{"!!!", "not-base64-json"} {
		_, err := Decode(input)
		if err == nil {
			t.Errorf("Decode(%q) should have failed", input)
		}
	}
}

func TestEncode_Deterministic(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := Encode(ts, 1)
	b := Encode(ts, 1)
	if a != b {
		t.Errorf("Encode not deterministic: %q != %q", a, b)
	}
}
