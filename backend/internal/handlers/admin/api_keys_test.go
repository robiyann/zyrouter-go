package admin

import (
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
