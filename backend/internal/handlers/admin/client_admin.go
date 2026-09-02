package admin

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/models"
)

func (h *AdminHandler) HandleCreateClientPolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID               string               `json:"id"`
		Name             string               `json:"name"`
		AllowedModels    []string             `json:"allowedModels"`
		AllowedPrefixes  []string             `json:"allowedPrefixes"`
		AllowedProviders []string             `json:"allowedProviders"`
		BlockedModels    []string             `json:"blockedModels"`
		RateLimit        *models.KeyRateLimit `json:"rateLimit"`
		ExpiresAt        *string              `json:"expiresAt"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.ID == "" {
		suffix, err := randomHex(8)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to generate policy id")
			return
		}
		body.ID = "policy-" + suffix
	}
	data := map[string]any{
		"allowedModels": body.AllowedModels, "allowedPrefixes": body.AllowedPrefixes,
		"allowedProviders": body.AllowedProviders, "blockedModels": body.BlockedModels,
	}
	if body.RateLimit != nil {
		data["rateLimit"] = body.RateLimit
	}
	if body.ExpiresAt != nil {
		data["expiresAt"] = body.ExpiresAt
	}
	policy, err := h.repo.CreateClientPolicy(body.ID, strings.TrimSpace(body.Name), data)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusCreated, policy)
}

func (h *AdminHandler) HandleCreateClient(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Email    string `json:"email"`
		PolicyID string `json:"policyId"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.PolicyID != "" {
		policy, err := h.repo.GetClientPolicy(body.PolicyID)
		if err != nil || policy == nil || policy.IsActive != 1 {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "policy not found or inactive")
			return
		}
	}
	if body.ID == "" {
		suffix, err := randomHex(8)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to generate client id")
			return
		}
		body.ID = "client-" + suffix
	}
	suffix, err := randomHex(32)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to generate client access token")
		return
	}
	accessToken := "clt_" + suffix
	client, err := h.repo.CreateClient(body.ID, strings.TrimSpace(body.Name), strings.TrimSpace(body.Email), db.HashClientToken(accessToken), body.PolicyID)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusCreated, map[string]any{
		"client": client, "accessToken": accessToken,
		"warning": "Store this access token securely. It will not be shown again.",
	})
}

func decodeJSON(r *http.Request, target any) error {
	return json.NewDecoder(r.Body).Decode(target)
}

func randomHex(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("secure random generation failed: %w", err)
	}
	return hex.EncodeToString(b), nil
}
