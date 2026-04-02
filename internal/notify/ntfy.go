package notify

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// NtfyConfig represents a shared ntfy notification configuration.
type NtfyConfig struct {
	ID    int64
	Name  string
	URL   string
	Topic string
	Token string
}

// NtfyPayload is the data to send in a notification.
type NtfyPayload struct {
	Title    string
	Body     string
	ClickURL string
}

// NtfySender sends notifications via ntfy.
type NtfySender interface {
	Send(ctx context.Context, cfg NtfyConfig, payload NtfyPayload) (statusCode int, responseBody string, err error)
}

// HTTPNtfySender implements NtfySender using a dedicated HTTP client.
type HTTPNtfySender struct {
	client *http.Client
}

// NewHTTPNtfySender creates a sender with a dedicated HTTP client and transport.
func NewHTTPNtfySender() *HTTPNtfySender {
	return &HTTPNtfySender{
		client: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        10,
				IdleConnTimeout:     30 * time.Second,
				DisableCompression:  true,
				DialContext: (&net.Dialer{
					Timeout: 5 * time.Second,
				}).DialContext,
			},
		},
	}
}

// Send dispatches a notification to an ntfy server. The caller should set a
// context timeout (e.g. 10s).
func (s *HTTPNtfySender) Send(ctx context.Context, cfg NtfyConfig, payload NtfyPayload) (int, string, error) {
	endpoint := fmt.Sprintf("%s/%s", strings.TrimRight(cfg.URL, "/"), cfg.Topic)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(payload.Body))
	if err != nil {
		return 0, "", fmt.Errorf("ntfy: build request: %w", err)
	}

	req.Header.Set("Title", payload.Title)
	req.Header.Set("Priority", "default")
	if payload.ClickURL != "" {
		req.Header.Set("Click", payload.ClickURL)
	}
	if cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("ntfy: send: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return resp.StatusCode, string(body), nil
}
