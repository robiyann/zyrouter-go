package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const clineRefreshURL = "https://api.cline.bot/api/v1/auth/refresh"

func init() {
	Register("cline", refreshCline)
	Register("clinepass", refreshCline)
}

// refreshCline refreshes Cline/ClinePass OAuth credentials. Cline uses a JSON
// body and returns its token payload nested under data.
func refreshCline(ctx context.Context, p *Params) (*TokenResult, error) {
	if p.RefreshToken == "" {
		return nil, fmt.Errorf("cline refresh token is empty")
	}

	body, err := json.Marshal(map[string]string{
		"refreshToken": p.RefreshToken,
		"grantType":    "refresh_token",
		"clientType":   "extension",
	})
	if err != nil {
		return nil, fmt.Errorf("cline marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, clineRefreshURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cline create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cline POST: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("cline read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cline refresh returned %d: %s", resp.StatusCode, truncateBody(respBody))
	}

	var payload struct {
		Data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresAt    string `json:"expiresAt"`
		} `json:"data"`
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    string `json:"expiresAt"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("cline parse response: %w", err)
	}

	accessToken := payload.Data.AccessToken
	refreshToken := payload.Data.RefreshToken
	expiresAt := payload.Data.ExpiresAt
	if accessToken == "" {
		accessToken = payload.AccessToken
	}
	if refreshToken == "" {
		refreshToken = payload.RefreshToken
	}
	if expiresAt == "" {
		expiresAt = payload.ExpiresAt
	}
	if accessToken == "" {
		return nil, fmt.Errorf("cline empty access token")
	}
	if !strings.HasPrefix(accessToken, "workos:") {
		accessToken = "workos:" + accessToken
	}
	if refreshToken == "" {
		refreshToken = p.RefreshToken
	}

	expiresIn := 0
	if expiresAt != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, expiresAt); parseErr == nil {
			expiresIn = maxInt(1, int(time.Until(parsed).Seconds()))
		}
	}

	return &TokenResult{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresIn: expiresIn}, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
