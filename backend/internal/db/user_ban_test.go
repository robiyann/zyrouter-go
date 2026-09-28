package db

import (
	"os"
	"testing"
	"time"
)

func TestSetUserBannedByTelegramIDBlocksAndRestoresOwnedKey(t *testing.T) {
	file, err := os.CreateTemp("", "user-ban-*.sqlite")
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
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := database.Exec(`INSERT INTO users (id, telegramUserId, telegramUsername, displayName, accountTypeId, isActive, verifiedAt, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		"user-ban-1", "99887766", "banned_user", "Banned User", "user", now, now, now); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUserApiKey("user-ban-1", "user", "user-ban-key", "sk-user-ban", "User key"); err != nil {
		t.Fatal(err)
	}

	valid, err := repo.ValidateApiKey("sk-user-ban")
	if err != nil || !valid {
		t.Fatalf("expected key to start valid, valid=%v err=%v", valid, err)
	}
	if err := repo.SetUserBannedByTelegramID("99887766", true); err != nil {
		t.Fatal(err)
	}
	valid, err = repo.ValidateApiKey("sk-user-ban")
	if err != nil || valid {
		t.Fatalf("expected banned user's key to be rejected, valid=%v err=%v", valid, err)
	}
	key, err := repo.GetApiKeyByKey("sk-user-ban")
	if err != nil || key == nil || key.IsActive != 0 {
		t.Fatalf("expected auth lookup to mark owned key inactive while banned: key=%+v err=%v", key, err)
	}

	if err := repo.SetUserBannedByTelegramID("99887766", false); err != nil {
		t.Fatal(err)
	}
	valid, err = repo.ValidateApiKey("sk-user-ban")
	if err != nil || !valid {
		t.Fatalf("expected unban to restore previously active key, valid=%v err=%v", valid, err)
	}
}
