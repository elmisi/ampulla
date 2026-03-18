package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/elmisi/ampulla/internal/event"
	"github.com/elmisi/ampulla/internal/store"
)

type contextKey string

const (
	projectContextKey    contextKey = "project"
	sdkClientContextKey  contextKey = "sentry_client"
)

// ProjectFromContext retrieves the authenticated project from request context.
func ProjectFromContext(ctx context.Context) *event.Project {
	p, _ := ctx.Value(projectContextKey).(*event.Project)
	return p
}

// SDKClientFromContext retrieves the sentry_client value from request context.
func SDKClientFromContext(ctx context.Context) string {
	s, _ := ctx.Value(sdkClientContextKey).(string)
	return s
}

type cachedKey struct {
	project   *event.Project
	key       *event.ProjectKey
	fetchedAt time.Time
}

type Middleware struct {
	db          *store.DB
	cache       map[string]*cachedKey
	mu          sync.RWMutex
	ttl         time.Duration
	lastCleanup time.Time
}

func NewMiddleware(db *store.DB) *Middleware {
	return &Middleware{
		db:          db,
		cache:       make(map[string]*cachedKey),
		ttl:         5 * time.Minute,
		lastCleanup: time.Now(),
	}
}

func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		publicKey := extractPublicKey(r)
		if publicKey == "" {
			http.Error(w, `{"error":"missing authentication"}`, http.StatusUnauthorized)
			return
		}

		project, key, err := m.lookupKey(r.Context(), publicKey)
		if err != nil {
			slog.Warn("auth failed", "key", publicKey, "error", err)
			http.Error(w, `{"error":"invalid key"}`, http.StatusUnauthorized)
			return
		}

		if !key.IsActive {
			http.Error(w, `{"error":"key disabled"}`, http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), projectContextKey, project)
		if client := extractSentryClient(r); client != "" {
			ctx = context.WithValue(ctx, sdkClientContextKey, client)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *Middleware) lookupKey(ctx context.Context, publicKey string) (*event.Project, *event.ProjectKey, error) {
	m.mu.RLock()
	if cached, ok := m.cache[publicKey]; ok && time.Since(cached.fetchedAt) < m.ttl {
		m.mu.RUnlock()
		return cached.project, cached.key, nil
	}
	m.mu.RUnlock()

	project, key, err := m.db.GetProjectByKey(ctx, publicKey)
	if err != nil {
		return nil, nil, err
	}

	m.mu.Lock()
	m.cache[publicKey] = &cachedKey{
		project:   project,
		key:       key,
		fetchedAt: time.Now(),
	}
	m.evictExpired()
	m.mu.Unlock()

	return project, key, nil
}

// evictExpired removes stale cache entries. Must be called with m.mu held for writing.
func (m *Middleware) evictExpired() {
	now := time.Now()
	if now.Sub(m.lastCleanup) < time.Minute {
		return
	}
	m.lastCleanup = now
	for k, v := range m.cache {
		if now.Sub(v.fetchedAt) >= m.ttl {
			delete(m.cache, k)
		}
	}
}

// extractSentryClient gets the sentry_client value from the X-Sentry-Auth header.
// Format: Sentry sentry_version=7, sentry_client=sentry.python/1.45.2, sentry_key=<key>
func extractSentryClient(r *http.Request) string {
	auth := r.Header.Get("X-Sentry-Auth")
	if auth == "" {
		return ""
	}
	for _, part := range strings.Split(auth, ",") {
		part = strings.TrimSpace(part)
		part = strings.TrimPrefix(part, "Sentry ")
		if strings.HasPrefix(part, "sentry_client=") {
			return strings.TrimPrefix(part, "sentry_client=")
		}
	}
	return ""
}

// extractPublicKey gets the DSN public key from the request.
// Checks X-Sentry-Auth header first, then sentry_key query param.
func extractPublicKey(r *http.Request) string {
	// Check X-Sentry-Auth header
	// Format: Sentry sentry_version=7, sentry_client=..., sentry_key=<key>
	auth := r.Header.Get("X-Sentry-Auth")
	if auth != "" {
		for _, part := range strings.Split(auth, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "sentry_key=") {
				return strings.TrimPrefix(part, "sentry_key=")
			}
			// Handle "Sentry sentry_key=..." (first param without leading comma)
			if strings.HasPrefix(part, "Sentry ") {
				for _, subpart := range strings.Split(part, ",") {
					subpart = strings.TrimSpace(subpart)
					subpart = strings.TrimPrefix(subpart, "Sentry ")
					if strings.HasPrefix(subpart, "sentry_key=") {
						return strings.TrimPrefix(subpart, "sentry_key=")
					}
				}
			}
		}
	}

	// Check query parameter
	if key := r.URL.Query().Get("sentry_key"); key != "" {
		return key
	}

	return ""
}
