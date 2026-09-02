package labels

import (
	"strings"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/providers"
)

// Provider returns a human-readable provider label while keeping the raw ID
// available separately for filtering and diagnostics.
func Provider(repo *db.Repo, provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "Gateway"
	}
	if repo != nil {
		if node, _, err := repo.GetProviderNodeByID(provider); err == nil && node != nil && node.Name != nil && strings.TrimSpace(*node.Name) != "" {
			return strings.TrimSpace(*node.Name)
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
func Prefix(repo *db.Repo, provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if repo != nil {
		if prefixes, err := repo.GetProviderPrefixes(); err == nil {
			if prefix := strings.TrimSpace(prefixes[provider]); prefix != "" {
				return strings.ToLower(prefix)
			}
		}
		if _, nodeData, err := repo.GetProviderNodeByID(provider); err == nil && nodeData != nil && strings.TrimSpace(nodeData.Prefix) != "" {
			return strings.ToLower(strings.TrimSpace(nodeData.Prefix))
		}
	}
	return providers.GetDefaultProviderAlias(provider)
}

// Model returns the model with its client-facing provider prefix.
func Model(repo *db.Repo, provider, model string) string {
	model = strings.TrimSpace(model)
	prefix := Prefix(repo, provider)
	if model == "" || prefix == "" || strings.HasPrefix(strings.ToLower(model), strings.ToLower(prefix)+"/") {
		return model
	}
	return prefix + "/" + model
}
