package chat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/middleware"
	"zyrouter/backend/internal/models"
)

func TestHandleModels_IncludesTokenLimits(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()

	if _, err := database.Exec(`INSERT INTO providerConnections
		(id, provider, authType, name, isActive, data, createdAt, updatedAt)
		VALUES ('limits-antigravity', 'antigravity', 'oauth', 'Limits Test', 1, '{}', '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`); err != nil {
		t.Fatalf("insert provider connection: %v", err)
	}

	repo := db.NewRepo(database)
	if _, err := database.Exec(`INSERT INTO kv (scope, key, value) VALUES ('modelAliases', 'claude-sonnet-4-6', '"anthropic/claude-sonnet-4-6"')`); err != nil {
		t.Fatalf("insert model alias: %v", err)
	}

	h := NewChatHandler(repo)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/models", nil)

	h.HandleModels(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			ID                  string `json:"id"`
			ContextLength       *int   `json:"context_length"`
			MaxCompletionTokens *int   `json:"max_completion_tokens"`
		} `json:"data"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(resp.Data) == 0 {
		t.Fatal("expected at least 1 model in /v1/models")
	}

	found := false
	for _, m := range resp.Data {
		if m.ID == "claude-sonnet-4-6" || strings.HasSuffix(m.ID, "/claude-sonnet-4-6") {
			found = true
			if m.ContextLength == nil || *m.ContextLength <= 0 {
				t.Errorf("expected positive context_length for claude-sonnet-4-6, got %v", m.ContextLength)
			}
		}
	}
	if !found {
		t.Error("claude-sonnet-4-6 not found in /v1/models response")
	}
}

func TestHandleModels_AllowsModelsForAllowedConnectionID(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	restrictions := `{"allowedProviders":["conn-allowed"]}`
	if _, err := database.Exec(`INSERT INTO apiKeys (id, key, name, isActive, restrictions, createdAt) VALUES ('model-key', 'sk-model-key', 'Model Key', 1, ?, '2026-07-18T00:00:00Z')`, restrictions); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO providerConnections (id, provider, authType, name, priority, isActive, data, createdAt, updatedAt) VALUES ('conn-allowed', 'deepseek', 'apikey', 'Allowed', 1, 1, '{"apiKey":"test"}', '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	key := &models.APIKey{ID: "model-key", Key: "sk-model-key", IsActive: 1, Restrictions: &restrictions}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ApiKeyContextKey, key))
	rec := httptest.NewRecorder()
	NewChatHandler(db.NewRepo(database)).HandleModels(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"deepseek/deepseek-chat"`) {
		t.Fatalf("allowed connection model missing: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHandleModels_AllowsProviderPolicyForConnectionModel(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	restrictions := `{"allowedProviders":["deepseek"]}`
	if _, err := database.Exec(`INSERT INTO apiKeys (id, key, name, isActive, restrictions, createdAt) VALUES ('provider-key', 'sk-provider-key', 'Provider Key', 1, ?, '2026-07-18T00:00:00Z')`, restrictions); err != nil {
		t.Fatal(err)
	}
	key := &models.APIKey{ID: "provider-key", Key: "sk-provider-key", IsActive: 1, Restrictions: &restrictions}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ApiKeyContextKey, key))
	rec := httptest.NewRecorder()
	NewChatHandler(db.NewRepo(database)).HandleModels(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"deepseek/deepseek-chat"`) {
		t.Fatalf("provider allowlist should include connection model: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHandleModels_IncludesActiveNoAuthProviderWithoutConnection(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	NewChatHandler(db.NewRepo(database)).HandleModels(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"id":"opencode/mimo-v2.5-free"`) {
		t.Fatal("expected active OpenCode no-auth model without a connection row")
	}
}

func TestValidateRequestPolicy_AllowsProviderAliasForCanonicalModel(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	restrictions := `{"allowedModels":["opencode/mimo-v2.5-free"],"allowedProviders":["opencode"]}`
	key := &models.APIKey{ID: "alias-key", Key: "sk-alias-key", IsActive: 1, Restrictions: &restrictions}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ApiKeyContextKey, key))
	h := NewChatHandler(db.NewRepo(database))
	info := &ModelInfo{Provider: "opencode", Model: "mimo-v2.5-free"}
	if err := h.validateRequestPolicy(req, "oc/mimo-v2.5-free", info); err != nil {
		t.Fatalf("canonical model should allow provider alias: %v", err)
	}
}
