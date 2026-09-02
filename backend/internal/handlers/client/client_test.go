package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"zyrouter/backend/internal/db"
	clientmw "zyrouter/backend/internal/middleware"
)

func setupClientTest(t *testing.T) (*db.Repo, string, func()) {
	t.Helper()
	file, err := os.CreateTemp("", "client-api-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	database, err := db.OpenDatabase(file.Name())
	if err != nil {
		os.Remove(file.Name())
		t.Fatal(err)
	}
	cleanup := func() {
		database.Close()
		os.Remove(file.Name())
	}
	repo := db.NewRepo(database)
	policy, err := repo.CreateClientPolicy("policy-basic", "Basic", map[string]any{"allowedPrefixes": []string{"ag", "ds"}})
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	_ = policy
	accessToken := "clt_test_access_token"
	if _, err := repo.CreateClient("client-1", "Client One", "client@example.invalid", db.HashClientToken(accessToken), "policy-basic"); err != nil {
		cleanup()
		t.Fatal(err)
	}
	return repo, accessToken, cleanup
}

func clientRouter(repo *db.Repo) *chi.Mux {
	h := NewHandler(repo)
	r := chi.NewRouter()
	r.Use(clientmw.RequireClientAccess(repo))
	r.Get("/api/client/profile", h.HandleProfile)
	r.Get("/api/client/policy", h.HandlePolicy)
	r.Get("/api/client/keys", h.HandleKeys)
	r.Post("/api/client/keys", h.HandleCreateKey)
	r.Delete("/api/client/keys/{id}", h.HandleRevokeKey)
	r.Get("/api/client/usage", h.HandleUsage)
	return r
}

func TestClientKeyGenerationInheritsServerPolicy(t *testing.T) {
	repo, token, cleanup := setupClientTest(t)
	defer cleanup()
	r := clientRouter(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/client/keys", strings.NewReader(`{"name":"CLI key","allowedPrefixes":["admin"]}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Key             string   `json:"key"`
		AllowedPrefixes []string `json:"allowedPrefixes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Key == "" || len(response.AllowedPrefixes) != 2 || response.AllowedPrefixes[0] != "ag" {
		t.Fatalf("unexpected generated key response: %+v", response)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/client/keys", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || strings.Contains(listRec.Body.String(), response.Key) {
		t.Fatalf("key listing exposed full key or failed: status=%d body=%s", listRec.Code, listRec.Body.String())
	}
}

func TestClientCanRevokeOnlyOwnedKey(t *testing.T) {
	repo, token, cleanup := setupClientTest(t)
	defer cleanup()
	key, err := repo.CreateClientApiKey("ck-owned", "sk-client-owned", "Owned", "client-1", "policy-basic", `{"allowedPrefixes":["ag"]}`)
	if err != nil {
		t.Fatal(err)
	}
	r := clientRouter(repo)
	req := httptest.NewRequest(http.MethodDelete, "/api/client/keys/"+key.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected revoke 200, got %d: %s", rec.Code, rec.Body.String())
	}
	stored, err := repo.GetApiKeyByKey(key.Key)
	if err != nil || stored == nil || stored.IsActive != 0 {
		t.Fatalf("expected owned key to be inactive: stored=%+v err=%v", stored, err)
	}

	foreign, err := repo.CreateClientApiKey("ck-foreign", "sk-client-foreign", "Foreign", "other-client", "policy-basic", `{"allowedPrefixes":["ag"]}`)
	if err != nil {
		t.Fatal(err)
	}
	foreignReq := httptest.NewRequest(http.MethodDelete, "/api/client/keys/"+foreign.ID, nil)
	foreignReq.Header.Set("Authorization", "Bearer "+token)
	foreignRec := httptest.NewRecorder()
	r.ServeHTTP(foreignRec, foreignReq)
	if foreignRec.Code != http.StatusNotFound {
		t.Fatalf("expected foreign key revoke to be 404, got %d", foreignRec.Code)
	}
}

func TestInvalidClientTokenIsRejected(t *testing.T) {
	repo, _, cleanup := setupClientTest(t)
	defer cleanup()
	r := clientRouter(repo)
	req := httptest.NewRequest(http.MethodGet, "/api/client/keys", nil)
	req.Header.Set("Authorization", "Bearer clt_invalid")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid client token to be rejected, got %d", rec.Code)
	}
}

func TestClientPolicyAndUsageAreScoped(t *testing.T) {
	repo, token, cleanup := setupClientTest(t)
	defer cleanup()
	if _, err := repo.CreateClientApiKey("ck-usage", "sk-client-usage", "Usage", "client-1", "policy-basic", `{"allowedPrefixes":["ag"]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateClientApiKey("ck-other", "sk-client-other", "Other", "client-2", "policy-basic", `{"allowedPrefixes":["ag"]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetClientPolicy("policy-basic"); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertUsageHistory("test", "model", "", "sk-client-usage", "/chat/completions", 10, 5, 0.01, "200", 15, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertUsageHistory("test", "model", "", "sk-client-other", "/chat/completions", 100, 100, 1.0, "200", 200, "", ""); err != nil {
		t.Fatal(err)
	}

	policyReq := httptest.NewRequest(http.MethodGet, "/api/client/policy", nil)
	policyReq.Header.Set("Authorization", "Bearer "+token)
	policyRec := httptest.NewRecorder()
	clientRouter(repo).ServeHTTP(policyRec, policyReq)
	if policyRec.Code != http.StatusOK || !strings.Contains(policyRec.Body.String(), `"allowedPrefixes":["ag","ds"]`) {
		t.Fatalf("unexpected policy response: %d %s", policyRec.Code, policyRec.Body.String())
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/client/usage", nil)
	usageReq.Header.Set("Authorization", "Bearer "+token)
	usageRec := httptest.NewRecorder()
	clientRouter(repo).ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK || !strings.Contains(usageRec.Body.String(), `"totalTokens":15`) {
		t.Fatalf("usage was not isolated to client-1: %d %s", usageRec.Code, usageRec.Body.String())
	}
}
