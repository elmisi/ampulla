package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/elmisi/ampulla-mcp/internal/client"
	"github.com/elmisi/ampulla-mcp/internal/tools"
)

// tokenExtraKey is the key used to stash the verified Bearer token inside
// auth.TokenInfo.Extra so the SDK's streamable handler factory can recover it
// without reparsing the Authorization header (which would risk whitespace /
// case-sensitivity drift vs. the SDK middleware).
const tokenExtraKey = "ampulla_token"

// httpTransport runs the MCP server in HTTP transport mode with Bearer
// pass-through authentication.
//
// Each incoming HTTP request must carry an Authorization: Bearer <token>
// header. The token is validated against Ampulla via /api/admin/tokens/whoami,
// and the same token is used to construct a per-session client that talks to
// Ampulla on behalf of the caller. There is no shared service token: the MCP
// server has no credentials of its own.
func httpTransport(addr, ampullaURL string) error {
	rootHandler := newHTTPHandler(ampullaURL)

	slog.Info("MCP HTTP server listening", "addr", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           rootHandler,
		ReadHeaderTimeout: 10 * time.Second,
		// No write timeout: SSE streams may stay open for the full session duration.
		IdleTimeout: 10 * time.Minute,
	}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// newHTTPHandler builds the full HTTP handler chain used by the MCP HTTP
// transport: Bearer auth middleware (from the SDK) wrapping the streamable
// MCP handler. Extracted so that the chain can be exercised end-to-end in
// tests without actually listening on a socket.
func newHTTPHandler(ampullaURL string) http.Handler {
	verifier := newTokenVerifier(ampullaURL)

	// Server factory: called by the SDK once per new session. The session is
	// then bound to the userID from TokenInfo (see streamable_server.go), so
	// subsequent requests with a different token are rejected with 403.
	//
	// We read the token from TokenInfo.Extra (populated by the verifier) rather
	// than reparsing the Authorization header, so parsing rules stay consistent
	// with auth.RequireBearerToken.
	getServer := func(req *http.Request) *mcp.Server {
		info := auth.TokenInfoFromContext(req.Context())
		if info == nil || info.Extra == nil {
			// Should never happen: the middleware runs before the handler.
			slog.Error("getServer: no TokenInfo in request context")
			return nil
		}
		token, _ := info.Extra[tokenExtraKey].(string)
		if token == "" {
			slog.Error("getServer: verified TokenInfo has no token")
			return nil
		}
		c, err := client.NewWithToken(ampullaURL, token)
		if err != nil {
			slog.Error("getServer: client construction failed", "error", err)
			return nil
		}
		s := mcp.NewServer(&mcp.Implementation{
			Name:    "ampulla-mcp",
			Version: "0.1.0",
		}, nil)
		tools.Register(s, c)
		return s
	}

	mcpHandler := mcp.NewStreamableHTTPHandler(getServer, nil)

	// Wrap the SDK handler with the SDK's own Bearer middleware. This enforces
	// RFC 6750 header parsing (case-insensitive scheme, flexible whitespace).
	return auth.RequireBearerToken(verifier, nil)(mcpHandler)
}

// newTokenVerifier returns an auth.TokenVerifier that validates incoming
// Bearer tokens by probing Ampulla's /api/admin/tokens/whoami endpoint.
//
// Error classification:
//   - client.ErrUnauthorized (Ampulla returned 401) → auth.ErrInvalidToken
//     (SDK will return 401 to the MCP client)
//   - any other error (network failure, 5xx, timeout, malformed JSON) →
//     returned as-is (SDK will return 500 to the MCP client)
//
// This distinction is critical: a transient backend outage must not be
// reported to clients as a credential revocation.
func newTokenVerifier(ampullaURL string) auth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		c, err := client.NewWithToken(ampullaURL, token)
		if err != nil {
			// This only fires if ampullaURL itself is malformed (it was
			// validated at startup, so shouldn't happen at runtime). Return
			// as a 500-class error, not an auth error.
			return nil, fmt.Errorf("mcp client construction failed: %w", err)
		}
		who, err := c.WhoAmIToken(ctx)
		if err != nil {
			if errors.Is(err, client.ErrUnauthorized) {
				// Genuine auth failure: map to SDK's ErrInvalidToken so the
				// middleware returns 401 with WWW-Authenticate.
				return nil, fmt.Errorf("%w: token not recognized by Ampulla", auth.ErrInvalidToken)
			}
			// Network / 5xx / JSON decode: preserve as a server-side error so
			// the SDK responds 500, not 401. Clients can then retry or surface
			// a "backend unavailable" state instead of "token revoked".
			return nil, fmt.Errorf("ampulla whoami probe failed: %w", err)
		}
		return &auth.TokenInfo{
			// UserID = stable identifier for session-binding. Using the token
			// ID (numeric) avoids leaking the prefix in SDK logs.
			UserID:     fmt.Sprintf("token:%d", who.ID),
			Expiration: time.Now().Add(1 * time.Hour),
			// Stash the verified token for getServer to retrieve, avoiding
			// any reparsing of the Authorization header.
			Extra: map[string]any{tokenExtraKey: token},
		}, nil
	}
}
