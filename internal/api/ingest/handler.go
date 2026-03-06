package ingest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/elmisi/ampulla/internal/auth"
	"github.com/elmisi/ampulla/internal/envelope"
	"github.com/elmisi/ampulla/internal/event"
)

type Handler struct {
	processor *event.Processor
}

func NewHandler(p *event.Processor) *Handler {
	return &Handler{processor: p}
}

// Envelope handles POST /api/{projectID}/envelope/
func (h *Handler) Envelope(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"no project in context"}`, http.StatusInternalServerError)
		return
	}

	env, err := envelope.Parse(r.Body)
	if err != nil {
		slog.Warn("envelope parse error", "project", project.ID, "error", err)
		http.Error(w, `{"error":"invalid envelope"}`, http.StatusBadRequest)
		return
	}

	// Process asynchronously — return 200 immediately
	// Use background context since request context will be cancelled after response
	go h.processor.Process(context.Background(), project.ID, env)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"id": env.Header.EventID})
}

// Store handles POST /api/{projectID}/store/ (legacy single-event endpoint)
func (h *Handler) Store(w http.ResponseWriter, r *http.Request) {
	project := auth.ProjectFromContext(r.Context())
	if project == nil {
		http.Error(w, `{"error":"no project in context"}`, http.StatusInternalServerError)
		return
	}

	env, err := envelope.ParseStoreRequest(r.Body)
	if err != nil {
		slog.Warn("store parse error", "project", project.ID, "error", err)
		http.Error(w, `{"error":"invalid event"}`, http.StatusBadRequest)
		return
	}

	go h.processor.Process(context.Background(), project.ID, env)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"id": env.Header.EventID})
}
