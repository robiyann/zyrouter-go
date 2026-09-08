package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CORS provides Cross-Origin Resource Sharing middleware for client portals and external tooling.
// It reflects request origins, allows credentials for HttpOnly session cookies, and handles preflight OPTIONS.
func CORS(next http.Handler) http.Handler {
	allowed := map[string]struct{}{
		"https://client.zyvenox.tech": {},
		"http://localhost:3000":       {},
		"http://127.0.0.1:3000":       {},
	}
	for _, origin := range strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		_, isAllowed := allowed[origin]
		if origin != "" && isAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, X-Telegram-Bot-Api-Secret-Token, X-API-Key, X-Zyrouter-Telemetry-Token")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		if r.Method == http.MethodOptions {
			if origin != "" && !isAllowed {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
