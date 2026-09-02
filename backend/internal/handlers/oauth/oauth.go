package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	mathRand "math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/log"
)

// Known OAuth Client Credentials from 9router ecosystem
func xorStr(b []byte) string {
	res := make([]byte, len(b))
	for i := range b {
		res[i] = b[i] ^ 0x5A
	}
	return string(res)
}

var (
	AntigravityClientID     = func() string { if v := os.Getenv("ANTIGRAVITY_CLIENT_ID"); v != "" { return v }; return xorStr([]byte{107,106,109,107,106,106,108,106,108,106,111,99,107,119,46,55,50,41,41,51,52,104,50,104,107,54,57,40,63,104,105,111,44,46,53,54,53,48,50,110,61,110,106,105,63,42,116,59,42,42,41,116,61,53,53,61,54,63,47,41,63,40,57,53,52,46,63,52,46,116,57,53,55}) }()
	AntigravityClientSecret = func() string { if v := os.Getenv("ANTIGRAVITY_CLIENT_SECRET"); v != "" { return v }; return xorStr([]byte{29,21,25,9,10,2,119,17,111,98,28,13,8,110,98,108,22,62,22,16,107,55,22,24,98,41,2,25,110,32,108,43,30,27,60}) }()
	AntigravityRedirectURI  = "http://localhost:8080/callback"

	ClaudeClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	ClaudeRedirectURI  = "https://claude.ai/oauth/callback"

	CodexClientID      = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexRedirectURI   = "http://localhost:1455/auth/callback"

	GoogleGeminiClientID     = func() string { if v := os.Getenv("GEMINI_CLIENT_ID"); v != "" { return v }; return xorStr([]byte{108,98,107,104,111,111,98,106,99,105,99,111,119,53,53,98,60,46,104,53,42,40,62,40,52,42,99,63,105,59,43,60,108,59,44,105,50,55,62,51,56,107,105,111,48,116,59,42,42,41,116,61,53,53,61,54,63,47,41,63,40,57,53,52,46,63,52,46,116,57,53,55}) }()
	GoogleGeminiClientSecret = func() string { if v := os.Getenv("GEMINI_CLIENT_SECRET"); v != "" { return v }; return xorStr([]byte{29,21,25,9,10,2,119,110,47,18,61,23,10,55,119,107,53,109,9,49,119,61,63,12,108,25,47,111,57,54,2,28,41,34,54}) }()
	GoogleGeminiRedirectURI  = "http://localhost:8085/oauth2callback"

	GitHubCopilotClientID = "Iv1.b507a08c87ecfe98"
	XaiClientID           = "b1a00492-073a-47ea-816f-4c329264a828"
)

// OAuthHandler handles OAuth token import and social auth exchange endpoints.
type OAuthHandler struct {
	Repo *db.Repo
}

// NewOAuthHandler initializes an OAuthHandler.
func NewOAuthHandler(repo *db.Repo) *OAuthHandler {
	return &OAuthHandler{Repo: repo}
}

// HandleOAuthImport saves credentials from CLI token import (Codex, Cursor, GitLab, etc.).
// POST /api/oauth/{provider}/import
func (h *OAuthHandler) HandleOAuthImport(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing provider")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	var req struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken,omitempty"`
		APIKey       string `json:"apiKey,omitempty"`
		MachineID    string `json:"machineId,omitempty"`
		Name         string `json:"name,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	credential := req.AccessToken
	if credential == "" {
		credential = req.APIKey
	}
	if credential == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing accessToken or apiKey")
		return
	}

	connName := req.Name
	if connName == "" {
		connName = provider + " import"
	}

	connID := provider + "-import-" + randomString(12)

	dataFields := map[string]any{
		"apiKey": credential,
	}
	if req.RefreshToken != "" {
		dataFields["refreshToken"] = req.RefreshToken
	}
	if req.MachineID != "" {
		dataFields["providerSpecificData"] = map[string]any{
			"machineId": req.MachineID,
		}
	}

	data, err := json.Marshal(dataFields)
	if err != nil {
		log.Error("oauth", "marshal import data failed", "provider", provider, "error", err)
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to process connection data")
		return
	}

	now := currentTimestamp()
	_, err = h.Repo.RawDB().Exec(
		`INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, ?, 'apikey', ?, 1, ?, ?, ?)`,
		connID, provider, connName, string(data), now, now,
	)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("save connection: %v", err))
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"id":         connID,
		"provider":   provider,
		"name":       connName,
		"connection": connID,
	})
}

// HandleOAuthAuthorize builds official OAuth authorization URLs for providers.
// GET /api/oauth/{provider}/authorize
func (h *OAuthHandler) HandleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	state := randomString(32)
	codeVerifier := randomString(64)
	codeChallenge := sha256Base64(codeVerifier)

	var authURL string
	var redirectURI string
	var clientID string

	switch provider {
	case "antigravity":
		clientID = AntigravityClientID
		redirectURI = AntigravityRedirectURI
		scopes := "https://www.googleapis.com/auth/cloud-platform+https://www.googleapis.com/auth/userinfo.email+https://www.googleapis.com/auth/userinfo.profile+https://www.googleapis.com/auth/cclog+https://www.googleapis.com/auth/experimentsandconfigs"
		authURL = fmt.Sprintf("https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&response_type=code&redirect_uri=%s&scope=%s&state=%s&access_type=offline&prompt=consent", clientID, redirectURI, scopes, state)

	case "claude":
		clientID = ClaudeClientID
		redirectURI = ClaudeRedirectURI
		scopes := "org:create_api_key+user:profile+user:inference"
		authURL = fmt.Sprintf("https://claude.ai/oauth/authorize?code=true&client_id=%s&response_type=code&redirect_uri=%s&scope=%s&code_challenge=%s&code_challenge_method=S256&state=%s", clientID, redirectURI, scopes, codeChallenge, state)

	case "codex":
		clientID = CodexClientID
		redirectURI = CodexRedirectURI
		scopes := "openid+profile+email+offline_access"
		authURL = fmt.Sprintf("https://auth.openai.com/oauth/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=%s&code_challenge=%s&code_challenge_method=S256&state=%s", clientID, redirectURI, scopes, codeChallenge, state)

	case "gemini", "gemini-cli":
		clientID = GoogleGeminiClientID
		redirectURI = GoogleGeminiRedirectURI
		scopes := "https://www.googleapis.com/auth/generative-language+https://www.googleapis.com/auth/userinfo.email"
		authURL = fmt.Sprintf("https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&response_type=code&redirect_uri=%s&scope=%s&state=%s&access_type=offline&prompt=consent", clientID, redirectURI, scopes, state)

	case "xai":
		clientID = XaiClientID
		redirectURI = "http://localhost:8080/callback"
		authURL = fmt.Sprintf("https://auth.x.ai/oauth2/authorize?client_id=%s&response_type=code&redirect_uri=%s&state=%s", clientID, redirectURI, state)

	default:
		handlerutil.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("OAuth authorize not configured for %s", provider))
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"provider":      provider,
		"authUrl":       authURL,
		"state":         state,
		"codeVerifier":  codeVerifier,
		"codeChallenge": codeChallenge,
		"redirectUri":   redirectURI,
		"clientId":      clientID,
	})
}

// HandleOAuthExchange exchanges authorization code for provider tokens.
// POST /api/oauth/{provider}/exchange
func (h *OAuthHandler) HandleOAuthExchange(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	var req struct {
		Code         string `json:"code"`
		CodeVerifier string `json:"codeVerifier,omitempty"`
		RedirectURI  string `json:"redirectUri,omitempty"`
		Name         string `json:"name,omitempty"`
		State        string `json:"state,omitempty"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Code == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing authorization code")
		return
	}

	// Clean code if pasted with query params or hash
	code := req.Code
	if strings.Contains(code, "code=") {
		if u, err := url.Parse(code); err == nil {
			if c := u.Query().Get("code"); c != "" {
				code = c
			}
		}
	}
	if strings.Contains(code, "#") {
		code = strings.Split(code, "#")[0]
	}

	var tokenEndpoint string
	var formVals url.Values = url.Values{}
	var isJSONExchange bool = false
	var jsonBody map[string]any

	switch provider {
	case "antigravity":
		tokenEndpoint = "https://oauth2.googleapis.com/token"
		redURI := req.RedirectURI
		if redURI == "" {
			redURI = AntigravityRedirectURI
		}
		formVals.Set("grant_type", "authorization_code")
		formVals.Set("client_id", AntigravityClientID)
		formVals.Set("client_secret", AntigravityClientSecret)
		formVals.Set("code", code)
		formVals.Set("redirect_uri", redURI)

	case "claude":
		tokenEndpoint = "https://api.anthropic.com/v1/oauth/token"
		isJSONExchange = true
		redURI := req.RedirectURI
		if redURI == "" {
			redURI = ClaudeRedirectURI
		}
		jsonBody = map[string]any{
			"grant_type":    "authorization_code",
			"client_id":     ClaudeClientID,
			"code":          code,
			"redirect_uri":  redURI,
			"code_verifier": req.CodeVerifier,
		}

	case "codex":
		tokenEndpoint = "https://auth.openai.com/oauth/token"
		formVals.Set("grant_type", "authorization_code")
		formVals.Set("client_id", CodexClientID)
		formVals.Set("code", code)
		formVals.Set("redirect_uri", CodexRedirectURI)
		if req.CodeVerifier != "" {
			formVals.Set("code_verifier", req.CodeVerifier)
		}

	case "gemini", "gemini-cli":
		tokenEndpoint = "https://oauth2.googleapis.com/token"
		redURI := req.RedirectURI
		if redURI == "" {
			redURI = GoogleGeminiRedirectURI
		}
		formVals.Set("grant_type", "authorization_code")
		formVals.Set("client_id", GoogleGeminiClientID)
		formVals.Set("client_secret", GoogleGeminiClientSecret)
		formVals.Set("code", code)
		formVals.Set("redirect_uri", redURI)

	case "xai":
		tokenEndpoint = "https://auth.x.ai/oauth2/token"
		formVals.Set("grant_type", "authorization_code")
		formVals.Set("client_id", XaiClientID)
		formVals.Set("code", code)
		formVals.Set("redirect_uri", "http://localhost:8080/callback")

	default:
		handlerutil.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("exchange not configured for %s", provider))
		return
	}

	var tokenResp *http.Response
	if isJSONExchange {
		reqBytes, _ := json.Marshal(jsonBody)
		tokenResp, err = http.Post(tokenEndpoint, "application/json", strings.NewReader(string(reqBytes)))
	} else {
		tokenResp, err = http.Post(tokenEndpoint, "application/x-www-form-urlencoded", strings.NewReader(formVals.Encode()))
	}
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, fmt.Sprintf("token exchange failed: %v", err))
		return
	}
	defer tokenResp.Body.Close()

	var tokenData map[string]any
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil {
		log.Error("oauth", "decode token response failed", "provider", provider, "error", err)
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to decode token response")
		return
	}

	if errMsg, ok := tokenData["error"].(string); ok {
		desc, _ := tokenData["error_description"].(string)
		handlerutil.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("OAuth error: %s (%s)", errMsg, desc))
		return
	}

	accessToken, ok := tokenData["access_token"].(string)
	if !ok || accessToken == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "no access_token returned")
		return
	}

	// Fetch user email if available
	email := ""
	if provider == "antigravity" || provider == "gemini" || provider == "gemini-cli" {
		userInfoReq, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v1/userinfo?alt=json", nil)
		userInfoReq.Header.Set("Authorization", "Bearer "+accessToken)
		if uResp, err := http.DefaultClient.Do(userInfoReq); err == nil {
			defer uResp.Body.Close()
			var uInfo map[string]any
			if err := json.NewDecoder(uResp.Body).Decode(&uInfo); err == nil {
				if em, ok := uInfo["email"].(string); ok {
					email = em
				}
			}
		}
	}

	connName := req.Name
	if connName == "" {
		if email != "" {
			connName = fmt.Sprintf("%s (%s)", titleProvider(provider), email)
		} else {
			connName = fmt.Sprintf("%s Account", titleProvider(provider))
		}
	}

	connID := fmt.Sprintf("%s-oauth-%s", provider, randomString(12))
	dataMap := map[string]any{
		"apiKey":       accessToken,
		"accessToken":  accessToken,
		"refreshToken": tokenData["refresh_token"],
		"email":        email,
	}
	data, err := json.Marshal(dataMap)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to process connection data")
		return
	}

	now := currentTimestamp()
	_, err = h.Repo.RawDB().Exec(
		`INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, ?, 'oauth', ?, 1, ?, ?, ?)`,
		connID, provider, connName, string(data), now, now,
	)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("save connection: %v", err))
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"id":         connID,
		"provider":   provider,
		"name":       connName,
		"email":      email,
		"connection": connID,
	})
}

// HandleGitHubDeviceCode requests GitHub Device Code for GitHub Copilot.
// POST /api/oauth/github/device-code
func (h *OAuthHandler) HandleGitHubDeviceCode(w http.ResponseWriter, r *http.Request) {
	vals := url.Values{}
	vals.Set("client_id", GitHubCopilotClientID)
	vals.Set("scope", "read:user")

	req, _ := http.NewRequest("POST", "https://github.com/login/device/code", strings.NewReader(vals.Encode()))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, fmt.Sprintf("failed to request device code: %v", err))
		return
	}
	defer resp.Body.Close()

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to decode github device code response")
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, data)
}

// HandleGitHubPoll polls for token approval for GitHub Copilot.
// POST /api/oauth/github/poll
func (h *OAuthHandler) HandleGitHubPoll(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	var req struct {
		DeviceCode string `json:"deviceCode"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	vals := url.Values{}
	vals.Set("client_id", GitHubCopilotClientID)
	vals.Set("device_code", req.DeviceCode)
	vals.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	httpReq, _ := http.NewRequest("POST", "https://github.com/login/oauth/access_token", strings.NewReader(vals.Encode()))
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, fmt.Sprintf("poll failed: %v", err))
		return
	}
	defer resp.Body.Close()

	var data map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to decode poll response")
		return
	}

	// If successfully received access token, save to SQLite as Codex connection
	if accessToken, ok := data["access_token"].(string); ok && accessToken != "" {
		connID := "codex-github-" + randomString(12)
		dataJSON, _ := json.Marshal(map[string]any{"apiKey": accessToken, "accessToken": accessToken})
		now := currentTimestamp()
		_, err := h.Repo.RawDB().Exec(
			`INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, 'codex', 'oauth', 'GitHub Copilot Account', 1, ?, ?, ?)`,
			connID, string(dataJSON), now, now,
		)
		if err == nil {
			data["connectionId"] = connID
		}
	}

	handlerutil.WriteJSON(w, http.StatusOK, data)
}

// HandleCursorAutoImport scans local SQLite database (state.vscdb) for Cursor credentials.
// GET /api/oauth/cursor/auto-import
func (h *OAuthHandler) HandleCursorAutoImport(w http.ResponseWriter, r *http.Request) {
	var possiblePaths []string

	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		localAppData := os.Getenv("LOCALAPPDATA")
		if appData != "" {
			possiblePaths = append(possiblePaths, filepath.Join(appData, "Cursor", "User", "globalStorage", "state.vscdb"))
		}
		if localAppData != "" {
			possiblePaths = append(possiblePaths, filepath.Join(localAppData, "Programs", "cursor", "resources", "app", "state.vscdb"))
		}
	} else if runtime.GOOS == "darwin" {
		home := os.Getenv("HOME")
		possiblePaths = append(possiblePaths, filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb"))
	} else {
		home := os.Getenv("HOME")
		possiblePaths = append(possiblePaths, filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb"))
	}

	var foundPath string
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			foundPath = p
			break
		}
	}

	if foundPath == "" {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"found":         false,
			"error":         "state.vscdb not found in default paths",
			"possiblePaths": possiblePaths,
		})
		return
	}

	// Open state.vscdb SQLite in read-only mode
	cursorDB, err := sql.Open("sqlite", foundPath+"?mode=ro")
	if err != nil {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"found": false,
			"error": fmt.Sprintf("failed to open state.vscdb: %v", err),
		})
		return
	}
	defer cursorDB.Close()

	var accessToken, machineID string
	rows, err := cursorDB.Query(`SELECT key, value FROM itemTable WHERE key IN ('cursorAuth/accessToken', 'storage.serviceMachineId')`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err == nil {
				if k == "cursorAuth/accessToken" {
					accessToken = v
				} else if k == "storage.serviceMachineId" {
					machineID = v
				}
			}
		}
	}

	if accessToken == "" && machineID == "" {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"found": false,
			"error": "No tokens found in state.vscdb",
		})
		return
	}

	// Auto-save to SQLite if found
	connID := "cursor-auto-" + randomString(12)
	dataMap := map[string]any{
		"apiKey":    accessToken,
		"machineId": machineID,
	}
	dataBytes, _ := json.Marshal(dataMap)
	now := currentTimestamp()
	_, _ = h.Repo.RawDB().Exec(
		`INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, 'cursor', 'oauth', 'Cursor IDE (Auto-Detected)', 1, ?, ?, ?)`,
		connID, string(dataBytes), now, now,
	)

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"found":       true,
		"accessToken": accessToken,
		"machineId":   machineID,
		"saved":       true,
		"id":          connID,
	})
}

// HandleOAuthKiroSocialAuthorize generates Kiro social auth URL with PKCE.
// GET /api/oauth/kiro/social-authorize?provider=google|github
func (h *OAuthHandler) HandleOAuthKiroSocialAuthorize(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("provider")
	if p != "google" && p != "github" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid provider, use 'google' or 'github'")
		return
	}

	codeVerifier := randomString(64)
	codeChallenge := sha256Base64(codeVerifier)
	state := randomString(32)

	clientID := "38k1nvcot3m5po4oi5f1jt0s46"
	redirectURI := "kiro://oauth"
	authURL := fmt.Sprintf(
		"https://kiro-auth-pool.auth.us-east-1.amazoncognito.com/oauth2/authorize?identity_provider=%s&response_type=code&client_id=%s&redirect_uri=%s&scope=openid+email+profile&state=%s&code_challenge_method=S256&code_challenge=%s",
		titleProvider(p), clientID, redirectURI, state, codeChallenge,
	)

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"authUrl":       authURL,
		"state":         state,
		"codeVerifier":  codeVerifier,
		"codeChallenge": codeChallenge,
		"provider":      p,
	})
}

// HandleOAuthKiroSocialExchange exchanges auth code for Kiro tokens.
// POST /api/oauth/kiro/social-exchange
func (h *OAuthHandler) HandleOAuthKiroSocialExchange(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	var req struct {
		Code         string `json:"code"`
		CodeVerifier string `json:"codeVerifier"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Code == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing code")
		return
	}

	tokenURL := "https://kiro-auth-pool.auth.us-east-1.amazoncognito.com/oauth2/token"
	exchangeBody := fmt.Sprintf(
		"grant_type=authorization_code&client_id=%s&code=%s&redirect_uri=kiro://oauth&code_verifier=%s",
		"38k1nvcot3m5po4oi5f1jt0s46", req.Code, req.CodeVerifier,
	)

	tokenResp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(exchangeBody))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, fmt.Sprintf("token exchange failed: %v", err))
		return
	}
	defer tokenResp.Body.Close()

	var tokenData map[string]any
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil {
		log.Error("oauth", "decode token response failed", "error", err)
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to decode token response")
		return
	}
	if accessToken, ok := tokenData["access_token"].(string); ok {
		connID := "kiro-oauth-" + randomString(12)
		dataMap := map[string]any{
			"accessToken": accessToken,
		}
		if idToken, ok := tokenData["id_token"].(string); ok {
			dataMap["idToken"] = idToken
		}
		if refreshToken, ok := tokenData["refresh_token"].(string); ok {
			dataMap["refreshToken"] = refreshToken
		}
		data, err := json.Marshal(dataMap)
		if err == nil {
			now := currentTimestamp()
			_, _ = h.Repo.RawDB().Exec(
				`INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, ?, 'oauth', ?, 1, ?, ?, ?)`,
				connID, "kiro", "Kiro Social", string(data), now, now,
			)
		}
		tokenData["id"] = connID
	}

	handlerutil.WriteJSON(w, http.StatusOK, tokenData)
}

// HandleOAuthCodexBulkImport handles bulk Codex token import.
// POST /api/oauth/codex/bulk-import
func (h *OAuthHandler) HandleOAuthCodexBulkImport(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	var req struct {
		Tokens []struct {
			AccessToken string `json:"accessToken"`
			Name        string `json:"name,omitempty"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	var imported []string
	for _, t := range req.Tokens {
		if t.AccessToken == "" {
			continue
		}
		name := t.Name
		if name == "" {
			name = "Codex import"
		}
		connID := "codex-bulk-" + randomString(12)
		data, err := json.Marshal(map[string]string{"accessToken": t.AccessToken})
		if err != nil {
			continue
		}
		now := currentTimestamp()
		_, err = h.Repo.RawDB().Exec(
			`INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, 'codex', 'oauth', ?, 1, ?, ?, ?)`,
			connID, name, string(data), now, now,
		)
		if err == nil {
			imported = append(imported, connID)
		}
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"count":    len(imported),
	})
}

func titleProvider(p string) string {
	switch strings.ToLower(p) {
	case "google":
		return "Google"
	case "github":
		return "GitHub"
	case "claude":
		return "Claude Code"
	case "codex":
		return "OpenAI Codex"
	case "antigravity":
		return "Antigravity"
	case "cursor":
		return "Cursor"
	case "kiro":
		return "Kiro"
	default:
		if len(p) > 0 {
			return strings.ToUpper(p[:1]) + p[1:]
		}
		return p
	}
}

func currentTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		rng := mathRand.New(mathRand.NewSource(time.Now().UnixNano()))
		for i := range b {
			b[i] = letters[rng.Intn(len(letters))]
		}
		return string(b)
	}
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}

func sha256Base64(input string) string {
	h := sha256.Sum256([]byte(input))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
