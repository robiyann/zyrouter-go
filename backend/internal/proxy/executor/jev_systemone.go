package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"zyrouter/backend/internal/proxy"
)

// isJevModel detects Jev / SystemOne requests by model name or state payload.
func isJevModel(body []byte) bool {
	var request struct {
		Model string `json:"model"`
		State any    `json:"state"`
	}
	if err := json.Unmarshal(body, &request); err == nil {
		m := strings.ToLower(request.Model)
		return strings.Contains(m, "jev") || request.State != nil
	}
	return false
}

func forwardJevSystemOne(w http.ResponseWriter, req *Request, apiKey string) error {
	cfg := *req.Config
	relayPath, usingEdgeRelay := cfg.StaticHeaders["x-relay-path"]
	cfg.StaticHeaders = proxy.BuildOpenCodeHeaders(cfg.StaticHeaders, req.SessionID, false)

	if usingEdgeRelay {
		cfg.StaticHeaders["x-relay-path"] = jevSystemOnePath(relayPath)
	} else {
		cfg.BaseURL = jevSystemOneURL(cfg.BaseURL)
	}

	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	resp, err := proxy.ForwardOpenAI(ctx, req.Client, &cfg, apiKey, req.Body, false)
	if err != nil {
		return fmt.Errorf("Jev SystemOne request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, bodyErr := proxy.UpstreamBody(resp)
		return bodyErr
	}

	return jsonResponse(ctx, w, resp.Body, false, req.ResponseBuf)
}

func forwardJevGoSystemOne(w http.ResponseWriter, req *Request, apiKey string) error {
	cfg := *req.Config
	relayPath, usingEdgeRelay := cfg.StaticHeaders["x-relay-path"]
	cfg.StaticHeaders = proxy.BuildOpenCodeHeaders(cfg.StaticHeaders, req.SessionID, false)

	if usingEdgeRelay {
		cfg.StaticHeaders["x-relay-path"] = jevGoSystemOnePath(relayPath)
	} else {
		cfg.BaseURL = jevGoSystemOneURL(cfg.BaseURL)
	}

	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	resp, err := proxy.ForwardOpenAI(ctx, req.Client, &cfg, apiKey, req.Body, false)
	if err != nil {
		return fmt.Errorf("Jev Go SystemOne request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, bodyErr := proxy.UpstreamBody(resp)
		return bodyErr
	}

	return jsonResponse(ctx, w, resp.Body, false, req.ResponseBuf)
}

func jevSystemOneURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return strings.TrimSuffix(baseURL, "/chat/completions") + "/systemone"
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/systemone"
	}
	if strings.HasSuffix(baseURL, "/systemone") {
		return baseURL
	}
	return "https://opencode.ai/zen/v1/systemone"
}

func jevSystemOnePath(path string) string {
	path = strings.TrimRight(path, "/")
	if strings.HasSuffix(path, "/chat/completions") {
		return strings.TrimSuffix(path, "/chat/completions") + "/systemone"
	}
	if strings.HasSuffix(path, "/v1") {
		return path + "/systemone"
	}
	if strings.HasSuffix(path, "/systemone") {
		return path
	}
	return "/zen/v1/systemone"
}

func jevGoSystemOneURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return strings.TrimSuffix(baseURL, "/chat/completions") + "/systemone"
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/systemone"
	}
	if strings.HasSuffix(baseURL, "/systemone") {
		return baseURL
	}
	return "https://opencode.ai/zen/go/v1/systemone"
}

func jevGoSystemOnePath(path string) string {
	path = strings.TrimRight(path, "/")
	if strings.HasSuffix(path, "/chat/completions") {
		return strings.TrimSuffix(path, "/chat/completions") + "/systemone"
	}
	if strings.HasSuffix(path, "/v1") {
		return path + "/systemone"
	}
	if strings.HasSuffix(path, "/systemone") {
		return path
	}
	return "/zen/go/v1/systemone"
}
