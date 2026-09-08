package chat

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/models"
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

func TestPublicModelEndpointsRejectPrefixes(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	h := NewChatHandler(db.NewRepo(database))

	infoRec := httptest.NewRecorder()
	h.HandleModelsInfo(infoRec, httptest.NewRequest(http.MethodGet, "/v1/models/info?id=deepseek/deepseek-chat", nil))
	if infoRec.Code != http.StatusForbidden || !strings.Contains(infoRec.Body.String(), "provider_prefix_forbidden") {
		t.Fatalf("models info accepted provider prefix: status=%d body=%s", infoRec.Code, infoRec.Body.String())
	}

	responseRec := httptest.NewRecorder()
	responseReq := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"deepseek/deepseek-chat","input":"hello"}`))
	h.HandleResponses(responseRec, responseReq)
	if responseRec.Code != http.StatusForbidden || !strings.Contains(responseRec.Body.String(), "provider_prefix_forbidden") {
		t.Fatalf("responses endpoint accepted provider prefix: status=%d body=%s", responseRec.Code, responseRec.Body.String())
	}
}

func TestCompositeAliasCanRouteInternalProviderTargetsWithoutLeakingThem(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	if err := repo.CreateCombo(&models.Combo{
		ID:       "combo-internal-targets",
		Name:     "internal-targets",
		Models:   `["deepseek/deepseek-chat","openai/gpt-4o"]`,
		Strategy: "round-robin",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetModelAlias("unified-chat", "combo:internal-targets"); err != nil {
		t.Fatal(err)
	}

	h := NewChatHandler(repo)
	info, err := h.resolveClientModel("unified-chat")
	if err != nil || info == nil || len(info.ComboModels) != 2 {
		t.Fatalf("composite alias did not resolve internal targets: info=%+v err=%v", info, err)
	}

	rec := httptest.NewRecorder()
	h.HandleModelsInfo(rec, httptest.NewRequest(http.MethodGet, "/v1/models/info?id=unified-chat", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("composite model info status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"member_count":2`) || strings.Contains(rec.Body.String(), "deepseek/") || strings.Contains(rec.Body.String(), "openai/") {
		t.Fatalf("internal combo targets leaked from public metadata: %s", rec.Body.String())
	}
}
