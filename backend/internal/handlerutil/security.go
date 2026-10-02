package handlerutil

import "strings"

// MaskCredential returns a stable, non-reversible display value for a secret.
// Keep enough context for operators to identify a key without exposing a
// bearer credential to telemetry, HTML, logs, or JSON responses.
func MaskCredential(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return "***"
	}
	return value[:7] + "..." + value[len(value)-4:]
}

// SanitizeClientIdentity removes API-key secrets from historical and newly
// generated telemetry. Human identities and the synthetic local/dashboard
// labels remain readable; key-shaped identities are always masked.
func SanitizeClientIdentity(identity, apiKeyID string) string {
	identity = strings.TrimSpace(identity)
	if identity == "" || identity == "Dashboard Admin" || identity == "Local Loopback Client" || strings.HasPrefix(identity, "@") || isNumericIdentity(identity) {
		return identity
	}
	if apiKeyID != "" || looksLikeCredential(identity) {
		return MaskCredential(identity)
	}
	return identity
}

func isNumericIdentity(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func looksLikeCredential(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "zy_") || strings.HasPrefix(value, "sk-") ||
		strings.HasPrefix(value, "api_") || strings.HasPrefix(value, "key_")
}
