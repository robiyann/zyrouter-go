package auth_test

import (
	"errors"
	"testing"
	"time"

	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/models"
)

func TestAPIKeyRestrictions(t *testing.T) {
	restrictionsJSON := `{"allowedModels":["gpt-4o","claude-3-5-sonnet-20241022"],"allowedPrefixes":["claude-*","deepseek-*"],"allowedProviders":["conn-openai-main"],"blockedModels":["claude-3-haiku"]}`

	key := &models.APIKey{
		ID:           "key-test-1",
		Key:          "sk-zy-test-key-12345",
		IsActive:     1,
		Restrictions: &restrictionsJSON,
	}

	tests := []struct {
		name        string
		targetModel string
		provider    string
		expectError bool
	}{
		{
			name:        "Allowed exact model",
			targetModel: "gpt-4o",
			provider:    "conn-openai-main",
			expectError: false,
		},
		{
			name:        "Allowed prefix claude-*",
			targetModel: "claude-3-opus",
			provider:    "conn-openai-main",
			expectError: false,
		},
		{
			name:        "Allowed prefix deepseek-*",
			targetModel: "deepseek-coder",
			provider:    "conn-openai-main",
			expectError: false,
		},
		{
			name:        "Explicit blocked model",
			targetModel: "claude-3-haiku",
			provider:    "conn-openai-main",
			expectError: true,
		},
		{
			name:        "Disallowed model not in list or prefix",
			targetModel: "llama-3.3-70b",
			provider:    "conn-openai-main",
			expectError: true,
		},
		{
			name:        "Disallowed provider",
			targetModel: "gpt-4o",
			provider:    "conn-unauthorized-provider",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := auth.ValidateKeyPolicy(key, tc.targetModel, tc.provider)
			if tc.expectError && err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !tc.expectError && err != nil {
				t.Fatalf("expected success for %s, got err: %v", tc.name, err)
			}
		})
	}
}

func TestAPIKeyNoRestrictions(t *testing.T) {
	key := &models.APIKey{
		ID:           "key-unrestricted",
		Key:          "sk-zy-unrestricted",
		IsActive:     1,
		Restrictions: nil,
	}

	if err := auth.ValidateKeyPolicy(key, "any-model-abc", "any-provider-xyz"); err != nil {
		t.Fatalf("unrestricted key should allow any model, got: %v", err)
	}
}

func TestAPIKeyAllowedPrefixesSupportProviderRoutes(t *testing.T) {
	restrictionsJSON := `{"allowedPrefixes":["ag", "claude-*"]}`
	key := &models.APIKey{IsActive: 1, Restrictions: &restrictionsJSON}

	allowed := []string{"ag/claude-sonnet", "claude-sonnet-4-6"}
	for _, model := range allowed {
		if !key.IsModelAllowed(model) {
			t.Errorf("expected model %q to be allowed", model)
		}
	}
	if key.IsModelAllowed("cx/gpt-4o") {
		t.Error("expected unlisted provider route prefix to be denied")
	}
}

func TestInvalidPolicyFailsClosed(t *testing.T) {
	invalid := "{not-json"
	key := &models.APIKey{IsActive: 1, Restrictions: &invalid}
	if err := auth.ValidateKeyPolicy(key, "gpt-4o", "openai"); err == nil {
		t.Fatal("expected invalid restrictions to be rejected")
	}
	if key.IsModelAllowed("gpt-4o") || key.IsProviderAllowed("openai") {
		t.Fatal("invalid restrictions must not allow access")
	}
}

func TestExpiredPolicyIsRejected(t *testing.T) {
	expired := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	policy := `{"expiresAt":"` + expired + `"}`
	key := &models.APIKey{IsActive: 1, Restrictions: &policy}
	if err := auth.ValidateKeyPolicy(key, "gpt-4o", "openai"); !errors.Is(err, auth.ErrKeyExpired) {
		t.Fatalf("expected expired key error, got %v", err)
	}
}

func TestBlockedModelMatchesResolvedRouteName(t *testing.T) {
	policy := `{"allowedPrefixes":["ag"],"blockedModels":["claude-*"]}`
	key := &models.APIKey{IsActive: 1, Restrictions: &policy}
	if key.IsModelAllowed("ag/claude-sonnet") {
		t.Fatal("blocked model wildcard must apply after removing route prefix")
	}
}
