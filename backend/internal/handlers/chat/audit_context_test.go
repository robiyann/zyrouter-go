package chat

import (
	"context"
	"testing"

	"zyrouter/backend/internal/middleware"
	"zyrouter/backend/internal/models"
)

func TestAuditMetadataIdentityPriority(t *testing.T) {
	username := "alice"
	telegramID := "123456"
	key := &models.APIKey{
		ID:               "key-1",
		Key:              "zy_secret-client-key",
		TelegramUsername: &username,
		TelegramUserID:   &telegramID,
	}
	ctx := context.WithValue(context.Background(), middleware.ApiKeyContextKey, key)
	ctx = context.WithValue(ctx, middleware.ClientIPContextKey, "203.0.113.10")

	metadata := auditMetadataFromContext(ctx)
	if metadata.ClientIdentity != "@alice" {
		t.Fatalf("expected username priority, got %q", metadata.ClientIdentity)
	}
	if metadata.ClientIP != "203.0.113.10" || metadata.APIKeyID != "key-1" {
		t.Fatalf("unexpected request metadata: %+v", metadata)
	}

	key.TelegramUsername = nil
	metadata = auditMetadataFromContext(ctx)
	if metadata.ClientIdentity != telegramID {
		t.Fatalf("expected Telegram ID fallback, got %q", metadata.ClientIdentity)
	}

	key.TelegramUserID = nil
	metadata = auditMetadataFromContext(ctx)
	if metadata.ClientIdentity != key.Key {
		t.Fatalf("expected full API key fallback, got %q", metadata.ClientIdentity)
	}
}
