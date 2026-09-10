package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"zyrouter/backend/internal/providers"
)

// ForwardOpenAI sends an OpenAI-format request to the provider endpoint.
func ForwardOpenAI(ctx context.Context, client *http.Client, cfg *providers.ProviderConfig, apiKey string, body []byte, isStream bool) (*http.Response, error) {
	headers := map[string]string{}
	if !cfg.NoAuth {
		if cfg.AuthTokenPrefix != "" && !strings.HasPrefix(apiKey, cfg.AuthTokenPrefix) {
			apiKey = cfg.AuthTokenPrefix + apiKey
		}
		switch cfg.AuthScheme {
		case "bearer":
			headers[cfg.AuthHeader] = "Bearer " + apiKey
		case "raw":
			headers[cfg.AuthHeader] = apiKey
		default:
			headers["Authorization"] = "Bearer " + apiKey
		}
		if cfg.AuthTokenPrefix == "workos:" {
			// Mirror the Cline extension identity headers used by 9router
			// original. Cline can reject otherwise valid OAuth credentials.
			headers["User-Agent"] = "9Router/1.8.4"
			headers["X-PLATFORM"] = runtime.GOOS
			headers["X-PLATFORM-VERSION"] = runtime.Version()
			headers["X-CLIENT-TYPE"] = "9router"
			headers["X-CLIENT-VERSION"] = "1.8.4"
			headers["X-CORE-VERSION"] = "1.8.4"
			headers["X-IS-MULTIROOT"] = "false"
		}
	}
	for k, v := range cfg.StaticHeaders {
		headers[k] = v
	}
	if isStream {
		headers["Accept"] = "text/event-stream"
	}
	resp, err := DoRequest(ctx, client, "POST", cfg.BaseURL, headers, body)
	if err != nil {
		return nil, fmt.Errorf("forward to %s: %w", cfg.BaseURL, err)
	}
	return resp, nil
}

// ReadBody reads and returns the response body (capped to prevent
// unbounded memory use), closing it.
func ReadBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
}

// UpstreamBody reads the body and wraps non-200 as UpstreamError.
func UpstreamBody(resp *http.Response) ([]byte, error) {
	body, err := ReadBody(resp)
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: body}
	}
	return body, nil
}

// BuildURL joins a base URL with a path segment.
func BuildURL(base, path string) string {
	if base == "" {
		return path
	}
	if path == "" {
		return base
	}
	return fmt.Sprintf("%s/%s", base, path)
}
