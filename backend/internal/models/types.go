package models

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// Meta represents a key-value meta record.
type Meta struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Setting represents a single settings record.
type Setting struct {
	ID   int    `json:"id"`
	Data string `json:"data"` // JSON string representation of the settings data
}

// ProviderConnection represents an upstream provider connection.
type ProviderConnection struct {
	ID        string  `json:"id"`
	Provider  string  `json:"provider"`
	AuthType  string  `json:"authType"`
	Name      *string `json:"name,omitempty"`
	Email     *string `json:"email,omitempty"`
	Priority  *int    `json:"priority,omitempty"`
	IsActive  int     `json:"isActive"` // 0 or 1
	Data      string  `json:"data"`     // JSON string representing additional provider config
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// ProviderNode represents a deployment node / executor config.
type ProviderNode struct {
	ID        string  `json:"id"`
	Type      *string `json:"type,omitempty"`
	Name      *string `json:"name,omitempty"`
	Data      string  `json:"data"` // JSON string representing node details
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// ProxyPool represents a pool of proxies.
type ProxyPool struct {
	ID         string  `json:"id"`
	IsActive   int     `json:"isActive"` // 0 or 1
	TestStatus *string `json:"testStatus,omitempty"`
	Data       string  `json:"data"` // JSON string representing proxy credentials and checks
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

// KeyRateLimit defines rate limiting per API key.
type KeyRateLimit struct {
	RequestsPerMinute int `json:"requestsPerMinute,omitempty"`
	TokensPerDay      int `json:"tokensPerDay,omitempty"`
}

// KeyRestrictions represents granular access control policy on an API key.
type KeyRestrictions struct {
	AllowedModels    []string      `json:"allowedModels,omitempty"`    // Exact model IDs whitelist (e.g. ["gpt-4o", "claude-3-5-sonnet"])
	AllowedPrefixes  []string      `json:"allowedPrefixes,omitempty"`  // Wildcard prefixes (e.g. ["claude-*", "gpt-*", "deepseek-*"])
	AllowedProviders []string      `json:"allowedProviders,omitempty"` // Provider connection IDs whitelist (e.g. ["conn-openai-main"])
	BlockedModels    []string      `json:"blockedModels,omitempty"`    // Explicitly blocked model IDs
	RateLimit        *KeyRateLimit `json:"rateLimit,omitempty"`
	ExpiresAt        *string       `json:"expiresAt,omitempty"`
}

// APIKey represents a client-facing authorization key.
type APIKey struct {
	ID           string  `json:"id"`
	Key          string  `json:"key"`
	Name         *string `json:"name,omitempty"`
	MachineID    *string `json:"machineId,omitempty"`
	IsActive     int     `json:"isActive"`               // 0 or 1
	Restrictions *string `json:"restrictions,omitempty"` // JSON string representing KeyRestrictions
	ClientID     *string `json:"clientId,omitempty"`
	PolicyID     *string `json:"policyId,omitempty"`
	CreatedAt    string  `json:"createdAt"`
}

// ClientPolicy is the server-owned policy inherited by client-generated keys.
type ClientPolicy struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsActive  int    `json:"isActive"`
	Data      string `json:"data"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Client is an isolated client-dashboard identity.
type Client struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Email     *string `json:"email,omitempty"`
	PolicyID  *string `json:"policyId,omitempty"`
	IsActive  int     `json:"isActive"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// ParseRestrictions parses the restrictions JSON string into KeyRestrictions struct.
func (k *APIKey) ParseRestrictions() (*KeyRestrictions, error) {
	if k.Restrictions == nil || strings.TrimSpace(*k.Restrictions) == "" {
		return nil, nil
	}
	var res KeyRestrictions
	if err := json.Unmarshal([]byte(*k.Restrictions), &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// IsModelAllowed checks whether a target model is allowed by this API key's policy.
func (k *APIKey) IsModelAllowed(model string) bool {
	res, err := k.ParseRestrictions()
	if err != nil {
		return false // Malformed policy must fail closed.
	}
	if res == nil {
		return true // No restrictions -> allow all
	}
	modelName := model
	if parts := strings.SplitN(model, "/", 2); len(parts) == 2 {
		modelName = parts[1]
	}

	// 1. Check explicit blocked models
	for _, blocked := range res.BlockedModels {
		if strings.EqualFold(blocked, model) || strings.EqualFold(blocked, modelName) {
			return false
		}
		if matched, _ := filepath.Match(strings.ToLower(blocked), strings.ToLower(model)); matched {
			return false
		}
		if matched, _ := filepath.Match(strings.ToLower(blocked), strings.ToLower(modelName)); matched {
			return false
		}
	}

	// If no whitelists are defined, allow by default
	if len(res.AllowedModels) == 0 && len(res.AllowedPrefixes) == 0 {
		return true
	}
	routePrefix := ""
	if parts := strings.SplitN(model, "/", 2); len(parts) == 2 {
		routePrefix = strings.TrimSpace(parts[0])
		modelName = parts[1]
	}

	// 2. Check exact allowed models
	for _, allowed := range res.AllowedModels {
		if strings.EqualFold(allowed, model) || (modelName != model && strings.EqualFold(allowed, modelName)) {
			return true
		}
	}

	// 3. Check allowed wildcard prefixes (e.g. "claude-*", "gpt-*")
	// A slash-qualified model also carries a client-facing provider prefix
	// (for example "ag/claude-sonnet"). Allow an exact route prefix such as
	// "ag" or "ag/*" without weakening model-name prefix matching.
	for _, prefix := range res.AllowedPrefixes {
		cleanPrefix := strings.TrimSpace(prefix)
		if routePrefix != "" {
			routePattern := strings.TrimSuffix(cleanPrefix, "*")
			routePattern = strings.TrimSuffix(routePattern, "/")
			if routePattern != "" && strings.EqualFold(routePattern, routePrefix) {
				return true
			}
		}
		if strings.HasSuffix(cleanPrefix, "*") {
			prefixPattern := strings.TrimSuffix(cleanPrefix, "*")
			if strings.HasPrefix(strings.ToLower(modelName), strings.ToLower(prefixPattern)) ||
				strings.HasPrefix(strings.ToLower(model), strings.ToLower(prefixPattern)) {
				return true
			}
		}
		if matched, _ := filepath.Match(strings.ToLower(cleanPrefix), strings.ToLower(model)); matched {
			return true
		}
	}

	return false
}

// IsProviderAllowed checks whether a target provider or connection ID is allowed.
func (k *APIKey) IsProviderAllowed(providerOrConnID string) bool {
	res, err := k.ParseRestrictions()
	if err != nil {
		return false // Malformed policy must fail closed.
	}
	if res == nil {
		return true
	}
	if len(res.AllowedProviders) == 0 {
		return true
	}
	for _, allowed := range res.AllowedProviders {
		if strings.EqualFold(allowed, providerOrConnID) {
			return true
		}
		if matched, _ := filepath.Match(strings.ToLower(allowed), strings.ToLower(providerOrConnID)); matched {
			return true
		}
	}
	return false
}

// Combo represents a multi-model routing combinated alias.
type Combo struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Kind      *string `json:"kind,omitempty"`
	Models    string  `json:"models"`   // JSON string representing model selection details
	Strategy  string  `json:"strategy"` // routing strategy: "fallback", "round-robin", "capacity", "fusion"
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// KV represents a scoped key-value store entry.
type KV struct {
	Scope string `json:"scope"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// UsageHistory represents logged usage metrics for a request.
type UsageHistory struct {
	ID               int64   `json:"id"`
	Timestamp        string  `json:"timestamp"`
	Provider         *string `json:"provider,omitempty"`
	Model            *string `json:"model,omitempty"`
	ConnectionID     *string `json:"connectionId,omitempty"`
	APIKey           *string `json:"apiKey,omitempty"`
	Endpoint         *string `json:"endpoint,omitempty"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	Cost             float64 `json:"cost"`
	Status           *string `json:"status,omitempty"`
	Tokens           *string `json:"tokens,omitempty"` // JSON metadata on pricing / raw details
	Meta             *string `json:"meta,omitempty"`   // Extra metadata JSON
}

// UsageDaily represents aggregated usage per day.
type UsageDaily struct {
	DateKey string `json:"dateKey"`
	Data    string `json:"data"` // JSON string representing daily stats details
}

// RequestDetail represents the cached payload/response logs of requests.
type RequestDetail struct {
	ID           string  `json:"id"`
	Timestamp    string  `json:"timestamp"`
	Provider     *string `json:"provider,omitempty"`
	Model        *string `json:"model,omitempty"`
	ConnectionID *string `json:"connectionId,omitempty"`
	Status       *string `json:"status,omitempty"`
	Data         string  `json:"data"` // JSON representation of request/response payload
}
