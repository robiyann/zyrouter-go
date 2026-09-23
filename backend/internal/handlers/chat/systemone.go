package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"zyrouter/backend/internal/handlerutil"
)

// HandleSystemOne handles TypeSafe AI / Jev System One typed decision requests.
func (h *ChatHandler) HandleSystemOne(w http.ResponseWriter, r *http.Request) {
	body, err := readSystemOneBody(r)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	var input struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &input); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	rawModel := strings.TrimSpace(input.Model)
	if rawModel == "" {
		rawModel = "jev-1.13-free"
	}

	modelInfo, err := h.resolveClientModel(rawModel)
	if err != nil {
		if fallbackInfo, fErr := h.resolveModel(rawModel); fErr == nil && fallbackInfo != nil {
			modelInfo = fallbackInfo
		} else if strings.Contains(strings.ToLower(rawModel), "jev") {
			modelInfo = &ModelInfo{Provider: "opencode", Model: "jev-1.13-free"}
		} else {
			handlerutil.WriteJSONError(w, modelResolutionStatus(err), err.Error())
			return
		}
	}

	if len(modelInfo.ComboModels) > 0 {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "SystemOne API does not support combo models")
		return
	}

	if err := h.validateRequestPolicy(r, rawModel, modelInfo); err != nil {
		handlerutil.WriteJSONError(w, http.StatusForbidden, fmt.Sprintf("Forbidden: %v", err))
		return
	}

	if err := h.validateRequestRateLimit(r); err != nil {
		handlerutil.WriteJSONError(w, http.StatusTooManyRequests, err.Error())
		return
	}

	var bodyMap map[string]any
	if err := json.Unmarshal(body, &bodyMap); err == nil {
		bodyMap["model"] = modelInfo.Model
		if normalized, err := json.Marshal(bodyMap); err == nil {
			body = normalized
		}
	}

	if err := h.reserveUserQuota(r, body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusTooManyRequests, err.Error())
		return
	}

	forwardErr := h.handleAccountFallback(r.Context(), w, modelInfo.Provider, modelInfo.Model, modelInfo.ConnectionID, body, false, false, "/v1/systemone")
	if forwardErr != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, forwardErr.Error())
		return
	}
}

func readSystemOneBody(r *http.Request) ([]byte, error) {
	var body map[string]any
	if err := handlerutil.DecodeJSON(r, &body); err != nil || body == nil {
		return nil, fmt.Errorf("invalid JSON body")
	}
	return json.Marshal(body)
}
