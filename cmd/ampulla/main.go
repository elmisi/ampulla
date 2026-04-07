package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elmisi/ampulla/internal/admin"
	adminapi "github.com/elmisi/ampulla/internal/api/admin"
	"github.com/elmisi/ampulla/internal/api/ingest"
	"github.com/elmisi/ampulla/internal/api/web"
	"github.com/elmisi/ampulla/internal/auth"
	"github.com/elmisi/ampulla/internal/config"
	"github.com/elmisi/ampulla/internal/event"
	"github.com/elmisi/ampulla/internal/notify"
	"github.com/elmisi/ampulla/internal/observe"
	"github.com/elmisi/ampulla/internal/store"
	"github.com/elmisi/ampulla/internal/version"
	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	setupLogger(cfg.LogLevel)
	slog.Info("ampulla starting", "version", version.String())

	if dsn := os.Getenv("SENTRY_DSN"); dsn != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              dsn,
			Release:          "ampulla@" + version.String(),
			Environment:      os.Getenv("SENTRY_ENVIRONMENT"),
			EnableTracing:    true,
			TracesSampleRate: 0.1,
		}); err != nil {
			slog.Warn("sentry init failed", "error", err)
		} else {
			defer sentry.Flush(2 * time.Second)
			slog.Info("sentry enabled")
		}
	}

	ctx := context.Background()

	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close() // closed after processor.Close() due to defer ordering

	if err := db.RunMigrations(cfg.DatabaseURL); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	ntfySender := notify.NewHTTPNtfySender()

	processor := event.NewProcessor(db, cfg.Domain)
	authMiddleware := auth.NewMiddleware(db)
	ingestHandler := ingest.NewHandler(processor)
	webHandler := web.NewHandler(db)
	defer processor.Close()

	r := chi.NewRouter()
	r.Use(panicObserver)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(requestLogger)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/api/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]string{"version": version.String()}
		if cfg.FrontendSentryDSN != "" {
			resp["sentryDsn"] = cfg.FrontendSentryDSN
		}
		json.NewEncoder(w).Encode(resp)
	})

	// Ingestion endpoints (auth required)
	r.Route("/api/{projectID}", func(r chi.Router) {
		r.Use(authMiddleware.Authenticate)
		r.Post("/envelope/", ingestHandler.Envelope)
		r.Post("/store/", ingestHandler.Store)
	})

	// Admin UI + API + Web API (all require admin to be enabled)
	if cfg.AdminEnabled() {
		adminAuth := admin.NewAuth(cfg.AdminUser, cfg.AdminPassword, cfg.SessionSecret)
		adminHandler := adminapi.NewHandler(db, cfg.Domain, ntfySender)

		// Web API (read-only, session-authenticated)
		r.Route("/api/0", func(r chi.Router) {
			r.Use(sentryTracing)
			r.Use(adminAuth.SessionMiddleware)
			r.Get("/organizations/", webHandler.ListOrganizations)
			r.Get("/organizations/{orgSlug}/projects/", webHandler.ListProjects)
			r.Get("/projects/{orgSlug}/{projectSlug}/issues/", webHandler.ListIssues)
			r.Get("/issues/{issueID}/events/", webHandler.ListEvents)
			r.Get("/organizations/{orgSlug}/events/", webHandler.ListTransactions)
		})

		r.Get("/admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently).ServeHTTP)
		r.Get("/admin/*", admin.UIHandler().ServeHTTP)

		r.Route("/api/admin", func(r chi.Router) {
			r.Use(sentryTracing)
			r.Post("/login", adminAuth.Login)
			r.Post("/logout", adminAuth.Logout)
			r.Group(func(r chi.Router) {
				r.Use(adminAuth.CombinedAuthMiddleware(db))
				r.Get("/me", adminAuth.Me)
				r.Get("/dashboard", adminHandler.Dashboard)

				r.Get("/organizations", adminHandler.ListOrganizations)
				r.Post("/organizations", adminHandler.CreateOrganization)
				r.Put("/organizations/{id}", adminHandler.UpdateOrganization)
				r.Delete("/organizations/{id}", adminHandler.DeleteOrganization)

				r.Get("/organizations/{orgSlug}/projects", adminHandler.ListProjects)
				r.Get("/projects", adminHandler.ListAllProjects)
				r.Post("/projects", adminHandler.CreateProject)
				r.Put("/projects/{id}", adminHandler.UpdateProject)
				r.Delete("/projects/{id}", adminHandler.DeleteProject)

				r.Get("/projects/{id}/keys", adminHandler.ListProjectKeys)
				r.Post("/projects/{id}/keys", adminHandler.CreateProjectKey)
				r.Put("/keys/{id}", adminHandler.ToggleProjectKey)
				r.Delete("/keys/{id}", adminHandler.DeleteProjectKey)

				r.Get("/issues", adminHandler.ListIssues)
				r.Get("/issues/{id}", adminHandler.GetIssue)
				r.Put("/issues/{id}", adminHandler.UpdateIssue)
				r.Delete("/issues/{id}", adminHandler.DeleteIssue)
				r.Get("/issues/{id}/events", adminHandler.ListIssueEvents)

				r.Get("/transactions", adminHandler.ListTransactions)
				r.Get("/transactions/{id}", adminHandler.GetTransaction)
				r.Get("/transactions/{id}/spans", adminHandler.ListTransactionSpans)
				r.Get("/performance", adminHandler.Performance)

				r.Get("/tokens", adminHandler.ListAPITokens)
				r.Post("/tokens", adminHandler.CreateAPIToken)
				r.Get("/tokens/whoami", adminHandler.WhoAmIToken)
				r.Delete("/tokens/{id}", adminHandler.DeleteAPIToken)

				r.Get("/ntfy-configs", adminHandler.ListNtfyConfigs)
				r.Post("/ntfy-configs", adminHandler.CreateNtfyConfig)
				r.Put("/ntfy-configs/{id}", adminHandler.UpdateNtfyConfig)
				r.Delete("/ntfy-configs/{id}", adminHandler.DeleteNtfyConfig)
				r.Post("/ntfy-configs/{id}/test", adminHandler.TestNtfyConfig)
			})
		})
		slog.Info("admin UI enabled", "path", "/admin/")
	}

	srv := &http.Server{
		Addr:         cfg.Addr(),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("starting server", "addr", cfg.Addr())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
}

func setupLogger(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l})))
}

// sentryTracing is a Chi middleware that creates a Sentry transaction for each request.
func sentryTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub := sentry.CurrentHub().Clone()
		ctx := sentry.SetHubOnContext(r.Context(), hub)

		name := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		txn := sentry.StartTransaction(ctx, name, sentry.ContinueFromRequest(r))
		txn.Op = "http.server"
		defer txn.Finish()

		r = r.WithContext(txn.Context())

		ww := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)

		txn.Status = httpStatus(ww.status)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// panicObserver captures panics to slog+Sentry before chi's Recoverer turns them into 500s.
func panicObserver(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer observe.RecoverPanic(r.Context(), "http", "path", r.URL.Path, "method", r.Method)
		next.ServeHTTP(w, r)
	})
}

// requestLogger logs HTTP requests with appropriate log levels.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip /health to reduce noise
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		ww := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)

		duration := time.Since(start)
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.status,
			"duration", duration.String(),
			"remote_ip", r.RemoteAddr,
		}
		if ww.status >= 400 {
			slog.Info("http request", attrs...)
		} else {
			slog.Debug("http request", attrs...)
		}
	})
}

func httpStatus(code int) sentry.SpanStatus {
	switch {
	case code < 400:
		return sentry.SpanStatusOK
	case code == 404:
		return sentry.SpanStatusNotFound
	case code == 403:
		return sentry.SpanStatusPermissionDenied
	case code == 429:
		return sentry.SpanStatusResourceExhausted
	case code < 500:
		return sentry.SpanStatusInvalidArgument
	default:
		return sentry.SpanStatusInternalError
	}
}
