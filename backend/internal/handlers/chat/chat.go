package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"time"

	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/clientstream"
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
	// Public requests must resolve through an admin-created alias before any
	// synthetic-request bypass is considered.
	if _, err := h.resolveClientModel(reqBody.Model); err != nil {
		status := modelResolutionStatus(err)
		handlerutil.WriteJSONError(w, status, err.Error())
		return
	}
	// Bypass synthetic requests (Claude Code naming, warmup, keepalive)
	if handleBypassRequest(w, body, reqBody.Model, reqBody.Stream) {
		return
	}

	modelInfo, err := h.resolveClientModel(reqBody.Model)
	if err != nil {
		handlerutil.WriteJSONError(w, modelResolutionStatus(err), err.Error())
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
	if err := h.reserveUserQuota(r, body); err != nil {
		status := http.StatusTooManyRequests
		if !errors.Is(err, auth.ErrRateLimitExceeded) {
			status = http.StatusInternalServerError
		}
		handlerutil.WriteJSONError(w, status, err.Error())
		return
	}
	ctx := context.WithValue(r.Context(), publicModelContextKey{}, reqBody.Model)
	h.publishClientStarted(r, reqBody.Model)

	if len(modelInfo.ComboModels) > 0 {
		if modelInfo.Strategy == "fusion" {
			h.handleFusion(ctx, w, body, modelInfo.ComboModels, modelInfo.Strategy, reqBody.Stream, false, reqBody.Model, modelInfo.StickyLimit)
			return
		}
		h.handleComboFallback(ctx, w, body, modelInfo.ComboModels, modelInfo.Strategy, reqBody.Stream, false, reqBody.Model, modelInfo.StickyLimit)
		return
	}

	h.handleSingleModel(ctx, w, body, modelInfo, reqBody.Stream, false)
}

type publicModelContextKey struct{}

func publicModelFromContext(ctx context.Context) string {
	if value, ok := ctx.Value(publicModelContextKey{}).(string); ok {
		return value
	}
	return "unknown"
}

func (h *ChatHandler) publishClientStarted(r *http.Request, model string) {
	key := middleware.GetAuthenticatedApiKey(r)
	if key == nil || key.UserID == nil || strings.TrimSpace(*key.UserID) == "" {
		return
	}
	requestID := middleware.GetRequestID(r)
	clientstream.Get().Publish(*key.UserID, clientstream.Event{
		ID: requestID + ":started", Type: "request.started", RequestID: requestID,
		Model: model, Status: "started", Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
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

	modelInfo, err := h.resolveClientModel(reqBody.Model)
	if err != nil {
		log.Error("chat", "resolve model failed", "error", err, "model", reqBody.Model)
		handlerutil.WriteJSONError(w, modelResolutionStatus(err), err.Error())
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
	if err := h.reserveUserQuota(r, body); err != nil {
		status := http.StatusTooManyRequests
		if !errors.Is(err, auth.ErrRateLimitExceeded) {
			status = http.StatusInternalServerError
		}
		handlerutil.WriteJSONError(w, status, err.Error())
		return
	}
	ctx := context.WithValue(r.Context(), publicModelContextKey{}, reqBody.Model)
	h.publishClientStarted(r, reqBody.Model)

	if len(modelInfo.ComboModels) > 0 {
		if modelInfo.Strategy == "fusion" {
			bodyJSON, err := json.Marshal(workingBody)
			if err != nil {
				handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to marshal request body")
				return
			}
			h.handleFusion(ctx, w, bodyJSON, modelInfo.ComboModels, modelInfo.Strategy, reqBody.Stream, translateResponse, reqBody.Model, modelInfo.StickyLimit)
			return
		}
		h.handleMessagesComboFallback(ctx, w, workingBody, modelInfo.ComboModels, modelInfo.Strategy, reqBody.Stream, reqBody.Model, modelInfo.StickyLimit)
		return
	}

	h.handleMessagesSingleModel(ctx, w, workingBody, modelInfo, reqBody.Stream, translateResponse)
}

// validateRequestPolicy validates the resolved model and every concrete model
// in a combo. Validation happens after resolution so aliases and fallback
// entries cannot bypass provider-prefix or connection restrictions.
func (h *ChatHandler) validateRequestPolicy(r *http.Request, requested string, info *ModelInfo) error {
	key := middleware.GetAuthenticatedApiKey(r)
	if key == nil || info == nil {
		return nil
	}
	if strings.Contains(requested, "/") {
		return auth.ErrProviderPrefixForbidden
	}
	// All keys inherit their model access from the account type tier.
	// Administrator tier bypasses restriction; other tiers require the alias in accountTypeModels.
	typeID := ""
	if key.AccountTypeID != nil {
		typeID = strings.TrimSpace(*key.AccountTypeID)
	}
	if typeID == "" && key.UserID == nil {
		typeID = "administrator"
	}
	if h.Repo != nil && typeID != "" && typeID != "administrator" {
		aliasTarget, err := h.Repo.GetModelAlias(requested)
		if err != nil {
			return fmt.Errorf("model alias lookup failed: %w", err)
		}
		if strings.TrimSpace(aliasTarget) == "" {
			return fmt.Errorf("model alias '%s' does not exist", requested)
		}
		allowed, err := h.Repo.IsAliasAllowedForAccountType(typeID, requested)
		if err != nil {
			return fmt.Errorf("account type lookup failed: %w", err)
		}
		if !allowed {
			return fmt.Errorf("model alias '%s' is not enabled for this account type", requested)
		}
	}

	validateProvider := func(modelInfo *ModelInfo) error {
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
		if !key.IsModelAllowed(requested) {
			return fmt.Errorf("%w: '%s'", auth.ErrModelNotAllowed, requested)
		}
		return validateProvider(info)
	}
	// A composite alias is the single public policy subject. Its internal
	// provider/model members are routing targets, not client model IDs.
	if !key.IsModelAllowed(requested) {
		return fmt.Errorf("%w: '%s'", auth.ErrModelNotAllowed, requested)
	}
	for _, entry := range info.ComboModels {
		entryInfo, err := h.resolveModel(entry)
		if err != nil {
			return fmt.Errorf("resolve combo member %q: %w", entry, err)
		}
		if err := validateProvider(entryInfo); err != nil {
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

// HandleModels responds with the admin-published public alias catalog only.
func (h *ChatHandler) HandleModels(w http.ResponseWriter, r *http.Request) {
	h.handlePublishedAliasModels(w, r)
	return
}

// handlePublishedAliasModels exposes only the admin-published alias registry.
// Provider inventories and raw provider/model identifiers are never public.
func (h *ChatHandler) handlePublishedAliasModels(w http.ResponseWriter, r *http.Request) {
	type modelObj struct {
		ID                  string `json:"id"`
		Object              string `json:"object"`
		Created             int64  `json:"created"`
		OwnedBy             string `json:"owned_by"`
		ContextLength       int    `json:"context_length,omitempty"`
		MaxCompletionTokens int    `json:"max_completion_tokens,omitempty"`
	}
	aliases, err := h.Repo.GetModelAliases()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to load published model aliases")
		return
	}
	apiKey := middleware.GetAuthenticatedApiKey(r)
	allowedAccountAliases := map[string]bool(nil)
	if apiKey != nil && apiKey.AccountTypeID != nil && strings.TrimSpace(*apiKey.AccountTypeID) != "" && strings.TrimSpace(*apiKey.AccountTypeID) != "administrator" {
		list, listErr := h.Repo.GetAccountTypeModels(strings.TrimSpace(*apiKey.AccountTypeID))
		if listErr != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to load account model permissions")
			return
		}
		allowedAccountAliases = make(map[string]bool, len(list))
		for _, alias := range list {
			allowedAccountAliases[alias] = true
		}
	}
	ordered := make([]string, 0, len(aliases))
	for alias := range aliases {
		ordered = append(ordered, string(alias))
	}
	sort.Strings(ordered)
	data := make([]modelObj, 0, len(ordered))
	for _, alias := range ordered {
		if allowedAccountAliases != nil && !allowedAccountAliases[alias] {
			continue
		}
		info, resolveErr := h.resolveModel(alias)
		if resolveErr != nil || info == nil {
			continue
		}
		if apiKey != nil {
			if !h.isPublicAliasAllowed(apiKey, alias, info) {
				continue
			}
		}
		ctxLen, maxOut := providers.GetModelTokenLimits(info.Model)
		data = append(data, modelObj{ID: alias, Object: "model", Created: time.Now().Unix(), OwnedBy: "zyrouter", ContextLength: ctxLen, MaxCompletionTokens: maxOut})
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (h *ChatHandler) isPublicAliasAllowed(key *models.APIKey, alias string, info *ModelInfo) bool {
	if key == nil || info == nil || !key.IsModelAllowed(alias) {
		return false
	}
	checkProvider := func(candidate *ModelInfo) bool {
		providerTarget := candidate.ConnectionID
		if providerTarget == "" {
			providerTarget = candidate.Provider
		}
		return key.IsProviderAllowed(providerTarget) || key.IsProviderAllowed(candidate.Provider)
	}
	if len(info.ComboModels) == 0 {
		return checkProvider(info)
	}
	for _, member := range info.ComboModels {
		memberInfo, err := h.resolveModel(member)
		if err != nil || !checkProvider(memberInfo) {
			return false
		}
	}
	return true
}

// HandleModelsInfo returns metadata for a specific model.
// GET /v1/models/info?id={modelId}
func (h *ChatHandler) HandleModelsInfo(w http.ResponseWriter, r *http.Request) {
	modelID := r.URL.Query().Get("id")
	if modelID == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing id query parameter")
		return
	}

	modelInfo, err := h.resolveClientModel(modelID)
	if err != nil {
		handlerutil.WriteJSONError(w, modelResolutionStatus(err), err.Error())
		return
	}

	ctxLen, maxOut := providers.GetModelTokenLimits(modelID)

	info := map[string]any{
		"id":                    modelID,
		"object":                "model",
		"owned_by":              "zyrouter",
		"endpoint":              "/v1/chat/completions",
		"context_length":        ctxLen,
		"max_completion_tokens": maxOut,
		"max_input_tokens":      ctxLen - maxOut,
		"max_output_tokens":     maxOut,
	}
	if len(modelInfo.ComboModels) > 0 {
		info["combo"] = true
		info["strategy"] = modelInfo.Strategy
		// Do not expose internal provider/model targets through the public
		// metadata endpoint. The public model ID is the composite alias.
		info["member_count"] = len(modelInfo.ComboModels)
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

// UpstreamTestResult captures the live ping test result of an upstream model.
type UpstreamTestResult struct {
	Status     string `json:"status"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	LatencyMs  int64  `json:"latencyMs"`
	StatusCode int    `json:"statusCode,omitempty"`
	Reply      string `json:"reply,omitempty"`
	Message    string `json:"message"`
	Error      string `json:"error,omitempty"`
}

// TestProviderModel executes a lightweight, real synthetic ping through the full gateway
// routing engine to verify connectivity, authentication, and model availability.
func (h *ChatHandler) TestProviderModel(ctx context.Context, provider, pinnedConnID, model, prompt string) (*UpstreamTestResult, error) {
	if prompt == "" {
		prompt = "ping"
	}

	// If provider is actually a connection ID, resolve it
	if conn, _ := h.Repo.GetProviderConnectionByID(provider); conn != nil {
		if pinnedConnID == "" {
			pinnedConnID = conn.ID
		}
		provider = conn.Provider
	}

	canonical := providers.ResolveAlias(provider)
	if canonical == "google" {
		canonical = "gemini"
	}

	reqBody := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 16,
		"stream":     false,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return &UpstreamTestResult{
			Status:   "error",
			Provider: provider,
			Model:    model,
			Error:    err.Error(),
			Message:  err.Error(),
		}, nil
	}

	rec := httptest.NewRecorder()
	start := time.Now()

	// Try both canonical and original provider name for connection matching
	targetProvider := canonical
	if pinnedConnID == "" {
		allConns, _ := h.Repo.GetProviderConnections(targetProvider, true)
		if len(allConns) == 0 && targetProvider != provider {
			if fallbackConns, _ := h.Repo.GetProviderConnections(provider, true); len(fallbackConns) > 0 {
				targetProvider = provider
			}
		}
	}

	err = h.handleAccountFallback(ctx, rec, targetProvider, model, pinnedConnID, bodyBytes, false, false, "/v1/chat/completions")
	latencyMs := time.Since(start).Milliseconds()

	if err != nil {
		var ue *upstreamError
		if errors.As(err, &ue) {
			errMsg := fmt.Sprintf("HTTP %d: %s", ue.StatusCode, strings.TrimSpace(string(ue.Body)))
			return &UpstreamTestResult{
				Status:     "error",
				Provider:   provider,
				Model:      model,
				StatusCode: ue.StatusCode,
				LatencyMs:  latencyMs,
				Error:      errMsg,
				Message:    errMsg,
			}, nil
		}
		return &UpstreamTestResult{
			Status:    "error",
			Provider:  provider,
			Model:     model,
			LatencyMs: latencyMs,
			Error:     err.Error(),
			Message:   err.Error(),
		}, nil
	}

	var respObj struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &respObj)
	reply := ""
	if len(respObj.Choices) > 0 {
		reply = strings.TrimSpace(respObj.Choices[0].Message.Content)
	} else if len(respObj.Content) > 0 {
		reply = strings.TrimSpace(respObj.Content[0].Text)
	}
	if reply == "" {
		reply = "OK"
	}

	return &UpstreamTestResult{
		Status:     "ok",
		Provider:   provider,
		Model:      model,
		StatusCode: http.StatusOK,
		LatencyMs:  latencyMs,
		Reply:      reply,
		Message:    fmt.Sprintf("Model responded in %dms", latencyMs),
	}, nil
}
