package chat

import (
	"context"
	"net/http"
	"os"
	"testing"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/middleware"
	"zyrouter/backend/internal/models"
)

func TestVerifiedUserKeyRequiresAllowedAlias(t *testing.T) {
	file, err := os.CreateTemp("", "user-policy-*.sqlite")
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
	userID, typeID := "usr_test", "user"
	key := &models.APIKey{IsActive: 1, UserID: &userID, AccountTypeID: &typeID}
	h := NewChatHandler(repo)
	r := (&http.Request{}).WithContext(context.WithValue(context.Background(), middleware.ApiKeyContextKey, key))
	allowedInfo := &ModelInfo{Provider: "opencode", Model: "mimo-v2.5-free"}
	if err := h.validateRequestPolicy(r, "mimo-free", allowedInfo); err != nil {
		t.Fatalf("allowed alias rejected: %v", err)
	}
	if err := h.validateRequestPolicy(r, "oc/mimo-v2.5-free", allowedInfo); err == nil {
		t.Fatal("prefixed model unexpectedly allowed for verified user")
	}
	if err := h.validateRequestPolicy(r, "other-model", allowedInfo); err == nil {
		t.Fatal("unregistered alias unexpectedly allowed")
	}
}
