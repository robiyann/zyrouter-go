package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"zyrouter/backend/internal/auth"
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

func TestAuthLogoutRejectsCrossOriginBrowserRequest(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	r := chi.NewRouter()
	SetupServerRouter(r, db.NewRepo(database), nil)
	req := httptest.NewRequest(http.MethodPost, "http://panel.example.invalid/api/auth/logout", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin logout to be rejected, got %d: %s", rec.Code, rec.Body.String())
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
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/keys"},
		{http.MethodGet, "/api/usage/stats"},
		{http.MethodGet, "/api/audit-logs/files"},
		{http.MethodGet, "/api/audit-logs/files/audit-2026-10-02-0001.jsonl"},
		{http.MethodDelete, "/api/audit-logs/files"},
		{http.MethodDelete, "/api/audit-logs/files/audit-2026-10-02-0001.jsonl"},
		{http.MethodGet, "/api/auth-logs"},
		{http.MethodGet, "/api/system/overview"},
		{http.MethodPost, "/proxy-pools/vercel-deploy"},
	} {
		req := httptest.NewRequest(route.method, "http://example.invalid"+route.path, nil)
		req.Header.Set("Authorization", "Bearer sk-client-boundary")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected client key to be denied %s %s, got %d: %s", route.method, route.path, rec.Code, rec.Body.String())
		}
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

func TestUserApiKeyCannotAccessAdminRoutes(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	userID := "user-admin-boundary"
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.Exec(`INSERT INTO users (id,telegramUserId,telegramUsername,displayName,accountTypeId,isActive,verifiedAt,createdAt,updatedAt) VALUES (?,?,?,?,?,1,?,?,?)`, userID, "tg-admin-boundary", "boundary", "Boundary User", "user", now, now, now); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUserApiKey(userID, "user", "uk-boundary", "sk-user-boundary", "User Key"); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	SetupServerRouter(r, repo, nil)
	req := httptest.NewRequest(http.MethodGet, "http://example.invalid/api/keys", nil)
	req.Header.Set("Authorization", "Bearer sk-user-boundary")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected user key to be denied admin route, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestApiKeyWithAdministratorTierCannotAccessAdminOrAuditRoutes(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)

	// Create an API key with accountTypeId: "administrator" (e.g. zy_fe004beb...)
	keyStr := "zy_fe004beb071ff1b5853fe6695b282c06"
	if _, err := repo.CreateApiKey("key-admin-tier", keyStr, "Admin Tier Test Key", "mac-1", "administrator", nil); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	SetupServerRouter(r, repo, nil)

	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodDelete, "/api/audit-logs/files"},
		{http.MethodDelete, "/api/audit-logs/files/audit-2026-10-02-0001.jsonl"},
		{http.MethodGet, "/api/audit-logs/files"},
		{http.MethodGet, "/api/keys"},
		{http.MethodGet, "/api/settings/database"},
		{http.MethodGet, "/api/auth-logs"},
		{http.MethodGet, "/api/system/overview"},
		{http.MethodPost, "/proxy-pools/vercel-deploy"},
	} {
		req := httptest.NewRequest(route.method, "http://example.invalid"+route.path, nil)
		req.Header.Set("Authorization", "Bearer "+keyStr)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected admin-tier API key to be denied %s %s, got %d: %s", route.method, route.path, rec.Code, rec.Body.String())
		}
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

func TestLoginReportsRemainingAttemptsAndLockout(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	settings, err := repo.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	hash := auth.HashPassword("correct-password")
	settings.Password = &hash
	if err := repo.UpdateSettingsData(settings); err != nil {
		t.Fatal(err)
	}
	handler := HandleAuthLogin(repo)
	ip := "198.51.100.77"
	for want := 4; want >= 1; want-- {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"wrong-password"}`))
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized || rec.Header().Get("X-Login-Attempts-Remaining") != strconv.Itoa(want) {
			t.Fatalf("attempt %d: status=%d remaining=%q", 5-want, rec.Code, rec.Header().Get("X-Login-Attempts-Remaining"))
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"wrong-password"}`))
	req.RemoteAddr = ip + ":5678"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("X-Login-Attempts-Remaining") != "0" || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("lockout response missing metadata: status=%d remaining=%q retry=%q", rec.Code, rec.Header().Get("X-Login-Attempts-Remaining"), rec.Header().Get("Retry-After"))
	}
}

func TestAuthLogin_ExternalSpoofedHeadersCannotBypassLockout(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	settings, err := repo.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	hash := auth.HashPassword("secure-master-password")
	settings.Password = &hash
	if err := repo.UpdateSettingsData(settings); err != nil {
		t.Fatal(err)
	}
	handler := HandleAuthLogin(repo)

	// An external attacker attempting to brute-force while rotating X-Real-IP headers
	externalIP := "203.0.113.88"
	for attempt := 1; attempt <= 4; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"bad-password"}`))
		req.RemoteAddr = externalIP + ":45678"
		req.Header.Set("X-Real-IP", fmt.Sprintf("10.0.0.%d", attempt))
		req.Header.Set("CF-Connecting-IP", fmt.Sprintf("1.1.1.%d", attempt))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		wantRemaining := strconv.Itoa(5 - attempt)
		if rec.Code != http.StatusUnauthorized || rec.Header().Get("X-Login-Attempts-Remaining") != wantRemaining {
			t.Fatalf("attempt %d: status=%d remaining=%q (expected %s)", attempt, rec.Code, rec.Header().Get("X-Login-Attempts-Remaining"), wantRemaining)
		}
	}

	// 5th attempt from same socket IP with yet another spoofed IP must trigger lockout
	lockoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"bad-password"}`))
	lockoutReq.RemoteAddr = externalIP + ":45678"
	lockoutReq.Header.Set("X-Real-IP", "10.0.0.99")
	lockoutReq.Header.Set("CF-Connecting-IP", "1.1.1.99")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, lockoutReq)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("attacker bypassed lockout via spoofed headers; status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Login-Attempts-Remaining") != "0" {
		t.Fatalf("expected 0 attempts remaining, got %q", rec.Header().Get("X-Login-Attempts-Remaining"))
	}
}

func TestAuthLogin_ProxyPreservesClientLockoutSeparation(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	settings, err := repo.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	hash := auth.HashPassword("secure-master-password")
	settings.Password = &hash
	if err := repo.UpdateSettingsData(settings); err != nil {
		t.Fatal(err)
	}
	handler := HandleAuthLogin(repo)

	// Legitimate requests arriving through Cloudflare Tunnel (127.0.0.1)
	proxyAddr := "127.0.0.1:8080"
	client1 := "198.51.100.91"
	client2 := "198.51.100.92"

	// Lock out Client 1
	for attempt := 1; attempt <= 5; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"wrong"}`))
		req.RemoteAddr = proxyAddr
		req.Header.Set("CF-Connecting-IP", client1)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Client 1 should now be locked
	reqBlocked := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"wrong"}`))
	reqBlocked.RemoteAddr = proxyAddr
	reqBlocked.Header.Set("CF-Connecting-IP", client1)
	recBlocked := httptest.NewRecorder()
	handler.ServeHTTP(recBlocked, reqBlocked)
	if recBlocked.Code != http.StatusTooManyRequests {
		t.Fatalf("client1 should be locked out, got status=%d", recBlocked.Code)
	}

	// Client 2 arriving through the same reverse proxy must NOT be locked out
	reqClean := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"wrong"}`))
	reqClean.RemoteAddr = proxyAddr
	reqClean.Header.Set("CF-Connecting-IP", client2)
	recClean := httptest.NewRecorder()
	handler.ServeHTTP(recClean, reqClean)
	if recClean.Code != http.StatusUnauthorized || recClean.Header().Get("X-Login-Attempts-Remaining") != "4" {
		t.Fatalf("client2 should have 4 attempts remaining, got code=%d remaining=%q", recClean.Code, recClean.Header().Get("X-Login-Attempts-Remaining"))
	}
}
