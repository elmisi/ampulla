package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elmisi/ampulla/internal/api/ingest"
	"github.com/elmisi/ampulla/internal/api/web"
	"github.com/elmisi/ampulla/internal/auth"
	"github.com/elmisi/ampulla/internal/config"
	"github.com/elmisi/ampulla/internal/event"
	"github.com/elmisi/ampulla/internal/store"
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

	ctx := context.Background()

	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.RunMigrations(cfg.DatabaseURL); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	processor := event.NewProcessor(db)
	authMiddleware := auth.NewMiddleware(db)
	ingestHandler := ingest.NewHandler(processor)
	webHandler := web.NewHandler(db)

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Ingestion endpoints (auth required)
	r.Route("/api/{projectID}", func(r chi.Router) {
		r.Use(authMiddleware.Authenticate)
		r.Post("/envelope/", ingestHandler.Envelope)
		r.Post("/store/", ingestHandler.Store)
	})

	// Web API (read-only, no auth for MVP)
	r.Route("/api/0", func(r chi.Router) {
		r.Get("/organizations/", webHandler.ListOrganizations)
		r.Get("/organizations/{orgSlug}/projects/", webHandler.ListProjects)
		r.Get("/projects/{orgSlug}/{projectSlug}/issues/", webHandler.ListIssues)
		r.Get("/issues/{issueID}/events/", webHandler.ListEvents)
		r.Get("/organizations/{orgSlug}/events/", webHandler.ListTransactions)
	})

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
