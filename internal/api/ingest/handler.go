package ingest

import (
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"io"
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

	body, err := maybeDecompress(r)
	if err != nil {
		slog.Warn("decompress error", "project", project.ID, "error", err)
		http.Error(w, `{"error":"decompression failed"}`, http.StatusBadRequest)
		return
	}
	defer body.Close()

	env, err := envelope.Parse(body)
	if err != nil {
		slog.Warn("envelope parse error", "project", project.ID, "error", err)
		http.Error(w, `{"error":"invalid envelope"}`, http.StatusBadRequest)
		return
	}

	sdkClient := auth.SDKClientFromContext(r.Context())
	if !h.processor.Enqueue(project.ID, env, sdkClient) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, `{"error":"server overloaded"}`, http.StatusServiceUnavailable)
		return
	}

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

	body, err := maybeDecompress(r)
	if err != nil {
		slog.Warn("decompress error", "project", project.ID, "error", err)
		http.Error(w, `{"error":"decompression failed"}`, http.StatusBadRequest)
		return
	}
	defer body.Close()

	env, err := envelope.ParseStoreRequest(body)
	if err != nil {
		slog.Warn("store parse error", "project", project.ID, "error", err)
		http.Error(w, `{"error":"invalid event"}`, http.StatusBadRequest)
		return
	}

	sdkClient2 := auth.SDKClientFromContext(r.Context())
	if !h.processor.Enqueue(project.ID, env, sdkClient2) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, `{"error":"server overloaded"}`, http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"id": env.Header.EventID})
}

// maybeDecompress returns the appropriate decompressing reader based on Content-Encoding.
func maybeDecompress(r *http.Request) (io.ReadCloser, error) {
	switch r.Header.Get("Content-Encoding") {
	case "gzip":
		return gzip.NewReader(r.Body)
	case "deflate":
		return flate.NewReader(r.Body), nil
	default:
		return r.Body, nil
	}
}
