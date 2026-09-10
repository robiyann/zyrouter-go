package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"zyrouter/backend/internal/providers"
)

func TestForwardOpenAI_ClineOAuthHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer workos:cline-token" {
			t.Errorf("Authorization = %q, want Cline workos prefix", got)
		}
		for key, want := range map[string]string{
			"User-Agent":         "9Router/1.8.4",
			"X-PLATFORM":         runtime.GOOS,
			"X-PLATFORM-VERSION": runtime.Version(),
			"X-CLIENT-TYPE":      "9router",
			"X-CLIENT-VERSION":   "1.8.4",
			"X-CORE-VERSION":     "1.8.4",
			"X-IS-MULTIROOT":     "false",
		} {
			if got := r.Header.Get(key); got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	config := providers.KnownProviders["cline"]
	config.BaseURL = server.URL
	response, err := ForwardOpenAI(context.Background(), server.Client(), &config, "cline-token", []byte(`{"model":"test","messages":[]}`), false)
	if err != nil {
		t.Fatalf("ForwardOpenAI returned error: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
}
