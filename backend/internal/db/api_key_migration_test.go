package db

import (
	"strings"
	"testing"
)

func TestMigrateLegacyGatewayKeys(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// OpenDatabase may have completed the migration on an empty database. Reset
	// only the test marker so this fixture represents an old production DB.
	if _, err := database.Exec(`DELETE FROM _meta WHERE key = ?`, legacyAPIKeyMigrationMetaKey); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO apiKeys (id, key, name, isActive, restrictions, createdAt, accountTypeId) VALUES
			('legacy-admin', 'legacy-secret-admin-123', 'Legacy admin', 1, '{"allowedModels":["gpt-test"]}', '2026-09-01T00:00:00Z', NULL),
			('legacy-inactive', 'legacy-secret-inactive-123', 'Legacy inactive', 0, NULL, '2026-09-01T00:00:00Z', NULL);
		INSERT INTO apiKeys (id, key, name, isActive, createdAt, clientId, accountTypeId) VALUES
			('legacy-client', 'legacy-secret-client-123', 'Legacy client', 1, '2026-09-01T00:00:00Z', 'client-1', NULL);
		INSERT INTO apiKeys (id, key, name, isActive, createdAt, userId, accountTypeId) VALUES
			('verified-user', 'zy_user_prefix', 'Verified user', 1, '2026-09-01T00:00:00Z', 'user-1', 'paid_user');`); err != nil {
		t.Fatal(err)
	}

	stats, err := MigrateLegacyGatewayKeys(database)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if stats.Found != 3 || stats.Hashed != 3 || stats.ClientID != 1 || stats.Skipped != 1 || stats.UserKeyRetained != 1 {
		t.Fatalf("unexpected migration stats: %+v", stats)
	}

	assertMigrated := func(id, raw, accountType string, active int) {
		t.Helper()
		var storedKey, hash, gotType, restrictions string
		var gotActive int
		if err := database.QueryRow(`SELECT key, keyHash, COALESCE(accountTypeId,''), isActive, COALESCE(restrictions,'') FROM apiKeys WHERE id = ?`, id).
			Scan(&storedKey, &hash, &gotType, &gotActive, &restrictions); err != nil {
			t.Fatal(err)
		}
		if storedKey == raw || hash != HashUserSecret(raw) || gotType != accountType || gotActive != active {
			t.Fatalf("unexpected migrated row %s: key=%q hashSet=%t type=%q active=%d", id, storedKey, hash != "", gotType, gotActive)
		}
		if id == "legacy-admin" && restrictions != `{"allowedModels":["gpt-test"]}` {
			t.Fatalf("migration changed restrictions: %s", restrictions)
		}
	}
	assertMigrated("legacy-admin", "legacy-secret-admin-123", "administrator", 1)
	assertMigrated("legacy-inactive", "legacy-secret-inactive-123", "administrator", 0)
	assertMigrated("legacy-client", "legacy-secret-client-123", "", 1)

	repo := NewRepo(database)
	for _, raw := range []string{"legacy-secret-admin-123", "legacy-secret-inactive-123"} {
		key, err := repo.GetApiKeyByKey(raw)
		if err != nil || key == nil {
			t.Fatalf("migrated key lookup failed for %q: %+v %v", raw, key, err)
		}
	}
	if key, err := repo.GetApiKeyByKey("legacy-secre"); err != nil || key != nil {
		t.Fatalf("masked key prefix was accepted: key=%+v err=%v", key, err)
	}
	active, err := repo.ValidateApiKey("legacy-secret-inactive-123")
	if err != nil || active {
		t.Fatalf("inactive migrated key was accepted: active=%v err=%v", active, err)
	}

	var userKey, userHash, userType string
	if err := database.QueryRow(`SELECT key, COALESCE(keyHash,''), accountTypeId FROM apiKeys WHERE id = 'verified-user'`).Scan(&userKey, &userHash, &userType); err != nil {
		t.Fatal(err)
	}
	if userKey != "zy_user_prefix" || userHash != "" || userType != "paid_user" {
		t.Fatalf("verified user key changed: key=%q hash=%q type=%q", userKey, userHash, userType)
	}

	var marker string
	if err := database.QueryRow(`SELECT value FROM _meta WHERE key = ?`, legacyAPIKeyMigrationMetaKey).Scan(&marker); err != nil || marker != "complete" {
		t.Fatalf("migration marker missing: value=%q err=%v", marker, err)
	}
	second, err := MigrateLegacyGatewayKeys(database)
	if err != nil || !second.AlreadyApplied {
		t.Fatalf("second migration was not idempotent: %+v err=%v", second, err)
	}

	var plaintext int
	if err := database.QueryRow(`SELECT COUNT(*) FROM apiKeys WHERE userId IS NULL AND keyHash IS NULL AND key IS NOT NULL`).Scan(&plaintext); err != nil {
		t.Fatal(err)
	}
	if plaintext != 0 {
		t.Fatalf("plaintext legacy rows remain: %d", plaintext)
	}
}

func TestMigrateLegacyGatewayKeysAvoidsPrefixCollision(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	if _, err := database.Exec(`DELETE FROM _meta WHERE key = ?`, legacyAPIKeyMigrationMetaKey); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO apiKeys (id, key, name, isActive, createdAt) VALUES
		('collision-a', 'same-prefix-secret-a', 'A', 1, '2026-09-01T00:00:00Z'),
		('collision-b', 'same-prefix-secret-b', 'B', 1, '2026-09-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateLegacyGatewayKeys(database); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.QueryRow(`SELECT COUNT(DISTINCT key) FROM apiKeys WHERE id IN ('collision-a','collision-b')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("migration did not keep masked keys unique")
	}

	var masked string
	if err := database.QueryRow(`SELECT key FROM apiKeys WHERE id = 'collision-a'`).Scan(&masked); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(masked, "same-prefix-secret-a") {
		t.Fatalf("masked key contains full secret: %q", masked)
	}
}
