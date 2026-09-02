package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"

	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/log"
	"zyrouter/backend/internal/models"
)

// ContextKey is a custom type for context keys to avoid collisions.
type ContextKey string

// ApiKeyContextKey is the context key for the authenticated API key object.
const ApiKeyContextKey ContextKey = "apiKey"

func isLoopbackHost(hostOrIP string) bool {
	if hostOrIP == "" {
		return false
	}
	host, _, err := net.SplitHostPort(hostOrIP)
	if err != nil {
		host = hostOrIP
	}
	host = strings.Trim(host, "[]")
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isLocalRequest(r *http.Request) bool {
	if r.Header.Get("X-9r-Via-Proxy") != "" {
		return false
	}
	// Nginx (the supported public entrypoint) forwards the original client IP
	// in X-Real-IP. Without honoring it, every proxied public request appears
	// to come from 127.0.0.1 and receives the local loopback grant.
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return isLoopbackHost(realIP)
	}
	if realIP := r.Header.Get("X-9r-Real-IP"); realIP != "" {
		return isLoopbackHost(realIP)
	}
	if isLoopbackHost(r.RemoteAddr) {
		return true
	}
	return isLoopbackHost(r.Host)
}

// RequireApiKey creates a middleware handler that authenticates requests using either
// a Dashboard Session Token, a local loopback grant (100% 9router parity), or an API Key from SQLite.
func RequireApiKey(repo *db.Repo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := ExtractAuthToken(r)

			// 1. If no token provided, check if request is local loopback (100% 9router parity)
			if tokenString == "" {
				if isLocalRequest(r) {
					localName := "Local Loopback Client"
					localKey := &models.APIKey{
						ID:       "local-loopback",
						Name:     &localName,
						Key:      "sk-local-loopback",
						IsActive: 1,
					}
					ctx := context.WithValue(r.Context(), ApiKeyContextKey, localKey)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "Authentication required. Provide API key or connect via local loopback.")
				return
			}

			// 2. Check if token is a valid Dashboard Session Token
			if auth.ValidateSession(tokenString) {
				adminName := "Dashboard Admin"
				adminKey := &models.APIKey{
					ID:       "session-admin",
					Name:     &adminName,
					Key:      tokenString,
					IsActive: 1,
				}
				ctx := context.WithValue(r.Context(), ApiKeyContextKey, adminKey)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// 3. Validate as an API Key from SQLite repository
			apiKeyObj, err := repo.GetApiKeyByKey(tokenString)
			if err != nil {
				log.Error("auth", "DB lookup error", "error", err)
				handlerutil.WriteJSONError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			if apiKeyObj == nil {
				// A loopback request may omit authentication, but an explicitly
				// supplied unknown token must never become an unrestricted local key.
				// Otherwise a typo or revoked client key bypasses its policy on localhost.
				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "Invalid authentication credentials.")
				return
			}

			if apiKeyObj.IsActive != 1 {
				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "Invalid or inactive API key.")
				return
			}
			if err := auth.ValidateKeyPolicy(apiKeyObj, "", ""); err != nil {
				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "Invalid API key policy.")
				return
			}

			// Inject API Key info into the request context for downstream handlers/logging
			ctx := context.WithValue(r.Context(), ApiKeyContextKey, apiKeyObj)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdminAccess accepts dashboard sessions, local admin access, and
// non-client API keys, but explicitly rejects keys owned by a client.
func RequireAdminAccess(repo *db.Repo) func(http.Handler) http.Handler {
	base := RequireApiKey(repo)
	return func(next http.Handler) http.Handler {
		return base(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := GetAuthenticatedApiKey(r)
			if key != nil && key.ClientID != nil && strings.TrimSpace(*key.ClientID) != "" && isAdminRoute(r.URL.Path) {
				handlerutil.WriteJSONError(w, http.StatusForbidden, "client API keys cannot access admin routes")
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
}

func isAdminRoute(path string) bool {
	for _, prefix := range []string{
		"/api/keys", "/api/providers", "/api/combos", "/api/settings", "/api/proxy-pools",
		"/api/model-aliases", "/api/custom-models", "/api/models/custom", "/api/provider-nodes",
		"/api/provider-prefixes", "/api/oauth", "/api/admin/", "/admin/", "/translator/",
		"/usage/", "/api/usage/", "/debug/", "/api/version",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return path == "/api/auth/change-password"
}

// GetAuthenticatedApiKey retrieves the authenticated APIKey object from the request context.
func GetAuthenticatedApiKey(r *http.Request) *models.APIKey {
	return GetAuthenticatedApiKeyFromContext(r.Context())
}

// GetAuthenticatedApiKeyFromContext retrieves the authenticated key for
// downstream routing code that only receives a request context.
func GetAuthenticatedApiKeyFromContext(ctx context.Context) *models.APIKey {
	val := ctx.Value(ApiKeyContextKey)
	if val == nil {
		return nil
	}
	keyObj, ok := val.(*models.APIKey)
	if !ok {
		return nil
	}
	return keyObj
}
func ExtractAuthToken(r *http.Request) string {
	// 1. Try Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return strings.TrimSpace(parts[1])
		}
		return ""
	}

	// 2. Try cookie "auth_token"
	if cookie, err := r.Cookie("auth_token"); err == nil && cookie.Value != "" {
		return strings.TrimSpace(cookie.Value)
	}

	// 3. Try custom X-API-Key or X-Auth-Token header as fallback
	if xApiKey := r.Header.Get("X-API-Key"); xApiKey != "" {
		return strings.TrimSpace(xApiKey)
	}
	if xAuthToken := r.Header.Get("X-Auth-Token"); xAuthToken != "" {
		return strings.TrimSpace(xAuthToken)
	}

	return ""
}

// ExtractApiKey extracts the client API key from the request (alias to ExtractAuthToken).
func ExtractApiKey(r *http.Request) string {
	return ExtractAuthToken(r)
}
