package admin

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateToken(t *testing.T) {
	plain, hash, prefix, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	if !strings.HasPrefix(plain, TokenPrefix) {
		t.Errorf("plaintext does not start with %q: %q", TokenPrefix, plain)
	}
	if len(plain) != len(TokenPrefix)+64 {
		t.Errorf("plaintext length = %d, want %d", len(plain), len(TokenPrefix)+64)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64 (sha256 hex)", len(hash))
	}
	if !strings.HasPrefix(prefix, TokenPrefix) {
		t.Errorf("prefix does not start with %q: %q", TokenPrefix, prefix)
	}
	if len(prefix) != 12 {
		t.Errorf("prefix length = %d, want 12", len(prefix))
	}
}

func TestGenerateToken_Unique(t *testing.T) {
	a, _, _, _ := GenerateToken()
	b, _, _, _ := GenerateToken()
	if a == b {
		t.Error("two GenerateToken calls returned the same plaintext")
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	a := HashToken("ampt_abc")
	b := HashToken("ampt_abc")
	if a != b {
		t.Errorf("hash mismatch: %q vs %q", a, b)
	}
}

func TestExtractBearer(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"Bearer ampt_abc", "ampt_abc"},
		{"bearer ampt_abc", ""},        // case-sensitive
		{"Basic dXNlcjpwYXNz", ""},     // wrong scheme
		{"", ""},                       // missing
		{"Bearer ", ""},                // empty token
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		got := extractBearer(req)
		if got != c.want {
			t.Errorf("extractBearer(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}
