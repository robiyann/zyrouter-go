package middleware

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"zyrouter/backend/internal/db"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	tmpFile, err := os.CreateTemp("", "test_middleware_*.sqlite")
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

	schema := []string{
		`CREATE TABLE apiKeys (
			id TEXT PRIMARY KEY,
			key TEXT UNIQUE NOT NULL,
			name TEXT,
			machineId TEXT,
			isActive INTEGER DEFAULT 1,
			createdAt TEXT NOT NULL
		);`,
	}

	for _, query := range schema {
		_, _ = database.Exec(query)
	}
	// Seed key data
	_, err = database.Exec(`INSERT INTO apiKeys (id, key, name, machineId, isActive, createdAt) VALUES
		('1', 'valid-token', 'Test Key 1', 'mac-1', 1, '2026-07-18T00:00:00Z'),
		('2', 'inactive-token', 'Test Key 2', 'mac-2', 0, '2026-07-18T00:00:00Z');`)
	if err != nil {
		cleanup()
		t.Fatalf("failed to seed apiKeys: %v", err)
	}

	return database, cleanup
}

func TestRequireApiKeyMiddleware(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	repo := db.NewRepo(database)
	middleware := RequireApiKey(repo)

	// Mock handler that returns 200 OK
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	testHandler := middleware(okHandler)

	tests := []struct {
		name           string
		setupRequest   func() *http.Request
		expectedStatus int
	}{
		{
			name: "Valid key in Authorization header",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "http://example.com/v1/chat/completions", nil)
				req.Header.Set("Authorization", "Bearer valid-token")
				return req
			},
			expectedStatus: http.StatusOK,
		},
		{
			// Query-param keys are intentionally not supported (they leak via
			// referrers/history); header auth is required.
			name: "Key in query parameter is rejected",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("GET", "http://example.com/v1/chat/completions?key=valid-token", nil)
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Inactive key in Authorization header",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "http://example.com/v1/chat/completions", nil)
				req.Header.Set("Authorization", "Bearer inactive-token")
				return req
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Missing API Key",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("GET", "http://example.com/v1/chat/completions", nil)
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "Malformed Authorization header",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "http://example.com/v1/chat/completions", nil)
				req.Header.Set("Authorization", "valid-token")
				return req
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			rr := httptest.NewRecorder()
			testHandler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestGetAuthenticatedApiKey(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	repo := db.NewRepo(database)
	middleware := RequireApiKey(repo)

	var retrievedKey string
	var hasKey bool

	mockHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKeyObj := GetAuthenticatedApiKey(r)
		if apiKeyObj != nil {
			retrievedKey = apiKeyObj.Key
			hasKey = true
		}
		w.WriteHeader(http.StatusOK)
	})

	testHandler := middleware(mockHandler)

	req := httptest.NewRequest("GET", "http://example.com/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr := httptest.NewRecorder()

	testHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	if !hasKey {
		t.Error("expected context to contain authenticated API key")
	}

	if retrievedKey != "valid-token" {
		t.Errorf("expected key 'valid-token', got '%s'", retrievedKey)
	}

	// Test case where no key is injected (directly calling mockHandler without middleware)
	reqNoMiddleware := httptest.NewRequest("GET", "http://example.com/v1/chat/completions", nil)
	apiKeyObj := GetAuthenticatedApiKey(reqNoMiddleware)
	if apiKeyObj != nil {
		t.Error("expected GetAuthenticatedApiKey to return nil when no key is injected")
	}
}

func TestRequireApiKey_RejectsUnknownTokenOnLoopback(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	handler := RequireApiKey(db.NewRepo(database))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "http://localhost/v1/models", nil)
	req.Header.Set("Authorization", "Bearer unknown-local-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unknown loopback token to be rejected, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRequireApiKeyRejectsBannedTelegramUser(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	now := "2026-09-29T00:00:00Z"
	if _, err := database.Exec(`INSERT INTO users (id, telegramUserId, telegramUsername, displayName, accountTypeId, isActive, verifiedAt, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		"user-banned-auth", "11223344", "banned", "Banned", "user", now, now, now); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUserApiKey("user-banned-auth", "user", "banned-auth-key", "banned-auth-secret", "Banned user key"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetUserBannedByTelegramID("11223344", true); err != nil {
		t.Fatal(err)
	}

	handler := RequireApiKey(repo)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer banned-auth-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected banned Telegram user's key to be rejected, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestIsLocalRequest_UsesForwardedClientIP(t *testing.T) {
	publicViaNginx := httptest.NewRequest(http.MethodGet, "http://localhost/v1/models", nil)
	publicViaNginx.RemoteAddr = "127.0.0.1:8080"
	publicViaNginx.Header.Set("X-Real-IP", "198.51.100.20")
	if isLocalRequest(publicViaNginx) {
		t.Fatal("public request forwarded by Nginx must not receive local loopback access")
	}

	localViaNginx := httptest.NewRequest(http.MethodGet, "http://localhost/v1/models", nil)
	localViaNginx.RemoteAddr = "127.0.0.1:8080"
	localViaNginx.Header.Set("X-Real-IP", "127.0.0.1")
	if !isLocalRequest(localViaNginx) {
		t.Fatal("loopback client IP should remain local")
	}
}

func TestIsLocalRequest_RejectsPublicHostFromLoopbackProxy(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://panel.zyvenox.tech/v1/models", nil)
	request.RemoteAddr = "127.0.0.1:8080"
	if isLocalRequest(request) {
		t.Fatal("public dashboard host must not receive a loopback grant")
	}
}

func TestIsLocalRequest_RejectsExternalRemoteAddrWithLocalhostHost(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://localhost/v1/models", nil)
	request.RemoteAddr = "203.0.113.50:8080"
	if isLocalRequest(request) {
		t.Fatal("external remote address must not receive local loopback access even with Host: localhost")
	}
}

func TestIsLocalRequest_RejectsSpoofedLoopbackHeaderFromExternalIP(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://localhost/v1/models", nil)
	request.RemoteAddr = "203.0.113.50:8080"
	request.Header.Set("X-Real-IP", "127.0.0.1")
	if isLocalRequest(request) {
		t.Fatal("external remote address must not spoof loopback via X-Real-IP")
	}
}

func TestIsLocalRequest_RejectsCloudflareTunnelAndReverseProxies(t *testing.T) {
	// 1. Cloudflare Tunnel with CF-Connecting-IP
	cfReq := httptest.NewRequest(http.MethodGet, "http://localhost/v1/models", nil)
	cfReq.RemoteAddr = "127.0.0.1:8080"
	cfReq.Header.Set("CF-Connecting-IP", "198.51.100.22")
	if isLocalRequest(cfReq) {
		t.Fatal("Cloudflare Tunnel request with CF-Connecting-IP must not receive loopback access")
	}

	// 2. Cloudflare Tunnel with CF-Ray
	cfRayReq := httptest.NewRequest(http.MethodGet, "http://localhost/v1/models", nil)
	cfRayReq.RemoteAddr = "127.0.0.1:8080"
	cfRayReq.Header.Set("CF-Ray", "8c91a2b3c4d5-SIN")
	if isLocalRequest(cfRayReq) {
		t.Fatal("Cloudflare request with CF-Ray must not receive loopback access")
	}

	// 3. Reverse proxy with X-Forwarded-For
	xffReq := httptest.NewRequest(http.MethodGet, "http://localhost/v1/models", nil)
	xffReq.RemoteAddr = "127.0.0.1:8080"
	xffReq.Header.Set("X-Forwarded-For", "203.0.113.88, 127.0.0.1")
	if isLocalRequest(xffReq) {
		t.Fatal("X-Forwarded-For containing public client IP must not receive loopback access")
	}

	// 4. Reverse proxy with True-Client-IP
	trueIPReq := httptest.NewRequest(http.MethodGet, "http://localhost/v1/models", nil)
	trueIPReq.RemoteAddr = "127.0.0.1:8080"
	trueIPReq.Header.Set("True-Client-IP", "203.0.113.99")
	if isLocalRequest(trueIPReq) {
		t.Fatal("True-Client-IP with public client IP must not receive loopback access")
	}
}

func TestExtractApiKey(t *testing.T) {
	tests := []struct {
		name     string
		req      *http.Request
		expected string
	}{
		{
			name: "Bearer Authorization header",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/", nil)
				r.Header.Set("Authorization", "Bearer test-key-123")
				return r
			}(),
			expected: "test-key-123",
		},
		{
			name: "lowercase bearer",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/", nil)
				r.Header.Set("Authorization", "bearer test-key")
				return r
			}(),
			expected: "test-key",
		},
		{
			name:     "query key param is rejected",
			req:      httptest.NewRequest("GET", "/?key=query-key", nil),
			expected: "",
		},
		{
			name:     "query api_key param is rejected",
			req:      httptest.NewRequest("GET", "/?api_key=alt-key", nil),
			expected: "",
		},
		{
			name:     "query apiKey param is rejected",
			req:      httptest.NewRequest("GET", "/?apiKey=camel-key", nil),
			expected: "",
		},
		{
			name: "X-API-Key header fallback",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/", nil)
				r.Header.Set("X-API-Key", "header-key")
				return r
			}(),
			expected: "header-key",
		},
		{
			name: "Bearer takes priority over X-API-Key header",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/", nil)
				r.Header.Set("Authorization", "Bearer bearer-key")
				r.Header.Set("X-API-Key", "header-key")
				return r
			}(),
			expected: "bearer-key",
		},
		{
			name:     "no auth returns empty",
			req:      httptest.NewRequest("GET", "/", nil),
			expected: "",
		},
		{
			name: "malformed auth with no space",
			req: func() *http.Request {
				r := httptest.NewRequest("GET", "/", nil)
				r.Header.Set("Authorization", "no-space")
				return r
			}(),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractApiKey(tt.req)
			if got != tt.expected {
				t.Errorf("ExtractApiKey() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRequestClientIP_UntrustedProxyCannotSpoof(t *testing.T) {
	externalRemote := "203.0.113.50:45678"

	// 1. Direct external connection with spoofed X-Real-IP
	req1 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req1.RemoteAddr = externalRemote
	req1.Header.Set("X-Real-IP", "1.1.1.1")
	if got := RequestClientIP(req1); got != "203.0.113.50" {
		t.Fatalf("expected real socket IP 203.0.113.50, got spoofed %q", got)
	}

	// 2. Direct external connection with spoofed CF-Connecting-IP
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req2.RemoteAddr = externalRemote
	req2.Header.Set("CF-Connecting-IP", "1.1.1.1")
	if got := RequestClientIP(req2); got != "203.0.113.50" {
		t.Fatalf("expected real socket IP 203.0.113.50, got spoofed %q", got)
	}

	// 3. Direct external connection with spoofed X-Forwarded-For
	req3 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req3.RemoteAddr = externalRemote
	req3.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")
	if got := RequestClientIP(req3); got != "203.0.113.50" {
		t.Fatalf("expected real socket IP 203.0.113.50, got spoofed %q", got)
	}

	// 4. Direct external connection with spoofed True-Client-IP
	req4 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req4.RemoteAddr = externalRemote
	req4.Header.Set("True-Client-IP", "1.1.1.1")
	if got := RequestClientIP(req4); got != "203.0.113.50" {
		t.Fatalf("expected real socket IP 203.0.113.50, got spoofed %q", got)
	}
}

func TestRequestClientIP_TrustedProxyHeaders(t *testing.T) {
	// 1. Loopback proxy (e.g. Cloudflare Tunnel / cloudflared) with CF-Connecting-IP
	req1 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req1.RemoteAddr = "127.0.0.1:8080"
	req1.Header.Set("CF-Connecting-IP", "198.51.100.22")
	if got := RequestClientIP(req1); got != "198.51.100.22" {
		t.Fatalf("expected 198.51.100.22 from CF-Connecting-IP, got %q", got)
	}

	// 2. Loopback proxy (e.g. Nginx/Caddy) with X-Real-IP
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req2.RemoteAddr = "127.0.0.1:8080"
	req2.Header.Set("X-Real-IP", "198.51.100.23")
	if got := RequestClientIP(req2); got != "198.51.100.23" {
		t.Fatalf("expected 198.51.100.23 from X-Real-IP, got %q", got)
	}

	// 3. Loopback proxy with X-Forwarded-For chain
	req3 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req3.RemoteAddr = "127.0.0.1:8080"
	req3.Header.Set("X-Forwarded-For", "198.51.100.24, 127.0.0.1")
	if got := RequestClientIP(req3); got != "198.51.100.24" {
		t.Fatalf("expected 198.51.100.24 from X-Forwarded-For, got %q", got)
	}

	// 4. Private network proxy (e.g. Docker / VPC internal proxy)
	req4 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req4.RemoteAddr = "10.0.0.2:8080"
	req4.Header.Set("X-Real-IP", "198.51.100.25")
	if got := RequestClientIP(req4); got != "198.51.100.25" {
		t.Fatalf("expected 198.51.100.25 from private network proxy, got %q", got)
	}

	// 5. Explicit TRUSTED_PROXIES configuration
	t.Setenv("TRUSTED_PROXIES", "203.0.113.99")
	req5 := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	req5.RemoteAddr = "203.0.113.99:12345"
	req5.Header.Set("X-Real-IP", "198.51.100.26")
	if got := RequestClientIP(req5); got != "198.51.100.26" {
		t.Fatalf("expected 198.51.100.26 via configured TRUSTED_PROXIES, got %q", got)
	}
}
