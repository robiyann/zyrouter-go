package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/handlerutil"
	"zyrouter/backend/internal/labels"
	"zyrouter/backend/internal/usagetracker"
)

const usagePingInterval = 25 * time.Second

// HandleUsageStream serves real-time active requests and usage stats over SSE for the dashboard.
func HandleUsageStream(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		tracker := usagetracker.GetTracker()

		send := func(b []byte) bool {
			if _, err := w.Write([]byte("data: " + string(b) + "\n\n")); err != nil {
				return false
			}
			flusher.Flush()
			return true
		}

		// Send initial state immediately
		initialState := tracker.GetActiveState(repo)
		initBytes, err := json.Marshal(initialState)
		if err == nil {
			if !send(initBytes) {
				return
			}
		}

		ch, unsubscribe := tracker.Subscribe()
		defer unsubscribe()

		ping := time.NewTicker(usagePingInterval)
		defer ping.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case b, ok := <-ch:
				if !ok {
					return
				}
				if !send(b) {
					return
				}
			case <-ping.C:
				if _, err := w.Write([]byte(": ping\n\n")); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

// HandleUsageStats returns current active and pending stats as JSON.
func HandleUsageStats(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tracker := usagetracker.GetTracker()
		state := tracker.GetActiveState(repo)
		stats, recentReqs, err := readUsageStats(repo, r)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		seen := make(map[string]bool)
		var combinedRecent []usagetracker.RecentRequest
		for _, req := range state.RecentRequests {
			k := fmt.Sprintf("%s|%s|%s", req.Timestamp, req.Provider, req.Model)
			if !seen[k] {
				seen[k] = true
				combinedRecent = append(combinedRecent, req)
			}
		}
		for _, req := range recentReqs {
			k := fmt.Sprintf("%s|%s|%s", req.Timestamp, req.Provider, req.Model)
			if !seen[k] {
				seen[k] = true
				combinedRecent = append(combinedRecent, req)
			}
		}
		if len(combinedRecent) > 50 {
			combinedRecent = combinedRecent[:50]
		}
		recent := combinedRecent
		response := map[string]any{
			"activeRequests":   state.ActiveRequests,
			"recentRequests":   recent,
			"errorProvider":    state.ErrorProvider,
			"pending":          state.Pending,
			"totalRequests":    stats.TotalRequests,
			"promptTokens":     stats.PromptTokens,
			"completionTokens": stats.CompletionTokens,
			"totalTokens":      stats.PromptTokens + stats.CompletionTokens,
			"totalCost":        stats.TotalCost,
			"daily":            stats.Daily,
		}
		handlerutil.WriteJSON(w, http.StatusOK, response)
	}
}

type usageStats struct {
	TotalRequests    int
	PromptTokens     int
	CompletionTokens int
	TotalCost        float64
	Daily            []map[string]any
}

func readUsageStats(repo *db.Repo, r *http.Request) (usageStats, []usagetracker.RecentRequest, error) {
	var result usageStats
	var recent []usagetracker.RecentRequest

	if repo == nil || repo.RawDB() == nil {
		result.Daily = []map[string]any{}
		return result, recent, nil
	}

	days := 30
	rawDays := strings.TrimSpace(r.URL.Query().Get("days"))
	if rawDays == "all" || rawDays == "0" {
		days = 0
	} else if rawDays != "" {
		if parsed, err := strconv.Atoi(rawDays); err == nil && parsed >= 0 && parsed <= 3650 {
			days = parsed
		}
	}

	buildQuery := func(daysLimit int) (usageStats, error) {
		var stats usageStats
		where := []string{"1=1"}
		var args []any

		if daysLimit > 0 {
			cutoff := time.Now().UTC().AddDate(0, 0, -daysLimit).Format("2006-01-02")
			where = append(where, "substr(timestamp, 1, 10) >= ?")
			args = append(args, cutoff)
		}
		if provider := r.URL.Query().Get("provider"); provider != "" {
			where = append(where, "provider = ?")
			args = append(args, provider)
		}
		if model := r.URL.Query().Get("model"); model != "" {
			where = append(where, "model = ?")
			args = append(args, model)
		}
		clause := strings.Join(where, " AND ")

		// Sum tokens from numeric columns OR fallback to JSON tokens object
		promptSql := "COALESCE(SUM(CASE WHEN promptTokens > 0 THEN promptTokens ELSE COALESCE(json_extract(tokens, '$.prompt_tokens'), json_extract(tokens, '$.input_tokens'), 0) END), 0)"
		compSql := "COALESCE(SUM(CASE WHEN completionTokens > 0 THEN completionTokens ELSE COALESCE(json_extract(tokens, '$.completion_tokens'), json_extract(tokens, '$.output_tokens'), 0) END), 0)"
		query := fmt.Sprintf("SELECT COUNT(*), %s, %s, COALESCE(SUM(cost), 0) FROM usageHistory WHERE %s", promptSql, compSql, clause)

		if err := repo.RawDB().QueryRow(query, args...).Scan(&stats.TotalRequests, &stats.PromptTokens, &stats.CompletionTokens, &stats.TotalCost); err != nil {
			return stats, fmt.Errorf("read usage totals: %w", err)
		}

		dailyQuery := fmt.Sprintf("SELECT substr(timestamp, 1, 10), COUNT(*), %s, %s, COALESCE(SUM(cost), 0) FROM usageHistory WHERE %s GROUP BY substr(timestamp, 1, 10) ORDER BY substr(timestamp, 1, 10) DESC LIMIT 90", promptSql, compSql, clause)
		rows, err := repo.RawDB().Query(dailyQuery, args...)
		if err != nil {
			return stats, fmt.Errorf("read daily usage: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var date string
			var requests, prompt, completion int
			var cost float64
			if err := rows.Scan(&date, &requests, &prompt, &completion, &cost); err == nil {
				stats.Daily = append(stats.Daily, map[string]any{
					"date":             date,
					"requests":         requests,
					"promptTokens":     prompt,
					"completionTokens": completion,
					"tokens":           prompt + completion,
					"cost":             cost,
				})
			}
		}
		if stats.Daily == nil {
			stats.Daily = []map[string]any{}
		}
		return stats, nil
	}

	var err error
	result, err = buildQuery(days)
	if err != nil {
		return result, recent, err
	}

	// If 7/30-day window returned 0 but historical data exists in table, fallback to all-time query
	if result.TotalRequests == 0 && days > 0 {
		if allStats, err := buildQuery(0); err == nil && allStats.TotalRequests > 0 {
			result = allStats
		}
	}

	// Read recent requests from SQLite history (last 50 requests)
	recentQuery := `SELECT timestamp, provider, model, 
		CASE WHEN promptTokens > 0 THEN promptTokens ELSE COALESCE(json_extract(tokens, '$.prompt_tokens'), json_extract(tokens, '$.input_tokens'), 0) END,
		CASE WHEN completionTokens > 0 THEN completionTokens ELSE COALESCE(json_extract(tokens, '$.completion_tokens'), json_extract(tokens, '$.output_tokens'), 0) END,
		status FROM usageHistory ORDER BY id DESC LIMIT 50`
	recentRows, err := repo.RawDB().Query(recentQuery)
	if err == nil {
		defer recentRows.Close()
		for recentRows.Next() {
			var ts, prov, mod, status string
			var prompt, completion int
			if err := recentRows.Scan(&ts, &prov, &mod, &prompt, &completion, &status); err == nil {
				displayProvider := labels.Provider(repo, prov)
				displayModel := labels.Model(repo, prov, mod)
				recent = append(recent, usagetracker.RecentRequest{
					Timestamp:        ts,
					Provider:         displayProvider,
					Model:            displayModel,
					PromptTokens:     prompt,
					CompletionTokens: completion,
					Status:           status,
				})
			}
		}
	}

	return result, recent, nil
}
