package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/log"
	"zyrouter/backend/internal/middleware"
	"zyrouter/backend/internal/models"
	"zyrouter/backend/internal/providers"
	"zyrouter/backend/internal/translator"
	"zyrouter/backend/internal/updater"
)

// HandleChatCompletions handles POST /v1/chat/completions (OpenAI format requests).
func (h *ChatHandler) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var reqBody struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if reqBody.Model == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing model")
		return
	}
	// Bypass synthetic requests (Claude Code naming, warmup, keepalive)
	if handleBypassRequest(w, body, reqBody.Model, reqBody.Stream) {
		return
	}

	modelInfo, err := h.resolveModel(reqBody.Model)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.validateRequestPolicy(r, reqBody.Model, modelInfo); err != nil {
		handlerutil.WriteJSONError(w, http.StatusForbidden, fmt.Sprintf("Forbidden: %v", err))
		return
	}
	if err := h.validateRequestRateLimit(r); err != nil {
		status := http.StatusTooManyRequests
		if !errors.Is(err, auth.ErrRateLimitExceeded) {
			status = http.StatusInternalServerError
		}
		handlerutil.WriteJSONError(w, status, err.Error())
		return
	}

	if len(modelInfo.ComboModels) > 0 {
		if modelInfo.Strategy == "fusion" {
			h.handleFusion(r.Context(), w, body, modelInfo.ComboModels, modelInfo.Strategy, reqBody.Stream, false, reqBody.Model, modelInfo.StickyLimit)
			return
		}
		h.handleComboFallback(r.Context(), w, body, modelInfo.ComboModels, modelInfo.Strategy, reqBody.Stream, false, reqBody.Model, modelInfo.StickyLimit)
		return
	}

	h.handleSingleModel(r.Context(), w, body, modelInfo, reqBody.Stream, false)
}

// handleSingleModel resolves a single ModelInfo and forwards the request upstream.
func (h *ChatHandler) handleSingleModel(ctx context.Context, w http.ResponseWriter, body []byte, modelInfo *ModelInfo, isStream bool, translateResponse bool) {
	cw := newCommittedResponseWriter(w)
	var upstreamBody map[string]any
	if err := json.Unmarshal(body, &upstreamBody); err != nil {
		handlerutil.WriteJSONError(cw, http.StatusBadRequest, "failed to parse request body")
		return
	}
	upstreamBody["model"] = modelInfo.Model

	upstreamJSON, err := json.Marshal(upstreamBody)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to marshal upstream request")
		return
	}

	result := h.handleAccountFallback(ctx, cw, modelInfo.Provider, modelInfo.Model, modelInfo.ConnectionID, upstreamJSON, isStream, translateResponse, "/v1/chat/completions")
	if result != nil {
		if cw.IsCommitted() {
			log.Error("chat", "upstream error after headers committed", "error", result)
			return
		}
		var ue *upstreamError
		if errors.As(result, &ue) {
			cw.Header().Set("Content-Type", "application/json")
			cw.WriteHeader(ue.StatusCode)
			cw.Write(ue.Body)
			return
		}
		handlerutil.WriteJSONError(cw, http.StatusBadGateway, fmt.Sprintf("upstream error: %v", result))
	}
}

// HandleMessages handles POST /v1/messages (Claude format requests).
func (h *ChatHandler) HandleMessages(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error("chat", "read body failed", "error", err)
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	var reqBody struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		log.Error("chat", "parse JSON failed", "error", err)
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if reqBody.Model == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing model")
		return
	}

	modelInfo, err := h.resolveModel(reqBody.Model)
	if err != nil {
		log.Error("chat", "resolve model failed", "error", err, "model", reqBody.Model)
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.validateRequestPolicy(r, reqBody.Model, modelInfo); err != nil {
		handlerutil.WriteJSONError(w, http.StatusForbidden, fmt.Sprintf("Forbidden: %v", err))
		return
	}
	if err := h.validateRequestRateLimit(r); err != nil {
		status := http.StatusTooManyRequests
		if !errors.Is(err, auth.ErrRateLimitExceeded) {
			status = http.StatusInternalServerError
		}
		handlerutil.WriteJSONError(w, status, err.Error())
		return
	}

	translateResponse := true
	var workingBody map[string]any
	if modelInfo.Provider == "claude" || modelInfo.Provider == "anthropic" {
		translateResponse = false
		if err := json.Unmarshal(body, &workingBody); err != nil {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	} else {
		openaiBody, err := translator.TranslateClaudeToOpenAI(body)
		if err != nil {
			log.Error("chat", "translate failed", "error", err)
			handlerutil.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("translation error: %v", err))
			return
		}
		if err := json.Unmarshal(openaiBody, &workingBody); err != nil {
			log.Error("chat", "parse translated failed", "error", err)
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to parse translated request")
			return
		}
	}
	workingBody["stream"] = reqBody.Stream

	if len(modelInfo.ComboModels) > 0 {
		if modelInfo.Strategy == "fusion" {
			bodyJSON, err := json.Marshal(workingBody)
			if err != nil {
				handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to marshal request body")
				return
			}
			h.handleFusion(r.Context(), w, bodyJSON, modelInfo.ComboModels, modelInfo.Strategy, reqBody.Stream, translateResponse, reqBody.Model, modelInfo.StickyLimit)
			return
		}
		h.handleMessagesComboFallback(r.Context(), w, workingBody, modelInfo.ComboModels, modelInfo.Strategy, reqBody.Stream, reqBody.Model, modelInfo.StickyLimit)
		return
	}

	h.handleMessagesSingleModel(r.Context(), w, workingBody, modelInfo, reqBody.Stream, translateResponse)
}

// validateRequestPolicy validates the resolved model and every concrete model
// in a combo. Validation happens after resolution so aliases and fallback
// entries cannot bypass provider-prefix or connection restrictions.
func (h *ChatHandler) validateRequestPolicy(r *http.Request, requested string, info *ModelInfo) error {
	key := middleware.GetAuthenticatedApiKey(r)
	if key == nil || info == nil {
		return nil
	}

	validateOne := func(requestedModel string, modelInfo *ModelInfo) error {
		// Only the provider's single active prefix is valid for model policy.
		activePrefix := h.GetActiveProviderPrefix(modelInfo.Provider, nil)
		activeModel := activePrefix + "/" + modelInfo.Model
		requestedPrefix := ""
		if parts := strings.SplitN(requestedModel, "/", 2); len(parts) == 2 {
			requestedPrefix = strings.ToLower(strings.TrimSpace(parts[0]))
		}
		if requestedPrefix != "" && !strings.EqualFold(requestedPrefix, activePrefix) {
			return fmt.Errorf("%w: '%s' (use prefix '%s')", auth.ErrModelNotAllowed, requestedModel, activePrefix)
		}
		modelAllowed := key.IsModelAllowed(requestedModel)
		if requestedPrefix == "" {
			modelAllowed = key.IsModelAllowed(activeModel)
		}
		if !modelAllowed {
			return fmt.Errorf("%w: '%s'", auth.ErrModelNotAllowed, requestedModel)
		}
		providerTarget := modelInfo.ConnectionID
		if providerTarget == "" {
			providerTarget = modelInfo.Provider
		}
		providerAllowed := providerTarget == "" || key.IsProviderAllowed(providerTarget) || key.IsProviderAllowed(modelInfo.Provider)
		if !providerAllowed {
			return fmt.Errorf("%w: '%s'", auth.ErrProviderNotAllowed, providerTarget)
		}
		return nil
	}

	if len(info.ComboModels) == 0 {
		return validateOne(requested, info)
	}
	for _, entry := range info.ComboModels {
		entryInfo, err := h.resolveModel(entry)
		if err != nil {
			return fmt.Errorf("resolve combo member %q: %w", entry, err)
		}
		if err := validateOne(entry, entryInfo); err != nil {
			return err
		}
	}
	return nil
}

func (h *ChatHandler) validateRequestRateLimit(r *http.Request) error {
	key := middleware.GetAuthenticatedApiKey(r)
	if key == nil || h.Repo == nil {
		return nil
	}
	restrictions, err := key.ParseRestrictions()
	if err != nil || restrictions == nil || restrictions.RateLimit == nil {
		return nil
	}
	ok, err := h.Repo.CheckAPIKeyRateLimit(key.Key, restrictions.RateLimit, time.Now())
	if err != nil {
		return fmt.Errorf("rate limit lookup failed: %w", err)
	}
	if !ok {
		return auth.ErrRateLimitExceeded
	}
	return nil
}

// handleMessagesSingleModel forwards a translated Claude request for a single model.
func (h *ChatHandler) handleMessagesSingleModel(ctx context.Context, w http.ResponseWriter, translatedReq map[string]any, modelInfo *ModelInfo, isStream bool, translateResponse bool) {
	cw := newCommittedResponseWriter(w)
	translatedReq["model"] = modelInfo.Model
	finalBody, err := json.Marshal(translatedReq)
	if err != nil {
		handlerutil.WriteJSONError(cw, http.StatusInternalServerError, "failed to marshal translated request")
		return
	}

	result := h.handleAccountFallback(ctx, cw, modelInfo.Provider, modelInfo.Model, modelInfo.ConnectionID, finalBody, isStream, translateResponse, "/v1/messages")
	if result != nil {
		if cw.IsCommitted() {
			log.Error("chat", "upstream error after headers committed", "error", result)
			return
		}
		var ue *upstreamError
		if errors.As(result, &ue) {
			cw.Header().Set("Content-Type", "application/json")
			cw.WriteHeader(ue.StatusCode)
			cw.Write(ue.Body)
			return
		}
		handlerutil.WriteJSONError(cw, http.StatusBadGateway, fmt.Sprintf("upstream error: %v", result))
	}
}

// HandleHealth responds with a simple health check status.
func (h *ChatHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleVersion responds with the proxy version details and update status.
func (h *ChatHandler) HandleVersion(w http.ResponseWriter, r *http.Request) {
	info := updater.GetCachedInfo()
	handlerutil.WriteJSON(w, http.StatusOK, info)
}

// HandleCheckUpdate fetches fresh update info from remote release server.
func (h *ChatHandler) HandleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	info, err := updater.CheckUpdate(r.Context())
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, fmt.Sprintf("check update failed: %v", err))
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, info)
}

// HandleTriggerUpdate performs immediate self-updating if an update is available.
func (h *ChatHandler) HandleTriggerUpdate(w http.ResponseWriter, r *http.Request) {
	info, err := updater.CheckUpdate(r.Context())
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, fmt.Sprintf("check update failed: %v", err))
		return
	}
	if !info.HasUpdate {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":  "up_to_date",
			"message": "9router-go is already on the latest version",
			"version": info.CurrentVersion,
		})
		return
	}
	if err := updater.PerformSelfUpdate(info.DownloadURL); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("self-update failed: %v", err))
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "updated",
		"message": "Update installed successfully. Process is restarting...",
		"version": info.LatestVersion,
	})
}

// HandleModels responds with the list of available model identifiers with strict provider prefixes.
func (h *ChatHandler) HandleModels(w http.ResponseWriter, r *http.Request) {
	type modelObj struct {
		ID                  string `json:"id"`
		Object              string `json:"object"`
		Created             int64  `json:"created"`
		OwnedBy             string `json:"owned_by"`
		ContextLength       int    `json:"context_length,omitempty"`
		MaxCompletionTokens int    `json:"max_completion_tokens,omitempty"`
	}

	apiKey := middleware.GetAuthenticatedApiKey(r)
	connectionProviderMap := make(map[string]string)

	seen := make(map[string]bool)
	var data []modelObj
	now := time.Now().Unix()

	addModel := func(id, owner string, connectionID ...string) {
		if id == "" || seen[id] {
			return
		}
		// Enforce API Key Restrictions if authenticated via client key
		if apiKey != nil {
			providerTarget := owner
			if len(connectionID) > 0 && connectionID[0] != "" {
				providerTarget = connectionID[0]
			}
			providerAllowed := apiKey.IsProviderAllowed(providerTarget)
			if len(connectionID) > 0 && connectionID[0] != "" {
				providerAllowed = providerAllowed || apiKey.IsProviderAllowed(owner)
				if connectionProvider, ok := connectionProviderMap[connectionID[0]]; ok {
					providerAllowed = providerAllowed || apiKey.IsProviderAllowed(connectionProvider)
				}
			}
			if !apiKey.IsModelAllowed(id) || !providerAllowed {
				return
			}
		}
		seen[id] = true
		ctxLen, maxOut := providers.GetModelTokenLimits(id)
		data = append(data, modelObj{
			ID:                  id,
			Object:              "model",
			Created:             now,
			OwnedBy:             owner,
			ContextLength:       ctxLen,
			MaxCompletionTokens: maxOut,
		})
	}

	// 1. Gather Provider Nodes lookup map
	nodeMap := make(map[string]*models.ProviderNode)
	nodePrefixMap := make(map[string]string)
	if nodes, err := h.Repo.GetProviderNodes(); err == nil {
		for _, n := range nodes {
			nodeMap[n.ID] = n
			var nd map[string]any
			if json.Unmarshal([]byte(n.Data), &nd) == nil {
				if p, ok := nd["prefix"].(string); ok && strings.TrimSpace(p) != "" {
					nodePrefixMap[n.ID] = strings.TrimSpace(strings.ToLower(p))
				}
			}
			if nodePrefixMap[n.ID] == "" && n.Name != nil && strings.TrimSpace(*n.Name) != "" {
				nodePrefixMap[n.ID] = strings.TrimSpace(strings.ToLower(*n.Name))
			}
		}
	}

	// 2. Fetch custom provider prefixes from kv
	customPrefixes := make(map[string]string)
	if prefMap, err := h.Repo.GetProviderPrefixes(); err == nil {
		for prov, pref := range prefMap {
			customPrefixes[strings.ToLower(prov)] = strings.TrimSpace(strings.ToLower(pref))
		}
	}

	// Helper to determine outputAlias for a provider/connection
	getOutputAlias := func(prov string, connData map[string]any) string {
		provLower := strings.ToLower(prov)
		// Check if provider is a providerNode (OpenAI / Anthropic compatible)
		if pref, ok := nodePrefixMap[prov]; ok && pref != "" {
			return pref
		}
		// Check connection data prefix
		if connData != nil {
			if p, ok := connData["prefix"].(string); ok && strings.TrimSpace(p) != "" {
				return strings.TrimSpace(strings.ToLower(p))
			}
		}
		// Check global custom prefix mapping from kv
		if p, ok := customPrefixes[provLower]; ok && p != "" {
			return p
		}
		// Return designated default alias (e.g. opencode -> oc, antigravity -> ag)
		return providers.GetDefaultProviderAlias(provLower)
	}

	// 3. Process Active Provider Connections (isActive == 1)
	activeProviders := make(map[string]bool)
	activePrefixes := make(map[string]bool)

	conns, _ := h.Repo.GetProviderConnections("", true)
	for _, conn := range conns {
		connectionProviderMap[conn.ID] = conn.Provider
		provLower := strings.ToLower(conn.Provider)
		if !providers.IsProviderEnabled(provLower) {
			continue
		}
		var connData map[string]any
		_ = json.Unmarshal([]byte(conn.Data), &connData)

		outputAlias := getOutputAlias(conn.Provider, connData)
		activeProviders[provLower] = true
		activePrefixes[outputAlias] = true

		// Aliases/synonyms for known providers
		if provLower == "github" || provLower == "copilot" {
			activeProviders["github"] = true
			activeProviders["copilot"] = true
			activeProviders["gh"] = true
			activePrefixes[outputAlias] = true
		} else if provLower == "antigravity" {
			activeProviders["antigravity"] = true
			activeProviders["ag"] = true
			activePrefixes[outputAlias] = true
		} else if provLower == "codex" {
			activeProviders["codex"] = true
			activeProviders["cx"] = true
			activePrefixes[outputAlias] = true
		} else if provLower == "opencode" {
			activeProviders["opencode"] = true
			activeProviders["opencode-go"] = true
			activePrefixes[outputAlias] = true
		}

		// Add official models for this active provider (strictly with outputAlias prefix)
		if officialList := providers.GetOfficialModels(provLower); len(officialList) > 0 {
			for _, m := range officialList {
				cleanModel := m
				if strings.HasPrefix(cleanModel, outputAlias+"/") {
					cleanModel = strings.TrimPrefix(cleanModel, outputAlias+"/")
				}
				addModel(outputAlias+"/"+cleanModel, conn.Provider, conn.ID)
			}
		}

		// Add connection-specific models (customModels, defaultModel, deployment)
		if connData != nil {
			if defModel, ok := connData["defaultModel"].(string); ok && defModel != "" {
				cleanModel := defModel
				if strings.HasPrefix(cleanModel, outputAlias+"/") {
					cleanModel = strings.TrimPrefix(cleanModel, outputAlias+"/")
				}
				addModel(outputAlias+"/"+cleanModel, conn.Provider, conn.ID)
			}
			if deployment, ok := connData["deployment"].(string); ok && deployment != "" {
				cleanModel := deployment
				if strings.HasPrefix(cleanModel, outputAlias+"/") {
					cleanModel = strings.TrimPrefix(cleanModel, outputAlias+"/")
				}
				addModel(outputAlias+"/"+cleanModel, conn.Provider, conn.ID)
			}
			if custModels, ok := connData["customModels"].([]any); ok {
				for _, cm := range custModels {
					if cms, ok := cm.(string); ok && cms != "" {
						cleanModel := cms
						if strings.HasPrefix(cleanModel, outputAlias+"/") {
							cleanModel = strings.TrimPrefix(cleanModel, outputAlias+"/")
						}
						addModel(outputAlias+"/"+cleanModel, conn.Provider, conn.ID)
					}
				}
			}
		}
	}

	// Public/no-auth providers do not require a providerConnections row. Keep
	// their official models visible to the dashboard and API policy builder so
	// active OpenCode Zen models can be explicitly allowlisted.
	for provider, cfg := range providers.KnownProviders {
		if !cfg.NoAuth && cfg.DefaultAPIKey == "" || !providers.IsProviderEnabled(provider) {
			continue
		}
		outputAlias := getOutputAlias(provider, nil)
		activeProviders[provider] = true
		activePrefixes[outputAlias] = true
		for _, model := range providers.GetOfficialModels(provider) {
			cleanModel := model
			if strings.HasPrefix(cleanModel, outputAlias+"/") {
				cleanModel = strings.TrimPrefix(cleanModel, outputAlias+"/")
			}
			addModel(outputAlias+"/"+cleanModel, provider)
		}
	}

	// 4. Custom Provider Nodes (OpenAI-compatible / Anthropic-compatible endpoints)
	for _, node := range nodeMap {
		prefix := nodePrefixMap[node.ID]
		if prefix == "" {
			continue
		}
		activePrefixes[prefix] = true
		activeProviders[strings.ToLower(node.ID)] = true

		var nodeData map[string]any
		if json.Unmarshal([]byte(node.Data), &nodeData) == nil {
			if modelsList, ok := nodeData["models"].([]any); ok {
				for _, m := range modelsList {
					if ms, ok := m.(string); ok && ms != "" {
						cleanModel := ms
						if strings.HasPrefix(cleanModel, prefix+"/") {
							cleanModel = strings.TrimPrefix(cleanModel, prefix+"/")
						}
						addModel(prefix+"/"+cleanModel, node.ID, node.ID)
					}
				}
			}
		}
	}

	// 5. Custom Models from DB (e.g. added via + Add Model)
	if customList, err := h.Repo.GetCustomModels(); err == nil {
		for _, cm := range customList {
			provAliasLower := strings.ToLower(cm.ProviderAlias)
			outputAlias := getOutputAlias(cm.ProviderAlias, nil)
			if len(activeProviders) == 0 || activeProviders[provAliasLower] || activePrefixes[outputAlias] {
				cleanModel := cm.ID
				if strings.HasPrefix(cleanModel, outputAlias+"/") {
					cleanModel = strings.TrimPrefix(cleanModel, outputAlias+"/")
				}
				owner := cm.ProviderAlias
				if canonical := h.resolveProviderPrefix(cm.ProviderAlias); canonical != "" {
					owner = canonical
				}
				addModel(outputAlias+"/"+cleanModel, owner)
			}
		}
	}

	// 6. Explicit Model Aliases from DB (e.g. gpt-4o -> codex/gpt-4o)
	// 6. Explicit Model Aliases from DB (emitted with their resolved target prefix)
	if aliases, err := h.Repo.GetModelAliases(); err == nil {
		for _, target := range aliases {
			targetLower := strings.ToLower(target)
			var targetProv string
			if strings.Contains(targetLower, "/") {
				targetProv = strings.Split(targetLower, "/")[0]
			} else {
				targetProv = targetLower
			}

			// Only expose if target provider/prefix is active
			if len(activeProviders) == 0 || activeProviders[targetProv] || activePrefixes[targetProv] {
				if strings.Contains(target, "/") {
					addModel(target, targetProv)
				}
			}
		}
	}

	// 7. Combos from DB (e.g. combo-fast-code)
	if combos, err := h.Repo.GetCombos(); err == nil {
		for _, c := range combos {
			addModel(c.Name, "combo")
		}
	}

	if data == nil {
		data = []modelObj{}
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
	})
}

// HandleModelsInfo returns metadata for a specific model.
// GET /v1/models/info?id={modelId}
func (h *ChatHandler) HandleModelsInfo(w http.ResponseWriter, r *http.Request) {
	modelID := r.URL.Query().Get("id")
	if modelID == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing id query parameter")
		return
	}

	modelInfo, err := h.resolveModel(modelID)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, fmt.Sprintf("model not found: %s", modelID))
		return
	}

	ctxLen, maxOut := providers.GetModelTokenLimits(modelID)

	info := map[string]any{
		"id":                    modelID,
		"object":                "model",
		"owned_by":              modelInfo.Provider,
		"endpoint":              "/v1/chat/completions",
		"context_length":        ctxLen,
		"max_completion_tokens": maxOut,
		"max_input_tokens":      ctxLen - maxOut,
		"max_output_tokens":     maxOut,
	}
	if len(modelInfo.ComboModels) > 0 {
		info["combo"] = true
		info["strategy"] = modelInfo.Strategy
		info["models"] = modelInfo.ComboModels
	}

	handlerutil.WriteJSON(w, http.StatusOK, info)
}

// HandleModelsByKind returns models filtered by service kind.
// GET /v1/models/{kind}
func (h *ChatHandler) HandleModelsByKind(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing kind")
		return
	}
	if kind != "chat" {
		handlerutil.WriteJSONError(w, http.StatusNotFound, fmt.Sprintf("unsupported model kind: %s", kind))
		return
	}
	// The proxy-first runtime exposes chat models only. Media and web model
	// categories were retired together with their handlers.
	h.HandleModels(w, r)
	return
}

// HandleCountTokens estimates Anthropic-format token count.
// POST /v1/messages/count_tokens
func (h *ChatHandler) HandleCountTokens(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	inputTokens := estimateAnthropicTokens(body)
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"input_tokens": inputTokens,
	})
}

// EstimateAnthropicTokens estimates input token count from Claude-format body.
func EstimateAnthropicTokens(body []byte) int {
	return estimateAnthropicTokens(body)
}

// estimateAnthropicTokens estimates input token count from Claude-format body.
// Matches JS estimateAnthropicInputTokens in count_tokens/route.js.
func estimateAnthropicTokens(body []byte) int {
	var msg struct {
		System any   `json:"system"`
		Tools  []any `json:"tools"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		return 0
	}

	var totalChars int
	if sysStr, ok := msg.System.(string); ok {
		totalChars += len(sysStr)
	} else if sysArr, ok := msg.System.([]any); ok {
		for _, item := range sysArr {
			totalChars += countValueChars(item)
		}
	}

	var req struct {
		Messages []struct {
			Content any `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err == nil {
		for _, m := range req.Messages {
			totalChars += messageContentChars(m.Content)
		}
	}

	var toolsReq struct {
		Tools []any `json:"tools"`
	}
	if err := json.Unmarshal(body, &toolsReq); err == nil {
		for _, t := range toolsReq.Tools {
			totalChars += countValueChars(t)
		}
	}

	if totalChars == 0 {
		totalChars = len(body)
	}
	return (totalChars + 3) / 4
}

// CountValueChars counts text characters in a generic JSON value.
func CountValueChars(v any) int {
	return countValueChars(v)
}

func countValueChars(v any) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case string:
		return len(val)
	case []byte:
		return len(val)
	case float64:
		return len(fmt.Sprintf("%v", val))
	case bool:
		if val {
			return 4
		}
		return 5
	case []any:
		n := 0
		for _, item := range val {
			n += countValueChars(item)
		}
		return n
	case map[string]any:
		n := 0
		for k, item := range val {
			n += len(k) + countValueChars(item)
		}
		return n
	}
	return 0
}

func messageContentChars(content any) int {
	if content == nil {
		return 0
	}
	switch c := content.(type) {
	case string:
		return len(c)
	case []any:
		n := 0
		for _, block := range c {
			n += contentBlockChars(block)
		}
		return n
	}
	return countValueChars(content)
}

// MessageContentChars counts characters in a message content field.
func MessageContentChars(msg any) int {
	return messageContentChars(msg)
}

// ContentBlockChars counts characters in a single content block.
func ContentBlockChars(block any) int {
	return contentBlockChars(block)
}

func contentBlockChars(block any) int {
	if block == nil {
		return 0
	}
	m, ok := block.(map[string]any)
	if !ok {
		return countValueChars(block)
	}
	switch m["type"] {
	case "text":
		return countValueChars(m["text"])
	case "tool_use":
		return countValueChars(m["name"]) + countValueChars(m["input"])
	case "tool_result":
		return countValueChars(m["content"])
	case "thinking":
		return countValueChars(m["thinking"])
	default:
		return countValueChars(block)
	}
}

// HandleOllamaChat handles Ollama-compatible /v1/api/chat endpoint.
// POST /v1/api/chat
func (h *ChatHandler) HandleOllamaChat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	// Ollama request format is close to OpenAI — forward to chat completions.
	newReq, _ := http.NewRequestWithContext(r.Context(), "POST", "/v1/chat/completions", bytes.NewReader(body))
	newReq.Header = r.Header
	h.HandleChatCompletions(w, newReq)
}
