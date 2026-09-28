package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"zyrouter/backend/internal/db"
)

func TestRequireUserSessionReturnsUserBanned(t *testing.T) {
	file, err := os.CreateTemp("", "user-session-ban-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	_ = file.Close()
	defer os.Remove(path)

	database, err := db.OpenDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repo := db.NewRepo(database)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.Exec(`INSERT INTO users (id, telegramUserId, telegramUsername, displayName, accountTypeId, isActive, verifiedAt, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?)`,
		"banned-session-user", "44556677", "banned", "Banned", "user", now, now, now); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUserSession("banned-session-user", "banned-session-token", time.Hour); err != nil {
		t.Fatal(err)
	}

	handler := RequireUserSession(repo)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/user/profile", nil)
	req.AddCookie(&http.Cookie{Name: "user_session", Value: "banned-session-token"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for banned user session, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"code":"user_banned"`) {
		t.Fatalf("expected user_banned error code, got %s", rec.Body.String())
	}
}
