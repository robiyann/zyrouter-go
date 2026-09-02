package client

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/middleware"
	"zyrouter/backend/internal/models"
)

type Handler struct{ Repo *db.Repo }

func NewHandler(repo *db.Repo) *Handler { return &Handler{Repo: repo} }

func (h *Handler) HandleProfile(w http.ResponseWriter, r *http.Request) {
	client := middleware.GetAuthenticatedClient(r)
	if client == nil {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "client authentication required")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, client)
}

func (h *Handler) HandlePolicy(w http.ResponseWriter, r *http.Request) {
	client := middleware.GetAuthenticatedClient(r)
	if client == nil || client.PolicyID == nil || *client.PolicyID == "" {
		handlerutil.WriteJSON(w, http.StatusNotFound, "client policy not configured")
		return
	}
	policy, err := h.Repo.GetClientPolicy(*client.PolicyID)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if policy == nil || policy.IsActive != 1 {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "client policy is inactive")
		return
	}
	response := map[string]any{
		"id": policy.ID, "name": policy.Name, "isActive": policy.IsActive,
		"createdAt": policy.CreatedAt, "updatedAt": policy.UpdatedAt,
	}
	var data map[string]any
	if err := json.Unmarshal([]byte(policy.Data), &data); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "client policy data is invalid")
		return
	}
	for key, value := range data {
		response[key] = value
	}
	handlerutil.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) HandleKeys(w http.ResponseWriter, r *http.Request) {
	client := middleware.GetAuthenticatedClient(r)
	if client == nil {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "client authentication required")
		return
	}
	keys, err := h.Repo.GetApiKeysByClientID(client.ID)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		item := map[string]any{"id": key.ID, "name": key.Name, "isActive": key.IsActive, "createdAt": key.CreatedAt}
		if key.PolicyID != nil {
			item["policyId"] = *key.PolicyID
		}
		item["keyPrefix"] = maskKey(key.Key)
		out = append(out, item)
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"keys": out})
}

func (h *Handler) HandleCreateKey(w http.ResponseWriter, r *http.Request) {
	client := middleware.GetAuthenticatedClient(r)
	if client == nil {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "client authentication required")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Name == "" {
		body.Name = "Client API Key"
	}
	if client.PolicyID == nil || *client.PolicyID == "" {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "client policy not configured")
		return
	}
	policy, err := h.Repo.GetClientPolicy(*client.PolicyID)
	if err != nil || policy == nil || policy.IsActive != 1 {
		handlerutil.WriteJSONError(w, http.StatusForbidden, "client policy is inactive")
		return
	}
	var restrictions models.KeyRestrictions
	if err := json.Unmarshal([]byte(policy.Data), &restrictions); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "client policy data is invalid")
		return
	}
	restrictionsJSON, _ := json.Marshal(restrictions)
	keyToken, err := randomToken(32)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to generate key")
		return
	}
	keyID, err := randomToken(12)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to generate key id")
		return
	}
	key, err := h.Repo.CreateClientApiKey("ck-"+keyID, "sk-client-"+keyToken, body.Name, client.ID, policy.ID, string(restrictionsJSON))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusCreated, map[string]any{
		"id": key.ID, "name": key.Name, "key": key.Key, "policyId": policy.ID,
		"allowedPrefixes": restrictions.AllowedPrefixes,
		"warning":         "Store this key securely. It will not be shown again.",
	})
}

func (h *Handler) HandleRevokeKey(w http.ResponseWriter, r *http.Request) {
	client := middleware.GetAuthenticatedClient(r)
	if client == nil {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "client authentication required")
		return
	}
	if err := h.Repo.RevokeClientApiKey(chi.URLParam(r, "id"), client.ID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			handlerutil.WriteJSONError(w, http.StatusNotFound, "key not found")
			return
		}
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *Handler) HandleUsage(w http.ResponseWriter, r *http.Request) {
	client := middleware.GetAuthenticatedClient(r)
	if client == nil {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "client authentication required")
		return
	}
	usage, err := h.Repo.GetClientUsage(client.ID)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, usage)
}

func randomToken(bytesLen int) (string, error) {
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:8] + "..."
}
