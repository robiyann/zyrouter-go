package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"zyrouter/backend/internal/constants"
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/labels"
	"zyrouter/backend/internal/log"
	"zyrouter/backend/internal/models"
	"zyrouter/backend/internal/providers"
	internalproxy "zyrouter/backend/internal/proxy"
)

// GetBestConnection retrieves the highest-priority active connection for a provider.
// When connectionID is non-empty, it fetches that specific connection directly.
func (h *ChatHandler) GetBestConnection(provider string, connectionID string, excludeIDs []string, model string) (*models.ProviderConnection, *ConnectionData, error) {
	return h.getBestConnection(provider, connectionID, excludeIDs, model)
}

func (h *ChatHandler) getBestConnection(provider string, connectionID string, excludeIDs []string, model string) (*models.ProviderConnection, *ConnectionData, error) {
	if !providers.IsProviderEnabled(provider) {
		return nil, nil, fmt.Errorf("provider %q is disabled by ENABLED_PROVIDERS", provider)
	}
	if model != "" && !h.Repo.IsProviderAvailable(provider, model) {
		log.Warn("health", "unhealthy provider", "provider", provider, "model", model)
	}

	var conn *models.ProviderConnection
	var err error

	if connectionID != "" {
		conn, err = h.Repo.GetProviderConnectionByID(connectionID)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch connection %s: %w", connectionID, err)
		}
		if conn == nil {
			return nil, nil, fmt.Errorf("connection %s not found", connectionID)
		}
	} else {
		connections, queryErr := h.Repo.GetProviderConnections(provider, true)
		if queryErr != nil {
			return nil, nil, fmt.Errorf("failed to query connections for %s: %w", provider, queryErr)
		}
		if len(connections) == 0 {
			if cfg, ok := providers.KnownProviders[provider]; ok && (cfg.NoAuth || cfg.DefaultAPIKey != "") {
				// Inject virtual connection for no-auth provider with optional proxy pool strategy from settings
				apiKey := cfg.DefaultAPIKey
				if apiKey == "" {
					apiKey = "public"
				}
				connData := &ConnectionData{
					APIKey:      apiKey,
					AccessToken: apiKey,
				}
				settings, err := h.Repo.GetSettings()
				if err == nil && settings != nil && settings.ProviderStrategies != nil {
					if strat, ok := settings.ProviderStrategies[provider]; ok {
						pickedID := strat.ProxyPoolID
						if strat.RotateStrategy != "" && strat.RotateStrategy != "none" {
							if pools, err := h.Repo.GetProxyPools(); err == nil && len(pools) > 0 {
								var activePoolIDs []string
								for _, p := range pools {
									if p.IsActive == 1 {
										activePoolIDs = append(activePoolIDs, p.ID)
									}
								}
								if len(activePoolIDs) > 0 {
									pickedID = h.pickRotatedProxyPool(provider, activePoolIDs, strat.RotateStrategy)
								}
							}
						}
						if pickedID != "" && pickedID != "__none__" {
							connData.ProxyPoolID = pickedID
						}
					}
				}
				publicName := "Public"
				conn := &models.ProviderConnection{
					ID:       "noauth",
					Provider: provider,
					Name:     &publicName,
					IsActive: 1,
				}
				h.applyProviderProxyStrategy(provider, connData)
				return conn, connData, nil
			}
			return nil, nil, fmt.Errorf("no active connections for provider: %s", provider)
		}

		excludeSet := make(map[string]bool, len(excludeIDs))
		for _, id := range excludeIDs {
			excludeSet[id] = true
		}

		var available []*models.ProviderConnection
		for _, c := range connections {
			if excludeSet[c.ID] {
				continue
			}
			// Skip connections that have an active per-connection model lock
			if model != "" {
				if locked, _ := h.Repo.IsConnectionModelLocked(c.ID, model); locked {
					continue
				}
			}
			available = append(available, c)
		}
		if len(available) == 0 {
			return nil, nil, fmt.Errorf("no available connections for provider: %s (all excluded)", provider)
		}

		// Check if provider has a specific routing/fallback strategy override (e.g. round-robin)
		if strat, ok := h.getProviderStrategy(provider); ok && strat.FallbackStrategy != nil && *strat.FallbackStrategy == "round-robin" && len(available) > 1 {
			limit := strat.StickyRoundRobinLimit
			if limit <= 0 {
				limit = 1
			}
			// Rotate connection selection across accounts
			var connIDs []string
			for _, a := range available {
				connIDs = append(connIDs, a.ID)
			}
			strategyName := "round-robin"
			if limit > 1 {
				strategyName = "sticky"
			}
			rotatedIDs := h.applyComboStrategy(strategyName, connIDs, "prov-acc-"+provider, limit, true)
			if len(rotatedIDs) > 0 {
				targetID := rotatedIDs[0]
				for _, a := range available {
					if a.ID == targetID {
						conn = a
						break
					}
				}
			}
		}

		if conn == nil {
			conn = available[0]
		}
	}

	var connData ConnectionData
	if conn.Data != "" {
		if err := json.Unmarshal([]byte(conn.Data), &connData); err != nil {
			return nil, nil, fmt.Errorf("failed to parse connection data: %w", err)
		}
	}
	// Provider-level proxy settings are inherited by normal authenticated
	// connections as well as the virtual no-auth connection. Previously this
	// inheritance only happened in the no-auth branch, so OpenCode connections
	// silently fell back to the default direct HTTP client.
	h.applyProviderProxyStrategy(provider, &connData)

	return conn, &connData, nil
}

func (h *ChatHandler) applyProviderProxyStrategy(provider string, connData *ConnectionData) {
	if h.Repo == nil || connData == nil {
		return
	}
	strategy, ok := h.getProviderStrategy(provider)
	if !ok {
		return
	}

	// A configured provider proxy is authoritative. Account-level assignments
	// are only used when the provider proxy mode is Direct (no proxy fields).
	if strategy.RotateStrategy != "" && strategy.RotateStrategy != "none" {
		pools, err := h.Repo.GetProxyPools()
		if err != nil {
			return
		}
		activeIDs := make([]string, 0, len(pools))
		for _, p := range pools {
			if p == nil || p.IsActive == 0 {
				continue
			}
			pool, poolErr := h.Repo.GetProxyPool(p.ID)
			if poolErr == nil && pool != nil && len(pool.URLs) > 0 {
				activeIDs = append(activeIDs, p.ID)
			}
		}
		connData.ProxyPoolID = h.pickRotatedProxyPool(provider, activeIDs, strategy.RotateStrategy)
		return
	}
	if strategy.ProxyPoolID != "" && strategy.ProxyPoolID != "__none__" {
		connData.ProxyPoolID = strategy.ProxyPoolID
	}
}

func (h *ChatHandler) getProviderStrategy(provider string) (db.ProviderStrategy, bool) {
	if h.Repo == nil {
		return db.ProviderStrategy{}, false
	}
	settings, err := h.Repo.GetSettings()
	if err != nil || settings == nil || settings.ProviderStrategies == nil {
		return db.ProviderStrategy{}, false
	}
	if strategy, ok := settings.ProviderStrategies[provider]; ok {
		return strategy, true
	}
	canonical := provider
	if mapped, exists := providers.ProviderAliasMap[strings.ToLower(provider)]; exists {
		canonical = mapped
	}
	strategy, ok := settings.ProviderStrategies[canonical]
	return strategy, ok
}

func (h *ChatHandler) displayProviderLabel(provider string) string {
	return labels.Provider(h.Repo, provider)
}

func (h *ChatHandler) displayModelLabel(provider, model string) string {
	return labels.Model(h.Repo, provider, model)
}

func (h *ChatHandler) displayAccountLabel(connectionID string) string {
	if connectionID == "" || connectionID == "noauth" || connectionID == "default" {
		return "Public"
	}
	if h.Repo != nil {
		if conn, err := h.Repo.GetProviderConnectionByID(connectionID); err == nil && conn != nil {
			if conn.Name != nil && strings.TrimSpace(*conn.Name) != "" {
				return strings.TrimSpace(*conn.Name)
			}
			if conn.Email != nil && strings.TrimSpace(*conn.Email) != "" {
				return strings.TrimSpace(*conn.Email)
			}
		}
	}
	if len(connectionID) > 8 {
		return "Account " + connectionID[:8] + "..."
	}
	return "Account " + connectionID
}

// GetProviderConfig returns the upstream configuration for a provider.
func (h *ChatHandler) GetProviderConfig(provider string, connData *ConnectionData) (*providers.ProviderConfig, error) {
	return h.getProviderConfig(provider, connData)
}

func (h *ChatHandler) getProviderConfig(provider string, connData *ConnectionData) (*providers.ProviderConfig, error) {
	var baseCfg *providers.ProviderConfig

	if connData != nil && connData.BaseURL != "" {
		baseCfg = &providers.ProviderConfig{
			BaseURL:    connData.BaseURL,
			AuthHeader: constants.HeaderAuthorization,
			AuthScheme: constants.AuthSchemeBearer,
		}
	} else if cfg, ok := providers.KnownProviders[provider]; ok {
		// Clone config so per-request headers don't mutate global registry
		cloned := cfg
		baseCfg = &cloned
	} else {
		node, nodeData, err := h.Repo.GetProviderNodeByID(provider)
		if err != nil {
			return nil, fmt.Errorf("failed to look up provider node %s: %w", provider, err)
		}
		if node != nil && nodeData != nil && nodeData.BaseURL != "" {
			baseURL := nodeData.BaseURL
			if !strings.HasSuffix(baseURL, "/chat/completions") {
				if strings.HasSuffix(baseURL, "/v1") || strings.HasSuffix(baseURL, "/v1/") {
					baseURL = strings.TrimRight(baseURL, "/") + "/chat/completions"
				} else {
					baseURL = strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
				}
			}
			baseCfg = &providers.ProviderConfig{
				BaseURL:    baseURL,
				AuthHeader: constants.HeaderAuthorization,
				AuthScheme: constants.AuthSchemeBearer,
			}
		}
	}

	if baseCfg == nil {
		return nil, fmt.Errorf("provider %q has no baseUrl in connection data and is not in KnownProviders", provider)
	}

	// Check if this connection uses an Edge Relay Proxy Pool (Vercel, Cloudflare, Deno)
	if connData != nil {
		var relayURL string
		var noProxy string

		if connData.ProxyPoolID != "" {
			if pool, err := h.Repo.GetProxyPool(connData.ProxyPoolID); err == nil && pool != nil && pool.IsActive {
				if pool.Type == "vercel" || pool.Type == "cloudflare" || pool.Type == "deno" {
					relayURL = pool.NextURL()
					noProxy = pool.NoProxy
				}
			}
		}
		if relayURL == "" && connData.ProviderSpecificData != nil {
			if u, ok := connData.ProviderSpecificData["vercelRelayUrl"].(string); ok && u != "" {
				relayURL = u
				if np, ok := connData.ProviderSpecificData["connectionNoProxy"].(string); ok {
					noProxy = np
				}
			}
		}

		if relayURL != "" && !internalproxy.ShouldBypassNoProxy(baseCfg.BaseURL, noProxy) {
			cloned := *baseCfg
			cloned.StaticHeaders = internalproxy.BuildEdgeRelayHeaders(baseCfg.BaseURL, cloned.StaticHeaders)
			cloned.BaseURL = relayURL
			return &cloned, nil
		}
	}

	return baseCfg, nil
}

// ExtractAPIKey gets the API key from a connection's data.
func ExtractAPIKey(connData *ConnectionData) string {
	return extractAPIKey(connData)
}

func extractAPIKey(connData *ConnectionData) string {
	if connData.APIKey != "" {
		return connData.APIKey
	}
	return connData.AccessToken
}

// GetClientForConnection returns an http.Client configured with ProxyPool transport if set.
func (h *ChatHandler) GetClientForConnection(connData *ConnectionData) *http.Client {
	return h.getClientForConnection(connData)
}

func (h *ChatHandler) getClientForConnection(connData *ConnectionData) *http.Client {
	if connData == nil {
		return h.Client
	}

	var proxyURLStr string
	var proxyType string
	var strictProxy bool

	// 1. Resolve from ProxyPool
	if connData.ProxyPoolID != "" {
		pool, err := h.Repo.GetProxyPool(connData.ProxyPoolID)
		if err == nil && pool != nil && pool.IsActive {
			proxyURLStr = pool.NextURL()
			proxyType = pool.Type
			strictProxy = pool.StrictProxy
		}
	}

	// 2. Fallback to legacy connection proxy
	if proxyURLStr == "" {
		proxyEnabled := connData.ConnectionProxyEnabled
		proxyURL := connData.ConnectionProxyURL
		if !proxyEnabled && connData.ProviderSpecificData != nil {
			if en, ok := connData.ProviderSpecificData["connectionProxyEnabled"].(bool); ok {
				proxyEnabled = en
			}
			if u, ok := connData.ProviderSpecificData["connectionProxyUrl"].(string); ok {
				proxyURL = u
			}
			if sp, ok := connData.ProviderSpecificData["strictProxy"].(bool); ok {
				strictProxy = sp
			}
		}
		if proxyEnabled && proxyURL != "" {
			proxyURLStr = proxyURL
			proxyType = "http"
		}
	}

	if proxyURLStr == "" {
		return h.Client
	}

	parsedURL, err := url.Parse(proxyURLStr)
	if err != nil {
		log.Warn("proxy", "invalid proxy pool url", "pool", connData.ProxyPoolID, "url", proxyURLStr, "error", err)
		if strictProxy {
			log.Error("proxy", "strict proxy enabled but proxy url invalid", "url", proxyURLStr)
		}
		return h.Client
	}

	if proxyType == "http" || proxyType == "" {
		transport := &http.Transport{
			Proxy: http.ProxyURL(parsedURL),
		}
		return &http.Client{
			Transport: transport,
			Timeout:   h.Client.Timeout,
		}
	}
	// For Edge Relays (vercel, cloudflare, deno), standard client is used because
	// URL rewriting and x-relay headers are handled at request time.
	return h.Client
}

func (h *ChatHandler) pickRotatedProxyPool(provider string, poolIDs []string, strategy string) string {
	if len(poolIDs) == 0 {
		return ""
	}
	if len(poolIDs) == 1 || strategy == "none" {
		return poolIDs[0]
	}
	if strategy == "random" {
		idx := int(time.Now().UnixNano() % int64(len(poolIDs)))
		return poolIDs[idx]
	}
	// Round-robin
	h.stickyMu.Lock()
	defer h.stickyMu.Unlock()
	if h.proxyRotState == nil {
		h.proxyRotState = make(map[string]int)
	}
	currIdx := h.proxyRotState[provider]
	picked := poolIDs[currIdx%len(poolIDs)]
	h.proxyRotState[provider] = (currIdx + 1) % len(poolIDs)
	return picked
}
