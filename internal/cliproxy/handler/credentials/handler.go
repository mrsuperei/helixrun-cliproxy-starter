package credentials

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

// Repository describes the data access methods required by the HTTP handler.
type Repository interface {
	List(ctx context.Context) ([]*coreauth.Auth, error)
	Get(ctx context.Context, id string) (*coreauth.Auth, error)
	Save(ctx context.Context, auth *coreauth.Auth) (string, error)
	Delete(ctx context.Context, id string) error
}

// Handler exposes credential CRUD endpoints guarded by the local management key.
type Handler struct {
	repo          Repository
	manager       *coreauth.Manager
	managementKey string
}

// New creates a credential handler.
func New(repo Repository, manager *coreauth.Manager, managementKey string) *Handler {
	return &Handler{repo: repo, manager: manager, managementKey: managementKey}
}

// Register attaches the credential endpoints to the provided mux.
func (h *Handler) Register(mux *http.ServeMux) {
	if h == nil || mux == nil {
		return
	}
	mux.Handle("/api/credentials", http.HandlerFunc(h.handleCollection))
	mux.Handle("/api/credentials/", http.HandlerFunc(h.handleSingle))
}

func (h *Handler) handleCollection(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(r) {
		writeError(w, http.StatusUnauthorized, "missing or invalid management key")
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.listCredentials(w, r)
	case http.MethodPost:
		h.createCredential(w, r)
	default:
		w.Header().Set("Allow", "GET,POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) handleSingle(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(r) {
		writeError(w, http.StatusUnauthorized, "missing or invalid management key")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/credentials/")
	id = strings.Trim(id, "/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "credential id required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.getCredential(w, r, id)
	case http.MethodDelete:
		h.deleteCredential(w, r, id)
	default:
		w.Header().Set("Allow", "GET,DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) listCredentials(w http.ResponseWriter, r *http.Request) {
	auths, err := h.repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]credentialResponse, 0, len(auths))
	for _, auth := range auths {
		if auth == nil {
			continue
		}
		items = append(items, marshalCredential(auth))
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": items})
}

func (h *Handler) getCredential(w http.ResponseWriter, r *http.Request, id string) {
	auth, err := h.repo.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if auth == nil {
		writeError(w, http.StatusNotFound, "credential not found")
		return
	}
	writeJSON(w, http.StatusOK, marshalCredential(auth))
}

func (h *Handler) createCredential(w http.ResponseWriter, r *http.Request) {
	var req credentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json payload")
		return
	}
	req.Provider = strings.TrimSpace(req.Provider)
	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}
	if req.Attributes == nil {
		req.Attributes = make(map[string]string)
	}
	if req.Metadata == nil {
		req.Metadata = make(map[string]any)
	}
	if _, ok := req.Metadata["type"]; !ok {
		req.Metadata["type"] = req.Provider
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = strings.ToLower(req.Provider) + "-" + uuid.NewString() + ".json"
	}
	now := time.Now().UTC()
	auth := &coreauth.Auth{
		ID:         id,
		Provider:   req.Provider,
		Label:      strings.TrimSpace(req.Label),
		Status:     coreauth.StatusActive,
		Attributes: cloneStringMap(req.Attributes),
		Metadata:   req.Metadata,
		CreatedAt:  now,
		UpdatedAt:  now,
		Disabled:   req.Disabled,
	}
	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	auth.FileName = auth.ID
	if _, err := h.manager.Register(r.Context(), auth); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	persisted, err := h.repo.Get(r.Context(), auth.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if persisted == nil {
		persisted = auth
	}
	writeJSON(w, http.StatusCreated, marshalCredential(persisted))
}

func (h *Handler) deleteCredential(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	existing, err := h.repo.Get(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "credential not found")
		return
	}
	if err := h.repo.Delete(ctx, id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if auth, ok := h.manager.GetByID(id); ok && auth != nil {
		auth.Disabled = true
		auth.Status = coreauth.StatusDisabled
		auth.StatusMessage = "removed via credential API"
		auth.UpdatedAt = time.Now().UTC()
		if _, err := h.manager.Update(ctx, auth); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) authorize(r *http.Request) bool {
	key := strings.TrimSpace(h.managementKey)
	if key == "" {
		return true
	}
	candidate := strings.TrimSpace(r.Header.Get("X-Management-Key"))
	if candidate == "" {
		if ah := strings.TrimSpace(r.Header.Get("Authorization")); ah != "" {
			parts := strings.SplitN(ah, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
				candidate = strings.TrimSpace(parts[1])
			} else {
				candidate = ah
			}
		}
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(key)) == 1
}

type credentialRequest struct {
	ID         string            `json:"id"`
	Provider   string            `json:"provider"`
	Label      string            `json:"label"`
	Attributes map[string]string `json:"attributes"`
	Metadata   map[string]any    `json:"metadata"`
	Disabled   bool              `json:"disabled"`
}

type credentialResponse struct {
	ID         string            `json:"id"`
	Provider   string            `json:"provider"`
	Label      string            `json:"label"`
	Status     coreauth.Status   `json:"status"`
	Disabled   bool              `json:"disabled"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

func marshalCredential(auth *coreauth.Auth) credentialResponse {
	if auth == nil {
		return credentialResponse{}
	}
	return credentialResponse{
		ID:         auth.ID,
		Provider:   auth.Provider,
		Label:      auth.Label,
		Status:     auth.Status,
		Disabled:   auth.Disabled,
		Attributes: cloneStringMap(auth.Attributes),
		Metadata:   cloneMetadata(auth.Metadata),
		CreatedAt:  auth.CreatedAt,
		UpdatedAt:  auth.UpdatedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
