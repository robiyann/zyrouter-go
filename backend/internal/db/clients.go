package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"zyrouter/backend/internal/models"
)

func HashClientToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (r *Repo) CreateClientPolicy(id, name string, data map[string]any) (*models.ClientPolicy, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	_, err = r.db.Exec(`INSERT INTO clientPolicies (id, name, isActive, data, createdAt, updatedAt) VALUES (?, ?, 1, ?, ?, ?)`, id, name, string(b), now, now)
	if err != nil {
		return nil, fmt.Errorf("create client policy: %w", err)
	}
	return &models.ClientPolicy{ID: id, Name: name, IsActive: 1, Data: string(b), CreatedAt: now, UpdatedAt: now}, nil
}

func (r *Repo) GetClientPolicy(id string) (*models.ClientPolicy, error) {
	var policy models.ClientPolicy
	err := r.db.QueryRow(`SELECT id, name, isActive, data, createdAt, updatedAt FROM clientPolicies WHERE id = ?`, id).
		Scan(&policy.ID, &policy.Name, &policy.IsActive, &policy.Data, &policy.CreatedAt, &policy.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

func (r *Repo) CreateClient(id, name, email, accessTokenHash, policyID string) (*models.Client, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(`INSERT INTO clients (id, name, email, accessTokenHash, policyId, isActive, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, 1, ?, ?)`, id, name, email, accessTokenHash, policyID, now, now)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}
	return &models.Client{ID: id, Name: name, Email: stringPtr(email), PolicyID: stringPtr(policyID), IsActive: 1, CreatedAt: now, UpdatedAt: now}, nil
}

func (r *Repo) GetClientByAccessTokenHash(tokenHash string) (*models.Client, error) {
	var client models.Client
	var email, policyID sql.NullString
	err := r.db.QueryRow(`SELECT id, name, email, policyId, isActive, createdAt, updatedAt FROM clients WHERE accessTokenHash = ? LIMIT 1`, tokenHash).
		Scan(&client.ID, &client.Name, &email, &policyID, &client.IsActive, &client.CreatedAt, &client.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if email.Valid {
		client.Email = &email.String
	}
	if policyID.Valid {
		client.PolicyID = &policyID.String
	}
	return &client, nil
}

func (r *Repo) GetApiKeysByClientID(clientID string) ([]*models.APIKey, error) {
	rows, err := r.db.Query(`SELECT id, key, name, machineId, isActive, restrictions, createdAt, clientId, policyId FROM apiKeys WHERE clientId = ? ORDER BY createdAt DESC`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []*models.APIKey
	for rows.Next() {
		var key models.APIKey
		if err := rows.Scan(&key.ID, &key.Key, &key.Name, &key.MachineID, &key.IsActive, &key.Restrictions, &key.CreatedAt, &key.ClientID, &key.PolicyID); err != nil {
			return nil, err
		}
		keys = append(keys, &key)
	}
	return keys, rows.Err()
}

func (r *Repo) CreateClientApiKey(id, key, name, clientID, policyID, restrictions string) (*models.APIKey, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(`INSERT INTO apiKeys (id, key, name, isActive, restrictions, createdAt, clientId, policyId) VALUES (?, ?, ?, 1, ?, ?, ?, ?)`, id, key, name, restrictions, now, clientID, policyID)
	if err != nil {
		return nil, fmt.Errorf("create client api key: %w", err)
	}
	return &models.APIKey{ID: id, Key: key, Name: stringPtr(name), IsActive: 1, Restrictions: stringPtr(restrictions), ClientID: stringPtr(clientID), PolicyID: stringPtr(policyID), CreatedAt: now}, nil
}

func (r *Repo) RevokeClientApiKey(id, clientID string) error {
	result, err := r.db.Exec(`UPDATE apiKeys SET isActive = 0 WHERE id = ? AND clientId = ?`, id, clientID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repo) GetClientUsage(clientID string) (map[string]any, error) {
	var requests, promptTokens, completionTokens int
	var cost float64
	err := r.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(uh.promptTokens), 0), COALESCE(SUM(uh.completionTokens), 0), COALESCE(SUM(uh.cost), 0)
		FROM usageHistory uh JOIN apiKeys ak ON ak.key = uh.apiKey
		WHERE ak.clientId = ?`, clientID).Scan(&requests, &promptTokens, &completionTokens, &cost)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"totalRequests":    requests,
		"promptTokens":     promptTokens,
		"completionTokens": completionTokens,
		"totalTokens":      promptTokens + completionTokens,
		"totalCost":        cost,
	}, nil
}

// CheckAPIKeyRateLimit checks persisted usage against a key's server-owned
// request/minute and token/day limits. Zero limits mean unlimited.
func (r *Repo) CheckAPIKeyRateLimit(key string, limit *models.KeyRateLimit, now time.Time) (bool, error) {
	if limit == nil {
		return true, nil
	}
	if limit.RequestsPerMinute > 0 {
		var requests int
		cutoff := now.UTC().Add(-time.Minute).Format(time.RFC3339)
		if err := r.db.QueryRow(`SELECT COUNT(*) FROM usageHistory WHERE apiKey = ? AND datetime(timestamp) >= datetime(?)`, key, cutoff).Scan(&requests); err != nil {
			return false, err
		}
		if requests >= limit.RequestsPerMinute {
			return false, nil
		}
	}
	if limit.TokensPerDay > 0 {
		var tokens sql.NullInt64
		start := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
		if err := r.db.QueryRow(`SELECT COALESCE(SUM(promptTokens + completionTokens), 0) FROM usageHistory WHERE apiKey = ? AND datetime(timestamp) >= datetime(?)`, key, start).Scan(&tokens); err != nil {
			return false, err
		}
		if tokens.Int64 >= int64(limit.TokensPerDay) {
			return false, nil
		}
	}
	return true, nil
}

func stringPtr(value string) *string { return &value }
