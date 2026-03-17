package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/elmisi/ampulla/internal/event"
	"github.com/elmisi/ampulla/internal/store"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db     *store.DB
	domain string
}

func NewHandler(db *store.DB, domain string) *Handler {
	return &Handler{db: db, domain: domain}
}

// --- Organizations ---

func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	orgs, err := h.db.ListOrganizations(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orgs)
}

func (h *Handler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Slug == "" {
		http.Error(w, `{"error":"name and slug required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Name) > 255 || len(req.Slug) > 64 {
		http.Error(w, `{"error":"name max 255 chars, slug max 64 chars"}`, http.StatusBadRequest)
		return
	}
	org, err := h.db.CreateOrganization(r.Context(), req.Name, req.Slug)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

func (h *Handler) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Slug == "" {
		http.Error(w, `{"error":"name and slug required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Name) > 255 || len(req.Slug) > 64 {
		http.Error(w, `{"error":"name max 255 chars, slug max 64 chars"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.UpdateOrganization(r.Context(), id, req.Name, req.Slug); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *Handler) DeleteOrganization(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.DeleteOrganization(r.Context(), id); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// --- Projects ---

func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
	orgSlug := chi.URLParam(r, "orgSlug")
	projects, err := h.db.ListProjects(r.Context(), orgSlug)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h *Handler) ListAllProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.db.ListAllProjects(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrgID    int64  `json:"orgId"`
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Platform string `json:"platform"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Slug == "" || req.OrgID == 0 {
		http.Error(w, `{"error":"orgId, name, and slug required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Name) > 255 || len(req.Slug) > 64 {
		http.Error(w, `{"error":"name max 255 chars, slug max 64 chars"}`, http.StatusBadRequest)
		return
	}
	proj, err := h.db.CreateProject(r.Context(), req.OrgID, req.Name, req.Slug, req.Platform)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, proj)
}

func (h *Handler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		Name      string `json:"name"`
		Slug      string `json:"slug"`
		Platform  string `json:"platform"`
		NtfyURL   string `json:"ntfyUrl"`
		NtfyTopic string `json:"ntfyTopic"`
		NtfyToken string `json:"ntfyToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Slug == "" {
		http.Error(w, `{"error":"name and slug required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Name) > 255 || len(req.Slug) > 64 {
		http.Error(w, `{"error":"name max 255 chars, slug max 64 chars"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.UpdateProject(r.Context(), id, req.Name, req.Slug, req.Platform, req.NtfyURL, req.NtfyTopic, req.NtfyToken); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.DeleteProject(r.Context(), id); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// --- Project Keys ---

type keyResponse struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"projectId"`
	PublicKey string `json:"public"`
	SecretKey string `json:"secret"`
	Label     string `json:"label"`
	IsActive  bool   `json:"isActive"`
	DSN       string `json:"dsn"`
}

func (h *Handler) keyToResponse(k event.ProjectKey) keyResponse {
	return keyResponse{
		ID:        k.ID,
		ProjectID: k.ProjectID,
		PublicKey: k.PublicKey,
		SecretKey: k.SecretKey,
		Label:     k.Label,
		IsActive:  k.IsActive,
		DSN:       fmt.Sprintf("https://%s@%s/%d", k.PublicKey, h.domain, k.ProjectID),
	}
}

func (h *Handler) ListProjectKeys(w http.ResponseWriter, r *http.Request) {
	projectID, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	keys, err := h.db.ListProjectKeys(r.Context(), projectID)
	if err != nil {
		serverError(w, err)
		return
	}
	resp := make([]keyResponse, len(keys))
	for i, k := range keys {
		resp[i] = h.keyToResponse(k)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateProjectKey(w http.ResponseWriter, r *http.Request) {
	projectID, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.Label == "" {
		req.Label = "Default"
	}
	if len(req.Label) > 128 {
		http.Error(w, `{"error":"label max 128 chars"}`, http.StatusBadRequest)
		return
	}
	key, err := h.db.CreateProjectKey(r.Context(), projectID, req.Label)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, h.keyToResponse(*key))
}

func (h *Handler) ToggleProjectKey(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		IsActive bool `json:"isActive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.ToggleProjectKey(r.Context(), id, req.IsActive); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *Handler) DeleteProjectKey(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.DeleteProjectKey(r.Context(), id); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// --- Issues ---

func (h *Handler) ListIssues(w http.ResponseWriter, r *http.Request) {
	projectID, _ := strconv.ParseInt(r.URL.Query().Get("project"), 10, 64)
	cursor, limit := parsePagination(r)
	issues, err := h.db.AdminListIssues(r.Context(), projectID, cursor, limit)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, issues)
}

func (h *Handler) UpdateIssue(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Status == "" {
		http.Error(w, `{"error":"status required"}`, http.StatusBadRequest)
		return
	}
	if req.Status != "unresolved" && req.Status != "resolved" && req.Status != "ignored" {
		http.Error(w, `{"error":"status must be unresolved, resolved, or ignored"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.UpdateIssueStatus(r.Context(), id, req.Status); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *Handler) DeleteIssue(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.DeleteIssue(r.Context(), id); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (h *Handler) ListIssueEvents(w http.ResponseWriter, r *http.Request) {
	issueID, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	cursor, limit := parsePagination(r)
	events, err := h.db.ListEventsByIssue(r.Context(), issueID, cursor, limit)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// --- Transactions ---

func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	projectID, _ := strconv.ParseInt(r.URL.Query().Get("project"), 10, 64)
	cursor, limit := parsePagination(r)
	txns, err := h.db.AdminListTransactions(r.Context(), projectID, cursor, limit)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, txns)
}

// --- Dashboard ---

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.DashboardStats(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// --- Performance ---

func (h *Handler) Performance(w http.ResponseWriter, r *http.Request) {
	projectID, _ := strconv.ParseInt(r.URL.Query().Get("project"), 10, 64)
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 || days > 90 {
		days = 7
	}
	stats, err := h.db.GetPerformanceStats(r.Context(), projectID, days)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// --- Helpers ---

func paramInt64(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, name), 10, 64)
}

func parsePagination(r *http.Request) (cursor int64, limit int) {
	cursor, _ = strconv.ParseInt(r.URL.Query().Get("cursor"), 10, 64)
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	return
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		w.Write([]byte("[]"))
		return
	}
	json.NewEncoder(w).Encode(v)
}

func serverError(w http.ResponseWriter, err error) {
	slog.Error("admin API error", "error", err)
	http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
}
