package usagetracker

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/labels"
)

const (
	pendingTimeout = 60 * time.Second
	ringCap        = 50
)

// ActiveRequest matches the Next.js dashboard activeRequests item shape.
type ActiveRequest struct {
	Model    string `json:"model"`
	Provider string `json:"provider"`
	Account  string `json:"account"`
	Count    int    `json:"count"`
}

// RecentRequest matches the dashboard recentRequests item shape with proxy & account traces.
type RecentRequest struct {
	ID               string  `json:"id,omitempty"`
	Timestamp        string  `json:"timestamp"`
	Model            string  `json:"model"`
	Provider         string  `json:"provider"`
	Account          string  `json:"account,omitempty"`
	Proxy            string  `json:"proxy,omitempty"`
	Strategy         string  `json:"strategy,omitempty"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	CachedTokens     int     `json:"cachedTokens,omitempty"`
	Cost             float64 `json:"cost,omitempty"`
	Latency          string  `json:"latency,omitempty"`
	DurationMs       int64   `json:"durationMs,omitempty"`
	Status           string  `json:"status"`
	PublicModel      string  `json:"publicModel,omitempty"`
	ErrorMessage     string  `json:"errorMessage,omitempty"`
	ClientIdentity   string  `json:"clientIdentity,omitempty"`
	ClientIP         string  `json:"clientIp,omitempty"`
	APIKeyID         string  `json:"apiKeyId,omitempty"`
}

// StreamPayload represents the payload sent over SSE on /api/usage/stream.
type StreamPayload struct {
	ActiveRequests []ActiveRequest `json:"activeRequests"`
	RecentRequests []RecentRequest `json:"recentRequests"`
	ErrorProvider  string          `json:"errorProvider"`
	Pending        PendingState    `json:"pending"`
}

// PendingState represents in-flight counts by model and account.
type PendingState struct {
	ByModel   map[string]int            `json:"byModel"`
	ByAccount map[string]map[string]int `json:"byAccount"`
}

// Tracker tracks in-flight requests, recent completions, and broadcasts SSE updates.
type Tracker struct {
	mu                sync.RWMutex
	byModel           map[string]int
	byAccount         map[string]map[string]int
	lastErrorProvider string
	lastErrorTs       int64
	recentRing        []RecentRequest
	subscribers       map[chan []byte]struct{}
	broadcastDebounce *time.Timer
}

type trackerSnapshot struct {
	byModel           map[string]int
	byAccount         map[string]map[string]int
	lastErrorProvider string
	lastErrorTs       int64
	recentRing        []RecentRequest
}

var globalTracker *Tracker
var once sync.Once

// GetTracker returns the singleton UsageTracker instance.
func GetTracker() *Tracker {
	once.Do(func() {
		globalTracker = NewTracker()
	})
	return globalTracker
}

// NewTracker creates a new Tracker instance.
func NewTracker() *Tracker {
	return &Tracker{
		byModel:     make(map[string]int),
		byAccount:   make(map[string]map[string]int),
		recentRing:  make([]RecentRequest, 0, ringCap),
		subscribers: make(map[chan []byte]struct{}),
	}
}

// TrackPending updates the in-flight count for a given model, provider, and connection.
func (t *Tracker) TrackPending(model, provider, connectionID string, started bool, isError bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	modelKey := model
	if provider != "" {
		modelKey = fmt.Sprintf("%s (%s)", model, provider)
	}

	delta := 1
	if !started {
		delta = -1
	}

	// Update byModel
	newModelCount := t.byModel[modelKey] + delta
	if newModelCount <= 0 {
		delete(t.byModel, modelKey)
	} else {
		t.byModel[modelKey] = newModelCount
	}

	// Update byAccount
	if connectionID != "" {
		accountMap, ok := t.byAccount[connectionID]
		if !ok && started {
			accountMap = make(map[string]int)
			t.byAccount[connectionID] = accountMap
		}
		if accountMap != nil {
			newAccCount := accountMap[modelKey] + delta
			if newAccCount <= 0 {
				delete(accountMap, modelKey)
				if len(accountMap) == 0 {
					delete(t.byAccount, connectionID)
				}
			} else {
				accountMap[modelKey] = newAccCount
			}
		}
	}

	if !started && isError && provider != "" {
		t.lastErrorProvider = provider
		t.lastErrorTs = time.Now().UnixMilli()
	}

	t.scheduleBroadcastLocked(nil)
}

// PushRecent adds a completed request to the ring buffer and notifies subscribers.
func (t *Tracker) PushRecent(req RecentRequest, repo *db.Repo) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Prepend to ring
	if len(t.recentRing) >= ringCap {
		t.recentRing = append([]RecentRequest{req}, t.recentRing[:ringCap-1]...)
	} else {
		t.recentRing = append([]RecentRequest{req}, t.recentRing...)
	}

	t.scheduleBroadcastLocked(repo)
}

// GetActiveState computes the current active state for SSE streaming.
func (t *Tracker) GetActiveState(repo *db.Repo) StreamPayload {
	t.mu.RLock()
	snapshot := t.snapshotLocked()
	t.mu.RUnlock()
	return t.buildPayload(repo, snapshot)
}

func (t *Tracker) snapshotLocked() trackerSnapshot {
	snapshot := trackerSnapshot{
		byModel:           make(map[string]int, len(t.byModel)),
		byAccount:         make(map[string]map[string]int, len(t.byAccount)),
		lastErrorProvider: t.lastErrorProvider,
		lastErrorTs:       t.lastErrorTs,
		recentRing:        append([]RecentRequest(nil), t.recentRing...),
	}
	for key, value := range t.byModel {
		snapshot.byModel[key] = value
	}
	for account, models := range t.byAccount {
		inner := make(map[string]int, len(models))
		for model, count := range models {
			inner[model] = count
		}
		snapshot.byAccount[account] = inner
	}
	return snapshot
}

func (t *Tracker) buildPayload(repo *db.Repo, snapshot trackerSnapshot) StreamPayload {
	// Build connection name map
	connMap := make(map[string]string)
	if repo != nil {
		if conns, err := repo.GetProviderConnections("", true); err == nil {
			for _, c := range conns {
				name := c.ID
				if c.Name != nil && *c.Name != "" {
					name = *c.Name
				} else if c.Email != nil && *c.Email != "" {
					name = *c.Email
				}
				connMap[c.ID] = name
			}
		}
	}

	var active []ActiveRequest
	for connID, models := range snapshot.byAccount {
		accName := connMap[connID]
		if accName == "" {
			if len(connID) > 8 {
				accName = fmt.Sprintf("Account %s...", connID[:8])
			} else {
				accName = "Account " + connID
			}
		}
		for modelKey, count := range models {
			if count > 0 {
				mName, pName := parseModelKey(modelKey)
				active = append(active, ActiveRequest{
					Model:    mName,
					Provider: pName,
					Account:  accName,
					Count:    count,
				})
			}
		}
	}

	// If byAccount was empty but byModel had items (e.g. no-auth/public requests)
	if len(active) == 0 {
		for modelKey, count := range snapshot.byModel {
			if count > 0 {
				mName, pName := parseModelKey(modelKey)
				active = append(active, ActiveRequest{
					Model:    mName,
					Provider: pName,
					Account:  "Public / Direct",
					Count:    count,
				})
			}
		}
	}

	recent := make([]RecentRequest, 0, ringCap)
	seen := make(map[string]bool)
	for _, r := range snapshot.recentRing {
		k := r.ID
		if k == "" {
			k = fmt.Sprintf("%s|%s|%s|%d|%d", r.Timestamp, r.Provider, r.Model, r.PromptTokens, r.CompletionTokens)
		}
		if !seen[k] {
			seen[k] = true
			recent = append(recent, r)
			if len(recent) >= ringCap {
				break
			}
		}
	}

	if len(recent) < ringCap && repo != nil && repo.RawDB() != nil {
		limit := ringCap - len(recent)
		q := `SELECT COALESCE(NULLIF(json_extract(meta, '$.requestId'), ''), printf('history-%d', id)), timestamp, provider, model,
			CASE WHEN promptTokens > 0 THEN promptTokens ELSE COALESCE(json_extract(tokens, '$.prompt_tokens'), json_extract(tokens, '$.input_tokens'), 0) END,
			CASE WHEN completionTokens > 0 THEN completionTokens ELSE COALESCE(json_extract(tokens, '$.completion_tokens'), json_extract(tokens, '$.output_tokens'), 0) END,
			status, COALESCE(json_extract(meta, '$.clientIdentity'), ''),
			COALESCE(json_extract(meta, '$.clientIp'), ''), COALESCE(json_extract(meta, '$.apiKeyId'), '')
			FROM usageHistory ORDER BY timestamp DESC, id DESC LIMIT ?`
		type rawHistoryRow struct {
			id, ts, prov, mod, status, clientIdentity, clientIP, apiKeyID string
			prompt, completion                                            int
		}
		rawRows := make([]rawHistoryRow, 0, limit)
		if rows, err := repo.RawDB().Query(q, limit); err == nil {
			for rows.Next() {
				var row rawHistoryRow
				if err := rows.Scan(&row.id, &row.ts, &row.prov, &row.mod, &row.prompt, &row.completion, &row.status, &row.clientIdentity, &row.clientIP, &row.apiKeyID); err == nil {
					rawRows = append(rawRows, row)
				}
			}
			_ = rows.Close()
		}
		for _, row := range rawRows {
			if row.status == "success" || row.status == "ok" {
				row.status = "200"
			}
			displayProvider := labels.Provider(repo, row.prov)
			displayModel := labels.Model(repo, row.prov, row.mod)
			if !seen[row.id] {
				seen[row.id] = true
				recent = append(recent, RecentRequest{
					ID: row.id, Timestamp: row.ts, Provider: displayProvider, Model: displayModel,
					PromptTokens: row.prompt, CompletionTokens: row.completion, Status: row.status,
					ClientIdentity: row.clientIdentity, ClientIP: row.clientIP, APIKeyID: row.apiKeyID,
				})
			}
		}
	}

	errProv := ""
	if time.Now().UnixMilli()-snapshot.lastErrorTs < 10000 {
		errProv = snapshot.lastErrorProvider
	}

	return StreamPayload{
		ActiveRequests: active,
		RecentRequests: recent,
		ErrorProvider:  errProv,
		Pending: PendingState{
			ByModel:   snapshot.byModel,
			ByAccount: snapshot.byAccount,
		},
	}
}

func parseModelKey(key string) (model, provider string) {
	// key format: "modelName (providerName)"
	var m, p string
	if n, _ := fmt.Sscanf(key, "%s (%s)", &m, &p); n == 2 {
		p = p[:len(p)-1] // remove trailing ')'
		return m, p
	}
	return key, "unknown"
}

// Subscribe registers a channel to receive encoded SSE payload events.
func (t *Tracker) Subscribe() (chan []byte, func()) {
	ch := make(chan []byte, 16)
	t.mu.Lock()
	t.subscribers[ch] = struct{}{}
	t.mu.Unlock()

	unsubscribe := func() {
		t.mu.Lock()
		delete(t.subscribers, ch)
		close(ch)
		t.mu.Unlock()
	}

	return ch, unsubscribe
}

func (t *Tracker) scheduleBroadcastLocked(repo *db.Repo) {
	if t.broadcastDebounce != nil {
		t.broadcastDebounce.Stop()
	}

	t.broadcastDebounce = time.AfterFunc(50*time.Millisecond, func() {
		t.mu.RLock()
		snapshot := t.snapshotLocked()
		subs := make([]chan []byte, 0, len(t.subscribers))
		for ch := range t.subscribers {
			subs = append(subs, ch)
		}
		t.mu.RUnlock()

		payload := t.buildPayload(repo, snapshot)
		b, err := json.Marshal(payload)
		if err != nil {
			return
		}

		for _, ch := range subs {
			select {
			case ch <- b:
			default:
			}
		}
	})
}
