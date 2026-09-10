package oauth

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"zyrouter/backend/internal/db"
)

func setupOAuthTestDB(t *testing.T) (*sql.DB, func()) {
	tmpFile, err := os.CreateTemp("", "test_oauth_*.sqlite")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	database, err := db.OpenDatabase(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("OpenDatabase failed: %v", err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS providerConnections (
		id TEXT PRIMARY KEY,
		provider TEXT NOT NULL,
		authType TEXT NOT NULL,
		name TEXT,
		isActive INTEGER DEFAULT 1,
		data TEXT NOT NULL,
		createdAt TEXT,
		updatedAt TEXT
	);`
	if _, err := database.Exec(schema); err != nil {
		database.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("exec schema failed: %v", err)
	}

	cleanup := func() {
		database.Close()
		os.Remove(tmpFile.Name())
	}
	return database, cleanup
}

func TestHandleOAuthImport_missingProvider(t *testing.T) {
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("POST", "/api/oauth//import", strings.NewReader(`{"accessToken":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.HandleOAuthImport(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleOAuthImport_missingToken(t *testing.T) {
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("POST", "/api/oauth/codex/import", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.HandleOAuthImport(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleOAuthImport_codex(t *testing.T) {
	database, cleanup := setupOAuthTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	handler := NewOAuthHandler(repo)

	body := `{"accessToken":"sk-codex-test","name":"My Codex"}`
	req := httptest.NewRequest("POST", "/api/oauth/codex/import", strings.NewReader(body))
	req.SetPathValue("provider", "codex")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleOAuthImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["provider"] != "codex" {
		t.Errorf("expected provider=codex, got %v", resp["provider"])
	}
	var authType, data string
	if err := database.QueryRow(`SELECT authType, data FROM providerConnections WHERE provider = 'codex'`).Scan(&authType, &data); err != nil {
		t.Fatalf("query imported connection: %v", err)
	}
	if authType != "oauth" {
		t.Fatalf("expected oauth authType, got %q", authType)
	}
	if !strings.Contains(data, `"accessToken":"sk-codex-test"`) {
		t.Fatalf("expected accessToken in imported data: %s", data)
	}
}

func TestHandleOAuthAuthorize_cline(t *testing.T) {
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("GET", "/api/oauth/cline/authorize?redirect_uri=http%3A%2F%2Flocalhost%3A20128%2Fcallback", nil)
	req.SetPathValue("provider", "cline")
	rec := httptest.NewRecorder()

	handler.HandleOAuthAuthorize(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	authURL, _ := response["authUrl"].(string)
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	if parsed.Host != "api.cline.bot" || parsed.Path != "/api/v1/auth/authorize" {
		t.Fatalf("unexpected Cline auth URL: %s", authURL)
	}
	if got := parsed.Query().Get("client_type"); got != "extension" {
		t.Errorf("client_type = %q, want extension", got)
	}
}

func TestHandleOAuthExchange_clineBase64(t *testing.T) {
	database, cleanup := setupOAuthTestDB(t)
	defer cleanup()
	handler := NewOAuthHandler(db.NewRepo(database))

	payload, err := json.Marshal(map[string]any{
		"accessToken":  "cline-access",
		"refreshToken": "cline-refresh",
		"email":        "cline@example.com",
		"expiresAt":    "2027-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	code := base64.RawURLEncoding.EncodeToString(payload)
	req := httptest.NewRequest("POST", "/api/oauth/cline/exchange", strings.NewReader(`{"code":"`+code+`","name":"Cline Test"}`))
	req.SetPathValue("provider", "cline")
	rec := httptest.NewRecorder()
	handler.HandleOAuthExchange(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var authType, data string
	if err := database.QueryRow(`SELECT authType, data FROM providerConnections WHERE provider = 'cline'`).Scan(&authType, &data); err != nil {
		t.Fatalf("query Cline connection: %v", err)
	}
	if authType != "oauth" || !strings.Contains(data, `"refreshToken":"cline-refresh"`) {
		t.Fatalf("unexpected Cline connection: authType=%q data=%s", authType, data)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHandleOAuthDeviceCodeAndPoll_grokCLI(t *testing.T) {
	database, cleanup := setupOAuthTestDB(t)
	defer cleanup()
	handler := NewOAuthHandler(db.NewRepo(database))

	previousClient := oauthHTTPClient
	defer func() { oauthHTTPClient = previousClient }()
	var mu sync.Mutex
	var calls []string
	oauthHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		calls = append(calls, r.URL.String())
		mu.Unlock()
		if r.URL.String() == grokCLIDeviceURL {
			return jsonResponse(http.StatusOK, `{"device_code":"device-1","user_code":"ABCD","verification_uri":"https://x.ai/verify","interval":1}`), nil
		}
		if r.URL.String() == grokCLITokenURL {
			return jsonResponse(http.StatusOK, `{"access_token":"grok-access","refresh_token":"grok-refresh","expires_in":3600,"id_token":"id-token"}`), nil
		}
		if r.URL.String() == grokCLIUserURL {
			return jsonResponse(http.StatusOK, `{"email":"grok@example.com","userId":"user-1","subscriptionTier":"pro"}`), nil
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	})}

	deviceReq := httptest.NewRequest("GET", "/api/oauth/grok-cli/device-code", nil)
	deviceReq.SetPathValue("provider", "grok-cli")
	deviceRec := httptest.NewRecorder()
	handler.HandleOAuthDeviceCode(deviceRec, deviceReq)
	if deviceRec.Code != http.StatusOK || !strings.Contains(deviceRec.Body.String(), `"device_code":"device-1"`) {
		t.Fatalf("unexpected device response: %d %s", deviceRec.Code, deviceRec.Body.String())
	}

	pollReq := httptest.NewRequest("POST", "/api/oauth/grok-cli/poll", strings.NewReader(`{"deviceCode":"device-1","name":"Grok Test"}`))
	pollReq.SetPathValue("provider", "grok-cli")
	pollRec := httptest.NewRecorder()
	handler.HandleOAuthDevicePoll(pollRec, pollReq)
	if pollRec.Code != http.StatusOK || !strings.Contains(pollRec.Body.String(), `"success":true`) {
		t.Fatalf("unexpected poll response: %d %s", pollRec.Code, pollRec.Body.String())
	}
	var provider, data string
	if err := database.QueryRow(`SELECT provider, data FROM providerConnections WHERE provider = 'grok-cli'`).Scan(&provider, &data); err != nil {
		t.Fatalf("query Grok connection: %v", err)
	}
	if provider != "grok-cli" || !strings.Contains(data, `"refreshToken":"grok-refresh"`) || !strings.Contains(data, `"email":"grok@example.com"`) || !strings.Contains(data, `"expiresAt"`) {
		t.Fatalf("unexpected Grok connection: %s", data)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("expected device, token, and profile calls; got %v", calls)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestHandleOAuthKiroSocialAuthorize_invalidProvider(t *testing.T) {
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("GET", "/api/oauth/kiro/social-authorize?provider=twitter", nil)
	rec := httptest.NewRecorder()
	handler.HandleOAuthKiroSocialAuthorize(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleOAuthKiroSocialAuthorize_google(t *testing.T) {
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("GET", "/api/oauth/kiro/social-authorize?provider=google", nil)
	rec := httptest.NewRecorder()
	handler.HandleOAuthKiroSocialAuthorize(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["provider"] != "google" {
		t.Errorf("expected provider=google, got %v", resp["provider"])
	}
	if resp["authUrl"] == nil {
		t.Error("expected authUrl")
	}
	if resp["codeVerifier"] == nil {
		t.Error("expected codeVerifier")
	}
}

func TestHandleOAuthKiroSocialExchange_missingCode(t *testing.T) {
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("POST", "/api/oauth/kiro/social-exchange", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.HandleOAuthKiroSocialExchange(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleOAuthCodexBulkImport(t *testing.T) {
	database, cleanup := setupOAuthTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	handler := NewOAuthHandler(repo)

	body := `{"tokens":[{"accessToken":"token-one"},{"accessToken":"token-two-longer"}]}`
	req := httptest.NewRequest("POST", "/api/oauth/codex/bulk-import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleOAuthCodexBulkImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &resp)
	count, _ := resp["count"].(float64)
	if count != 2 {
		t.Errorf("expected count=2, got %v", count)
	}
}

func TestTitleProvider(t *testing.T) {
	if titleProvider("google") != "Google" {
		t.Errorf("expected Google, got %s", titleProvider("google"))
	}
	if titleProvider("github") != "GitHub" {
		t.Errorf("expected GitHub, got %s", titleProvider("github"))
	}
	if titleProvider("other") != "Other" {
		t.Errorf("expected Other, got %s", titleProvider("other"))
	}
}
