package ingest

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/elmisi/ampulla/internal/auth"
	"github.com/elmisi/ampulla/internal/envelope"
	"github.com/elmisi/ampulla/internal/event"
	"github.com/go-chi/chi/v5"
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

	// Validate URL project ID matches authenticated project
	urlProjectID, err := strconv.ParseInt(chi.URLParam(r, "projectID"), 10, 64)
	if err != nil || urlProjectID != project.ID {
		http.Error(w, `{"error":"project mismatch"}`, http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit

	env, err := envelope.Parse(r.Body)
	if err != nil {
		slog.Warn("envelope parse error", "project", project.ID, "error", err)
		http.Error(w, `{"error":"invalid envelope"}`, http.StatusBadRequest)
		return
	}

	h.processor.Enqueue(project.ID, env)

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

	// Validate URL project ID matches authenticated project
	urlProjectID, err := strconv.ParseInt(chi.URLParam(r, "projectID"), 10, 64)
	if err != nil || urlProjectID != project.ID {
		http.Error(w, `{"error":"project mismatch"}`, http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit

	env, err := envelope.ParseStoreRequest(r.Body)
	if err != nil {
		slog.Warn("store parse error", "project", project.ID, "error", err)
		http.Error(w, `{"error":"invalid event"}`, http.StatusBadRequest)
		return
	}

	h.processor.Enqueue(project.ID, env)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"id": env.Header.EventID})
}
