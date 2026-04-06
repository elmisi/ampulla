package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNewWithToken(t *testing.T) {
	c, err := NewWithToken("http://localhost:8090", "ampt_test")
	if err != nil {
		t.Fatalf("NewWithToken: %v", err)
	}
	if c.token != "ampt_test" {
		t.Errorf("token = %q, want ampt_test", c.token)
	}
}

func TestNewWithToken_EmptyToken(t *testing.T) {
	_, err := NewWithToken("http://localhost:8090", "")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestTokenAuth_SendsBearerHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"projects":[]}`))
	}))
	defer srv.Close()

	c, err := NewWithToken(srv.URL, "ampt_secret")
	if err != nil {
		t.Fatalf("NewWithToken: %v", err)
	}
	if _, err := c.GetDashboard(context.Background()); err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if gotAuth != "Bearer ampt_secret" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer ampt_secret")
	}
}

func TestTokenAuth_NoLoginCalled(t *testing.T) {
	var loginCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/admin/login" {
			loginCalls++
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"projects":[]}`))
	}))
	defer srv.Close()

	c, _ := NewWithToken(srv.URL, "ampt_secret")
	c.Login(context.Background()) // should be no-op
	c.GetDashboard(context.Background())

	if loginCalls != 0 {
		t.Errorf("login called %d times with token auth, want 0", loginCalls)
	}
}

func TestNew_RejectsMalformedURL(t *testing.T) {
	for _, bad := range []string{"", "ampulla.example.com", "ftp://example.com"} {
		_, err := New(bad, "u", "p")
		if err == nil {
			t.Errorf("New(%q): expected error for malformed URL", bad)
		}
	}
}

func TestNew_RejectsPlainHTTP(t *testing.T) {
	_, err := New("http://example.com", "u", "p")
	if err == nil {
		t.Fatal("expected error for http:// non-localhost URL")
	}
}

func TestNew_AllowsHTTPLocalhost(t *testing.T) {
	for _, host := range []string{"http://localhost:8090", "http://127.0.0.1:8090", "http://[::1]:8090"} {
		if _, err := New(host, "u", "p"); err != nil {
			t.Errorf("New(%q): unexpected error: %v", host, err)
		}
	}
}

func TestLogin_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/admin/login" && r.Method == "POST" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "tok"})
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "admin", "pass")
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login: %v", err)
	}
}

func TestLogin_BadCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "admin", "wrong")
	err := c.Login(context.Background())
	if err == nil {
		t.Fatal("expected error for bad credentials")
	}
}

func TestRetryOn401(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/admin/login" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "tok"})
			w.WriteHeader(200)
			return
		}
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"projects": []any{}})
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "admin", "pass")
	c.Login(context.Background())

	dash, err := c.GetDashboard(context.Background())
	if err != nil {
		t.Fatalf("GetDashboard after retry: %v", err)
	}
	if dash == nil {
		t.Fatal("expected non-nil dashboard")
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls to dashboard, got %d", calls.Load())
	}
}

func TestGetDashboard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/admin/login" {
			w.WriteHeader(200)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"projects":[{"id":1,"name":"test","slug":"test","platform":"python","issuesTotal":5,"issuesUnresolved":2,"transactionsTotal":100}]}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "admin", "pass")
	c.Login(context.Background())

	dash, err := c.GetDashboard(context.Background())
	if err != nil {
		t.Fatalf("GetDashboard: %v", err)
	}
	if len(dash.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(dash.Projects))
	}
	p := dash.Projects[0]
	if p.ID != 1 || p.Name != "test" || p.IssuesTotal != 5 {
		t.Errorf("unexpected project: %+v", p)
	}
}

func TestListIssues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/admin/login" {
			w.WriteHeader(200)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":10,"project":1,"title":"err","culprit":"fp","level":"error","status":"unresolved","firstSeen":"2026-04-04T10:00:00Z","lastSeen":"2026-04-04T11:00:00Z","count":3}]`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "admin", "pass")
	c.Login(context.Background())

	issues, err := c.ListIssues(context.Background(), 1, "", 25)
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != 10 {
		t.Errorf("unexpected issues: %+v", issues)
	}
}

func TestUserAgent(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		if r.URL.Path == "/api/admin/login" {
			w.WriteHeader(200)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"projects":[]}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "admin", "pass")
	c.Login(context.Background())
	c.GetDashboard(context.Background())

	if ua != "ampulla-mcp/"+version {
		t.Errorf("User-Agent = %q, want %q", ua, "ampulla-mcp/"+version)
	}
}
