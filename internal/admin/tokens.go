package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// TokenPrefix is the prefix used for all Ampulla API tokens.
const TokenPrefix = "ampt_"

// TokenStore is the database contract for API tokens.
type TokenStore interface {
	CreateAPIToken(ctx context.Context, name, hash, prefix string) (int64, time.Time, error)
	ListAPITokens(ctx context.Context) ([]APITokenRow, error)
	DeleteAPIToken(ctx context.Context, id int64) error
	FindAPITokenByHash(ctx context.Context, hash string) (*APITokenRow, error)
	TouchAPIToken(ctx context.Context, id int64) error
}

// APITokenRow is the storage representation of an API token.
type APITokenRow struct {
	ID         int64
	Name       string
	Prefix     string
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// GenerateToken creates a new random token (returns plaintext, hash, prefix).
func GenerateToken() (plaintext, hash, prefix string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", "", err
	}
	body := hex.EncodeToString(buf)
	plaintext = TokenPrefix + body
	hash = HashToken(plaintext)
	prefix = plaintext[:12] // ampt_ + 7 hex chars
	return plaintext, hash, prefix, nil
}

// HashToken computes the sha256 hex digest of a token (constant length 64).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// extractBearer returns the token from "Authorization: Bearer <token>" or empty string.
func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}

// tokenContextKey is the context key for the authenticated APITokenRow.
type tokenContextKey struct{}

// TokenFromContext returns the APITokenRow associated with the request, or nil
// when the request was authenticated via session cookie or no auth.
func TokenFromContext(ctx context.Context) *APITokenRow {
	v := ctx.Value(tokenContextKey{})
	if v == nil {
		return nil
	}
	return v.(*APITokenRow)
}

// CombinedAuthMiddleware accepts either a Bearer token or a session cookie.
//
// Logic:
//   - Authorization: Bearer <token> present → validate token, no cookie check
//   - No Authorization header → validate session cookie
//   - Invalid Bearer token → 401 (does not fall back to cookie)
//
// On successful Bearer auth, the APITokenRow is stored in the request context
// (retrievable via TokenFromContext).
func (a *Auth) CombinedAuthMiddleware(store TokenStore) func(http.Handler) http.Handler {
	sessionMW := a.SessionMiddleware
	return func(next http.Handler) http.Handler {
		sessionGuarded := sessionMW(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearer(r)
			if token == "" {
				// No Bearer token: require session cookie
				sessionGuarded.ServeHTTP(w, r)
				return
			}

			hash := HashToken(token)
			row, err := store.FindAPITokenByHash(r.Context(), hash)
			if err != nil || row == nil {
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}

			// Best-effort touch (do not block request on failure)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				_ = store.TouchAPIToken(ctx, row.ID)
			}()

			ctx := context.WithValue(r.Context(), tokenContextKey{}, row)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// validateTokenName ensures the name is non-empty and within length limits.
func validateTokenName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 100 {
		return fmt.Errorf("name max 100 chars")
	}
	return nil
}
