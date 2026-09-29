package db

import (
	"os"
	"testing"
)

func TestListUsersPageBoundsRowsAndJoinsActiveKey(t *testing.T) {
	file, err := os.CreateTemp("", "users-page-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	_ = file.Close()
	defer os.Remove(path)

	database, err := OpenDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repo := NewRepo(database)
	for i, telegramID := range []string{"1001", "1002", "1003"} {
		created := "2026-09-29T00:0" + string(rune('0'+i)) + ":00Z"
		if _, err := database.Exec(`INSERT INTO users (id, telegramUserId, telegramUsername, displayName, accountTypeId, isActive, verifiedAt, createdAt, updatedAt) VALUES (?, ?, ?, ?, 'user', 1, ?, ?, ?)`,
			"page-user-"+telegramID, telegramID, "user"+telegramID, "User "+telegramID, created, created, created); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreateUserApiKey("page-user-1003", "user", "page-key", "page-secret", "Page key"); err != nil {
		t.Fatal(err)
	}

	rows, total, err := repo.ListUsersPage(1, 2, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(rows) != 2 {
		t.Fatalf("expected total=3 and 2 bounded rows, got total=%d rows=%d", total, len(rows))
	}
	if rows[0].User.TelegramUserID != "1003" || !rows[0].HasActiveKey || rows[0].KeyPrefix == "" {
		t.Fatalf("expected newest user with active-key projection, got %+v", rows[0])
	}
	filtered, filteredTotal, err := repo.ListUsersPage(1, 25, "1002", "", "")
	if err != nil || filteredTotal != 1 || len(filtered) != 1 || filtered[0].User.TelegramUserID != "1002" {
		t.Fatalf("expected server-side search result, total=%d rows=%+v err=%v", filteredTotal, filtered, err)
	}
}
