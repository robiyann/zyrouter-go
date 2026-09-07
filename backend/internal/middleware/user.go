package middleware

import (
	"context"
	"net/http"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/models"
)

type userContextKey string

const authenticatedUserKey userContextKey = "verifiedUser"

// RequireUserSession authenticates a server-issued session created after Telegram verification.
func RequireUserSession(repo *db.Repo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractAuthToken(r)
			if token == "" {
				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "user session required")
				return
			}
			user, err := repo.GetUserBySession(token)
			if err != nil || user == nil || user.IsActive != 1 {
				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "invalid or expired user session")
				return
			}
			ctx := context.WithValue(r.Context(), authenticatedUserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetAuthenticatedUser(r *http.Request) *models.User {
	user, _ := r.Context().Value(authenticatedUserKey).(*models.User)
	return user
}
