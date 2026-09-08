package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"zyrouter/backend/internal/db"
)

func TestAdminAPIKeyIsHashedAndNotRevealable(t *testing.T) {
	file, err := os.CreateTemp("", "admin-key-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	defer os.Remove(file.Name())
	database, err := db.OpenDatabase(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	h := NewAdminHandler(db.NewRepo(database))
	req := httptest.NewRequest(http.MethodPost, "/api/keys", strings.NewReader(`{"name":"production"}`))
	created := httptest.NewRecorder()
	h.HandleCreateKey(created, req)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var response struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID == "" || !strings.HasPrefix(response.Key, "zy_") {
		t.Fatalf("unexpected create response: %+v", response)
	}

	var storedKey, storedHash, accountType string
	if err := database.QueryRow(`SELECT key, keyHash, accountTypeId FROM apiKeys WHERE id = ?`, response.ID).Scan(&storedKey, &storedHash, &accountType); err != nil {
		t.Fatal(err)
	}
	if storedKey == response.Key || storedHash != db.HashUserSecret(response.Key) || accountType != "administrator" {
		t.Fatalf("secret was not stored safely: key=%q hashSet=%t type=%q", storedKey, storedHash != "", accountType)
	}

	reveal := httptest.NewRecorder()
	router := chi.NewRouter()
	router.Get("/api/keys/{id}/reveal", h.HandleRevealKey)
	router.ServeHTTP(reveal, httptest.NewRequest(http.MethodGet, "/api/keys/"+response.ID+"/reveal", nil))
	if reveal.Code != http.StatusGone || strings.Contains(reveal.Body.String(), response.Key) {
		t.Fatalf("reveal endpoint exposed or misreported secret: status=%d body=%s", reveal.Code, reveal.Body.String())
	}
	if err := h.repo.SetModelAliasRecord("fast-gemini", "google", "gemini-2.5-pro", nil, []string{"chat"}, 1); err != nil {
		t.Fatal(err)
	}
	aliases := httptest.NewRecorder()
	h.HandleGetModelAliases(aliases, httptest.NewRequest(http.MethodGet, "/api/model-aliases", nil))
	if aliases.Code != http.StatusOK || !strings.Contains(aliases.Body.String(), `"alias":"fast-gemini"`) {
		t.Fatalf("structured alias listing failed: status=%d body=%s", aliases.Code, aliases.Body.String())
	}
}

func TestModelPolicyPreviewUsesPublishedAliases(t *testing.T) {
	file, err := os.CreateTemp("", "admin-policy-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	defer os.Remove(file.Name())
	database, err := db.OpenDatabase(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repo := db.NewRepo(database)
	if err := repo.SetModelAliasRecord("fast-gemini", "google", "gemini-2.5-pro", nil, nil, 1); err != nil {
		t.Fatal(err)
	}
	h := NewAdminHandler(repo)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/model-policy/preview", strings.NewReader(`{"models":["fast-gemini"],"restrictions":{"allowedModels":["fast-gemini"],"allowedProviders":["google"]}}`))
	rec := httptest.NewRecorder()
	h.HandlePreviewModelPolicy(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"allowed":true`) {
		t.Fatalf("unexpected policy preview: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSecuritySummaryIsAggregateOnly(t *testing.T) {
	file, err := os.CreateTemp("", "admin-security-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	defer os.Remove(file.Name())
	database, err := db.OpenDatabase(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	h := NewAdminHandler(db.NewRepo(database))
	rec := httptest.NewRecorder()
	h.HandleSecuritySummary(rec, httptest.NewRequest(http.MethodGet, "/api/admin/security/summary", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"legacyGatewayPlaintext":0`) || strings.Contains(rec.Body.String(), "keyHash") {
		t.Fatalf("unexpected or sensitive security summary: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminAPIKeySupportsAccountTypeID(t *testing.T) {
	file, err := os.CreateTemp("", "admin-key-type-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	defer os.Remove(file.Name())
	database, err := db.OpenDatabase(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	h := NewAdminHandler(db.NewRepo(database))
	req := httptest.NewRequest(http.MethodPost, "/api/keys", strings.NewReader(`{"name":"user-key","accountTypeId":"user"}`))
	created := httptest.NewRecorder()
	h.HandleCreateKey(created, req)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var response struct {
		ID            string `json:"id"`
		Key           string `json:"key"`
		AccountTypeID string `json:"accountTypeId"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.AccountTypeID != "user" {
		t.Fatalf("expected accountTypeId 'user', got %q", response.AccountTypeID)
	}

	var storedType string
	if err := database.QueryRow(`SELECT accountTypeId FROM apiKeys WHERE id = ?`, response.ID).Scan(&storedType); err != nil {
		t.Fatal(err)
	}
	if storedType != "user" {
		t.Fatalf("expected stored accountTypeId 'user', got %q", storedType)
	}

	// Update accountTypeId
	updateReq := httptest.NewRequest(http.MethodPut, "/api/keys/"+response.ID, strings.NewReader(`{"accountTypeId":"paid_user"}`))
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", response.ID)
	updateReq = updateReq.WithContext(context.WithValue(updateReq.Context(), chi.RouteCtxKey, chiCtx))
	updated := httptest.NewRecorder()
	h.HandleUpdateKey(updated, updateReq)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}

	if err := database.QueryRow(`SELECT accountTypeId FROM apiKeys WHERE id = ?`, response.ID).Scan(&storedType); err != nil {
		t.Fatal(err)
	}
	if storedType != "paid_user" {
		t.Fatalf("expected updated stored accountTypeId 'paid_user', got %q", storedType)
	}
}

func setupAdminTestDB(t *testing.T) (*db.Repo, func()) {
	file, err := os.CreateTemp("", "admin-test-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	database, err := db.OpenDatabase(file.Name())
	if err != nil {
		os.Remove(file.Name())
		t.Fatal(err)
	}
	repo := db.NewRepo(database)
	cleanup := func() {
		database.Close()
		os.Remove(file.Name())
	}
	return repo, cleanup
}

func TestHandleTestModelAlias(t *testing.T) {
	repo, cleanup := setupAdminTestDB(t)
	defer cleanup()
	if err := repo.SetModelAliasRecord("test-alias", "google", "gemini-2.5-pro", nil, nil, 1); err != nil {
		t.Fatal(err)
	}
	h := NewAdminHandler(repo)

	testReq := httptest.NewRequest(http.MethodPost, "/api/model-aliases/test-alias/test", nil)
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("alias", "test-alias")
	testReq = testReq.WithContext(context.WithValue(testReq.Context(), chi.RouteCtxKey, chiCtx))
	rec := httptest.NewRecorder()
	h.HandleTestModelAlias(rec, testReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("test alias status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"resolved":true`) || !strings.Contains(rec.Body.String(), `"google"`) {
		t.Fatalf("unexpected alias test body: %s", rec.Body.String())
	}
}

func TestHandleFetchProviderConnectionModels_IncludesCatalog(t *testing.T) {
	repo, cleanup := setupAdminTestDB(t)
	defer cleanup()

	h := NewAdminHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/providers/gemini/models", nil)
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", "gemini")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
	rec := httptest.NewRecorder()

	h.HandleFetchProviderConnectionModels(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Provider string           `json:"provider"`
		Models   []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if len(payload.Models) == 0 {
		t.Fatalf("expected models from catalog, got 0")
	}
	found := false
	for _, m := range payload.Models {
		if id, _ := m["id"].(string); strings.Contains(id, "gemini") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected gemini model in models list: %s", rec.Body.String())
	}
}

func TestHandleTestProviderModel_Validation(t *testing.T) {
	repo, cleanup := setupAdminTestDB(t)
	defer cleanup()

	h := NewAdminHandler(repo)

	// Missing model should return 400
	req := httptest.NewRequest(http.MethodPost, "/api/providers/gemini/test-model", strings.NewReader(`{"model":""}`))
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add("id", "gemini")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, chiCtx))
	rec := httptest.NewRecorder()

	h.HandleTestProviderModel(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty model, got %d", rec.Code)
	}
}

func TestHandleSetModelAliasRejectsCaseInsensitiveCollision(t *testing.T) {
	repo, cleanup := setupAdminTestDB(t)
	defer cleanup()
	if err := repo.SetModelAliasRecord("Fast-Gemini", "gemini", "gemini-2.5-pro", nil, nil, 1); err != nil {
		t.Fatal(err)
	}

	h := NewAdminHandler(repo)
	req := httptest.NewRequest(http.MethodPost, "/api/model-aliases", strings.NewReader(`{"alias":"fast-gemini","target":"openai/gpt-4o","provider":"openai","upstreamModel":"gpt-4o"}`))
	rec := httptest.NewRecorder()
	h.HandleSetModelAlias(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "model_alias_conflict") {
		t.Fatalf("expected case-insensitive alias collision, status=%d body=%s", rec.Code, rec.Body.String())
	}
}
