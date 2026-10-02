package handlerutil

import "strings"

// MaskCredential returns a stable, non-reversible display value for a secret.
// For short secrets (<= 12 chars), masks completely with "******".
// For longer keys (e.g. zy_fe004beb071ff1...), preserves 7-character prefix and 4-character suffix.
func MaskCredential(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return "******"
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
	if apiKeyID != "" || LooksLikeCredential(identity) {
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

// LooksLikeCredential checks if a string matches common API key or bearer token formats.
func LooksLikeCredential(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(v, "zy_") || strings.HasPrefix(v, "sk-") ||
		strings.HasPrefix(v, "api_") || strings.HasPrefix(v, "key_") ||
		strings.HasPrefix(v, "clt_") || strings.HasPrefix(v, "bearer ") {
		return true
	}
	if len(v) >= 16 && !strings.Contains(v, " ") {
		return true
	}
	return false
}
