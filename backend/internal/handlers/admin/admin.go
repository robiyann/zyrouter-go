package admin

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"zyrouter/backend/internal/auditlog"
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/models"
)

type AdminHandler struct {
	repo *db.Repo
}

func NewAdminHandler(repo *db.Repo) *AdminHandler {
	return &AdminHandler{repo: repo}
}

// GenerateRandomKey generates a secure random API key starting with "sk-zy-".
func GenerateRandomKey() string {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "sk-zy-" + hex.EncodeToString([]byte("fallback-random-key-12345"))
	}
	return "sk-zy-" + hex.EncodeToString(bytes)
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
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"key": key.Key})
}

func maskAPIKey(value string) string {
	if len(value) <= 10 {
		return "***"
	}
	return value[:7] + "..." + value[len(value)-4:]
}

func (h *AdminHandler) HandleCreateKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name         string                  `json:"name"`
		Key          string                  `json:"key,omitempty"`
		MachineID    string                  `json:"machineId,omitempty"`
		Restrictions *models.KeyRestrictions `json:"restrictions,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	keyStr := strings.TrimSpace(body.Key)
	if keyStr == "" {
		keyStr = GenerateRandomKey()
	}

	id := "key-" + hex.EncodeToString([]byte(keyStr[:min(len(keyStr), 8)])) + "-" + hex.EncodeToString([]byte(body.Name[:min(len(body.Name), 4)]))
	if len(id) > 36 {
		id = id[:36]
	}

	var restrictionsStr *string
	if body.Restrictions != nil {
		b, _ := json.Marshal(body.Restrictions)
		s := string(b)
		restrictionsStr = &s
	}

	apiKey, err := h.repo.CreateApiKey(id, keyStr, body.Name, body.MachineID, restrictionsStr)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handlerutil.WriteJSON(w, http.StatusCreated, apiKey)
}

func (h *AdminHandler) HandleUpdateKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing key id")
		return
	}

	var body struct {
		Name         *string                 `json:"name,omitempty"`
		IsActive     *int                    `json:"isActive,omitempty"`
		Restrictions *models.KeyRestrictions `json:"restrictions,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var restrictionsStr *string
	if body.Restrictions != nil {
		b, _ := json.Marshal(body.Restrictions)
		s := string(b)
		restrictionsStr = &s
	}

	if err := h.repo.UpdateApiKey(id, body.Name, body.IsActive, restrictionsStr); err != nil {
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

// HandleFetchProviderConnectionModels fetches available models from the provider upstream endpoint.
func (h *AdminHandler) HandleFetchProviderConnectionModels(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "connection id or provider id is required")
		return
	}

	var conn *models.ProviderConnection
	// 1. Try finding connection by primary key ID
	conn, _ = h.repo.GetProviderConnectionByID(id)

	// 2. Fallback: Try finding by provider ID (e.g. "openai-compatible-...", "openai", "antigravity")
	if conn == nil {
		if conns, err := h.repo.GetProviderConnections(id, true); err == nil && len(conns) > 0 {
			conn = conns[0]
		}
	}

	var connData map[string]any
	if conn != nil {
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

	// If provider is a custom node, get baseUrl from node if empty
	if baseUrl == "" {
		if node, nodeData, err := h.repo.GetProviderNodeByID(provider); err == nil && node != nil && nodeData != nil {
			baseUrl = nodeData.BaseURL
		}
	}
	if baseUrl == "" {
		if node, nodeData, err := h.repo.GetProviderNodeByID(id); err == nil && node != nil && nodeData != nil {
			baseUrl = nodeData.BaseURL
		}
	}
	if baseUrl == "" {
		if node, nodeData, err := h.repo.GetProviderNodeByPrefix(id); err == nil && node != nil && nodeData != nil {
			baseUrl = nodeData.BaseURL
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	// 1. OpenAI / Compatible
	if baseUrl != "" {
		url := strings.TrimRight(baseUrl, "/") + "/models"
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to connect to upstream: "+err.Error())
			return
		}
		defer resp.Body.Close()

		var respObj struct {
			Data   []map[string]any `json:"data"`
			Models []map[string]any `json:"models"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&respObj)
		modelsList := respObj.Data
		if len(modelsList) == 0 {
			modelsList = respObj.Models
		}

		var result []map[string]any
		for _, m := range modelsList {
			mID, _ := m["id"].(string)
			if mID == "" {
				mID, _ = m["name"].(string)
			}
			if mID != "" {
				result = append(result, map[string]any{
					"id":   mID,
					"name": mID,
				})
			}
		}
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"provider": provider,
			"models":   result,
		})
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"provider": provider,
		"models":   []any{},
	})
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
	if err := h.repo.UpdateCombo(&combo); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, combo)
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
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"aliases": aliases,
	})
}

func (h *AdminHandler) HandleSetModelAlias(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Alias  string `json:"alias"`
		Target string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.repo.SetModelAlias(body.Alias, body.Target); err != nil {
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
