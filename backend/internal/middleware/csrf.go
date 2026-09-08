package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"zyrouter/backend/internal/handlerutil"
)

// BrowserOriginAllowed provides a lightweight same-site CSRF boundary for
// cookie-authenticated browser requests. API clients without Origin remain
// supported; browser cross-origin requests are checked against the portal.
func BrowserOriginAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	allowed := map[string]struct{}{
		"https://client.zyvenox.tech": {},
		"http://localhost:3000":       {},
		"http://127.0.0.1:3000":       {},
	}
	requestOrigin := strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	if _, ok := allowed[requestOrigin]; ok {
		return true
	}
	// Same-origin deployments may use a custom internal host in staging.
	return strings.EqualFold(parsed.Host, r.Host) && strings.EqualFold(parsed.Scheme, forwardedScheme(r))
}

func forwardedScheme(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); value != "" {
		return strings.ToLower(value)
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// RequireBrowserCSRF rejects unsafe browser requests from untrusted origins.
func RequireBrowserCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !BrowserOriginAllowed(r) {
			handlerutil.WriteJSONError(w, http.StatusForbidden, "csrf_origin_forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}
