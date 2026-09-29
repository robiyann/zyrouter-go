package labels

import (
	"strings"

	"zyrouter/backend/internal/providers"
)

// Resolver is the small repository surface needed to resolve friendly labels.
// Keeping this interface here avoids a package cycle when the db package uses
// the same label formatting for its own query results.
type Resolver interface {
	ProviderNodeName(string) (string, error)
	ProviderNodePrefix(string) (string, error)
	GetProviderPrefixes() (map[string]string, error)
}

// Provider returns a human-readable provider label while keeping the raw ID
// available separately for filtering and diagnostics.
func Provider(repo Resolver, provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "Gateway"
	}
	if repo != nil {
		if name, err := repo.ProviderNodeName(provider); err == nil && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	lower := strings.ToLower(provider)
	if strings.HasPrefix(lower, "anthropic-compatible-") {
		return "Anthropic Compatible"
	}
	if strings.HasPrefix(lower, "openai-compatible-") {
		return "OpenAI Compatible"
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
}

// Prefix returns the active client-facing model prefix for a provider.
func Prefix(repo Resolver, provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if repo != nil {
		if prefixes, err := repo.GetProviderPrefixes(); err == nil {
			if prefix := strings.TrimSpace(prefixes[provider]); prefix != "" {
				return strings.ToLower(prefix)
			}
		}
		if prefix, err := repo.ProviderNodePrefix(provider); err == nil && strings.TrimSpace(prefix) != "" {
			return strings.ToLower(strings.TrimSpace(prefix))
		}
	}
	return providers.GetDefaultProviderAlias(provider)
}

// Model returns the model with its client-facing provider prefix.
func Model(repo Resolver, provider, model string) string {
	model = strings.TrimSpace(model)
	prefix := Prefix(repo, provider)
	if model == "" || prefix == "" || strings.HasPrefix(strings.ToLower(model), strings.ToLower(prefix)+"/") {
		return model
	}
	return prefix + "/" + model
}
