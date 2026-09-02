package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"zyrouter/backend/internal/log"

	"zyrouter/backend/internal/auditlog"
	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/middleware"
	"zyrouter/backend/internal/models"
	"zyrouter/backend/internal/providers"
	"zyrouter/backend/internal/proxy/executor"
	"zyrouter/backend/internal/tokensaver"
	"zyrouter/backend/internal/tracing"
	"zyrouter/backend/internal/translator"
	"zyrouter/backend/internal/usagetracker"
)

// handleAccountFallback attempts to forward a request with automatic account fallback.
func (h *ChatHandler) handleAccountFallback(
	ctx context.Context,
	w http.ResponseWriter,
	provider string,
	model string,
	pinnedConnectionID string,
	body []byte,
	isStream bool,
	translateResponse bool,
	endpoint string,
) error {
	apiKey := middleware.GetAuthenticatedApiKeyFromContext(ctx)
	providerLabel := h.displayProviderLabel(provider)
	modelLabel := h.displayModelLabel(provider, model)
	if pinnedConnectionID != "" {
		if apiKey != nil && !apiKey.IsProviderAllowed(pinnedConnectionID) {
			return fmt.Errorf("%w: '%s'", auth.ErrProviderNotAllowed, pinnedConnectionID)
		}
		connObj, connData, err := h.getBestConnection(provider, pinnedConnectionID, nil, model)
		if err != nil {
			return fmt.Errorf("pinned connection %s: %w", pinnedConnectionID, err)
		}
		log.Debug("fallback", "pinned", "provider", providerLabel, "model", modelLabel, "pinnedConn", pinnedConnectionID, "connObj", connObj.ID)
		return h.tryForwardWithConnection(ctx, w, provider, model, connObj.ID, connData, body, isStream, translateResponse, endpoint)
	}

	if !h.Repo.IsProviderAvailable(provider, model) {
		log.Warn("fallback", "skip unhealthy", "provider", providerLabel, "model", modelLabel, "providerId", provider)
		return fmt.Errorf("provider %s/%s is unhealthy", provider, model)
	}

	allConns, err := h.Repo.GetProviderConnections(provider, true)
	if err != nil || len(allConns) == 0 {
		if cfg, ok := providers.KnownProviders[provider]; ok && (cfg.NoAuth || cfg.DefaultAPIKey != "") {
			connObj, connData, gErr := h.getBestConnection(provider, "", nil, model)
			if gErr == nil && connObj != nil && connData != nil {
				return h.tryForwardWithConnection(ctx, w, provider, model, connObj.ID, connData, body, isStream, translateResponse, endpoint)
			}
			apiKey := cfg.DefaultAPIKey
			if apiKey == "" {
				apiKey = "public"
			}
			return h.tryForwardWithConnection(ctx, w, provider, model, "default", &ConnectionData{APIKey: apiKey}, body, isStream, translateResponse, endpoint)
		}
		return fmt.Errorf("no active connections for provider: %s", provider)
	}

	var excludeIDs []string
	var lastErr error
	// Resolve the account once through the normal selector. This is important
	// for provider-level round-robin: the old loop pinned every attempt to the
	// priority-ordered account list, so successful requests stayed on account 1.
	orderedConns := allConns
	var preferredID string
	var preferredData *ConnectionData
	if selected, selectedData, selectErr := h.getBestConnection(provider, "", nil, model); selectErr == nil && selected != nil && selectedData != nil {
		if apiKey == nil || apiKey.IsProviderAllowed(selected.ID) {
			preferredID = selected.ID
			preferredData = selectedData
			orderedConns = make([]*models.ProviderConnection, 0, len(allConns))
			orderedConns = append(orderedConns, selected)
			for _, c := range allConns {
				if c.ID != selected.ID {
					orderedConns = append(orderedConns, c)
				}
			}
		}
	}

	for _, c := range orderedConns {
		if slices.Contains(excludeIDs, c.ID) {
			continue
		}
		if apiKey != nil && !apiKey.IsProviderAllowed(c.ID) {
			continue
		}
		connObj := c
		var connData *ConnectionData
		var err error
		if c.ID == preferredID {
			connData = preferredData
		} else {
			connObj, connData, err = h.getBestConnection(provider, c.ID, nil, model)
		}
		if err != nil || connObj == nil {
			continue
		}
		apiKey := extractAPIKey(connData)
		if apiKey == "" {
			providerCfg, pErr := h.getProviderConfig(provider, connData)
			if pErr == nil && providerCfg.DefaultAPIKey != "" {
				apiKey = providerCfg.DefaultAPIKey
			} else {
				continue
			}
		}
		log.Info("fallback", "trying connection", "provider", providerLabel, "providerId", provider, "model", modelLabel, "modelId", model, "account", c.Name, "email", c.Email, "priority", c.Priority)
		if err := h.tryForwardWithConnection(ctx, w, provider, model, c.ID, connData, body, isStream, translateResponse, endpoint); err == nil {
			return nil
		} else {
			lastErr = err
		}
		var ue *upstreamError
		if errors.As(lastErr, &ue) && providers.RetryableStatusCodes[ue.StatusCode] {
			// Extract error text from upstream body for classification
			errorText := extractErrorText(ue.Body)
			// Get current backoff level from this connection
			currentBackoffLevel := h.Repo.GetConnectionBackoffLevel(connObj.ID)
			// Classify error to get dynamic cooldown
			classification := providers.ClassifyError(ue.StatusCode, errorText, currentBackoffLevel)
			cooldownSec := int((classification.CooldownMs + 999) / 1000) // ceil to seconds
			errMsg := errorText
			if errMsg == "" {
				errMsg = fmt.Sprintf("%d upstream error", ue.StatusCode)
			}
			h.Repo.LockConnectionModel(connObj.ID, model, cooldownSec, classification.NewBackoffLevel)
			log.Warn("fallback", "account failed, cycling to next connection", "failed_account", c.Name, "email", c.Email, "provider", providerLabel, "providerId", provider, "model", modelLabel, "modelId", model, "status", ue.StatusCode, "cooldown_s", cooldownSec)
			excludeIDs = append(excludeIDs, c.ID)
			continue
		}
		return lastErr
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("no available connections for provider: %s", provider)
}

// tryForwardWithConnection attempts a single upstream request using the given connection data.
func (h *ChatHandler) tryForwardWithConnection(
	ctx context.Context,
	w http.ResponseWriter,
	provider string,
	model string,
	connectionID string,
	connData *ConnectionData,
	body []byte,
	isStream bool,
	translateResponse bool,
	endpoint string,
) error {
	ctx = translator.WithUsageCapture(ctx)

	providerCfg, err := h.getProviderConfig(provider, connData)
	if err != nil {
		return fmt.Errorf("get config for %s/%s: %w", provider, model, err)
	}

	apiKey := extractAPIKey(connData)
	if apiKey == "" {
		if providerCfg.DefaultAPIKey != "" {
			apiKey = providerCfg.DefaultAPIKey
		} else {
			return &upstreamError{StatusCode: http.StatusUnauthorized, Body: []byte(`{"error":{"message":"no API key found","type":"auth_error","code":401}}`)}
		}
	}

	if connectionID != "" {
		rekey, _, err := h.refreshOAuthTokenIfExpired(connectionID, apiKey)
		if err == nil {
			apiKey = rekey
		} else {
			log.Warn("fallback", "OAuth token refresh error", "conn", connectionID, "error", err)
		}
	}
	// Extract account label, proxy pool, and routing strategy details for observability
	providerLabel := h.displayProviderLabel(provider)
	modelLabel := h.displayModelLabel(provider, model)
	accountLabel := h.displayAccountLabel(connectionID)
	if connectionID != "" && h.Repo != nil {
		if c, err := h.Repo.GetProviderConnectionByID(connectionID); err == nil && c != nil {
			accountLabel = h.displayAccountLabel(connectionID)
		}
	}

	proxyLabel := "Direct"
	if connData != nil && connData.ProxyPoolID != "" && connData.ProxyPoolID != "__none__" {
		if pool, pErr := h.Repo.GetProxyPool(connData.ProxyPoolID); pErr == nil && pool != nil {
			proxyLabel = pool.Name
			if pool.Type != "" {
				proxyLabel = fmt.Sprintf("%s (%s)", pool.Name, strings.ToUpper(pool.Type))
			}
		} else {
			proxyLabel = connData.ProxyPoolID
		}
	}

	stratLabel := "fallback"
	if strat, ok := h.getProviderStrategy(provider); ok && strat.FallbackStrategy != nil && *strat.FallbackStrategy != "" {
		stratLabel = *strat.FallbackStrategy
		if stratLabel == "round-robin" && strat.StickyRoundRobinLimit > 1 {
			stratLabel = fmt.Sprintf("round-robin (sticky=%d)", strat.StickyRoundRobinLimit)
		}
	}

	log.Info("router", "dispatch", "provider", providerLabel, "providerId", provider, "model", modelLabel, "modelId", model, "account", accountLabel, "connectionId", connectionID, "proxy", proxyLabel, "strategy", stratLabel, "stream", isStream)

	pipedBody := h.applyTokenSavers(body)
	start := time.Now()
	metrics := &streamMetrics{}
	var fwdErr error

	usagetracker.GetTracker().TrackPending(modelLabel, providerLabel, connectionID, true, false)
	defer func() {
		hasErr := fwdErr != nil
		usagetracker.GetTracker().TrackPending(modelLabel, providerLabel, connectionID, false, hasErr)
	}()
	httpClient := h.getClientForConnection(connData)

	if exec := executor.Get(provider); exec != nil {
		fwdErr = exec(w, &executor.Request{
			Ctx:           ctx,
			Client:        httpClient,
			Config:        providerCfg,
			APIKey:        apiKey,
			Body:          pipedBody,
			IsStream:      isStream,
			TranslateResp: translateResponse,
			ResponseBuf:   &metrics.ResponseBuf,
			StartTime:     start,
			TTFT:          &metrics.TTFT,
		})
	} else if providerCfg.IsGeminiNative() {
		fwdErr = h.forwardGeminiNativeRequest(ctx, w, provider, providerCfg, apiKey, connectionID, pipedBody, isStream, translateResponse, httpClient, metrics)
	} else {
		fwdErr = h.forwardRequest(ctx, w, providerCfg, apiKey, pipedBody, isStream, translateResponse, metrics)
	}

	var ue *upstreamError
	if errors.As(fwdErr, &ue) && ue.StatusCode == http.StatusUnauthorized && connectionID != "" {
		refreshedKey, _, rErr := h.forceRefreshOAuthToken(connectionID)
		if rErr == nil && refreshedKey != "" && refreshedKey != apiKey {
			log.Info("fallback", "reactive 401 token refresh success, retrying request", "conn", connectionID)
			apiKey = refreshedKey
			if exec := executor.Get(provider); exec != nil {
				fwdErr = exec(w, &executor.Request{
					Ctx:           ctx,
					Client:        httpClient,
					Config:        providerCfg,
					APIKey:        apiKey,
					Body:          pipedBody,
					IsStream:      isStream,
					TranslateResp: translateResponse,
					ResponseBuf:   &metrics.ResponseBuf,
					StartTime:     start,
					TTFT:          &metrics.TTFT,
				})
			} else if providerCfg.IsGeminiNative() {
				fwdErr = h.forwardGeminiNativeRequest(ctx, w, provider, providerCfg, apiKey, connectionID, pipedBody, isStream, translateResponse, httpClient, metrics)
			} else {
				fwdErr = h.forwardRequest(ctx, w, providerCfg, apiKey, pipedBody, isStream, translateResponse, metrics)
			}
		}
	}

	latencyMs := time.Since(start).Milliseconds()

	// Lightweight request trace for /debug/traces (provider/model latency).
	status := "error"
	if fwdErr == nil {
		status = "200"
	} else if ue, ok := fwdErr.(*upstreamError); ok && ue.StatusCode > 0 {
		status = fmt.Sprintf("%d", ue.StatusCode)
	}
	tracing.Record(tracing.Span{
		Provider:   provider,
		Model:      model,
		Status:     status,
		DurationMs: latencyMs,
		TTFTMs:     metrics.TTFT,
	})

	if fwdErr == nil {
		// Clear any existing model lock on success (matching Next.js clearAccountError)
		if unlockErr := h.Repo.UnlockConnectionModel(connectionID, model); unlockErr != nil {
			log.Warn("fallback", "unlock failed", "provider", provider, "model", model, "error", unlockErr)
		}
		usage := translator.GetAndClearUsage(ctx)
		if usage == nil {
			usage = &translator.OpenAIUsage{}
		}
		logInfo := &UsageLogInfo{
			Provider:     provider,
			Model:        model,
			ConnectionID: connectionID,
			ProxyPoolID:  connData.ProxyPoolID,
			APIKey:       apiKey,
			Endpoint:     endpoint,
		}
		h.logUsage(logInfo, usage, latencyMs, body, metrics)
	} else {
		var ue *upstreamError
		statusCode := 0
		var errBodyStr string
		if errors.As(fwdErr, &ue) {
			statusCode = ue.StatusCode
			errBodyStr = string(ue.Body)
		}
		if projectProbeCached(connectionID) {
			log.Debug("fallback", "upstream skipped (cached no-project)", "provider", provider, "model", model, "conn", connectionID, "error", fwdErr)
		} else {
			log.Warn("fallback", "upstream failed", "provider", provider, "model", model, "conn", connectionID, "status", statusCode, "error", fwdErr)
		}

		// Also record failed request in usagetracker so recent log reflects error immediately
		now := time.Now()
		reqID := fmt.Sprintf("%d-%s", now.UnixMilli(), model)
		usagetracker.GetTracker().PushRecent(usagetracker.RecentRequest{
			ID:               reqID,
			Timestamp:        now.Format(time.RFC3339),
			Model:            modelLabel,
			Provider:         providerLabel,
			Account:          accountLabel,
			Proxy:            proxyLabel,
			Strategy:         stratLabel,
			PromptTokens:     0,
			CompletionTokens: 0,
			DurationMs:       latencyMs,
			Latency:          fmt.Sprintf("%.2fs", float64(latencyMs)/1000.0),
			Status:           fmt.Sprintf("%d", statusCode),
		}, h.Repo)

		// Record in SQLite requestDetails
		reqData, _ := json.Marshal(map[string]any{
			"id": reqID, "provider": providerLabel, "providerId": provider, "model": modelLabel, "modelId": model,
			"connectionId": connectionID, "account": accountLabel,
			"proxy": proxyLabel, "strategy": stratLabel, "status": "error",
			"statusCode": statusCode,
			"timestamp":  now.Format("2006-01-02T15:04:05.000Z"),
			"latency":    map[string]int64{"total": latencyMs},
			"error":      fwdErr.Error(),
		})
		_ = h.Repo.InsertRequestDetail(reqID, provider, model, connectionID, "error", string(reqData))
		auditlog.Get().Log(&auditlog.AuditEntry{
			ID:           reqID,
			Timestamp:    now.Format(time.RFC3339Nano),
			Endpoint:     endpoint,
			Provider:     provider,
			Model:        model,
			ConnectionID: connectionID,
			APIKey:       apiKey,
			Status:       "error",
			StatusCode:   statusCode,
			DurationMs:   latencyMs,
			ClientRequest: auditlog.HTTPPayload{
				Method: "POST",
				URL:    endpoint,
				Body:   string(body),
			},
			ProviderResponse: auditlog.HTTPPayload{
				Body: errBodyStr,
			},
			Error: fwdErr.Error(),
		})
	}
	return fwdErr
}

// applyTokenSavers runs RTK compression and prompt injection on the request body.
// false from compress/inject means nothing changed (or unparseable) — keep original, not a failure.
func (h *ChatHandler) applyTokenSavers(body []byte) []byte {
	// Prompt-injection guard: tag (never block) flagged user content. Early
	// detection here means operators can see abuse before it reaches upstream.
	// Toggle via settings.injectionGuardEnabled (off bypasses the scan).
	if h.TokenSaver.InjectionGuardEnabled() {
		if inj := tokensaver.DetectInjection(body); inj.Flagged {
			log.Warn("guard", "prompt injection flagged", "reasons", inj.Reasons, "messageId", inj.MessageID)
		}
	}
	out := body
	if h.TokenSaver.RTKEnabled() {
		if next, did := tokensaver.CompressMessages(out); did {
			out = next
		}
	}
	if h.TokenSaver.CavemanEnabled() {
		prompt := tokensaver.GetCavemanPrompt(h.TokenSaver.CavemanLevel())
		if next, did := tokensaver.InjectSystemPrompt(out, prompt); did {
			out = next
		}
	}
	if h.TokenSaver.PonytailEnabled() {
		prompt := tokensaver.GetPonytailPrompt(h.TokenSaver.PonytailLevel())
		if next, did := tokensaver.InjectSystemPrompt(out, prompt); did {
			out = next
		}
	}
	return out
}

// extractErrorText attempts to extract a human-readable error message from an upstream error JSON body.
// Returns "" when the body isn't parseable or has no message field.
func extractErrorText(body []byte) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		return parsed.Error.Message
	}
	return ""
}

// extractRetryAfter extracts a retryAfter ISO timestamp from an upstream error JSON body.
// Checks common field names: retryAfter, retry_after, resetsAt, resets_at.
// Returns "" when not found or not parseable.
func extractRetryAfter(body []byte) string {
	var parsed struct {
		RetryAfter string `json:"retryAfter"`
		RetryAlt   string `json:"retry_after"`
		ResetsAt   string `json:"resetsAt"`
		ResetsAlt  string `json:"resets_at"`
		Error      struct {
			RetryAfter string `json:"retryAfter"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	if parsed.RetryAfter != "" {
		return parsed.RetryAfter
	}
	if parsed.RetryAlt != "" {
		return parsed.RetryAlt
	}
	if parsed.ResetsAt != "" {
		return parsed.ResetsAt
	}
	if parsed.ResetsAlt != "" {
		return parsed.ResetsAlt
	}
	if parsed.Error.RetryAfter != "" {
		return parsed.Error.RetryAfter
	}
	return ""
}

// formatRetryAfter formats an ISO timestamp into a human-readable "reset after Xm Ys" string.
// Returns "" when the timestamp is empty, unparseable, or in the past.
func formatRetryAfter(isoTimestamp string) string {
	if isoTimestamp == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, isoTimestamp)
	if err != nil {
		return ""
	}
	diffMs := time.Until(parsed)
	if diffMs <= 0 {
		return "reset after 0s"
	}
	totalSec := int((diffMs + 999) / 1000) // ceil
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60
	var parts []string
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if s > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}
	return "reset after " + strings.Join(parts, " ")
}
