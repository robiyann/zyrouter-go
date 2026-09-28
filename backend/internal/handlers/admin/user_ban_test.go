package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHandleSetUserBanUsesTelegramUserID(t *testing.T) {
	repo, cleanup := setupAdminTestDB(t)
	defer cleanup()
	if _, err := repo.RawDB().Exec(`INSERT INTO users (id, telegramUserId, telegramUsername, displayName, accountTypeId, isActive, verifiedAt, createdAt, updatedAt) VALUES ('user-ban-admin', '55443322', 'target', 'Target', 'user', 1, '2026-09-29T00:00:00Z', '2026-09-29T00:00:00Z', '2026-09-29T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	h := NewAdminHandler(repo)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/users/telegram/55443322/ban", strings.NewReader(`{"banned":true}`))
	route := chi.NewRouteContext()
	route.URLParams.Add("telegramUserId", "55443322")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
	rec := httptest.NewRecorder()
	h.HandleSetUserBan(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || response["banned"] != true {
		t.Fatalf("unexpected ban response: %s", rec.Body.String())
	}
	var active int
	if err := repo.RawDB().QueryRow(`SELECT isActive FROM users WHERE telegramUserId=?`, "55443322").Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("expected Telegram user to be inactive after ban, got %d", active)
	}
}
