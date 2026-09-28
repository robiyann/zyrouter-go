package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// UpstreamError captures a non-200 upstream response.
type UpstreamError struct {
	StatusCode int
	Body       []byte
}

func (e *UpstreamError) Error() string {
	if len(e.Body) == 0 {
		return fmt.Sprintf("upstream returned %d", e.StatusCode)
	}
	msg := extractErrorFromBody(e.Body)
	if msg != "" {
		return fmt.Sprintf("upstream returned %d: %s", e.StatusCode, msg)
	}
	return fmt.Sprintf("upstream returned %d", e.StatusCode)
}

func extractErrorFromBody(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	var obj map[string]any
	if json.Unmarshal(trimmed, &obj) == nil {
		if errVal, ok := obj["error"]; ok {
			if errMap, ok := errVal.(map[string]any); ok {
				if msg, ok := errMap["message"].(string); ok && strings.TrimSpace(msg) != "" {
					return strings.TrimSpace(msg)
				}
			} else if errMsg, ok := errVal.(string); ok && strings.TrimSpace(errMsg) != "" {
				return strings.TrimSpace(errMsg)
			}
		}
		if msg, ok := obj["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
		if detail, ok := obj["detail"].(string); ok && strings.TrimSpace(detail) != "" {
			return strings.TrimSpace(detail)
		}
		if msg, ok := obj["msg"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
	}
	if len(trimmed) > 0 && len(trimmed) <= 300 && !bytes.HasPrefix(trimmed, []byte("<")) {
		return string(trimmed)
	}
	return ""
}
// DoRequest sends an HTTP POST to url with body and auth, returns the raw response.
// Caller must close resp.Body.
func DoRequest(ctx context.Context, client *http.Client, method, url string, headers map[string]string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("upstream returned %d and body read failed: %w", resp.StatusCode, readErr)
		}
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: errBody}
	}
	return resp, nil
}

