package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlers/shared"
	"zyrouter/backend/internal/log"
	"zyrouter/backend/internal/providers"
	"zyrouter/backend/internal/proxy/executor"
	"zyrouter/backend/internal/proxy/oauth"
)

// NewChatHandler creates a ChatHandler with the given repository and a streaming-capable HTTP client.
// Pass a TokenSaverConfig to enable token saver features, or nil for all-off defaults.
func NewChatHandler(repo *db.Repo, ts ...*shared.TokenSaverConfig) *ChatHandler {
	executor.RegisterAll()
	oauth.RegisterAll()
	cfg := &shared.TokenSaverConfig{}
	if len(ts) > 0 && ts[0] != nil {
		cfg = ts[0]
	}
	// Timeout: 0 is required so long SSE streams are not cut short, but a
	// ResponseHeaderTimeout bounds how long we wait for the upstream to
	// start responding — closing the "accept then go silent" gap without
	// killing a stream that has already begun.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 2 * time.Minute
	return &ChatHandler{
		Repo: repo,
		Client: &http.Client{
			Transport: transport,
			Timeout:   0, // no timeout for streaming support
		},
		TokenSaver:  cfg,
		stickyState: make(map[string]*comboStickyState),
	}
}

// ResolveModel resolves a model string through aliases, combos, and provider/model parsing.
// Exported so other handlers (media, responses, etc.) can resolve model names.
func (h *ChatHandler) ResolveModel(modelStr string) (*ModelInfo, error) {
	return h.resolveModel(modelStr)
}

// GetActiveProviderPrefix returns the single active prefix for a given provider.
// Precedence:
// 1. Custom prefix in kv table (scope 'providerPrefixes')
// 2. Connection data prefix (if provided)
// 3. Default provider alias from CanonicalDefaultAliasMap or canonical provider ID.
func (h *ChatHandler) GetActiveProviderPrefix(provider string, connData map[string]any) string {
	provLower := strings.ToLower(strings.TrimSpace(provider))
	if connData != nil {
		if p, ok := connData["prefix"].(string); ok && strings.TrimSpace(p) != "" {
			return strings.TrimSpace(strings.ToLower(p))
		}
	}
	if h.Repo != nil {
		if prefixes, err := h.Repo.GetProviderPrefixes(); err == nil {
			if customPref, ok := prefixes[provLower]; ok && strings.TrimSpace(customPref) != "" {
				return strings.TrimSpace(strings.ToLower(customPref))
			}
		}
	}
	return providers.GetDefaultProviderAlias(provLower)
}

// resolveProviderPrefix resolves a user-supplied prefix string to its canonical provider ID.
// Under Option 3, this enforces that requestedPrefix MUST match the SINGLE active prefix
// of the provider. If requestedPrefix does not match the active prefix of the provider
// (e.g. calling "opencode" when the active prefix is "oc", or "oc" when prefix is "opencode"),
// resolution fails and returns an empty string.
func (h *ChatHandler) resolveProviderPrefix(prefix string) string {
	prefixLower := strings.ToLower(strings.TrimSpace(prefix))
	if prefixLower == "" {
		return ""
	}

	var customPrefixes map[string]string
	if h.Repo != nil {
		if cp, err := h.Repo.GetProviderPrefixes(); err == nil {
			customPrefixes = cp
		}
	}

	// 1. Check if prefixLower matches any custom prefix configured in KV
	for prov, customPref := range customPrefixes {
		if strings.ToLower(strings.TrimSpace(customPref)) == prefixLower {
			return prov
		}
	}

	// If prefixLower is a canonical provider name that has a custom prefix in KV,
	// but the custom prefix is different from prefixLower, reject!
	if customPref, hasCustom := customPrefixes[prefixLower]; hasCustom {
		if strings.ToLower(strings.TrimSpace(customPref)) != prefixLower {
			return "" // Rejected: must use the configured custom prefix!
		}
	}

	// 2. Check active provider connections with connection-level prefix
	if h.Repo != nil {
		if conns, err := h.Repo.GetProviderConnections("", true); err == nil {
			for _, conn := range conns {
				var d map[string]any
				if json.Unmarshal([]byte(conn.Data), &d) == nil {
					if p, ok := d["prefix"].(string); ok && strings.TrimSpace(p) != "" {
						if strings.ToLower(strings.TrimSpace(p)) == prefixLower {
							return conn.Provider
						}
					}
				}
			}
		}
	}

	// 3. Check designated default aliases from CanonicalDefaultAliasMap
	for canon, defaultAlias := range providers.CanonicalDefaultAliasMap {
		if strings.ToLower(defaultAlias) == prefixLower {
			// If this provider has an overriding custom prefix in KV, skip
			if customPref, hasCustom := customPrefixes[canon]; hasCustom {
				if strings.ToLower(strings.TrimSpace(customPref)) == prefixLower {
					return canon
				}
				continue
			}
			return canon
		}
	}

	// If prefixLower is a canonical provider with a designated default alias
	// (e.g. "opencode" with default alias "oc"), but no KV override was set,
	// reject calling it with the canonical name!
	if defaultAlias, hasDefault := providers.CanonicalDefaultAliasMap[prefixLower]; hasDefault {
		if strings.ToLower(defaultAlias) != prefixLower {
			return "" // Rejected: must use the active alias!
		}
	}

	// 4. Check ProviderAliasMap (for other short aliases)
	if canon, ok := providers.ProviderAliasMap[prefixLower]; ok {
		if customPref, hasCustom := customPrefixes[canon]; hasCustom {
			if strings.ToLower(strings.TrimSpace(customPref)) == prefixLower {
				return canon
			}
			return ""
		}
		// If canonical provider has a designated default alias that differs, reject
		if defAlias, hasDef := providers.CanonicalDefaultAliasMap[canon]; hasDef && strings.ToLower(defAlias) != prefixLower {
			return ""
		}
		return canon
	}

	// 5. Canonical fallback for known providers without default aliases (e.g. "openai", "deepseek", "gemini")
	if _, ok := providers.KnownProviders[prefixLower]; ok {
		if customPref, hasCustom := customPrefixes[prefixLower]; hasCustom {
			if strings.ToLower(strings.TrimSpace(customPref)) != prefixLower {
				return ""
			}
		}
		return prefixLower
	}

	return prefixLower
}

func resolveProviderAlias(alias string) string {
	if canonical, ok := providers.ProviderAliasMap[alias]; ok {
		return canonical
	}
	return alias
}

// resolveModelEntry parses a single "provider/model" string into a ModelInfo
// without combo or alias resolution (used when iterating combo entries).
// If the entry has no "/" (i.e. it's a combo name), it resolves the combo
// and returns its first concrete model with the combined model list.
func (h *ChatHandler) resolveModelEntry(entry string) *ModelInfo {
	if !strings.Contains(entry, "/") {
		combo, err := h.Repo.GetComboByName(entry)
		if err == nil && combo != nil && combo.Models != "" {
			var subModels []string
			if err := json.Unmarshal([]byte(combo.Models), &subModels); err == nil && len(subModels) > 0 {
				first := h.resolveModelEntry(subModels[0])
				if first != nil {
					first.ComboModels = subModels
					first.Strategy = combo.Strategy
					return first
				}
			}
		}
		return nil
	}
	parts := strings.SplitN(entry, "/", 2)
	provider := h.resolveProviderPrefix(parts[0])
	if provider == "" {
		if info := h.resolvePrefixProvider(parts[0], parts[1]); info != nil {
			return info
		}
		return nil
	}
	if _, ok := providers.KnownProviders[provider]; !ok {
		if info := h.resolvePrefixProvider(provider, parts[1]); info != nil {
			return info
		}
	}
	return &ModelInfo{Provider: provider, Model: parts[1]}
}

// flattenComboModels recursively expands combo-name entries into concrete
// "provider/model" leaves, keeping order and deduping consecutive identical
// leaves so a nested combo can't create pointless rotation slots. Guards
// against cyclic combo references by skipping recursive cycles. Inner-combo
// strategies are not applied here; the top-level combo's strategy governs
// the flattened list.
func (h *ChatHandler) flattenComboModels(models []string) ([]string, error) {
	out := make([]string, 0, len(models))
	seen := make(map[string]bool)
	var walk func([]string) error
	walk = func(ms []string) error {
		for _, m := range ms {
			if !strings.Contains(m, "/") {
				if seen[m] {
					log.Warn("combo", "cyclic combo reference detected, skipping", "combo", m)
					continue
				}
				if combo, err := h.Repo.GetComboByName(m); err == nil && combo != nil && combo.Models != "" {
					var sub []string
					if err := json.Unmarshal([]byte(combo.Models), &sub); err == nil {
						seen[m] = true
						if err := walk(sub); err != nil {
							return err
						}
						delete(seen, m)
						continue
					}
				}
				if aliasTarget, err := h.Repo.GetModelAlias(m); err == nil && aliasTarget != "" && strings.Contains(aliasTarget, "/") {
					m = aliasTarget
				}
			}
			if len(out) == 0 || out[len(out)-1] != m {
				out = append(out, m)
			}
		}
		return nil
	}
	if err := walk(models); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("combo has no valid leaf models")
	}
	return out, nil
}

// resolveModel resolves a model string through aliases, combos, and provider/model parsing.
// Returns the first concrete ModelInfo found, or an error.
func (h *ChatHandler) resolveModel(modelStr string) (*ModelInfo, error) {
	if modelStr == "" {
		return nil, fmt.Errorf("missing model")
	}

	// 0. Check exact model alias first (even if string contains "/")
	if aliasTarget, err := h.Repo.GetModelAlias(modelStr); err == nil && aliasTarget != "" {
		if strings.Contains(aliasTarget, "/") {
			parts := strings.SplitN(aliasTarget, "/", 2)
			provider := h.resolveProviderPrefix(parts[0])
			if provider != "" {
				if _, ok := providers.KnownProviders[provider]; !ok {
					if info := h.resolvePrefixProvider(provider, parts[1]); info != nil {
						return info, nil
					}
				}
				return &ModelInfo{
					Provider: provider,
					Model:    parts[1],
				}, nil
			}
			if info := h.resolvePrefixProvider(parts[0], parts[1]); info != nil {
				return info, nil
			}
		}
	}

	// 1. Standard format: "provider/model"
	if strings.Contains(modelStr, "/") {
		parts := strings.SplitN(modelStr, "/", 2)
		providerAlias := parts[0]
		model := parts[1]
		provider := h.resolveProviderPrefix(providerAlias)
		if provider != "" {
			if _, ok := providers.KnownProviders[provider]; !ok {
				if info := h.resolvePrefixProvider(provider, model); info != nil {
					return info, nil
				}
			}
			return &ModelInfo{Provider: provider, Model: model}, nil
		}
		if info := h.resolvePrefixProvider(providerAlias, model); info != nil {
			return info, nil
		}
		return nil, fmt.Errorf("could not resolve model: %s", modelStr)
	}

	// 2. Check if it's a model alias (e.g., "gpt-4o" -> "openai/gpt-4o")
	aliasTarget, err := h.Repo.GetModelAlias(modelStr)
	if err == nil && aliasTarget != "" {
		if strings.Contains(aliasTarget, "/") {
			parts := strings.SplitN(aliasTarget, "/", 2)
			provider := h.resolveProviderPrefix(parts[0])
			if provider != "" {
				if _, ok := providers.KnownProviders[provider]; !ok {
					if info := h.resolvePrefixProvider(provider, parts[1]); info != nil {
						return info, nil
					}
				}
				return &ModelInfo{
					Provider: provider,
					Model:    parts[1],
				}, nil
			}
			if info := h.resolvePrefixProvider(parts[0], parts[1]); info != nil {
				return info, nil
			}
		}
	}

	// 3. Check if it's a combo name
	combo, err := h.Repo.GetComboByName(modelStr)
	if err == nil && combo != nil && combo.Models != "" {
		var modelStrings []string
		if err := json.Unmarshal([]byte(combo.Models), &modelStrings); err == nil && len(modelStrings) > 0 {
			// Flatten nested combos into concrete leaves so rotation covers
			// every reachable model (a nested combo entry used to collapse to
			// its first leaf, so combo-wombo -> free-tier never rotated).
			flattened, flatErr := h.flattenComboModels(modelStrings)
			if flatErr != nil {
				return nil, flatErr
			}
			if len(flattened) > 0 {
				firstInfo := h.resolveModelEntry(flattened[0])
				if firstInfo == nil {
					firstInfo, _ = h.resolveModel(flattened[0])
				}
				if firstInfo != nil {
					firstInfo.ComboModels = flattened
					firstInfo.Strategy = combo.Strategy
					return firstInfo, nil
				}
			}
		}
	}

	// 4. Check custom models in DB
	if customList, err := h.Repo.GetCustomModels(); err == nil && len(customList) > 0 {
		for _, cm := range customList {
			if cm.ID == modelStr {
				return &ModelInfo{Provider: cm.ProviderAlias, Model: modelStr}, nil
			}
		}
	}

	return nil, fmt.Errorf("could not resolve model: %s", modelStr)
}

// resolvePrefixProvider checks if a provider name is a providerNode prefix.
// If so, it finds the matching connection and returns a pinned ModelInfo.
func (h *ChatHandler) resolvePrefixProvider(prefix string, model string) *ModelInfo {
	node, _, err := h.Repo.GetProviderNodeByPrefix(prefix)
	if err != nil || node == nil {
		return nil
	}

	conn, _, err := h.getBestConnection(node.ID, "", nil, model)
	if err != nil || conn == nil {
		return nil
	}

	return &ModelInfo{
		Provider:     node.ID,
		Model:        model,
		ConnectionID: conn.ID,
	}
}
