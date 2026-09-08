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

	start := httptest.NewRecorder()
	r.ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/verify/start", nil))
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

	create := httptest.NewRequest(http.MethodPost, "/key", strings.NewReader(`{}`))
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
	if err != nil || oldKey == nil || oldKey.IsActive != 0 {
		t.Fatalf("old key remained active after rotation: %+v err=%v", oldKey, err)
	}
	if active, err := repo.GetActiveUserApiKey("usr_invalid"); err != nil || active != nil {
		t.Fatalf("unexpected invalid user key lookup: %+v %v", active, err)
	}

	// Test re-login with existing Telegram user
	reloginStart := httptest.NewRecorder()
	r.ServeHTTP(reloginStart, httptest.NewRequest(http.MethodPost, "/verify/start", nil))
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
