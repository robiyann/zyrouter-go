package admin

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"zyrouter/backend/internal/auditlog"
	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlers/chat"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/models"
	"zyrouter/backend/internal/providers"
)

type ChatTester interface {
	TestProviderModel(ctx context.Context, provider, pinnedConnID, model, prompt string) (*chat.UpstreamTestResult, error)
}

type AdminHandler struct {
	repo       *db.Repo
	chatTester ChatTester
}

func NewAdminHandler(repo *db.Repo) *AdminHandler {
	return &AdminHandler{repo: repo}
}

func (h *AdminHandler) SetChatTester(tester ChatTester) {
	h.chatTester = tester
}

// GenerateRandomKey generates a secure random API key starting with "zy_".
func GenerateRandomKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "zy_" + hex.EncodeToString(bytes), nil
}

// ==========================================
// API Keys Management Handlers
// ==========================================

func (h *AdminHandler) HandleGetKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.repo.GetApiKeys()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if keys == nil {
		keys = []*models.APIKey{}
	}
	masked := make([]*models.APIKey, 0, len(keys))
	for _, key := range keys {
		copy := *key
		copy.Key = maskAPIKey(key.Key)
		masked = append(masked, &copy)
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"keys": masked})
}

// HandleRevealKey returns one full key only when an authenticated admin
// explicitly requests it for a copy action. List responses never contain it.
func (h *AdminHandler) HandleRevealKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	key, err := h.repo.GetApiKeyByID(id)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if key == nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "API key not found")
		return
	}
	// A hash-backed key can never be revealed. The only full secret response is
	// returned by the create endpoint, once, in the same request that generated it.
	handlerutil.WriteJSONError(w, http.StatusGone, "API key secret is not recoverable; rotate the key")
}

func maskAPIKey(value string) string {
	if len(value) <= 10 {
		return "***"
	}
	return value[:7] + "..." + value[len(value)-4:]
}

func (h *AdminHandler) HandleCreateKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name          string                  `json:"name"`
		MachineID     string                  `json:"machineId,omitempty"`
		AccountTypeID string                  `json:"accountTypeId,omitempty"`
		Restrictions  *models.KeyRestrictions `json:"restrictions,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	keyStr, err := GenerateRandomKey()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to generate api key")
		return
	}

	id := "key_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	var restrictionsStr *string
	if body.Restrictions != nil {
		b, _ := json.Marshal(body.Restrictions)
		s := string(b)
		restrictionsStr = &s
	}

	accountTypeID := strings.TrimSpace(body.AccountTypeID)
	if accountTypeID == "" {
		accountTypeID = "administrator"
	}
	if typ, err := h.repo.GetAccountType(accountTypeID); err != nil || typ == nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid accountTypeId: "+accountTypeID)
		return
	}

	apiKey, err := h.repo.CreateApiKey(id, keyStr, body.Name, body.MachineID, accountTypeID, restrictionsStr)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	accountTypeVal := "administrator"
	if apiKey.AccountTypeID != nil && *apiKey.AccountTypeID != "" {
		accountTypeVal = *apiKey.AccountTypeID
	}

	handlerutil.WriteJSON(w, http.StatusCreated, map[string]any{
		"id": apiKey.ID, "name": apiKey.Name, "key": keyStr,
		"accountTypeId": accountTypeVal, "isActive": apiKey.IsActive,
		"restrictions": apiKey.Restrictions, "createdAt": apiKey.CreatedAt,
		"warning": "Store this key securely. It will not be shown again.",
	})
}

func (h *AdminHandler) HandleUpdateKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing key id")
		return
	}

	var body struct {
		Name          *string                 `json:"name,omitempty"`
		IsActive      *int                    `json:"isActive,omitempty"`
		AccountTypeID *string                 `json:"accountTypeId,omitempty"`
		Restrictions  *models.KeyRestrictions `json:"restrictions,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.AccountTypeID != nil && strings.TrimSpace(*body.AccountTypeID) != "" {
		trimmed := strings.TrimSpace(*body.AccountTypeID)
		if typ, err := h.repo.GetAccountType(trimmed); err != nil || typ == nil {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid accountTypeId: "+trimmed)
			return
		}
		body.AccountTypeID = &trimmed
	}

	var restrictionsStr *string
	if body.Restrictions != nil {
		b, _ := json.Marshal(body.Restrictions)
		s := string(b)
		restrictionsStr = &s
	}

	if err := h.repo.UpdateApiKey(id, body.Name, body.IsActive, body.AccountTypeID, restrictionsStr); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AdminHandler) HandleDeleteKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing key id")
		return
	}
	if err := h.repo.DeleteApiKey(id); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ==========================================
// Provider Connections Handlers
// ==========================================

func (h *AdminHandler) HandleGetProviders(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	connections, err := h.repo.GetProviderConnections(provider, false)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if connections == nil {
		connections = []*models.ProviderConnection{}
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"connections": connections,
	})
}

func (h *AdminHandler) HandleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var conn models.ProviderConnection
	if err := json.NewDecoder(r.Body).Decode(&conn); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if conn.ID == "" {
		bytes := make([]byte, 8)
		rand.Read(bytes)
		conn.ID = "conn-" + hex.EncodeToString(bytes)
	}
	if conn.AuthType == "" {
		conn.AuthType = "apikey"
	}
	if conn.Name == nil || strings.TrimSpace(*conn.Name) == "" {
		name := generatedConnectionName(conn.Provider, conn.Email, conn.Data)
		conn.Name = &name
	}
	if conn.Priority == nil || *conn.Priority <= 0 {
		priority := h.nextProviderPriority(conn.Provider)
		conn.Priority = &priority
	}
	conn.IsActive = 1

	if err := h.repo.CreateProviderConnectionFull(&conn); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusCreated, conn)
}

func generatedConnectionName(provider string, email *string, rawData string) string {
	if email != nil && strings.TrimSpace(*email) != "" {
		return strings.TrimSpace(*email)
	}
	var data map[string]any
	_ = json.Unmarshal([]byte(rawData), &data)
	for _, field := range []string{"apiKey", "accessToken", "token", "sessionCookie"} {
		if value, ok := data[field].(string); ok && strings.TrimSpace(value) != "" {
			value = strings.TrimSpace(value)
			suffix := value
			if len(suffix) > 6 {
				suffix = suffix[len(suffix)-6:]
			}
			return fmt.Sprintf("%s account (%s)", provider, suffix)
		}
	}
	if strings.TrimSpace(provider) != "" {
		return fmt.Sprintf("%s account", provider)
	}
	return "provider account"
}

func (h *AdminHandler) nextProviderPriority(provider string) int {
	connections, err := h.repo.GetProviderConnections(provider, false)
	if err != nil {
		return 1
	}
	next := 1
	for _, connection := range connections {
		if connection.Priority != nil && *connection.Priority >= next {
			next = *connection.Priority + 1
		}
	}
	return next
}

func (h *AdminHandler) HandleGetProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	conn, err := h.repo.GetProviderConnectionByID(id)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if conn == nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "provider connection not found")
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, conn)
}

func (h *AdminHandler) HandleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing provider connection id")
		return
	}

	existing, err := h.repo.GetProviderConnectionByID(id)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "provider connection not found")
		return
	}

	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if v, ok := raw["provider"].(string); ok && v != "" {
		existing.Provider = v
	}
	if v, ok := raw["authType"].(string); ok && v != "" {
		existing.AuthType = v
	}
	if v, ok := raw["name"].(string); ok {
		existing.Name = &v
	}
	if v, ok := raw["email"].(string); ok {
		existing.Email = &v
	}
	if v, ok := raw["priority"].(float64); ok {
		p := int(v)
		existing.Priority = &p
	}
	if v, ok := raw["isActive"].(float64); ok {
		existing.IsActive = int(v)
	} else if v, ok := raw["isActive"].(bool); ok {
		if v {
			existing.IsActive = 1
		} else {
			existing.IsActive = 0
		}
	}
	if v, ok := raw["data"].(string); ok {
		existing.Data = v
	} else if proxyPoolId, ok := raw["proxyPoolId"]; ok {
		var d map[string]any
		_ = json.Unmarshal([]byte(existing.Data), &d)
		if d == nil {
			d = make(map[string]any)
		}
		if proxyPoolId == nil || proxyPoolId == "__none__" {
			delete(d, "proxyPoolId")
		} else if ps, ok := proxyPoolId.(string); ok {
			d["proxyPoolId"] = ps
		}
		nb, _ := json.Marshal(d)
		existing.Data = string(nb)
	}

	if err := h.repo.UpdateProviderConnection(existing); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, existing)
}

// HandleFetchProviderConnectionModels fetches available models from the provider upstream endpoint,
// merging with official catalog models and custom models so admins always have a complete list.
func (h *AdminHandler) HandleFetchProviderConnectionModels(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "connection id or provider id is required")
		return
	}

	var conn *models.ProviderConnection
	conn, _ = h.repo.GetProviderConnectionByID(id)
	if conn == nil {
		if conns, err := h.repo.GetProviderConnections(id, true); err == nil && len(conns) > 0 {
			conn = conns[0]
		}
	}

	var connData map[string]any
	if conn != nil && conn.Data != "" {
		_ = json.Unmarshal([]byte(conn.Data), &connData)
	}
	if connData == nil {
		connData = make(map[string]any)
	}

	apiKey, _ := connData["apiKey"].(string)
	baseUrl, _ := connData["baseUrl"].(string)
	provider := id
	if conn != nil {
		provider = conn.Provider
	}

	canonical := strings.ToLower(provider)
	if mapped, ok := providers.ProviderAliasMap[canonical]; ok {
		canonical = mapped
	}
	if canonical == "google" {
		canonical = "gemini"
	}

	// If provider is a custom node, get baseUrl from node if empty
	if baseUrl == "" {
		if node, nodeData, err := h.repo.GetProviderNodeByID(provider); err == nil && node != nil && nodeData != nil && nodeData.BaseURL != "" {
			baseUrl = nodeData.BaseURL
		}
	}
	if baseUrl == "" {
		if node, nodeData, err := h.repo.GetProviderNodeByID(id); err == nil && node != nil && nodeData != nil && nodeData.BaseURL != "" {
			baseUrl = nodeData.BaseURL
		}
	}
	if baseUrl == "" {
		if node, nodeData, err := h.repo.GetProviderNodeByPrefix(id); err == nil && node != nil && nodeData != nil && nodeData.BaseURL != "" {
			baseUrl = nodeData.BaseURL
		}
	}

	// Fallback to KnownProviders endpoint if baseUrl is still empty
	if baseUrl == "" {
		if known, ok := providers.KnownProviders[canonical]; ok {
			if strings.Contains(known.BaseURL, "/chat/completions") {
				baseUrl = strings.TrimSuffix(known.BaseURL, "/chat/completions")
			} else if strings.Contains(known.BaseURL, "/messages") {
				baseUrl = strings.TrimSuffix(known.BaseURL, "/messages")
			} else {
				baseUrl = known.BaseURL
			}
			if apiKey == "" && (known.NoAuth || known.DefaultAPIKey != "") {
				apiKey = known.DefaultAPIKey
			}
		}
	}

	seen := make(map[string]bool)
	var result []map[string]any

	addModel := func(mID string) {
		mID = strings.TrimSpace(mID)
		if mID == "" || seen[mID] {
			return
		}
		seen[mID] = true
		result = append(result, map[string]any{
			"id":   mID,
			"name": mID,
		})
	}

	// 1. Try Live fetch if baseUrl is available
	if baseUrl != "" {
		client := &http.Client{Timeout: 10 * time.Second}
		url := strings.TrimRight(baseUrl, "/") + "/models"
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
		if err == nil {
			if apiKey != "" {
				req.Header.Set("Authorization", "Bearer "+apiKey)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode < 400 {
					var respObj struct {
						Data   []map[string]any `json:"data"`
						Models []map[string]any `json:"models"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&respObj); err == nil {
						modelsList := respObj.Data
						if len(modelsList) == 0 {
							modelsList = respObj.Models
						}
						for _, m := range modelsList {
							mID, _ := m["id"].(string)
							if mID == "" {
								mID, _ = m["name"].(string)
							}
							addModel(mID)
						}
					}
				}
			}
		}
	}

	// 2. Add catalog models for canonical and provider
	for _, catModel := range providers.OfficialProviderModels[canonical] {
		addModel(catModel)
	}
	if canonical != provider {
		for _, catModel := range providers.OfficialProviderModels[provider] {
			addModel(catModel)
		}
	}

	// 3. Add custom models from database
	if customList, err := h.repo.GetCustomModelsByProvider(provider); err == nil {
		for _, cm := range customList {
			addModel(cm)
		}
	}
	if canonical != provider {
		if customList, err := h.repo.GetCustomModelsByProvider(canonical); err == nil {
			for _, cm := range customList {
				addModel(cm)
			}
		}
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"provider": provider,
		"models":   result,
	})
}

// ProviderTestResult represents the outcome of an upstream model ping test.
type ProviderTestResult struct {
	Status     string `json:"status"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	LatencyMs  int64  `json:"latencyMs"`
	StatusCode int    `json:"statusCode,omitempty"`
	Reply      string `json:"reply,omitempty"`
	Message    string `json:"message"`
	Error      string `json:"error,omitempty"`
}

func (h *AdminHandler) testProviderModel(ctx context.Context, id, model, prompt string) (*ProviderTestResult, error) {
	var conn *models.ProviderConnection
	conn, _ = h.repo.GetProviderConnectionByID(id)
	if conn == nil {
		if conns, err := h.repo.GetProviderConnections(id, true); err == nil && len(conns) > 0 {
			conn = conns[0]
		}
	}

	canonicalLookup := strings.ToLower(id)
	if mapped, ok := providers.ProviderAliasMap[canonicalLookup]; ok {
		canonicalLookup = mapped
	}
	if canonicalLookup == "google" {
		canonicalLookup = "gemini"
	}
	if conn == nil && canonicalLookup != id {
		if conns, err := h.repo.GetProviderConnections(canonicalLookup, true); err == nil && len(conns) > 0 {
			conn = conns[0]
		}
	}

	provider := id
	var pinnedConnID string
	if conn != nil {
		provider = conn.Provider
		pinnedConnID = conn.ID
	}

	if h.chatTester != nil {
		res, err := h.chatTester.TestProviderModel(ctx, provider, pinnedConnID, model, prompt)
		if err == nil && res != nil {
			return &ProviderTestResult{
				Status:     res.Status,
				Provider:   res.Provider,
				Model:      res.Model,
				LatencyMs:  res.LatencyMs,
				StatusCode: res.StatusCode,
				Reply:      res.Reply,
				Message:    res.Message,
				Error:      res.Error,
			}, nil
		}
	}

	canonical := strings.ToLower(provider)
	if mapped, ok := providers.ProviderAliasMap[canonical]; ok {
		canonical = mapped
	}
	if canonical == "google" {
		canonical = "gemini"
	}

	var connData map[string]any
	if conn != nil && conn.Data != "" {
		_ = json.Unmarshal([]byte(conn.Data), &connData)
	}
	if connData == nil {
		connData = make(map[string]any)
	}

	apiKey, _ := connData["apiKey"].(string)
	baseUrl, _ := connData["baseUrl"].(string)
	proxyPoolId, _ := connData["proxyPoolId"].(string)

	if baseUrl == "" {
		if node, nodeData, err := h.repo.GetProviderNodeByID(provider); err == nil && node != nil && nodeData != nil && nodeData.BaseURL != "" {
			baseUrl = nodeData.BaseURL
		}
	}
	if baseUrl == "" {
		if node, nodeData, err := h.repo.GetProviderNodeByID(id); err == nil && node != nil && nodeData != nil && nodeData.BaseURL != "" {
			baseUrl = nodeData.BaseURL
		}
	}

	var cfg providers.ProviderConfig
	if known, ok := providers.KnownProviders[canonical]; ok {
		cfg = known
		if baseUrl != "" {
			cfg.BaseURL = baseUrl
		}
		if apiKey == "" && (known.NoAuth || known.DefaultAPIKey != "") {
			apiKey = known.DefaultAPIKey
			if apiKey == "" {
				apiKey = "public"
			}
		}
	} else if baseUrl != "" {
		cfg = providers.ProviderConfig{
			BaseURL:    baseUrl,
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
		}
	} else {
		return &ProviderTestResult{
			Status:   "error",
			Provider: provider,
			Model:    model,
			Error:    fmt.Sprintf("no endpoint configuration found for provider '%s'", provider),
			Message:  fmt.Sprintf("Provider '%s' has no known upstream endpoint", provider),
		}, nil
	}

	if !strings.HasSuffix(cfg.BaseURL, "/chat/completions") && !strings.HasSuffix(cfg.BaseURL, "/messages") {
		cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyPoolId != "" && proxyPoolId != "__none__" {
		if pool, err := h.repo.GetProxyPool(proxyPoolId); err == nil && pool != nil && len(pool.URLs) > 0 {
			if parsedProxy, err := url.Parse(pool.URLs[0]); err == nil {
				transport.Proxy = http.ProxyURL(parsedProxy)
			}
		}
	}

	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}

	reqPayload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 16,
		"stream":     false,
	}
	bodyBytes, _ := json.Marshal(reqPayload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return &ProviderTestResult{
			Status:   "error",
			Provider: provider,
			Model:    model,
			Error:    err.Error(),
			Message:  "Failed to create request: " + err.Error(),
		}, nil
	}

	req.Header.Set("Content-Type", "application/json")
	if !cfg.NoAuth && apiKey != "" {
		switch cfg.AuthScheme {
		case "raw":
			req.Header.Set(cfg.AuthHeader, apiKey)
		default:
			header := cfg.AuthHeader
			if header == "" {
				header = "Authorization"
			}
			req.Header.Set(header, "Bearer "+apiKey)
		}
	}
	if canonical == "anthropic" {
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	for k, v := range cfg.StaticHeaders {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := client.Do(req)
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		return &ProviderTestResult{
			Status:    "error",
			Provider:  provider,
			Model:     model,
			LatencyMs: latencyMs,
			Error:     err.Error(),
			Message:   fmt.Sprintf("Connection failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		return &ProviderTestResult{
			Status:     "error",
			Provider:   provider,
			Model:      model,
			StatusCode: resp.StatusCode,
			LatencyMs:  latencyMs,
			Error:      errMsg,
			Message:    errMsg,
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
	_ = json.Unmarshal(respBody, &respObj)
	reply := ""
	if len(respObj.Choices) > 0 {
		reply = strings.TrimSpace(respObj.Choices[0].Message.Content)
	} else if len(respObj.Content) > 0 {
		reply = strings.TrimSpace(respObj.Content[0].Text)
	}
	if reply == "" {
		reply = "OK"
	}

	return &ProviderTestResult{
		Status:     "ok",
		Provider:   provider,
		Model:      model,
		StatusCode: resp.StatusCode,
		LatencyMs:  latencyMs,
		Reply:      reply,
		Message:    fmt.Sprintf("Model responded in %dms", latencyMs),
	}, nil
}

// HandleTestProviderModel handles POST /api/providers/{id}/test-model.
func (h *AdminHandler) HandleTestProviderModel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "provider or connection id is required")
		return
	}

	var reqBody struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	reqBody.Model = strings.TrimSpace(reqBody.Model)
	if reqBody.Model == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "model is required")
		return
	}
	prompt := strings.TrimSpace(reqBody.Prompt)
	if prompt == "" {
		prompt = "hi"
	}

	res, _ := h.testProviderModel(r.Context(), id, reqBody.Model, prompt)
	handlerutil.WriteJSON(w, http.StatusOK, res)
}

func (h *AdminHandler) HandleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.repo.DeleteProviderConnection(id); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ==========================================
// Combos Handlers
// ==========================================

func (h *AdminHandler) HandleGetCombos(w http.ResponseWriter, r *http.Request) {
	combos, err := h.repo.GetCombos()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if combos == nil {
		combos = []*models.Combo{}
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"combos": combos,
	})
}

func (h *AdminHandler) HandleCreateCombo(w http.ResponseWriter, r *http.Request) {
	var combo models.Combo
	if err := json.NewDecoder(r.Body).Decode(&combo); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if combo.ID == "" {
		bytes := make([]byte, 8)
		rand.Read(bytes)
		combo.ID = "combo-" + hex.EncodeToString(bytes)
	}
	if combo.Strategy == "" {
		combo.Strategy = "fallback"
	}
	if err := h.validateComboAliases(combo.Models); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repo.CreateCombo(&combo); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusCreated, combo)
}

func (h *AdminHandler) HandleUpdateCombo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var combo models.Combo
	if err := json.NewDecoder(r.Body).Decode(&combo); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	combo.ID = id
	if err := h.validateComboAliases(combo.Models); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repo.UpdateCombo(&combo); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, combo)
}

func (h *AdminHandler) validateComboAliases(raw string) error {
	var members []string
	if err := json.Unmarshal([]byte(raw), &members); err != nil || len(members) == 0 {
		return fmt.Errorf("combo must contain a non-empty JSON array of published model aliases")
	}
	for _, member := range members {
		member = strings.TrimSpace(member)
		if member == "" || strings.Contains(member, "/") {
			return fmt.Errorf("combo members must be bare published model aliases")
		}
		target, err := h.repo.GetModelAlias(member)
		if err != nil || strings.TrimSpace(target) == "" {
			return fmt.Errorf("combo member %q is not a published model alias", member)
		}
	}
	return nil
}

func (h *AdminHandler) HandleDeleteCombo(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.repo.DeleteCombo(id); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ==========================================
// Settings Handlers
// ==========================================

func (h *AdminHandler) HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.repo.GetSettings()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, settings)
}

func (h *AdminHandler) HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	b, _ := json.Marshal(body)
	if err := h.repo.SaveSettings(&models.Setting{ID: 1, Data: string(b)}); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ==========================================
// Proxy Pools Handlers
// ==========================================

func (h *AdminHandler) HandleGetProxyPools(w http.ResponseWriter, r *http.Request) {
	pools, err := h.repo.GetProxyPools()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var out []map[string]any
	for _, p := range pools {
		item := map[string]any{
			"id":         p.ID,
			"isActive":   p.IsActive == 1,
			"testStatus": p.TestStatus,
			"createdAt":  p.CreatedAt,
			"updatedAt":  p.UpdatedAt,
		}
		var extra map[string]any
		if json.Unmarshal([]byte(p.Data), &extra) == nil {
			for k, v := range extra {
				item[k] = v
			}
		}
		if item["name"] == nil || item["name"] == "" {
			item["name"] = p.ID
		}
		out = append(out, item)
	}
	if out == nil {
		out = []map[string]any{}
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"proxyPools": out,
	})
}

func (h *AdminHandler) HandleCreateProxyPool(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id, _ := body["id"].(string)
	if id == "" {
		bytes := make([]byte, 8)
		rand.Read(bytes)
		id = "pool-" + hex.EncodeToString(bytes)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	dataBytes, _ := json.Marshal(body)
	testStatus := "unknown"
	pool := models.ProxyPool{
		ID:         id,
		IsActive:   1,
		TestStatus: &testStatus,
		Data:       string(dataBytes),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := h.repo.CreateProxyPool(&pool); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusCreated, pool)
}

func (h *AdminHandler) HandleDeleteProxyPool(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.repo.DeleteProxyPool(id); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// HandleTestProxyPool tests a proxy pool (HTTP/SOCKS5 proxy or Vercel/Cloudflare/Deno relay).
// POST /api/proxy-pools/{id}/test
func (h *AdminHandler) HandleTestProxyPool(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "proxy pool id is required")
		return
	}

	var pool *models.ProxyPool
	pools, err := h.repo.GetProxyPools()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, p := range pools {
		if p.ID == id {
			pool = p
			break
		}
	}
	if pool == nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "proxy pool not found")
		return
	}

	var extra map[string]any
	if json.Unmarshal([]byte(pool.Data), &extra) == nil && extra != nil {
		// parsed
	} else {
		extra = make(map[string]any)
	}

	proxyUrl, _ := extra["proxyUrl"].(string)
	if proxyUrl == "" {
		proxyUrl, _ = extra["url"].(string)
	}
	pType, _ := extra["type"].(string)
	if pType == "" {
		pType = "http"
	}

	client := &http.Client{Timeout: 10 * time.Second}
	start := time.Now()
	var testOk bool
	var testStatusStr string
	var errStr string
	var httpStatus int

	if pType == "vercel" || pType == "cloudflare" || pType == "deno" {
		// Test Serverless Relay
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, proxyUrl, nil)
		if err != nil {
			errStr = err.Error()
		} else {
			req.Header.Set("x-relay-target", "https://httpbin.org")
			req.Header.Set("x-relay-path", "/get")
			resp, doErr := client.Do(req)
			if doErr != nil {
				errStr = doErr.Error()
			} else {
				defer resp.Body.Close()
				httpStatus = resp.StatusCode
				testOk = resp.StatusCode < 400
				if !testOk {
					errStr = fmt.Sprintf("Relay returned HTTP %d", resp.StatusCode)
				}
			}
		}
	} else {
		// Test standard HTTP / SOCKS proxy
		if proxyUrl != "" {
			if proxyURLParsed, err := url.Parse(proxyUrl); err == nil {
				transport := &http.Transport{
					Proxy: http.ProxyURL(proxyURLParsed),
				}
				proxyClient := &http.Client{
					Transport: transport,
					Timeout:   10 * time.Second,
				}
				req, err := http.NewRequestWithContext(r.Context(), http.MethodHead, "https://google.com", nil)
				if err != nil {
					errStr = err.Error()
				} else {
					resp, doErr := proxyClient.Do(req)
					if doErr != nil {
						errStr = doErr.Error()
					} else {
						defer resp.Body.Close()
						httpStatus = resp.StatusCode
						testOk = resp.StatusCode < 400
					}
				}
			} else {
				errStr = "Invalid proxy URL: " + err.Error()
			}
		} else {
			errStr = "No proxy URL configured"
		}
	}

	elapsedMs := time.Since(start).Milliseconds()
	now := time.Now().UTC().Format(time.RFC3339)

	if testOk {
		testStatusStr = "active"
		extra["lastError"] = nil
	} else {
		testStatusStr = "error"
		if errStr == "" {
			errStr = "Proxy connection test failed"
		}
		extra["lastError"] = errStr
	}
	extra["lastTestedAt"] = now

	// Update record in SQLite
	dataBytes, _ := json.Marshal(extra)
	pool.Data = string(dataBytes)
	pool.TestStatus = &testStatusStr
	if testOk {
		pool.IsActive = 1
	}
	pool.UpdatedAt = now

	h.repo.RawDB().Exec(
		`UPDATE proxyPools SET isActive = ?, testStatus = ?, data = ?, updatedAt = ? WHERE id = ?`,
		pool.IsActive, testStatusStr, pool.Data, pool.UpdatedAt, pool.ID,
	)

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":         testOk,
		"status":     httpStatus,
		"error":      errStr,
		"elapsedMs":  elapsedMs,
		"testedAt":   now,
		"testStatus": testStatusStr,
	})
}

// ==========================================
// Model Aliases Handlers
// ==========================================

func (h *AdminHandler) HandleGetModelAliases(w http.ResponseWriter, r *http.Request) {
	aliases, err := h.repo.GetModelAliases()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	records, recordsErr := h.repo.GetModelAliasRecords()
	if recordsErr != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, recordsErr.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"aliases": aliases, "records": records})
}

func (h *AdminHandler) HandleSetModelAlias(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Alias         string   `json:"alias"`
		Target        string   `json:"target"`
		Provider      string   `json:"provider"`
		UpstreamModel string   `json:"upstreamModel"`
		ConnectionID  *string  `json:"connectionId,omitempty"`
		Capabilities  []string `json:"capabilities,omitempty"`
		IsActive      *int     `json:"isActive,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Alias) == "" {
		body.Alias = chi.URLParam(r, "alias")
	}
	body.Alias = strings.TrimSpace(body.Alias)
	body.Target = strings.TrimSpace(body.Target)
	if body.Alias == "" || strings.Contains(body.Alias, "/") {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "alias must be a non-empty bare client model ID")
		return
	}
	var conflictingAlias string
	err := h.repo.RawDB().QueryRow(`
		SELECT alias FROM (
			SELECT alias FROM modelAliases
			UNION ALL
			SELECT key AS alias FROM kv WHERE scope = 'modelAliases'
		) WHERE lower(alias) = lower(?) LIMIT 1`, body.Alias).Scan(&conflictingAlias)
	if err != nil && err != sql.ErrNoRows {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to validate model alias uniqueness")
		return
	}
	if err == nil && conflictingAlias != body.Alias {
		handlerutil.WriteJSONError(w, http.StatusConflict, "model_alias_conflict: alias IDs are case-insensitive")
		return
	}
	if strings.HasPrefix(strings.ToLower(body.Target), "combo:") {
		if strings.TrimSpace(body.Target[len("combo:"):]) == "" {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "combo alias target must name a combo")
			return
		}
	} else if parts := strings.SplitN(body.Target, "/", 2); len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "target must contain exactly one provider mapping and an upstream model ID")
		return
	}
	if r.Method == http.MethodPost {
		if existing, err := h.repo.GetModelAlias(body.Alias); err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		} else if existing != "" {
			handlerutil.WriteJSONError(w, http.StatusConflict, "model_alias_conflict: alias already exists; use PUT to update it")
			return
		}
	}
	var setErr error
	if strings.TrimSpace(body.Provider) != "" || strings.TrimSpace(body.UpstreamModel) != "" {
		active := 1
		if body.IsActive != nil {
			active = *body.IsActive
		}
		setErr = h.repo.SetModelAliasRecord(body.Alias, body.Provider, body.UpstreamModel, body.ConnectionID, body.Capabilities, active)
	} else {
		setErr = h.repo.SetModelAlias(body.Alias, body.Target)
	}
	if err := setErr; err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AdminHandler) HandleDeleteModelAlias(w http.ResponseWriter, r *http.Request) {
	alias := chi.URLParam(r, "alias")
	if err := h.repo.DeleteModelAlias(alias); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AdminHandler) HandleTestModelAlias(w http.ResponseWriter, r *http.Request) {
	alias := chi.URLParam(r, "alias")
	if strings.TrimSpace(alias) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing alias parameter")
		return
	}
	target, err := h.repo.GetModelAlias(alias)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if strings.TrimSpace(target) == "" {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "model alias not found: "+alias)
		return
	}
	rec, _ := h.repo.GetModelAliasRecord(alias)
	provider := ""
	upstreamModel := ""
	if rec != nil {
		provider = rec.Provider
		upstreamModel = rec.UpstreamModel
	} else if parts := strings.SplitN(target, "/", 2); len(parts) == 2 {
		provider = parts[0]
		upstreamModel = parts[1]
	} else if strings.HasPrefix(strings.ToLower(target), "combo:") {
		provider = "__combo__"
		upstreamModel = strings.TrimPrefix(target, "combo:")
	}

	if r.URL.Query().Get("live") == "true" {
		if provider != "" && upstreamModel != "" && provider != "__combo__" {
			testRes, _ := h.testProviderModel(r.Context(), provider, upstreamModel, "ping")
			if testRes != nil {
				handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
					"status":        testRes.Status,
					"alias":         alias,
					"target":        target,
					"provider":      provider,
					"upstreamModel": upstreamModel,
					"resolved":      true,
					"latencyMs":     testRes.LatencyMs,
					"statusCode":    testRes.StatusCode,
					"reply":         testRes.Reply,
					"error":         testRes.Error,
					"message":       testRes.Message,
				})
				return
			}
		}
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"alias":         alias,
		"target":        target,
		"provider":      provider,
		"upstreamModel": upstreamModel,
		"resolved":      true,
		"message":       fmt.Sprintf("Alias '%s' resolves to %s / %s", alias, provider, upstreamModel),
	})
}

// HandlePreviewModelPolicy evaluates a draft API-key policy against published
// aliases without creating or mutating a key.
func (h *AdminHandler) HandlePreviewModelPolicy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountTypeID string                 `json:"accountTypeId,omitempty"`
		Models        []string               `json:"models"`
		Restrictions  models.KeyRestrictions `json:"restrictions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.Models) == 0 {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "models must contain at least one published alias")
		return
	}
	restrictionsJSON, _ := json.Marshal(body.Restrictions)
	restrictionsText := string(restrictionsJSON)
	typeID := strings.TrimSpace(body.AccountTypeID)
	key := &models.APIKey{IsActive: 1, Restrictions: &restrictionsText, AccountTypeID: &typeID}
	results := make([]map[string]any, 0, len(body.Models))
	for _, alias := range body.Models {
		alias = strings.TrimSpace(alias)
		result := map[string]any{"alias": alias, "allowed": false}
		if alias == "" || strings.Contains(alias, "/") {
			result["reason"] = "provider_prefix_forbidden_or_invalid_alias"
			results = append(results, result)
			continue
		}
		target, err := h.repo.GetModelAlias(alias)
		if err != nil || target == "" {
			result["reason"] = "model_alias_required"
			results = append(results, result)
			continue
		}
		provider := target
		if parts := strings.SplitN(target, "/", 2); len(parts) == 2 {
			provider = parts[0]
		}
		if typeID != "" && typeID != "administrator" {
			allowed, err := h.repo.IsAliasAllowedForAccountType(typeID, alias)
			if err != nil || !allowed {
				result["reason"] = "unauthorized_model_for_tier"
				results = append(results, result)
				continue
			}
		}
		if err := auth.ValidateKeyPolicy(key, alias, provider); err != nil {
			result["reason"] = err.Error()
		} else {
			result["allowed"] = true
			result["reason"] = "allowed"
		}
		results = append(results, result)
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}

// HandleSecuritySummary returns aggregate security state without returning
// secrets, hashes, credentials, or raw log payloads.
func (h *AdminHandler) HandleSecuritySummary(w http.ResponseWriter, r *http.Request) {
	queries := map[string]string{
		"totalKeys":              `SELECT COUNT(*) FROM apiKeys`,
		"hashedKeys":             `SELECT COUNT(*) FROM apiKeys WHERE keyHash IS NOT NULL AND trim(keyHash)<>''`,
		"legacyGatewayPlaintext": `SELECT COUNT(*) FROM apiKeys WHERE userId IS NULL AND (clientId IS NULL OR trim(clientId)='') AND keyHash IS NULL AND key IS NOT NULL`,
		"legacyGatewayHashed":    `SELECT COUNT(*) FROM apiKeys WHERE userId IS NULL AND (clientId IS NULL OR trim(clientId)='') AND keyHash IS NOT NULL`,
		"clientKeys":             `SELECT COUNT(*) FROM apiKeys WHERE clientId IS NOT NULL AND trim(clientId)<>''`,
		"userKeys":               `SELECT COUNT(*) FROM apiKeys WHERE userId IS NOT NULL`,
		"inactiveKeys":           `SELECT COUNT(*) FROM apiKeys WHERE isActive=0`,
	}
	result := make(map[string]any, len(queries)+1)
	for name, query := range queries {
		var count int
		if err := h.repo.RawDB().QueryRow(query).Scan(&count); err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to load security summary")
			return
		}
		result[name] = count
	}
	var marker string
	if err := h.repo.RawDB().QueryRow(`SELECT COALESCE(value,'') FROM _meta WHERE key = 'migration.api_keys_hash.v1'`).Scan(&marker); err != nil {
		marker = ""
	}
	result["migrationMarker"] = marker
	handlerutil.WriteJSON(w, http.StatusOK, result)
}

// ==========================================
// Custom Models Handlers
// ==========================================

func (h *AdminHandler) HandleGetCustomModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider != "" {
		models, err := h.repo.GetCustomModelsByProvider(provider)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"customModels": models})
		return
	}

	entries, err := h.repo.GetCustomModels()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"customModels": entries})
}

func (h *AdminHandler) HandleAddCustomModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider      string `json:"provider"`
		ProviderAlias string `json:"providerAlias"`
		ID            string `json:"id"`
		Model         string `json:"model"`
		Type          string `json:"type"`
		Kind          string `json:"kind"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	provider := body.Provider
	if provider == "" {
		provider = body.ProviderAlias
	}
	modelID := body.ID
	if modelID == "" {
		modelID = body.Model
	}
	if modelID == "" {
		modelID = body.Name
	}
	mType := body.Type
	if mType == "" {
		mType = body.Kind
	}
	if mType == "" {
		mType = "llm"
	}
	name := body.Name
	if name == "" {
		name = modelID
	}

	if provider == "" || modelID == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "provider/providerAlias and id/model are required")
		return
	}
	exists, err := h.repo.CustomModelExists(provider, modelID, mType)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if exists {
		handlerutil.WriteJSONError(w, http.StatusConflict, "model id sudah ada pada provider ini; gunakan model id yang berbeda")
		return
	}
	if err := h.repo.AddCustomModel(provider, modelID, mType, name); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":        "created",
		"id":            modelID,
		"providerAlias": provider,
		"type":          mType,
		"name":          name,
	})
}

func (h *AdminHandler) HandleDeleteCustomModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider      string `json:"provider"`
		ProviderAlias string `json:"providerAlias"`
		ID            string `json:"id"`
		Model         string `json:"model"`
		Type          string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
		provider := body.Provider
		if provider == "" {
			provider = body.ProviderAlias
		}
		modelID := body.ID
		if modelID == "" {
			modelID = body.Model
		}
		if provider != "" && modelID != "" {
			mType := body.Type
			if mType == "" {
				mType = "llm"
			}
			if err := h.repo.DeleteCustomModel(provider, modelID, mType); err != nil {
				handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
	}

	// Query params support (e.g. ?providerAlias=...&id=...&type=...)
	qProv := r.URL.Query().Get("providerAlias")
	if qProv == "" {
		qProv = r.URL.Query().Get("provider")
	}
	qID := r.URL.Query().Get("id")
	if qID == "" {
		qID = r.URL.Query().Get("model")
	}
	if qProv != "" && qID != "" {
		mType := r.URL.Query().Get("type")
		if mType == "" {
			mType = "llm"
		}
		_ = h.repo.DeleteCustomModel(qProv, qID, mType)
		handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	key := r.URL.Query().Get("key")
	if key == "" {
		key = chi.URLParam(r, "key")
	}
	if key != "" {
		parts := strings.Split(key, "|")
		if len(parts) >= 2 {
			mType := "llm"
			if len(parts) >= 3 {
				mType = parts[2]
			}
			_ = h.repo.DeleteCustomModel(parts[0], parts[1], mType)
			handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
	}

	handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing provider and id")
}

// ==========================================
// Provider Nodes (OpenAI & Anthropic Compatible)
// ==========================================

func (h *AdminHandler) HandleGetProviderNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.repo.GetProviderNodes()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var out []map[string]any
	for _, n := range nodes {
		item := map[string]any{
			"id":        n.ID,
			"type":      n.Type,
			"name":      n.Name,
			"createdAt": n.CreatedAt,
			"updatedAt": n.UpdatedAt,
		}
		var data map[string]any
		if json.Unmarshal([]byte(n.Data), &data) == nil {
			for k, v := range data {
				item[k] = v
			}
		}
		out = append(out, item)
	}
	if out == nil {
		out = []map[string]any{}
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"nodes": out})
}

func (h *AdminHandler) HandleCreateProviderNode(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id, _ := body["id"].(string)
	nodeType, _ := body["type"].(string)
	name, _ := body["name"].(string)
	if nodeType == "" {
		nodeType = "openai-compatible"
	}
	if name == "" {
		name = "Custom Node"
	}
	if id == "" {
		prefix, _ := body["prefix"].(string)
		if prefix == "" {
			prefix = "custom"
		}
		id = fmt.Sprintf("%s-%s-%d", nodeType, prefix, time.Now().UnixNano()%100000)
	}

	data := make(map[string]any)
	for k, v := range body {
		if k != "id" && k != "type" && k != "name" {
			data[k] = v
		}
	}

	node, err := h.repo.CreateProviderNode(id, nodeType, name, data)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusCreated, node)
}

func (h *AdminHandler) HandleUpdateProviderNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing node id")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name, _ := body["name"].(string)
	data := make(map[string]any)
	for k, v := range body {
		if k != "id" && k != "type" && k != "name" {
			data[k] = v
		}
	}
	if err := h.repo.UpdateProviderNode(id, name, data); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AdminHandler) HandleDeleteProviderNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing node id")
		return
	}
	if err := h.repo.DeleteProviderNode(id); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AdminHandler) HandleValidateProviderNode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BaseURL string `json:"baseUrl"`
		APIKey  string `json:"apiKey"`
		Type    string `json:"type"`
		ModelID string `json:"modelId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.BaseURL == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "baseUrl is required")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	normalizedBase := strings.TrimRight(strings.TrimSpace(body.BaseURL), "/")

	// 1. Anthropic Compatible Validation
	if body.Type == "anthropic-compatible" {
		if strings.HasSuffix(normalizedBase, "/messages") {
			normalizedBase = strings.TrimSuffix(normalizedBase, "/messages")
		}
		modelsUrl := normalizedBase + "/models"
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, modelsUrl, nil)
		if err != nil {
			handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
			return
		}
		if body.APIKey != "" {
			req.Header.Set("x-api-key", body.APIKey)
			req.Header.Set("anthropic-version", "2023-06-01")
			req.Header.Set("Authorization", "Bearer "+body.APIKey)
		}
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode < 400 {
				handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": true, "status": resp.StatusCode})
				return
			}
			if resp.StatusCode == 401 || resp.StatusCode == 403 {
				handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": false, "error": "API key unauthorized (HTTP " + fmt.Sprint(resp.StatusCode) + ")"})
				return
			}
		}

		// Fallback: test inference via /messages or /chat/completions
		testModel := body.ModelID
		if testModel == "" {
			testModel = "claude-3-5-sonnet-20241022"
		}
		msgBody := map[string]any{
			"model":      testModel,
			"max_tokens": 1,
			"messages":   []map[string]string{{"role": "user", "content": "ping"}},
		}
		b, _ := json.Marshal(msgBody)
		reqPost, err := http.NewRequestWithContext(r.Context(), http.MethodPost, normalizedBase+"/messages", bytes.NewReader(b))
		if err == nil {
			reqPost.Header.Set("Content-Type", "application/json")
			if body.APIKey != "" {
				reqPost.Header.Set("x-api-key", body.APIKey)
				reqPost.Header.Set("anthropic-version", "2023-06-01")
				reqPost.Header.Set("Authorization", "Bearer "+body.APIKey)
			}
			respPost, errPost := client.Do(reqPost)
			if errPost == nil {
				defer respPost.Body.Close()
				if respPost.StatusCode < 400 {
					handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": true, "status": respPost.StatusCode, "method": "messages"})
					return
				}
				handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": false, "error": fmt.Sprintf("Inference check returned HTTP %d", respPost.StatusCode)})
				return
			}
		}

		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": false, "error": "Could not connect to Anthropic compatible endpoint"})
		return
	}

	// 2. OpenAI Compatible Validation (Default)
	modelsUrl := normalizedBase + "/models"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, modelsUrl, nil)
	if err != nil {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	if body.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+body.APIKey)
	}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode < 400 {
			handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": true, "status": resp.StatusCode})
			return
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": false, "error": "API key unauthorized (HTTP " + fmt.Sprint(resp.StatusCode) + ")"})
			return
		}
	}

	// Fallback: test inference via /chat/completions
	testModel := body.ModelID
	if testModel == "" {
		testModel = "gpt-4o-mini"
	}
	chatBody := map[string]any{
		"model":      testModel,
		"max_tokens": 1,
		"messages":   []map[string]string{{"role": "user", "content": "ping"}},
	}
	b, _ := json.Marshal(chatBody)
	reqPost, err := http.NewRequestWithContext(r.Context(), http.MethodPost, normalizedBase+"/chat/completions", bytes.NewReader(b))
	if err == nil {
		reqPost.Header.Set("Content-Type", "application/json")
		if body.APIKey != "" {
			reqPost.Header.Set("Authorization", "Bearer "+body.APIKey)
		}
		respPost, errPost := client.Do(reqPost)
		if errPost == nil {
			defer respPost.Body.Close()
			if respPost.StatusCode < 400 {
				handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": true, "status": respPost.StatusCode, "method": "chat"})
				return
			}
			handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": false, "error": fmt.Sprintf("Chat check returned HTTP %d", respPost.StatusCode)})
			return
		}
	}

	if err != nil {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"valid": false, "error": "Endpoint unreachable or invalid response"})
}

// ==========================================
// Provider Prefixes Handlers
// ==========================================

func (h *AdminHandler) HandleGetProviderPrefixes(w http.ResponseWriter, r *http.Request) {
	prefixes, err := h.repo.GetProviderPrefixes()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"prefixes": prefixes})
}

func (h *AdminHandler) HandleSetProviderPrefix(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string `json:"provider"`
		Prefix   string `json:"prefix"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Provider == "" || body.Prefix == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "provider and prefix are required")
		return
	}
	if err := h.repo.SetProviderPrefix(body.Provider, body.Prefix); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "provider": body.Provider, "prefix": body.Prefix})
}

func (h *AdminHandler) HandleDeleteProviderPrefix(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		var body struct {
			Provider string `json:"provider"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Provider != "" {
			provider = body.Provider
		}
	}
	if provider == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing provider")
		return
	}
	if err := h.repo.DeleteProviderPrefix(provider); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ==========================================
// Full Database Backup & Migration Handlers
// ==========================================

func (h *AdminHandler) HandleExportDatabase(w http.ResponseWriter, r *http.Request) {
	backup, err := h.repo.ExportDB()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to export database: "+err.Error())
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="zyrouter-backup-%s.json"`, time.Now().Format("2006-01-02")))
	handlerutil.WriteJSON(w, http.StatusOK, backup)
}

func (h *AdminHandler) HandleImportDatabase(w http.ResponseWriter, r *http.Request) {
	var backup db.DatabaseBackup
	if err := json.NewDecoder(r.Body).Decode(&backup); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid backup JSON payload: "+err.Error())
		return
	}
	if err := h.repo.ImportDB(&backup); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to restore database: "+err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Database restored successfully"})
}

// ==========================================
// Audit Logs REST Endpoints
// ==========================================

// HandleListAuditFiles returns the list of historical and active audit log files.
// GET /api/audit-logs/files
func (h *AdminHandler) HandleListAuditFiles(w http.ResponseWriter, r *http.Request) {
	files, err := auditlog.Get().ListLogFiles()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"files": files,
	})
}

// HandleDownloadAuditFile serves a specific audit log file for download or inspection.
// GET /api/audit-logs/files/{filename}
func (h *AdminHandler) HandleDownloadAuditFile(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	filePath, err := auditlog.Get().GetLogFilePath(filename)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "audit log file not found")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	http.ServeFile(w, r, filePath)
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
