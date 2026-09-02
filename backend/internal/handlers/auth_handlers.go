package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/middleware"
)

// HandleAuthLogin handles dashboard password login and sets the auth_token cookie.
func HandleAuthLogin(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// Read stored password hash from settings
		var storedHash string
		if settings, err := repo.GetSettings(); err == nil && settings != nil && settings.Password != nil {
			storedHash = *settings.Password
		}

		if !auth.CheckPassword(req.Password, storedHash) {
			message := "Invalid password."
			if storedHash == "" {
				message = "Dashboard password is not configured. Set INITIAL_PASSWORD and restart Zyrouter."
			}
			handlerutil.WriteJSONError(w, http.StatusUnauthorized, message)
			return
		}

		token := auth.CreateSession()

		// Set session cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "auth_token",
			Value:    token,
			Path:     "/",
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		})

		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"token":   token,
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

		handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "Password updated successfully"})
	}
}
