// Package antigravityquota fetches Antigravity quota data using the OAuth
// connections already stored by Zyrouter.
package antigravityquota

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/models"
	"zyrouter/backend/internal/providers"
)

const (
	DefaultUserAgent     = "antigravity/ide/2.11.0 darwin/arm64"
	DefaultClientName    = "antigravity"
	DefaultClientVersion = "2.11.0"
	CacheTTL             = 60 * time.Second
)

var defaultBaseURLs = []string{
	"https://daily-cloudcode-pa.googleapis.com",
	"https://cloudcode-pa.googleapis.com",
}

// Window is a quota window returned by retrieveUserQuotaSummary.
type Window struct {
	ID                  string  `json:"id"`
	Label               string  `json:"label"`
	Group               string  `json:"group"`
	RemainingPercentage float64 `json:"remainingPercentage"`
	ResetAt             string  `json:"resetAt,omitempty"`
}

// AccountQuota contains aggregate quota windows for one
// provider connection. Error is scoped to this account so other accounts can
// still be displayed when one token or endpoint fails.
type AccountQuota struct {
	ConnectionID string   `json:"connectionId"`
	Email        string   `json:"email,omitempty"`
	Name         string   `json:"name,omitempty"`
	Windows      []Window `json:"windows,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// Snapshot is the bot-facing quota response.
type Snapshot struct {
	FetchedAt time.Time      `json:"fetchedAt"`
	Accounts  []AccountQuota `json:"accounts"`
}

type Service struct {
	repo      *db.Repo
	client    *http.Client
	baseURLs  []string
	refreshMu sync.Mutex
	cacheMu   sync.Mutex
	cachedAt  time.Time
	cached    *Snapshot
}

func NewService(repo *db.Repo) *Service {
	return NewServiceWithClient(repo, &http.Client{Timeout: 15 * time.Second}, defaultBaseURLs)
}

func NewServiceWithClient(repo *db.Repo, client *http.Client, baseURLs []string) *Service {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	clean := make([]string, 0, len(baseURLs))
	for _, base := range baseURLs {
		if base = strings.TrimRight(strings.TrimSpace(base), "/"); base != "" {
			clean = append(clean, base)
		}
	}
	if len(clean) == 0 {
		clean = append(clean, defaultBaseURLs...)
	}
	return &Service{repo: repo, client: client, baseURLs: clean}
}

// Snapshot returns a cached snapshot unless force is true. Refreshes are
// serialized to avoid duplicate quota calls when several Telegram updates
// arrive together.
func (s *Service) Snapshot(ctx context.Context, force bool) (*Snapshot, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("quota service is not initialized")
	}

	s.cacheMu.Lock()
	if !force && s.cached != nil && time.Since(s.cachedAt) < CacheTTL {
		cached := cloneSnapshot(s.cached)
		s.cacheMu.Unlock()
		return cached, nil
	}
	s.cacheMu.Unlock()

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	// Another waiter may have refreshed while this goroutine was waiting.
	s.cacheMu.Lock()
	if !force && s.cached != nil && time.Since(s.cachedAt) < CacheTTL {
		cached := cloneSnapshot(s.cached)
		s.cacheMu.Unlock()
		return cached, nil
	}
	s.cacheMu.Unlock()

	connections, err := s.repo.GetProviderConnections("antigravity", true)
	if err != nil {
		return nil, fmt.Errorf("load Antigravity connections: %w", err)
	}
	snapshot := &Snapshot{FetchedAt: time.Now().UTC(), Accounts: make([]AccountQuota, len(connections))}
	// Bound concurrency so a large account pool does not create a burst of
	// OAuth/quota requests while still avoiding a slow one-by-one refresh.
	workerCount := 4
	if len(connections) < workerCount {
		workerCount = len(connections)
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				snapshot.Accounts[index] = s.fetchAccount(ctx, connections[index])
			}
		}()
	}
	for index := range connections {
		jobs <- index
	}
	close(jobs)
	workers.Wait()

	s.cacheMu.Lock()
	s.cachedAt = time.Now()
	s.cached = cloneSnapshot(snapshot)
	s.cacheMu.Unlock()
	return snapshot, nil
}

func (s *Service) fetchAccount(ctx context.Context, connection *models.ProviderConnection) AccountQuota {
	account := AccountQuota{ConnectionID: connection.ID}
	if connection.Email != nil {
		account.Email = *connection.Email
	}
	if connection.Name != nil {
		account.Name = *connection.Name
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(connection.Data), &data); err != nil {
		account.Error = "credential data in database is invalid"
		return account
	}
	oauth := providers.ParseOAuthConnection(data)
	if oauth == nil || oauth.AccessToken == "" {
		account.Error = "access token is missing"
		return account
	}

	accessToken := oauth.AccessToken
	refreshToken := oauth.RefreshToken
	if oauth.IsExpired() {
		if refreshToken == "" {
			account.Error = "access token expired and refresh token is missing"
			return account
		}
		cfg, ok := providers.KnownOAuthConfigs["antigravity"]
		if !ok {
			account.Error = "Antigravity OAuth configuration is missing"
			return account
		}
		refreshed, err := providers.RefreshTokenContext(ctx, s.client, cfg, refreshToken)
		if err != nil {
			account.Error = fmt.Sprintf("OAuth refresh failed: %v", err)
			return account
		}
		expiresIn := refreshed.ExpiresIn
		if expiresIn <= 0 {
			expiresIn = 3600
		}
		expiresAt := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second).Format(time.RFC3339)
		if err := s.repo.UpdateProviderConnectionOAuthTokens(connection.ID, refreshed.AccessToken, refreshed.RefreshToken, expiresAt); err != nil {
			account.Error = fmt.Sprintf("save refreshed OAuth token failed: %v", err)
			return account
		}
		accessToken = refreshed.AccessToken
		if refreshed.RefreshToken != "" {
			refreshToken = refreshed.RefreshToken
		}
	}

	var projectID string
	if value, ok := data["projectId"].(string); ok {
		projectID = strings.TrimSpace(value)
	}
	if projectID == "" {
		if nested, ok := data["providerSpecificData"].(map[string]interface{}); ok {
			projectID, _ = nested["projectId"].(string)
		}
	}

	summary, summaryErr := s.fetchSummary(ctx, accessToken, projectID)
	account.Windows = summary
	if summaryErr != nil {
		account.Error = fmt.Sprintf("weekly/5-hour summary unavailable: %v", summaryErr)
	}
	return account
}

func (s *Service) fetchSummary(ctx context.Context, token, projectID string) ([]Window, error) {
	body, err := s.postQuota(ctx, ":retrieveUserQuotaSummary", token, projectID)
	if err != nil {
		return nil, err
	}
	windows := parseSummary(body)
	if len(windows) == 0 {
		return nil, fmt.Errorf("summary response contained no quota windows")
	}
	return windows, nil
}

func (s *Service) postQuota(ctx context.Context, action, token, projectID string) ([]byte, error) {
	payload := []byte("{}")
	if strings.TrimSpace(projectID) != "" {
		payload, _ = json.Marshal(map[string]string{"project": projectID})
	}
	var lastErr error
	for _, base := range s.baseURLs {
		body, status, err := s.post(ctx, base+"/v1internal"+action, token, payload)
		if err == nil && status >= 200 && status < 300 {
			return body, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("HTTP %d: %s", status, truncate(body, 300))
		}

		// Some deployments reject a project field on quota summary requests.
		// Retry once without it before moving to the fallback host.
		if projectID != "" && (status == http.StatusBadRequest || status == http.StatusForbidden) {
			body, status, err = s.post(ctx, base+"/v1internal"+action, token, []byte("{}"))
			if err == nil && status >= 200 && status < 300 {
				return body, nil
			}
			if err != nil {
				lastErr = err
			} else {
				lastErr = fmt.Errorf("HTTP %d: %s", status, truncate(body, 300))
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all quota endpoints failed")
	}
	return nil, lastErr
}

func (s *Service) post(ctx context.Context, endpoint, token string, payload []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("X-Client-Name", DefaultClientName)
	req.Header.Set("X-Client-Version", DefaultClientVersion)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var reader io.Reader = resp.Body
	var gzipReader *gzip.Reader
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, resp.StatusCode, fmt.Errorf("decode gzip response: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}
	body, err := io.ReadAll(io.LimitReader(reader, 10*1024*1024))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}
	return body, resp.StatusCode, nil
}

func parseSummary(body []byte) []Window {
	var root interface{}
	if json.Unmarshal(body, &root) != nil {
		return nil
	}
	var windows []Window
	walkSummary(root, "", &windows)
	sort.SliceStable(windows, func(i, j int) bool {
		return windowOrder(windows[i].ID) < windowOrder(windows[j].ID)
	})
	return windows
}

func walkSummary(value interface{}, inheritedGroup string, windows *[]Window) {
	object, ok := value.(map[string]interface{})
	if !ok {
		if list, ok := value.([]interface{}); ok {
			for _, item := range list {
				walkSummary(item, inheritedGroup, windows)
			}
		}
		return
	}
	group := firstString(object, "displayName", "name", "group")
	if group == "" {
		group = inheritedGroup
	}
	if buckets, ok := object["buckets"].([]interface{}); ok {
		for _, raw := range buckets {
			bucket, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			fraction, ok := firstNumber(bucket, "remainingFraction", "remaining_fraction")
			if !ok {
				continue
			}
			id := firstString(bucket, "bucketId", "bucket_id", "id")
			window := firstString(bucket, "window")
			if id == "" {
				id = inferBucketID(group, window)
			}
			if id == "" {
				continue
			}
			label := firstString(bucket, "displayName", "display_name", "description")
			if label == "" {
				label = labelForBucket(id)
			}
			*windows = append(*windows, Window{
				ID: id, Label: label, Group: group,
				RemainingPercentage: clampFraction(fraction) * 100,
				ResetAt:             firstString(bucket, "resetTime", "reset_time"),
			})
		}
	}
	for key, child := range object {
		if key == "buckets" {
			continue
		}
		if key == "quotaSummary" || key == "quota_summary" || key == "summary" || key == "groups" || key == "quota" {
			walkSummary(child, group, windows)
		}
	}
}

func cloneSnapshot(snapshot *Snapshot) *Snapshot {
	if snapshot == nil {
		return nil
	}
	clone := &Snapshot{FetchedAt: snapshot.FetchedAt, Accounts: make([]AccountQuota, len(snapshot.Accounts))}
	for i, account := range snapshot.Accounts {
		clone.Accounts[i] = account
		clone.Accounts[i].Windows = append([]Window(nil), account.Windows...)
	}
	return clone
}

func clampFraction(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func firstString(object map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNumber(object map[string]interface{}, keys ...string) (float64, bool) {
	for _, key := range keys {
		switch value := object[key].(type) {
		case float64:
			return value, true
		case json.Number:
			parsed, err := value.Float64()
			if err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func inferBucketID(group, window string) string {
	prefix := "3p"
	if strings.Contains(strings.ToLower(group), "gemini") {
		prefix = "gemini"
	}
	switch strings.ToLower(window) {
	case "5h", "five_hour", "five-hour", "session":
		return prefix + "-5h"
	case "weekly", "7d", "seven_day", "seven-day":
		return prefix + "-weekly"
	default:
		return ""
	}
}

func labelForBucket(id string) string {
	switch id {
	case "gemini-5h":
		return "Gemini 5-hour"
	case "gemini-weekly":
		return "Gemini Weekly"
	case "3p-5h":
		return "Claude/GPT 5-hour"
	case "3p-weekly":
		return "Claude/GPT Weekly"
	default:
		return id
	}
}

func windowOrder(id string) int {
	switch id {
	case "gemini-5h":
		return 0
	case "gemini-weekly":
		return 1
	case "3p-5h":
		return 2
	case "3p-weekly":
		return 3
	default:
		return 99
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncate(data []byte, max int) string {
	if len(data) <= max {
		return string(data)
	}
	return string(data[:max]) + "..."
}
