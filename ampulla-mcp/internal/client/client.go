// Package client provides an HTTP client for the Ampulla Admin API.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

const version = "0.1.0"

// Client talks to the Ampulla Admin API.
//
// Two authentication modes:
//   - Bearer token (preferred): set token via NewWithToken
//   - Session cookies (legacy): set user/password via New
type Client struct {
	baseURL    string
	httpClient *http.Client
	user       string
	password   string
	token      string // Bearer token; if set, login flow is skipped

	mu        sync.Mutex
	loginOnce *singleflight
}

// singleflight ensures at most one login runs at a time.
type singleflight struct {
	mu     sync.Mutex
	inflight bool
	result   error
	done     chan struct{}
}

func newSingleflight() *singleflight {
	return &singleflight{}
}

// Do runs fn at most once concurrently. Concurrent callers share the result.
func (sf *singleflight) Do(fn func() error) error {
	sf.mu.Lock()
	if sf.inflight {
		ch := sf.done
		sf.mu.Unlock()
		<-ch
		sf.mu.Lock()
		err := sf.result
		sf.mu.Unlock()
		return err
	}
	sf.inflight = true
	sf.done = make(chan struct{})
	sf.mu.Unlock()

	err := fn()

	sf.mu.Lock()
	sf.result = err
	sf.inflight = false
	close(sf.done)
	sf.mu.Unlock()
	return err
}

// NewWithToken creates a Client that authenticates with a Bearer token.
// baseURL must use https unless it's localhost.
func NewWithToken(baseURL, token string) (*Client, error) {
	c, err := newBase(baseURL)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("AMPULLA_TOKEN is empty")
	}
	c.token = token
	return c, nil
}

// New creates a Client that uses session cookie authentication.
// baseURL must use https unless it's localhost.
func New(baseURL, user, password string) (*Client, error) {
	c, err := newBase(baseURL)
	if err != nil {
		return nil, err
	}
	c.user = user
	c.password = password
	return c, nil
}

// newBase performs URL validation and constructs the shared parts of a Client.
func newBase(baseURL string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid AMPULLA_URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid AMPULLA_URL: must include scheme and host (e.g. https://ampulla.example.com)")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid AMPULLA_URL: scheme must be http or https")
	}
	host := u.Hostname()
	if u.Scheme == "http" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return nil, fmt.Errorf("AMPULLA_URL must use https (http allowed only for localhost)")
	}

	jar, _ := cookiejar.New(nil)
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     &unsecureJar{jar: jar},
		},
		loginOnce: newSingleflight(),
	}, nil
}

// unsecureJar wraps a cookie jar and strips the Secure flag on set.
// This allows the MCP client to reuse session cookies from Ampulla
// (which sets Secure: true) even over http://localhost connections.
// The transport-level security is already enforced by New().
type unsecureJar struct {
	jar http.CookieJar
}

func (j *unsecureJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	for _, c := range cookies {
		c.Secure = false
	}
	j.jar.SetCookies(u, cookies)
}

func (j *unsecureJar) Cookies(u *url.URL) []*http.Cookie {
	return j.jar.Cookies(u)
}

// Login authenticates against the Ampulla Admin API.
// No-op when using token authentication.
func (c *Client) Login(ctx context.Context) error {
	if c.token != "" {
		return nil
	}
	return c.loginOnce.Do(func() error {
		return c.doLogin(ctx)
	})
}

func (c *Client) doLogin(ctx context.Context) error {
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, c.user, c.password)
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/admin/login",
		strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ampulla-mcp/"+version)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("autenticazione fallita — verificare AMPULLA_USER/AMPULLA_PASSWORD")
	}
	slog.Debug("login successful")
	return nil
}

// doGet performs a GET request with automatic 401 retry.
func (c *Client) doGet(ctx context.Context, path string) (*http.Response, error) {
	return c.doRequest(ctx, "GET", path, nil)
}

// doRequest performs an HTTP request with automatic 401 retry (cookie auth only).
func (c *Client) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	resp, err := c.rawRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && c.token == "" {
		// Cookie-based auth: try a fresh login and retry once.
		resp.Body.Close()
		if err := c.Login(ctx); err != nil {
			return nil, err
		}
		return c.rawRequest(ctx, method, path, body)
	}
	return resp, nil
}

func (c *Client) rawRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ampulla-mcp/"+version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.httpClient.Do(req)
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	resp, err := c.doGet(ctx, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// GetDashboard returns project stats from the dashboard endpoint.
func (c *Client) GetDashboard(ctx context.Context) (*DashboardResponse, error) {
	var dash DashboardResponse
	if err := c.getJSON(ctx, "/api/admin/dashboard", &dash); err != nil {
		return nil, err
	}
	return &dash, nil
}

// ListIssues returns issues for a project with cursor pagination.
func (c *Client) ListIssues(ctx context.Context, projectID int64, cursor string, limit int) ([]Issue, error) {
	path := fmt.Sprintf("/api/admin/issues?project=%d&limit=%d", projectID, limit)
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	var issues []Issue
	if err := c.getJSON(ctx, path, &issues); err != nil {
		return nil, err
	}
	return issues, nil
}

// GetIssue returns a single issue by ID.
func (c *Client) GetIssue(ctx context.Context, id int64) (*Issue, error) {
	var issue Issue
	if err := c.getJSON(ctx, fmt.Sprintf("/api/admin/issues/%d", id), &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// ListIssueEvents returns events for an issue with cursor pagination.
func (c *Client) ListIssueEvents(ctx context.Context, issueID int64, cursor string, limit int) ([]Event, error) {
	path := fmt.Sprintf("/api/admin/issues/%d/events?limit=%d", issueID, limit)
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	var events []Event
	if err := c.getJSON(ctx, path, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// ListTransactions returns transactions for a project with cursor pagination.
func (c *Client) ListTransactions(ctx context.Context, projectID int64, cursor string, limit int) ([]Transaction, error) {
	path := fmt.Sprintf("/api/admin/transactions?project=%d&limit=%d", projectID, limit)
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	var txns []Transaction
	if err := c.getJSON(ctx, path, &txns); err != nil {
		return nil, err
	}
	return txns, nil
}

// UpdateIssueStatus changes an issue's status (resolved, unresolved, ignored).
func (c *Client) UpdateIssueStatus(ctx context.Context, id int64, status string) error {
	body, _ := json.Marshal(map[string]string{"status": status})
	resp, err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/admin/issues/%d", id), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PUT /api/admin/issues/%d: status %d", id, resp.StatusCode)
	}
	return nil
}

// GetTransactionSpans returns the spans for a transaction.
func (c *Client) GetTransactionSpans(ctx context.Context, txnID int64) ([]Span, error) {
	var spans []Span
	if err := c.getJSON(ctx, fmt.Sprintf("/api/admin/transactions/%d/spans", txnID), &spans); err != nil {
		return nil, err
	}
	return spans, nil
}

// GetPerformanceStats returns aggregate performance data.
func (c *Client) GetPerformanceStats(ctx context.Context, projectID int64, days int) (*PerformanceStats, error) {
	var stats PerformanceStats
	if err := c.getJSON(ctx, fmt.Sprintf("/api/admin/performance?project=%d&days=%d", projectID, days), &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}
