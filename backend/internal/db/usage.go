package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type UsageLogRow struct {
	ID, Timestamp, Provider, Model, PublicModel, Status string
	PromptTokens, CompletionTokens                      int
	DurationMs                                          *int
}

// GetUsageDaily returns the daily usage JSON data for a given date key.
func (r *Repo) GetUsageDaily(dateKey string) (string, error) {
	var data string
	err := r.db.QueryRow(`SELECT data FROM usageDaily WHERE dateKey = ?`, dateKey).Scan(&data)
	if err != nil {
		return "", fmt.Errorf("get daily usage %s: %w", dateKey, err)
	}
	return data, nil
}

// InsertUsageHistory logs a single request's token usage to the usageHistory table.
func (r *Repo) InsertUsageHistory(provider, model, connectionID, apiKey, endpoint string, promptTokens, completionTokens int, cost float64, status string, totalTokens int, meta string, tokensJSON string) error {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(
		`INSERT INTO usageHistory (timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens, meta)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		timestamp, provider, model, connectionID, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokensJSON, meta,
	)
	if err != nil {
		return fmt.Errorf("insert usage history: %w", err)
	}
	return nil
}

func (r *Repo) InsertUserUsageHistory(userID, provider, model, connectionID, apiKey, endpoint string, promptTokens, completionTokens int, cost float64, status string, totalTokens int, meta string, tokensJSON string) error {
	timestamp := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(`INSERT INTO usageHistory (timestamp,provider,model,connectionId,apiKey,endpoint,promptTokens,completionTokens,cost,status,tokens,meta,userId) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		timestamp, provider, model, connectionID, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokensJSON, meta, userID)
	if err != nil {
		return fmt.Errorf("insert user usage history: %w", err)
	}
	return nil
}

func (r *Repo) GetUserUsage(userID string) (map[string]any, error) {
	var requests, prompt, completion int64
	var cost float64
	err := r.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(promptTokens),0),COALESCE(SUM(completionTokens),0),COALESCE(SUM(cost),0) FROM usageHistory WHERE userId=?`, userID).Scan(&requests, &prompt, &completion, &cost)
	if err != nil {
		return nil, err
	}
	return map[string]any{"totalRequests": requests, "promptTokens": prompt, "completionTokens": completion, "totalTokens": prompt + completion, "totalCost": cost}, nil
}

// GetUserUsageLogs returns only the public, user-owned request ledger. Internal
// provider and connection identifiers are intentionally not selected.
func (r *Repo) GetUserUsageLogs(userID string, limit, offset int) (map[string]any, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usageHistory WHERE userId = ?`, userID).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(`
		SELECT COALESCE(NULLIF(json_extract(meta, '$.requestId'), ''), printf('history-%d', id)), timestamp,
		       COALESCE(NULLIF(json_extract(meta, '$.publicModel'), ''), 'unknown'),
		       promptTokens, completionTokens, status,
		       COALESCE(json_extract(meta, '$.latencyMs'), 0)
		FROM usageHistory WHERE userId = ? ORDER BY id DESC LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id, ts, model, status string
		var prompt, completion, latency int
		if err := rows.Scan(&id, &ts, &model, &prompt, &completion, &status, &latency); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{
			"requestId": id, "timestamp": ts, "model": model, "status": status,
			"promptTokens": prompt, "completionTokens": completion,
			"totalTokens": prompt + completion, "durationMs": latency,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "total": total, "limit": limit, "offset": offset}, nil
}

// GetClientUsageLogs returns the same sanitized ledger scoped to a machine client.
func (r *Repo) GetClientUsageLogs(clientID string, limit, offset int) (map[string]any, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usageHistory WHERE json_extract(meta, '$.clientId') = ?`, clientID).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.db.Query(`
		SELECT COALESCE(NULLIF(json_extract(meta, '$.requestId'), ''), printf('history-%d', id)), timestamp,
		       COALESCE(NULLIF(json_extract(meta, '$.publicModel'), ''), 'unknown'), promptTokens, completionTokens, status,
		       COALESCE(json_extract(meta, '$.latencyMs'), 0)
		FROM usageHistory WHERE json_extract(meta, '$.clientId') = ? ORDER BY id DESC LIMIT ? OFFSET ?`, clientID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]map[string]any, 0, limit)
	for rows.Next() {
		var id, ts, model, status string
		var prompt, completion, latency int
		if err := rows.Scan(&id, &ts, &model, &prompt, &completion, &status, &latency); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"requestId": id, "timestamp": ts, "model": model, "status": status, "promptTokens": prompt, "completionTokens": completion, "totalTokens": prompt + completion, "durationMs": latency})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "total": total, "limit": limit, "offset": offset}, nil
}

// GetRecentUsageByAliases avoids the global in-memory ring limit for client
// telemetry. It selects only rows matching server-owned public aliases.
func (r *Repo) GetRecentUsageByAliases(aliasTargets map[string]string, limit int) ([]UsageLogRow, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	if len(aliasTargets) == 0 {
		return []UsageLogRow{}, nil
	}
	clauses := make([]string, 0, len(aliasTargets)*2)
	args := make([]any, 0, len(aliasTargets)*4)
	for alias, target := range aliasTargets {
		clauses = append(clauses, `json_extract(meta, '$.publicModel') = ?`)
		args = append(args, alias)
		parts := strings.SplitN(target, "/", 2)
		if len(parts) == 2 && !strings.HasPrefix(target, "combo:") {
			clauses = append(clauses, `(provider = ? AND model = ?)`)
			args = append(args, parts[0], parts[1])
		}
	}
	query := `SELECT COALESCE(NULLIF(json_extract(meta, '$.requestId'), ''), printf('history-%d', id)), timestamp, provider, model, COALESCE(NULLIF(json_extract(meta, '$.publicModel'), ''), ''), promptTokens, completionTokens, status, json_extract(meta, '$.latencyMs') FROM usageHistory WHERE ` + strings.Join(clauses, " OR ") + ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]UsageLogRow, 0, limit)
	for rows.Next() {
		var row UsageLogRow
		var latency sql.NullInt64
		if err := rows.Scan(&row.ID, &row.Timestamp, &row.Provider, &row.Model, &row.PublicModel, &row.PromptTokens, &row.CompletionTokens, &row.Status, &latency); err != nil {
			return nil, err
		}
		if latency.Valid { value := int(latency.Int64); row.DurationMs = &value }
		result = append(result, row)
	}
	if err := rows.Err(); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return result, nil
}

// UpsertUsageDaily inserts or replaces a daily usage aggregation record.
// The data parameter should be a JSON string matching the 9Router daily aggregation format.
// NOTE: INSERT OR REPLACE is an atomic full-row replace of the pre-merged JSON
// blob. Merging happens in-process (see handlers/chat/usage.go dailyUsageMu), so
// concurrent writers from MULTIPLE processes can still clobber each other. This
// is documented as single-writer unless the aggregation moves SQL-side.
func (r *Repo) UpsertUsageDaily(dateKey string, data string) error {
	_, err := r.db.Exec(
		`INSERT OR REPLACE INTO usageDaily (dateKey, data) VALUES (?, ?)`,
		dateKey, data,
	)
	if err != nil {
		return fmt.Errorf("upsert daily usage %s: %w", dateKey, err)
	}
	return nil
}

// InsertRequestDetail logs a request detail record for the Recent Requests dashboard tab.
func (r *Repo) InsertRequestDetail(id, provider, model, connectionID, status string, data string) error {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	_, err := r.db.Exec(
		`INSERT OR IGNORE INTO requestDetails (id, timestamp, provider, model, connectionId, status, data) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, timestamp, provider, model, connectionID, status, data,
	)
	if err != nil {
		return fmt.Errorf("insert request detail %s: %w", id, err)
	}
	return nil
}

// UpdateConnectionLastUsed updates the lastUsedAt timestamp and increments
// consecutiveUseCount for the given provider connection.
func (r *Repo) UpdateConnectionLastUsed(connectionID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(
		`UPDATE providerConnections SET lastUsedAt = ?, consecutiveUseCount = COALESCE(consecutiveUseCount, 0) + 1 WHERE id = ?`,
		now, connectionID,
	)
	if err != nil {
		return fmt.Errorf("update connection last used %s: %w", connectionID, err)
	}
	return nil
}
