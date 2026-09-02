package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func TestDashboardPassword_IsCompatibleWithOriginalBcrypt(t *testing.T) {
	password := "shared-dashboard-password"
	hash := HashPassword(password)
	if !strings.HasPrefix(hash, "$2") {
		t.Fatalf("expected bcrypt hash, got %q", hash)
	}
	if !CheckPassword(password, hash) {
		t.Fatal("bcrypt password should validate")
	}
	if CheckPassword("wrong-password", hash) {
		t.Fatal("wrong password should be rejected")
	}
}

func TestDashboardPassword_CheckPasswordAcceptsLegacySHA256(t *testing.T) {
	salt := "00112233445566778899aabbccddeeff"
	digest := sha256.Sum256([]byte(salt + ":legacy-password"))
	legacyHash := fmt.Sprintf("sha256$%s$%s", salt, hex.EncodeToString(digest[:]))
	if !CheckPassword("legacy-password", legacyHash) {
		t.Fatal("legacy SHA-256 password should remain readable")
	}
}
