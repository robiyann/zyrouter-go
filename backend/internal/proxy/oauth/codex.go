package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	codexClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexTokenURL = "https://auth.openai.com/oauth/token"
)

func init() {
	Register("codex", refreshCodex)
}

var (
	codexRefreshMu       sync.Mutex
	lastCodexRefresh     = make(map[string]*TokenResult)
	lastCodexRefreshTime = make(map[string]time.Time)
)

// refreshCodex performs token refresh against OpenAI Auth for Codex.
func refreshCodex(ctx context.Context, p *Params) (*TokenResult, error) {
	if p.RefreshToken == "" {
		return nil, fmt.Errorf("codex refresh token is empty")
	}

	codexRefreshMu.Lock()
	if cached, ok := lastCodexRefresh[p.RefreshToken]; ok {
		if time.Since(lastCodexRefreshTime[p.RefreshToken]) < 15*time.Second {
			codexRefreshMu.Unlock()
			return cached, nil
		}
	}
	codexRefreshMu.Unlock()

	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}

	// 1. Try form-urlencoded first (Official standard OAuth for OpenAI)
	formVals := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {codexClientID},
		"refresh_token": {p.RefreshToken},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", codexTokenURL, strings.NewReader(formVals.Encode()))
	if err == nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "codex_cli_rs/0.136.0")

		resp, doErr := client.Do(req)
		if doErr == nil {
			defer resp.Body.Close()
			respBody, _ := io.ReadAll(resp.Body)
			if resp.StatusCode == http.StatusOK {
				var result struct {
					AccessToken  string `json:"access_token"`
					RefreshToken string `json:"refresh_token,omitempty"`
					IdToken      string `json:"id_token,omitempty"`
					ExpiresIn    int    `json:"expires_in"`
				}
				if json.Unmarshal(respBody, &result) == nil && result.AccessToken != "" {
					tr := &TokenResult{
						AccessToken:  result.AccessToken,
						RefreshToken: result.RefreshToken,
						ExpiresIn:    result.ExpiresIn,
					}
					codexRefreshMu.Lock()
					lastCodexRefresh[p.RefreshToken] = tr
					lastCodexRefreshTime[p.RefreshToken] = time.Now()
					if result.RefreshToken != "" {
						lastCodexRefresh[result.RefreshToken] = tr
						lastCodexRefreshTime[result.RefreshToken] = time.Now()
					}
					codexRefreshMu.Unlock()
					return tr, nil
				}
			}
		}
	}

	// 2. Fallback: JSON body encoding
	body := map[string]string{
		"client_id":     codexClientID,
		"grant_type":    "refresh_token",
		"refresh_token": p.RefreshToken,
	}
	jsonBody, _ := json.Marshal(body)

	reqJson, err := http.NewRequestWithContext(ctx, "POST", codexTokenURL, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("codex create request: %w", err)
	}
	reqJson.Header.Set("Content-Type", "application/json")
	reqJson.Header.Set("Accept", "application/json")
	reqJson.Header.Set("User-Agent", "codex_cli_rs/0.136.0")

	resp, err := client.Do(reqJson)
	if err != nil {
		return nil, fmt.Errorf("codex POST: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("codex read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("codex refresh returned %d: %s", resp.StatusCode, truncateBody(respBody))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token,omitempty"`
		IdToken      string `json:"id_token,omitempty"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("codex parse response: %w", err)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("codex empty access token")
	}

	tr := &TokenResult{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
	}
	codexRefreshMu.Lock()
	lastCodexRefresh[p.RefreshToken] = tr
	lastCodexRefreshTime[p.RefreshToken] = time.Now()
	if result.RefreshToken != "" {
		lastCodexRefresh[result.RefreshToken] = tr
		lastCodexRefreshTime[result.RefreshToken] = time.Now()
	}
	codexRefreshMu.Unlock()
	return tr, nil
}
