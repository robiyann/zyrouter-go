package chat

import (
	"encoding/json"
	"net/http"
	"testing"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/models"
	"zyrouter/backend/internal/translator"
)

func TestGetBestConnection_InheritsProviderProxyStrategy(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)

	pool, err := repo.InsertProxyPool(db.ProxyPoolData{
		Name: "opencode-proxy", ProxyURL: "http://proxy.example.com:8080", Type: "http",
	})
	if err != nil {
		t.Fatalf("insert pool: %v", err)
	}
	poolID := pool["id"].(string)
	settings, _ := json.Marshal(map[string]any{"providerStrategies": map[string]any{
		"opencode": map[string]any{"proxyPoolId": poolID, "rotateStrategy": "none"},
	}})
	if err := repo.SaveSettings(&models.Setting{ID: 1, Data: string(settings)}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO providerConnections (id, provider, authType, name, priority, isActive, data, createdAt, updatedAt) VALUES (?, 'opencode', 'apikey', 'OpenCode', 1, 1, ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`,
		"opencode-1", `{"apiKey":"test"}`); err != nil {
		t.Fatalf("insert connection: %v", err)
	}

	h := NewChatHandler(repo)
	_, connData, err := h.GetBestConnection("opencode", "", nil, "")
	if err != nil {
		t.Fatalf("GetBestConnection failed: %v", err)
	}
	if connData.ProxyPoolID != poolID {
		t.Fatalf("expected provider proxy pool %q, got %q", poolID, connData.ProxyPoolID)
	}
	client := h.GetClientForConnection(connData)
	if client == h.Client {
		t.Fatal("expected inherited provider proxy to create a proxied client")
	}
}

func TestLogUsage_ReportsResolvedProxyPoolForNoAuth(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	pool, err := repo.InsertProxyPool(db.ProxyPoolData{
		Name: "usage-relay", ProxyURL: "https://relay.example.com", Type: "vercel",
	})
	if err != nil {
		t.Fatalf("insert pool: %v", err)
	}
	poolID := pool["id"].(string)

	h := NewChatHandler(repo)
	h.LogUsage(&UsageLogInfo{
		Provider: "opencode", Model: "mimo-v2.5-free", ConnectionID: "noauth",
		ProxyPoolID: poolID, APIKey: "public", Endpoint: "/v1/chat/completions",
	}, &translator.OpenAIUsage{PromptTokens: 1, CompletionTokens: 1}, 1, []byte(`{"messages":[]}`), nil)

	var data string
	if err := database.QueryRow(`SELECT data FROM requestDetails WHERE provider = 'opencode' AND model = 'mimo-v2.5-free' ORDER BY timestamp DESC LIMIT 1`).Scan(&data); err != nil {
		t.Fatalf("query request detail: %v", err)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(data), &detail); err != nil {
		t.Fatalf("decode request detail: %v", err)
	}
	if got := detail["proxy"]; got != "usage-relay (VERCEL)" {
		t.Fatalf("expected resolved proxy label, got %v", got)
	}
}

func TestGetClientForConnection_ProxyPool(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()

	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS proxyPools (
		id TEXT PRIMARY KEY,
		isActive INTEGER DEFAULT 1,
		testStatus TEXT,
		data TEXT NOT NULL,
		createdAt TEXT NOT NULL,
		updatedAt TEXT NOT NULL
	);`); err != nil {
		t.Fatalf("failed to create proxyPools table: %v", err)
	}

	repo := db.NewRepo(database)
	pool, err := repo.InsertProxyPool(db.ProxyPoolData{
		Name:     "sg-proxy",
		ProxyURL: "http://user:pass@proxy.example.com:8080",
		Type:     "http",
	})
	if err != nil {
		t.Fatalf("failed to insert proxy pool: %v", err)
	}

	poolID := pool["id"].(string)

	h := &ChatHandler{
		Client: &http.Client{},
		Repo:   repo,
	}

	connData := &ConnectionData{
		ProxyPoolID: poolID,
	}

	client := h.GetClientForConnection(connData)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client == h.Client {
		t.Fatal("expected new client with custom proxy transport, got default client")
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport == nil {
		t.Fatal("expected *http.Transport")
	}

	req, _ := http.NewRequest("GET", "https://api.openai.com/v1/models", nil)
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("proxy resolve error: %v", err)
	}
	if proxyURL == nil || proxyURL.String() != "http://user:pass@proxy.example.com:8080" {
		t.Errorf("expected proxy URL http://user:pass@proxy.example.com:8080, got %v", proxyURL)
	}
}
