package user

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/middleware"
)

func TestTelegramVerificationAndOneActiveHashedKey(t *testing.T) {
	file, err := os.CreateTemp("", "user-flow-*.sqlite")
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
	if err := repo.SetModelAlias("mimo-free", "oc/mimo-v2.5-free"); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetAccountTypeModels("user", []string{"mimo-free"}); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(repo)
	r := chi.NewRouter()
	r.Post("/verify/start", h.StartVerification)
	r.Get("/verify/{id}", h.VerificationStatus)
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireUserSession(repo))
		r.Post("/key", h.GenerateKey)
		r.Post("/key/rotate", h.RotateKey)
	})

	const testEdgeSecret = "cf-edge-secret-test-token-2026"
	t.Setenv("CF_EDGE_SHARED_SECRET", testEdgeSecret)

	startReq := httptest.NewRequest(http.MethodPost, "/verify/start", nil)
	startReq.Header.Set("X-Zyrouter-Edge-Secret", testEdgeSecret)
	start := httptest.NewRecorder()
	r.ServeHTTP(start, startReq)
	if start.Code != http.StatusCreated {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body.String())
	}
	var challenge struct {
		ID string `json:"challengeId"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &challenge); err != nil || challenge.ID == "" {
		t.Fatalf("bad challenge: %s", start.Body.String())
	}
	verificationCookie := start.Header().Get("Set-Cookie")
	if _, err := repo.VerifyChallenge(challenge.ID, "12345", "tester", "Test User", "user"); err != nil {
		t.Fatal(err)
	}

	status := httptest.NewRecorder()
	statusReq := httptest.NewRequest(http.MethodGet, "/verify/"+challenge.ID, nil)
	statusReq.Header.Set("X-Zyrouter-Edge-Secret", testEdgeSecret)
	statusReq.Header.Set("Cookie", strings.Split(verificationCookie, ";")[0])
	r.ServeHTTP(status, statusReq)
	if status.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", status.Code, status.Body.String())
	}
	if strings.Contains(status.Body.String(), `"sessionToken"`) {
		t.Fatalf("session must not be exposed to browser JavaScript: %s", status.Body.String())
	}
	cookie := status.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "user_session=") {
		t.Fatalf("missing HttpOnly session cookie: %s", status.Body.String())
	}

	// A client-controlled accountTypeId must be ignored; key tier comes only
	// from the authenticated user's server-side account record.
	create := httptest.NewRequest(http.MethodPost, "/key", strings.NewReader(`{"accountTypeId":"administrator"}`))
	create.Header.Set("Cookie", strings.Split(cookie, ";")[0])
	created := httptest.NewRecorder()
	r.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var keyResponse struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &keyResponse); err != nil || keyResponse.Key == "" {
		t.Fatalf("missing key: %s", created.Body.String())
	}
	stored, err := repo.GetApiKeyByKey(keyResponse.Key)
	if err != nil || stored == nil {
		t.Fatalf("lookup key: %+v %v", stored, err)
	}
	if stored.KeyHash == nil || strings.Contains(stored.Key, keyResponse.Key) {
		t.Fatalf("key was not stored hashed: %+v", stored)
	}
	if stored.AccountTypeID == nil || *stored.AccountTypeID != "user" {
		t.Fatalf("client key inherited an unexpected account type: %+v", stored.AccountTypeID)
	}

	second := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/key", strings.NewReader(`{}`))
	req2.Header.Set("Cookie", strings.Split(cookie, ";")[0])
	r.ServeHTTP(second, req2)
	if second.Code != http.StatusConflict {
		t.Fatalf("expected one-key conflict, got %d: %s", second.Code, second.Body.String())
	}

	rotate := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/key/rotate", strings.NewReader(`{}`))
	req3.Header.Set("Cookie", strings.Split(cookie, ";")[0])
	r.ServeHTTP(rotate, req3)
	if rotate.Code != http.StatusCreated {
		t.Fatalf("rotate status=%d body=%s", rotate.Code, rotate.Body.String())
	}
	oldKey, err := repo.GetApiKeyByKey(keyResponse.Key)
	if err != nil || oldKey != nil {
		t.Fatalf("old key was not deleted after rotation: %+v err=%v", oldKey, err)
	}
	if active, err := repo.GetActiveUserApiKey("usr_invalid"); err != nil || active != nil {
		t.Fatalf("unexpected invalid user key lookup: %+v %v", active, err)
	}

	// Test re-login with existing Telegram user
	reloginStart := httptest.NewRecorder()
	reloginReq := httptest.NewRequest(http.MethodPost, "/verify/start", nil)
	reloginReq.Header.Set("X-Zyrouter-Edge-Secret", testEdgeSecret)
	r.ServeHTTP(reloginStart, reloginReq)
	var reloginChallenge struct {
		ID string `json:"challengeId"`
	}
	_ = json.Unmarshal(reloginStart.Body.Bytes(), &reloginChallenge)
	reloginUser, err := repo.VerifyChallenge(reloginChallenge.ID, "12345", "tester_updated", "Test User Updated", "user")
	if err != nil {
		t.Fatalf("re-login failed for existing telegram user: %v", err)
	}
	if reloginUser.TelegramUsername == nil || *reloginUser.TelegramUsername != "tester_updated" {
		t.Fatalf("re-login username not updated: %+v", reloginUser)
	}
}

func TestEdgeSecretAllowed_FailClosedInProduction(t *testing.T) {
	t.Setenv("NODE_ENV", "production")
	t.Setenv("CF_EDGE_SHARED_SECRET", "")

	req := httptest.NewRequest(http.MethodPost, "/verify/start", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	if edgeSecretAllowed(req) {
		t.Fatal("edgeSecretAllowed must fail-closed in production when CF_EDGE_SHARED_SECRET is unset")
	}
}

func TestEdgeSecretAllowed_RejectsInvalidSecret(t *testing.T) {
	t.Setenv("CF_EDGE_SHARED_SECRET", "correct-secret-token")

	// Missing header
	req1 := httptest.NewRequest(http.MethodPost, "/verify/start", nil)
	if edgeSecretAllowed(req1) {
		t.Fatal("edgeSecretAllowed must reject when header is missing")
	}

	// Wrong header
	req2 := httptest.NewRequest(http.MethodPost, "/verify/start", nil)
	req2.Header.Set("X-Zyrouter-Edge-Secret", "wrong-secret-token")
	if edgeSecretAllowed(req2) {
		t.Fatal("edgeSecretAllowed must reject when secret does not match")
	}

	// Correct header
	req3 := httptest.NewRequest(http.MethodPost, "/verify/start", nil)
	req3.Header.Set("X-Zyrouter-Edge-Secret", "correct-secret-token")
	if !edgeSecretAllowed(req3) {
		t.Fatal("edgeSecretAllowed must accept when secret matches")
	}
}

func TestEdgeSecretAllowed_RejectsExternalClientInDev(t *testing.T) {
	t.Setenv("NODE_ENV", "development")
	t.Setenv("CF_EDGE_SHARED_SECRET", "")

	// Direct connection from external internet IP without edge secret configured
	req := httptest.NewRequest(http.MethodPost, "/verify/start", nil)
	req.RemoteAddr = "203.0.113.88:45678"
	if edgeSecretAllowed(req) {
		t.Fatal("edgeSecretAllowed must reject external socket connection when CF_EDGE_SHARED_SECRET is unset")
	}
}
