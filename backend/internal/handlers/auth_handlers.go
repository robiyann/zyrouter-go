package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/authlog"
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/middleware"
)

const (
	loginFailureWindow = 5 * time.Minute
	loginFailureLimit  = 5
	loginLockDuration  = 5 * time.Minute
	loginLimiterMaxIPs = 10_000
)

type loginFailureState struct {
	firstFailure time.Time
	lastFailure  time.Time
	count        int
	lockedUntil  time.Time
}

var loginLimiter = struct {
	sync.Mutex
	entries map[string]loginFailureState
}{entries: make(map[string]loginFailureState)}

func loginClientIP(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Real-IP")); value != "" {
		return value
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func loginLocked(ip string, now time.Time) (time.Duration, bool) {
	loginLimiter.Lock()
	defer loginLimiter.Unlock()
	state, ok := loginLimiter.entries[ip]
	if !ok || now.After(state.lockedUntil) {
		return 0, false
	}
	return time.Until(state.lockedUntil), true
}

func recordLoginFailure(ip string, now time.Time) (time.Duration, bool, int) {
	loginLimiter.Lock()
	defer loginLimiter.Unlock()
	if len(loginLimiter.entries) >= loginLimiterMaxIPs {
		for candidate, state := range loginLimiter.entries {
			if now.Sub(state.lastFailure) > loginFailureWindow && now.After(state.lockedUntil) {
				delete(loginLimiter.entries, candidate)
			}
		}
	}
	state := loginLimiter.entries[ip]
	if state.firstFailure.IsZero() || now.Sub(state.firstFailure) > loginFailureWindow {
		state = loginFailureState{firstFailure: now}
	}
	state.count++
	state.lastFailure = now
	if state.count >= loginFailureLimit {
		state.lockedUntil = now.Add(loginLockDuration)
	}
	loginLimiter.entries[ip] = state
	if state.lockedUntil.After(now) {
		return time.Until(state.lockedUntil), true, 0
	}
	remaining := loginFailureLimit - state.count
	if remaining < 0 {
		remaining = 0
	}
	return 0, false, remaining
}

func clearLoginFailures(ip string) {
	loginLimiter.Lock()
	delete(loginLimiter.entries, ip)
	loginLimiter.Unlock()
}

// HandleAuthLogin handles dashboard password login and sets the auth_token cookie.
func HandleAuthLogin(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := loginClientIP(r)
		if retryAfter, locked := loginLocked(ip, time.Now()); locked {
			recordAuthEvent(r, "login_locked", http.StatusTooManyRequests, "login temporarily locked")
			w.Header().Set("Retry-After", strconv.FormatInt(int64(retryAfter.Seconds())+1, 10))
			handlerutil.WriteJSONError(w, http.StatusTooManyRequests, "Too many failed login attempts. Try again later.")
			return
		}
		var raw map[string]json.RawMessage
		if err := handlerutil.DecodeJSON(r, &raw); err != nil || raw == nil {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		passwordRaw, ok := raw["password"]
		if !ok {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		var password string
		if err := json.Unmarshal(passwordRaw, &password); err != nil {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if len(password) == 0 || len(password) > 128 {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// Read stored password hash from settings
		var storedHash string
		var settings *db.SettingsData
		if loaded, err := repo.GetSettings(); err == nil && loaded != nil {
			settings = loaded
			if loaded.Password != nil {
				storedHash = *loaded.Password
			}
		}

		if !auth.CheckPassword(password, storedHash) {
			retryAfter, locked, attemptsRemaining := recordLoginFailure(ip, time.Now())
			w.Header().Set("X-Login-Attempts-Remaining", strconv.Itoa(attemptsRemaining))
			event := "login_failed"
			if locked {
				event = "login_locked"
				recordAuthEvent(r, event, http.StatusTooManyRequests, "login temporarily locked")
				w.Header().Set("Retry-After", strconv.FormatInt(int64(retryAfter.Seconds())+1, 10))
				handlerutil.WriteJSONError(w, http.StatusTooManyRequests, "Too many failed login attempts. Try again later.")
				return
			}
			recordAuthEvent(r, event, http.StatusUnauthorized, "invalid password")
			message := "Invalid password."
			if storedHash == "" {
				message = "Dashboard password is not configured. Set INITIAL_PASSWORD and restart Zyrouter."
			}
			handlerutil.WriteJSONError(w, http.StatusUnauthorized, message)
			return
		}
		clearLoginFailures(ip)
		// Transparently upgrade legacy SHA-256 hashes after a successful login.
		// Plaintext hashes are no longer accepted by CheckPassword.
		if settings != nil && auth.NeedsPasswordRehash(storedHash) {
			if upgraded := auth.HashPassword(password); upgraded != "" {
				settings.Password = &upgraded
				if err := repo.UpdateSettingsData(settings); err != nil {
					// Do not fail an otherwise valid login because a best-effort
					// rehash could not be persisted; log-in remains secure.
					recordAuthEvent(r, "password_rehash_failed", http.StatusInternalServerError, "legacy password hash upgrade failed")
				}
			}
		}
		recordAuthEvent(r, "login_success", http.StatusOK, "dashboard session created")

		token := auth.CreateSession()
		if token == "" {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to create session")
			return
		}

		// Set session cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "auth_token",
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(auth.SessionDuration.Seconds()),
		})

		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"message": "Authenticated successfully",
		})
	}
}

func recordAuthEvent(r *http.Request, event string, status int, detail string) {
	ip := loginClientIP(r)
	authlog.Record(db.AuthLogEntry{Event: event, IP: ip, Method: r.Method, Path: r.URL.Path, Status: status, RequestID: middleware.GetRequestID(r), UserAgent: r.UserAgent(), Referer: r.Referer(), Detail: detail})
}

// HandleAuthLogout invalidates the session and clears the cookie.
func HandleAuthLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := middleware.ExtractAuthToken(r)
		if token != "" {
			auth.InvalidateSession(token)
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "auth_token",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
			MaxAge:   -1,
			SameSite: http.SameSiteLaxMode,
		})

		handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// HandleAuthStatus returns current authentication status.
func HandleAuthStatus(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := middleware.ExtractAuthToken(r)
		isAuthenticated := auth.ValidateSession(token)

		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"authenticated": isAuthenticated,
		})
	}
}

// HandleAuthChangePassword updates the encrypted dashboard password in settings.
func HandleAuthChangePassword(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if err := handlerutil.DecodeJSON(r, &req); err != nil {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if len(req.NewPassword) < 12 || len(req.NewPassword) > 128 {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "New password must be between 12 and 128 characters")
			return
		}

		settings, err := repo.GetSettings()
		if err != nil || settings == nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to read settings")
			return
		}

		var storedHash string
		if settings.Password != nil {
			storedHash = *settings.Password
		}

		if !auth.CheckPassword(req.CurrentPassword, storedHash) {
			handlerutil.WriteJSONError(w, http.StatusUnauthorized, "Current password is incorrect")
			return
		}

		newHash := auth.HashPassword(req.NewPassword)
		if newHash == "" {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		settings.Password = &newHash
		if err := repo.UpdateSettingsData(settings); err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to update password: "+err.Error())
			return
		}
		auth.InvalidateAllSessions()

		handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "Password updated successfully"})
	}
}
