package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"zyrouter/backend/internal/db"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	tmpFile, err := os.CreateTemp("", "test_router_*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	database, err := db.OpenDatabase(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("OpenDatabase failed: %v", err)
	}

	cleanup := func() {
		database.Close()
		os.Remove(tmpFile.Name())
	}
	return database, cleanup
}

func TestSetupRoutes(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	repo := db.NewRepo(database)
	r := chi.NewRouter()
	SetupRoutes(r, repo, nil)

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/chat/completions"}, {http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/messages"}, {http.MethodPost, "/v1/messages"},
		{http.MethodGet, "/models"}, {http.MethodGet, "/v1/models"},
		{http.MethodGet, "/models/info"}, {http.MethodGet, "/v1/models/info"},
	} {
		req := httptest.NewRequest(route.method, route.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code == http.StatusMethodNotAllowed || w.Code == http.StatusNotFound {
			t.Errorf("expected %s %s route to be registered, got status %d", route.method, route.path, w.Code)
		}
	}

	for _, path := range []string{"/images/generations", "/headroom/status", "/api/mitm/status", "/cli-tools/all-statuses"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
			t.Errorf("expected retired route %s to be unavailable, got status %d", path, w.Code)
		}
	}
	for _, path := range []string{"/models/image", "/v1/models/embedding", "/models/video"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("expected retired model kind %s to be unavailable, got status %d", path, w.Code)
		}
	}
}

func TestClientApiBoundaryRequiresClientToken(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	r := chi.NewRouter()
	SetupServerRouter(r, db.NewRepo(database), nil)

	for _, path := range []string{"/api/client/profile", "/api/client/keys", "/api/client/usage"} {
		req := httptest.NewRequest(http.MethodGet, "http://example.invalid"+path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected %s to require client token, got %d", path, w.Code)
		}
	}
}

func TestClientApiKeyCannotAccessAdminRoutes(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	if _, err := repo.CreateClientPolicy("policy-admin-boundary", "Basic", map[string]any{"allowedPrefixes": []string{"ds"}}); err != nil {
		t.Fatal(err)
	}
	clientToken := "clt_boundary_token"
	if _, err := repo.CreateClient("client-boundary", "Boundary Client", "", db.HashClientToken(clientToken), "policy-admin-boundary"); err != nil {
		t.Fatal(err)
	}
	key, err := repo.CreateClientApiKey("ck-boundary", "sk-client-boundary", "Client Key", "client-boundary", "policy-admin-boundary", `{"allowedPrefixes":["ds"]}`)
	if err != nil {
		t.Fatal(err)
	}
	_ = key

	r := chi.NewRouter()
	SetupServerRouter(r, repo, nil)
	req := httptest.NewRequest(http.MethodGet, "http://example.invalid/api/keys", nil)
	req.Header.Set("Authorization", "Bearer sk-client-boundary")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected client key to be denied admin route, got %d: %s", rec.Code, rec.Body.String())
	}
	proxyReq := httptest.NewRequest(http.MethodGet, "http://example.invalid/models", nil)
	proxyReq.Header.Set("Authorization", "Bearer sk-client-boundary")
	proxyRec := httptest.NewRecorder()
	r.ServeHTTP(proxyRec, proxyReq)
	if proxyRec.Code == http.StatusForbidden {
		t.Fatalf("client API key should remain usable on proxy/model routes, got %d", proxyRec.Code)
	}

	clientReq := httptest.NewRequest(http.MethodGet, "http://example.invalid/api/client/profile", nil)
	clientReq.Header.Set("Authorization", "Bearer "+clientToken)
	clientRec := httptest.NewRecorder()
	r.ServeHTTP(clientRec, clientReq)
	if clientRec.Code != http.StatusOK {
		t.Fatalf("expected client token to access client route, got %d: %s", clientRec.Code, clientRec.Body.String())
	}
	var profile map[string]any
	if err := json.Unmarshal(clientRec.Body.Bytes(), &profile); err != nil || profile["id"] != "client-boundary" {
		t.Fatalf("unexpected client profile: %s", clientRec.Body.String())
	}
}

func TestExpiredClientKeyCannotAccessModels(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	expired := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	policy := `{"expiresAt":"` + expired + `"}`
	if _, err := db.NewRepo(database).CreateClientApiKey("ck-expired", "sk-client-expired", "Expired", "client-expired", "policy-expired", policy); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	SetupServerRouter(r, db.NewRepo(database), nil)
	req := httptest.NewRequest(http.MethodGet, "http://example.invalid/models", nil)
	req.Header.Set("Authorization", "Bearer sk-client-expired")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected expired key to be rejected globally, got %d", rec.Code)
	}
}

func TestLoginRequiresExplicitDashboardPassword(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"123456"}`))
	rec := httptest.NewRecorder()
	HandleAuthLogin(db.NewRepo(database))(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected default password to be rejected, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "123456") {
		t.Fatalf("login error should not disclose a default password: %s", rec.Body.String())
	}
}

func TestLoginRejectsNonObjectOrMissingPasswordBody(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	handler := HandleAuthLogin(db.NewRepo(database))
	for _, body := range []string{"null", `"string"`, "[]", `{"wrong":""}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s: expected 400, got %d", body, rec.Code)
		}
	}
}
