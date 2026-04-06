package admin

import (
	"encoding/json"
	"net/http"

	"github.com/elmisi/ampulla/internal/admin"
	"github.com/elmisi/ampulla/internal/event"
)

// CreateAPIToken creates a new API token. The plaintext is returned only once.
func (h *Handler) CreateAPIToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, `{"error":"name is required"}`, http.StatusBadRequest)
		return
	}
	if len(req.Name) > 100 {
		http.Error(w, `{"error":"name max 100 chars"}`, http.StatusBadRequest)
		return
	}

	plaintext, hash, prefix, err := admin.GenerateToken()
	if err != nil {
		serverError(w, err)
		return
	}

	id, createdAt, err := h.db.CreateAPIToken(r.Context(), req.Name, hash, prefix)
	if err != nil {
		serverError(w, err)
		return
	}

	// Return the plaintext exactly once.
	writeJSON(w, http.StatusCreated, event.APIToken{
		ID:             id,
		Name:           req.Name,
		Prefix:         prefix,
		CreatedAt:      createdAt,
		PlaintextToken: plaintext,
	})
}

// ListAPITokens returns all API tokens (without plaintext).
func (h *Handler) ListAPITokens(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.ListAPITokens(r.Context())
	if err != nil {
		serverError(w, err)
		return
	}
	tokens := make([]event.APIToken, 0, len(rows))
	for _, t := range rows {
		tokens = append(tokens, event.APIToken{
			ID:         t.ID,
			Name:       t.Name,
			Prefix:     t.Prefix,
			CreatedAt:  t.CreatedAt,
			LastUsedAt: t.LastUsedAt,
		})
	}
	writeJSON(w, http.StatusOK, tokens)
}

// DeleteAPIToken removes an API token by ID.
func (h *Handler) DeleteAPIToken(w http.ResponseWriter, r *http.Request) {
	id, err := paramInt64(r, "id")
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.DeleteAPIToken(r.Context(), id); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
