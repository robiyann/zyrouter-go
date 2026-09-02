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
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/middleware"
)

const (
	loginFailureWindow = 5 * time.Minute
	loginFailureLimit  = 5
	loginLockDuration  = 5 * time.Minute
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

func recordLoginFailure(ip string, now time.Time) (time.Duration, bool) {
	loginLimiter.Lock()
	defer loginLimiter.Unlock()
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
		return time.Until(state.lockedUntil), true
	}
	return 0, false
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
			w.Header().Set("Retry-After", strconv.FormatInt(int64(retryAfter.Seconds())+1, 10))
			handlerutil.WriteJSONError(w, http.StatusTooManyRequests, "Too many failed login attempts. Try again later.")
			return
		}
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil || raw == nil {
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

		// Read stored password hash from settings
		var storedHash string
		if settings, err := repo.GetSettings(); err == nil && settings != nil && settings.Password != nil {
			storedHash = *settings.Password
		}

		if !auth.CheckPassword(password, storedHash) {
			retryAfter, locked := recordLoginFailure(ip, time.Now())
			if locked {
				w.Header().Set("Retry-After", strconv.FormatInt(int64(retryAfter.Seconds())+1, 10))
				handlerutil.WriteJSONError(w, http.StatusTooManyRequests, "Too many failed login attempts. Try again later.")
				return
			}
			message := "Invalid password."
			if storedHash == "" {
				message = "Dashboard password is not configured. Set INITIAL_PASSWORD and restart Zyrouter."
			}
			handlerutil.WriteJSONError(w, http.StatusUnauthorized, message)
			return
		}
		clearLoginFailures(ip)

		token := auth.CreateSession()

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

		var hasCustomPassword bool
		if settings, err := repo.GetSettings(); err == nil && settings != nil && settings.Password != nil {
			hasCustomPassword = *settings.Password != ""
		}

		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"authenticated":     isAuthenticated,
			"hasCustomPassword": hasCustomPassword,
			"authMode":          "password",
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if len(req.NewPassword) < 4 {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "New password must be at least 4 characters")
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
		settings.Password = &newHash
		if err := repo.UpdateSettingsData(settings); err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to update password: "+err.Error())
			return
		}
		auth.InvalidateAllSessions()

		handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "Password updated successfully"})
	}
}
