package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// SessionDuration is the lifetime of a dashboard web login session.
	SessionDuration = 30 * 24 * time.Hour
)

type SessionStore interface {
	SaveSession(token string, expiry time.Time) error
	GetSession(token string) (time.Time, bool, error)
	DeleteSession(token string) error
	LoadAllActiveSessions() (map[string]time.Time, error)
}

var (
	sessionMu      sync.RWMutex
	activeSessions = make(map[string]time.Time) // token -> expiry (in-memory cache)
	globalStore    SessionStore
)

// InitSessionStore initializes the persistent session store and preloads active sessions.
func InitSessionStore(store SessionStore) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	globalStore = store
	if store != nil {
		if loaded, err := store.LoadAllActiveSessions(); err == nil && loaded != nil {
			for k, v := range loaded {
				activeSessions[k] = v
			}
		}
	}
}

// HashPassword uses bcrypt so passwords remain compatible with the original
// 9router dashboard. CheckPassword still accepts the legacy SHA-256 format.
func HashPassword(password string) string {
	if hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost); err == nil {
		return string(hash)
	}

	// Keep a fallback for an unlikely bcrypt failure, preserving local login.
	salt := make([]byte, 16)
	rand.Read(salt)
	saltHex := hex.EncodeToString(salt)
	h := sha256.New()
	h.Write([]byte(saltHex + ":" + password))
	hashHex := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("sha256$%s$%s", saltHex, hashHex)
}

func CheckPassword(password, storedHash string) bool {
	password = strings.TrimSpace(password)
	storedHash = strings.TrimSpace(storedHash)

	if storedHash == "" {
		return false
	}

	// 1. Check bcrypt hashes produced by the original dashboard.
	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)) == nil {
		return true
	}

	// 2. Check legacy SHA-256 salted hashes.
	parts := strings.Split(storedHash, "$")
	if len(parts) == 3 && parts[0] == "sha256" {
		saltHex := parts[1]
		expectedHashHex := parts[2]
		h := sha256.New()
		h.Write([]byte(saltHex + ":" + password))
		actualHashHex := hex.EncodeToString(h.Sum(nil))
		if subtle.ConstantTimeCompare([]byte(actualHashHex), []byte(expectedHashHex)) == 1 {
			return true
		}
	}

	// 3. Plain text comparison for old development databases.
	if subtle.ConstantTimeCompare([]byte(password), []byte(storedHash)) == 1 {
		return true
	}

	return false
}

// CreateSession generates a new 32-byte secure session token.
func CreateSession() string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	expiry := time.Now().Add(SessionDuration)

	sessionMu.Lock()
	activeSessions[token] = expiry
	store := globalStore
	sessionMu.Unlock()

	if store != nil {
		_ = store.SaveSession(token, expiry)
	}
	return token
}

// ValidateSession checks if a session token is valid and not expired.
func ValidateSession(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}

	sessionMu.RLock()
	expiry, exists := activeSessions[token]
	store := globalStore
	sessionMu.RUnlock()

	now := time.Now()
	if exists {
		if now.Before(expiry) {
			return true
		}
		// Expired
		sessionMu.Lock()
		delete(activeSessions, token)
		sessionMu.Unlock()
		if store != nil {
			_ = store.DeleteSession(token)
		}
		return false
	}

	// Cache miss: check persistent SQLite store
	if store != nil {
		if exp, found, err := store.GetSession(token); err == nil && found {
			if now.Before(exp) {
				sessionMu.Lock()
				activeSessions[token] = exp
				sessionMu.Unlock()
				return true
			}
		}
	}

	return false
}

// InvalidateSession removes a session token.
func InvalidateSession(token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	sessionMu.Lock()
	delete(activeSessions, token)
	store := globalStore
	sessionMu.Unlock()

	if store != nil {
		_ = store.DeleteSession(token)
	}
}
