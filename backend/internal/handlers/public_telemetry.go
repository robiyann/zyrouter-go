package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/middleware"
	"zyrouter/backend/internal/models"
	"zyrouter/backend/internal/providers"
	"zyrouter/backend/internal/usagetracker"
)

var clientTelemetryCache struct {
	sync.Mutex
	at   time.Time
	data map[string]any
}

func publicAlias(repo *db.Repo, model string) string {
	requestedProvider, requestedModel := "", strings.TrimSpace(model)
	if slash := strings.IndexByte(requestedModel, '/'); slash >= 0 {
		requestedProvider, requestedModel = strings.ToLower(requestedModel[:slash]), strings.TrimSpace(requestedModel[slash+1:])
	}
	if repo != nil {
		if aliases, err := repo.GetModelAliases(); err == nil {
			matches := make([]string, 0, 2)
			for alias, target := range aliases {
				if strings.HasPrefix(target, "combo:") {
					continue
				}
				targetProvider, targetModel := "", target
				if slash := strings.IndexByte(target, '/'); slash >= 0 {
					targetProvider, targetModel = strings.ToLower(target[:slash]), target[slash+1:]
				}
				providerMatches := requestedProvider == "" || providers.ResolveAlias(requestedProvider) == providers.ResolveAlias(targetProvider)
				if providerMatches && strings.TrimSpace(targetModel) == requestedModel {
					matches = append(matches, alias)
				}
			}
			if len(matches) == 1 {
				return matches[0]
			}
		}
	}
	return "unpublished"
}

func allowedAliasesForClient(repo *db.Repo, r *http.Request) map[string]bool {
	allowed := make(map[string]bool)
	if user := middleware.GetAuthenticatedUser(r); user != nil {
		aliases, _ := repo.GetAccountTypeModels(user.AccountTypeID)
		for _, alias := range aliases {
			allowed[strings.ToLower(alias)] = true
		}
		return allowed
	}
	if client := middleware.GetAuthenticatedClient(r); client != nil && client.PolicyID != nil {
		if policy, err := repo.GetClientPolicy(*client.PolicyID); err == nil && policy != nil {
			var data models.KeyRestrictions
			if json.Unmarshal([]byte(policy.Data), &data) == nil {
				for _, alias := range data.AllowedModels {
					allowed[strings.ToLower(alias)] = true
				}
			}
		}
	}
	return allowed
}

func filterClientTelemetry(data map[string]any, allowed map[string]bool) map[string]any {
	if allowed == nil {
		return data
	}
	filtered := make(map[string]any, len(data))
	for key, value := range data {
		if key != "recent" {
			filtered[key] = value
		}
	}
	recent := make([]map[string]any, 0)
	if items, ok := data["recent"].([]map[string]any); ok {
		for _, item := range items {
			alias, _ := item["publicModel"].(string)
			if alias == "" {
				alias, _ = item["model"].(string)
			}
			if allowed[strings.ToLower(alias)] {
				recent = append(recent, item)
			}
		}
	}
	filtered["recent"] = recent
	return filtered
}

func clientTelemetry(repo *db.Repo, allowed map[string]bool) (map[string]any, error) {
	clientTelemetryCache.Lock()
	if clientTelemetryCache.data != nil && time.Since(clientTelemetryCache.at) < time.Second {
		data := clientTelemetryCache.data
		clientTelemetryCache.Unlock()
		return filterClientTelemetry(data, allowed), nil
	}
	clientTelemetryCache.Unlock()
	snapshot, err := buildPublicTelemetry(repo)
	if err != nil {
		return nil, err
	}
	recent := make([]map[string]any, 0, len(snapshot.RecentRequests))
	for _, item := range snapshot.RecentRequests {
		modelName, _ := item["publicModel"].(string)
		if strings.TrimSpace(modelName) == "" {
			modelName = publicAlias(repo, fmt.Sprint(item["model"]))
		}
		recent = append(recent, map[string]any{
			"timestamp": item["timestamp"], "model": modelName, "publicModel": modelName,
			"status": item["status"], "promptTokens": item["promptTokens"],
			"completionTokens": item["completionTokens"], "totalTokens": item["totalTokens"],
			"durationMs": item["durationMs"],
		})
	}
	data := map[string]any{
		"timestamp": snapshot.Timestamp, "totalRequests": snapshot.TotalRequests,
		"promptTokens": snapshot.PromptTokens, "completionTokens": snapshot.CompletionTokens,
		"totalTokens": snapshot.TotalTokens, "activeRequests": snapshot.ActiveRequests, "recent": recent,
	}
	clientTelemetryCache.Lock()
	clientTelemetryCache.at = time.Now()
	clientTelemetryCache.data = data
	clientTelemetryCache.Unlock()
	return filterClientTelemetry(data, allowed), nil
}

func HandleClientTelemetryStats(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := clientTelemetry(repo, allowedAliasesForClient(repo, r))
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to load global telemetry")
			return
		}
		handlerutil.WriteJSON(w, http.StatusOK, body)
	}
}

func HandleClientTelemetryStream(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("X-Accel-Buffering", "no")
		send := func() bool {
			body, err := clientTelemetry(repo, allowedAliasesForClient(repo, r))
			if err != nil {
				return false
			}
			payload, _ := json.Marshal(body)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return false
			}
			flusher.Flush()
			return true
		}
		if !send() {
			return
		}
		subscriber, unsubscribe := usagetracker.GetTracker().Subscribe()
		defer unsubscribe()
		ping := time.NewTicker(20 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-subscriber:
				if !send() {
					return
				}
			case <-ping.C:
				if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

// Public telemetry is deliberately token-gated. It exposes aggregate traffic
// metrics without exposing account names, API keys, proxy URLs, or payloads.
func publicTelemetryTokenOK(r *http.Request) bool {
	expected := os.Getenv("PUBLIC_TELEMETRY_TOKEN")
	if expected == "" {
		return false
	}
	provided := r.Header.Get("X-Zyrouter-Telemetry-Token")
	if provided == "" {
		provided = r.Header.Get("Authorization")
		if len(provided) > 7 && provided[:7] == "Bearer " {
			provided = provided[7:]
		}
	}
	if provided == "" || len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func publicTelemetryHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Zyrouter-Telemetry-Token")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
}

type publicTelemetryModel struct {
	Model            string `json:"model"`
	Requests         int    `json:"requests"`
	PromptTokens     int    `json:"promptTokens"`
	CompletionTokens int    `json:"completionTokens"`
	TotalTokens      int    `json:"totalTokens"`
}

type publicTelemetrySnapshot struct {
	Timestamp        string                 `json:"timestamp"`
	TotalRequests    int                    `json:"totalRequests"`
	PromptTokens     int                    `json:"promptTokens"`
	CompletionTokens int                    `json:"completionTokens"`
	TotalTokens      int                    `json:"totalTokens"`
	ActiveRequests   int                    `json:"activeRequests"`
	ByModel          []publicTelemetryModel `json:"byModel"`
	RecentRequests   []map[string]any       `json:"recentRequests"`
}

func buildPublicTelemetry(repo *db.Repo) (publicTelemetrySnapshot, error) {
	snapshot := publicTelemetrySnapshot{
		Timestamp:      time.Now().UTC().Format(time.RFC3339Nano),
		ByModel:        []publicTelemetryModel{},
		RecentRequests: []map[string]any{},
	}
	if repo == nil || repo.RawDB() == nil {
		return snapshot, nil
	}
	const tokenExpr = `CASE WHEN promptTokens > 0 THEN promptTokens ELSE COALESCE(json_extract(tokens, '$.prompt_tokens'), 0) END`
	const completionExpr = `CASE WHEN completionTokens > 0 THEN completionTokens ELSE COALESCE(json_extract(tokens, '$.completion_tokens'), 0) END`
	if err := repo.RawDB().QueryRow(`SELECT COUNT(*), COALESCE(SUM(`+tokenExpr+`), 0), COALESCE(SUM(`+completionExpr+`), 0) FROM usageHistory`).Scan(&snapshot.TotalRequests, &snapshot.PromptTokens, &snapshot.CompletionTokens); err != nil {
		return snapshot, err
	}
	snapshot.TotalTokens = snapshot.PromptTokens + snapshot.CompletionTokens
	rows, err := repo.RawDB().Query(`SELECT model, COUNT(*), COALESCE(SUM(` + tokenExpr + `), 0), COALESCE(SUM(` + completionExpr + `), 0) FROM usageHistory GROUP BY model ORDER BY COUNT(*) DESC LIMIT 200`)
	if err != nil {
		return snapshot, err
	}
	defer rows.Close()
	for rows.Next() {
		var model string
		var requests, prompt, completion int
		if err := rows.Scan(&model, &requests, &prompt, &completion); err != nil {
			return snapshot, err
		}
		snapshot.ByModel = append(snapshot.ByModel, publicTelemetryModel{Model: model, Requests: requests, PromptTokens: prompt, CompletionTokens: completion, TotalTokens: prompt + completion})
	}
	state := usagetracker.GetTracker().GetActiveState(repo)
	for _, active := range state.ActiveRequests {
		snapshot.ActiveRequests += active.Count
	}
	for _, recent := range state.RecentRequests {
		snapshot.RecentRequests = append(snapshot.RecentRequests, map[string]any{
			"id": recent.ID, "timestamp": recent.Timestamp, "model": recent.Model,
			"publicModel": recent.PublicModel,
			"provider":    recent.Provider, "status": recent.Status,
			"promptTokens": recent.PromptTokens, "completionTokens": recent.CompletionTokens,
			"totalTokens": recent.PromptTokens + recent.CompletionTokens,
			"durationMs":  recent.DurationMs, "latency": recent.Latency,
		})
	}
	return snapshot, rows.Err()
}

func HandlePublicTelemetryStats(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicTelemetryHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !publicTelemetryTokenOK(r) {
			handlerutil.WriteJSONError(w, http.StatusUnauthorized, "public telemetry token required")
			return
		}
		snapshot, err := buildPublicTelemetry(repo)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		handlerutil.WriteJSON(w, http.StatusOK, snapshot)
	}
}

func HandlePublicTelemetryStream(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicTelemetryHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !publicTelemetryTokenOK(r) {
			handlerutil.WriteJSONError(w, http.StatusUnauthorized, "public telemetry token required")
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		send := func() bool {
			snapshot, err := buildPublicTelemetry(repo)
			if err != nil {
				return false
			}
			body, _ := json.Marshal(snapshot)
			if _, err := fmt.Fprintf(w, "data: %s\n\n", body); err != nil {
				return false
			}
			flusher.Flush()
			return true
		}
		if !send() {
			return
		}
		subscriber, unsubscribe := usagetracker.GetTracker().Subscribe()
		defer unsubscribe()
		ping := time.NewTicker(25 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-subscriber:
				if !send() {
					return
				}
			case <-ping.C:
				if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}
