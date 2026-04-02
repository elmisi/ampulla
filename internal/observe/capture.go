package observe

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
)

// Error logs an error via slog and captures it to Sentry best-effort.
func Error(ctx context.Context, msg string, err error, attrs ...any) {
	args := append([]any{"error", err}, attrs...)
	slog.Error(msg, args...)

	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		hub.CaptureException(err)
	} else {
		sentry.CaptureException(err)
	}
}

// Message logs a message via slog and captures it to Sentry best-effort.
func Message(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	slog.Log(ctx, level, msg, attrs...)

	if level >= slog.LevelWarn {
		if hub := sentry.GetHubFromContext(ctx); hub != nil {
			hub.CaptureMessage(msg)
		} else {
			sentry.CaptureMessage(msg)
		}
	}
}

// RecoverPanic recovers from a panic, logs it with a full stack trace, and
// sends it to Sentry best-effort. Use as: defer observe.RecoverPanic(ctx, "worker", "project", projectID)
func RecoverPanic(ctx context.Context, where string, attrs ...any) {
	r := recover()
	if r == nil {
		return
	}

	stack := string(debug.Stack())
	args := append([]any{"panic", r, "where", where, "stack", stack}, attrs...)
	slog.Error("panic recovered", args...)

	err, ok := r.(error)
	if !ok {
		err = fmt.Errorf("panic: %v", r)
	}

	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		hub.RecoverWithContext(ctx, err)
	} else {
		sentry.CurrentHub().RecoverWithContext(ctx, err)
	}
}

// throttleEntry tracks the last execution time for a Throttled key.
type throttleEntry struct {
	mu   sync.Mutex
	last time.Time
}

var (
	throttleMu      sync.Mutex
	throttleEntries = make(map[string]*throttleEntry)
)

// Throttled executes fn at most once per key per interval. Thread-safe.
func Throttled(key string, interval time.Duration, fn func()) {
	throttleMu.Lock()
	entry, ok := throttleEntries[key]
	if !ok {
		entry = &throttleEntry{}
		throttleEntries[key] = entry
	}
	throttleMu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := time.Now()
	if now.Sub(entry.last) < interval {
		return
	}
	entry.last = now
	fn()
}
