package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/usagetracker"
)

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
			"provider": recent.Provider, "status": recent.Status,
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
