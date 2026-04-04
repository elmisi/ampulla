package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/elmisi/ampulla/internal/cursor"
	"github.com/elmisi/ampulla/internal/store"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db *store.DB
}

func NewHandler(db *store.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.db.ListOrganizations(r.Context())
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, orgs)
}

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	orgSlug := chi.URLParam(r, "orgSlug")
	projects, err := h.db.ListProjects(r.Context(), orgSlug)
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, projects)
}

func (h *Handler) ListIssues(w http.ResponseWriter, r *http.Request) {
	orgSlug := chi.URLParam(r, "orgSlug")
	projectSlug := chi.URLParam(r, "projectSlug")
	cur, limit := parsePagination(r)

	issues, err := h.db.ListIssues(r.Context(), orgSlug, projectSlug, cur, limit)
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, issues)
}

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	issueIDStr := chi.URLParam(r, "issueID")
	issueID, err := strconv.ParseInt(issueIDStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid issue ID"}`, http.StatusBadRequest)
		return
	}
	cur, limit := parsePagination(r)

	events, err := h.db.ListEventsByIssue(r.Context(), issueID, cur, limit)
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, events)
}

func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	orgSlug := chi.URLParam(r, "orgSlug")
	cur, limit := parsePagination(r)

	txns, err := h.db.ListTransactions(r.Context(), orgSlug, cur, limit)
	if err != nil {
		httpError(w, err)
		return
	}
	writeJSON(w, txns)
}

func parsePagination(r *http.Request) (cur cursor.Token, limit int) {
	cur, _ = cursor.Decode(r.URL.Query().Get("cursor"))
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	return
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	// Return empty array instead of null for empty slices
	if v == nil {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, err error) {
	http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
}
