package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// fakeAmpulla returns a test server that serves /api/admin/tokens/whoami with
// the supplied status + body.
func fakeAmpulla(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/tokens/whoami" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}))
}

func TestVerifier_ValidToken(t *testing.T) {
	srv := fakeAmpulla(t, 200, `{"id":42,"name":"test","prefix":"ampt_xxxxxxx","createdAt":"2026-01-01T00:00:00Z"}`)
	defer srv.Close()

	verify := newTokenVerifier(srv.URL)
	info, err := verify(context.Background(), "ampt_abc", nil)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil TokenInfo")
	}
	if info.UserID != "token:42" {
		t.Errorf("UserID = %q, want token:42", info.UserID)
	}
	if info.Expiration.IsZero() {
		t.Error("Expiration should be set (SDK requires it)")
	}
	tokenInExtra, _ := info.Extra[tokenExtraKey].(string)
	if tokenInExtra != "ampt_abc" {
		t.Errorf("Extra[%q] = %q, want ampt_abc", tokenExtraKey, tokenInExtra)
	}
}

func TestVerifier_RejectsOn401(t *testing.T) {
	srv := fakeAmpulla(t, 401, `{"error":"invalid token"}`)
	defer srv.Close()

	verify := newTokenVerifier(srv.URL)
	_, err := verify(context.Background(), "ampt_bad", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("401 must map to auth.ErrInvalidToken, got %v", err)
	}
}

func TestVerifier_Preserves5xxAsServerError(t *testing.T) {
	srv := fakeAmpulla(t, 503, `service unavailable`)
	defer srv.Close()

	verify := newTokenVerifier(srv.URL)
	_, err := verify(context.Background(), "ampt_ok", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("5xx must NOT be classified as invalid token, got %v", err)
	}
	if errors.Is(err, auth.ErrOAuth) {
		t.Errorf("5xx must NOT be classified as OAuth error, got %v", err)
	}
}

func TestVerifier_PreservesNetworkErrorAsServerError(t *testing.T) {
	// Point at a closed port on localhost. Dial will fail immediately.
	verify := newTokenVerifier("http://127.0.0.1:1")
	_, err := verify(context.Background(), "ampt_ok", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("network error must NOT be classified as invalid token, got %v", err)
	}
}

func TestVerifier_Preserves404AsServerError(t *testing.T) {
	// Simulate a deployment where /api/admin/tokens/whoami doesn't exist
	// (e.g. Ampulla is too old). Should not be reported as a bad token.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	verify := newTokenVerifier(srv.URL)
	_, err := verify(context.Background(), "ampt_ok", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("404 must NOT be classified as invalid token, got %v", err)
	}
}

// fakeAmpullaRecording returns a test server that:
//   - serves /api/admin/tokens/whoami with a canned 200 OK
//   - serves /api/admin/dashboard with an empty project list
//
// The token captured from the Authorization header of each request is stored
// in gotToken so tests can assert that the full middleware → getServer chain
// forwarded the exact same token the verifier saw.
func fakeAmpullaRecording(t *testing.T, gotToken *atomic.Value) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record the token that arrived on this request.
		authz := r.Header.Get("Authorization")
		if strings.HasPrefix(authz, "Bearer ") {
			gotToken.Store(strings.TrimPrefix(authz, "Bearer "))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/admin/tokens/whoami":
			_, _ = w.Write([]byte(`{"id":1,"name":"test","prefix":"ampt_xxxxxxx","createdAt":"2026-01-01T00:00:00Z"}`))
		case "/api/admin/dashboard":
			_, _ = w.Write([]byte(`{"projects":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// sendInitialize sends an MCP initialize request to mcpURL with the given
// authorization header value and returns the HTTP status and body.
func sendInitialize(t *testing.T, mcpURL, authHeaderValue string) (int, string) {
	t.Helper()
	const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}`
	req, err := http.NewRequest(http.MethodPost, mcpURL+"/", strings.NewReader(initBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if authHeaderValue != "" {
		req.Header.Set("Authorization", authHeaderValue)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// TestHandlerChain_BearerParsingVariants exercises the full middleware chain
// (RequireBearerToken → TokenInfo.Extra → getServer → downstream Ampulla call)
// against the Authorization header parsing variants that RFC 6750 allows.
// It regression-locks the P2 review finding where a reimplemented header
// parser in getServer drifted from the SDK middleware rules.
func TestHandlerChain_BearerParsingVariants(t *testing.T) {
	cases := []struct {
		name       string
		authHeader string
	}{
		{"standard", "Bearer ampt_canonical"},
		{"lowercase_scheme", "bearer ampt_lowercase"},
		{"mixed_case_scheme", "BeArEr ampt_mixedcase"},
		{"extra_inner_whitespace", "Bearer    ampt_whitespace"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotToken atomic.Value
			ampulla := fakeAmpullaRecording(t, &gotToken)
			defer ampulla.Close()

			mcpSrv := httptest.NewServer(newHTTPHandler(ampulla.URL))
			defer mcpSrv.Close()

			status, body := sendInitialize(t, mcpSrv.URL, tc.authHeader)
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", status, body)
			}
			// The response should be an SSE event stream containing the init result.
			if !strings.Contains(body, `"protocolVersion":"2024-11-05"`) {
				t.Errorf("unexpected body: %s", body)
			}

			// The token forwarded to Ampulla must match the one embedded in the
			// header, regardless of header whitespace/case.
			stored, _ := gotToken.Load().(string)
			// Extract expected token from the header variant:
			fields := strings.Fields(tc.authHeader)
			expected := fields[len(fields)-1]
			if stored != expected {
				t.Errorf("token forwarded to Ampulla = %q, want %q", stored, expected)
			}
		})
	}
}

// TestHandlerChain_MissingAuth verifies that the middleware rejects requests
// with no Authorization header before getServer runs.
func TestHandlerChain_MissingAuth(t *testing.T) {
	var gotToken atomic.Value
	ampulla := fakeAmpullaRecording(t, &gotToken)
	defer ampulla.Close()

	mcpSrv := httptest.NewServer(newHTTPHandler(ampulla.URL))
	defer mcpSrv.Close()

	status, _ := sendInitialize(t, mcpSrv.URL, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
	if gotToken.Load() != nil {
		t.Error("no Ampulla request should have been made with missing auth")
	}
}

// TestHandlerChain_UpstreamOutageReturns500 verifies that a backend outage
// does not leak as an auth failure to the MCP client. This regression-locks
// the P2 review finding where all whoami probe errors were mapped to 401.
func TestHandlerChain_UpstreamOutageReturns500(t *testing.T) {
	// Ampulla returns 503 on every request.
	ampulla := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ampulla.Close()

	mcpSrv := httptest.NewServer(newHTTPHandler(ampulla.URL))
	defer mcpSrv.Close()

	status, _ := sendInitialize(t, mcpSrv.URL, "Bearer ampt_backend_is_down")
	if status == http.StatusUnauthorized {
		t.Errorf("status = 401, but backend outage must not be classified as auth failure")
	}
	if status != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", status)
	}
}
