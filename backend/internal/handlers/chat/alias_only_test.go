package chat

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/db"
)

func TestPublicModelResolutionRequiresAliasAndRejectsProviderPrefix(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	h := NewChatHandler(repo)

	info, err := h.resolveClientModel("fast-model")
	if err != nil || info == nil || info.Provider != "deepseek" {
		t.Fatalf("published alias did not resolve: info=%+v err=%v", info, err)
	}
	if _, err := h.resolveClientModel("deepseek/deepseek-chat"); !errors.Is(err, auth.ErrProviderPrefixForbidden) {
		t.Fatalf("provider prefix was not rejected: %v", err)
	}
	if _, err := h.resolveClientModel("deepseek-chat"); !errors.Is(err, auth.ErrModelAliasRequired) {
		t.Fatalf("raw upstream model was not rejected: %v", err)
	}
}

func TestPublicModelsExposeOnlyPublishedAliases(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	h := NewChatHandler(db.NewRepo(database))
	rec := httptest.NewRecorder()
	h.HandleModels(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("models status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"fast-model"`) {
		t.Fatalf("published alias missing from model list: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `deepseek/deepseek-chat`) || strings.Contains(rec.Body.String(), `"id":"deepseek-chat"`) {
		t.Fatalf("raw provider model leaked into public model list: %s", rec.Body.String())
	}
}

func TestPublicChatRejectsRawAndProviderPrefixedModels(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	h := NewChatHandler(db.NewRepo(database))
	for _, model := range []string{"deepseek/deepseek-chat", "deepseek-chat"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[]}`))
		h.HandleChatCompletions(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("model %q was not rejected: status=%d body=%s", model, rec.Code, rec.Body.String())
		}
	}
}
