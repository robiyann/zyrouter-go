package chat

import (
	"context"
	"strings"

	"zyrouter/backend/internal/middleware"
)

// auditRequestMetadata is derived from the authenticated inbound request, not
// from the upstream provider connection. This distinction is important because
// UsageLogInfo.APIKey is the provider credential used for forwarding.
type auditRequestMetadata struct {
	ClientAPIKey   string
	APIKeyID       string
	ClientIdentity string
	ClientIP       string
}

// auditMetadataFromContext returns the admin-only request identity using the
// requested priority: Telegram username, Telegram user ID, then the presented
// API key. The raw key is only carried into admin audit sinks; it is never sent
// to client-owned telemetry endpoints.
func auditMetadataFromContext(ctx context.Context) auditRequestMetadata {
	key := middleware.GetAuthenticatedApiKeyFromContext(ctx)
	if key == nil {
		return auditRequestMetadata{ClientIP: middleware.GetClientIPFromContext(ctx)}
	}

	metadata := auditRequestMetadata{
		ClientAPIKey: strings.TrimSpace(key.Key),
		APIKeyID:     strings.TrimSpace(key.ID),
		ClientIP:     middleware.GetClientIPFromContext(ctx),
	}
	if username := strings.TrimSpace(valueOrEmpty(key.TelegramUsername)); username != "" {
		metadata.ClientIdentity = "@" + strings.TrimPrefix(username, "@")
	} else if telegramUserID := strings.TrimSpace(valueOrEmpty(key.TelegramUserID)); telegramUserID != "" {
		metadata.ClientIdentity = telegramUserID
	} else if key.ID == "session-admin" {
		metadata.ClientIdentity = "Dashboard Admin"
	} else if key.ID == "local-loopback" {
		metadata.ClientIdentity = "Local Loopback Client"
	} else {
		metadata.ClientIdentity = metadata.ClientAPIKey
	}
	return metadata
}
