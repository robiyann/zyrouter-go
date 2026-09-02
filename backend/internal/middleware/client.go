package middleware

import (
	"context"
	"net/http"
	"strings"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/models"
)

type clientContextKey string

const clientKey clientContextKey = "client"

// RequireClientAccess authenticates the future client dashboard with a
// server-issued client access token, keeping it separate from admin/API keys.
func RequireClientAccess(repo *db.Repo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractAuthToken(r)
			if token == "" {
				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "client authentication required")
				return
			}
			client, err := repo.GetClientByAccessTokenHash(db.HashClientToken(token))
			if err != nil {
				handlerutil.WriteJSONError(w, http.StatusInternalServerError, "client authentication lookup failed")
				return
			}
			if client == nil || client.IsActive != 1 {
				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "invalid or inactive client")
				return
			}
			ctx := context.WithValue(r.Context(), clientKey, client)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetAuthenticatedClient(r *http.Request) *models.Client {
	client, _ := r.Context().Value(clientKey).(*models.Client)
	return client
}

func ClientBearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return ""
}
