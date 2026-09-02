package db

import (
	"database/sql"
	"fmt"
	"time"
)

// SaveSession persists a dashboard session token and its expiration timestamp to the SQLite kv table.
func (r *Repo) SaveSession(token string, expiry time.Time) error {
	expiryStr := expiry.UTC().Format(time.RFC3339)
	_, err := r.db.Exec(
		`INSERT INTO kv (scope, key, value) VALUES ('dashboard_sessions', ?, ?)
		 ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`,
		token, expiryStr,
	)
	if err != nil {
		return fmt.Errorf("save session to db: %w", err)
	}
	return nil
}

// GetSession retrieves a session expiry from SQLite kv table.
func (r *Repo) GetSession(token string) (time.Time, bool, error) {
	var expiryStr string
	err := r.db.QueryRow(
		`SELECT value FROM kv WHERE scope = 'dashboard_sessions' AND key = ? LIMIT 1`,
		token,
	).Scan(&expiryStr)

	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("query session: %w", err)
	}

	t, err := time.Parse(time.RFC3339, expiryStr)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse session expiry: %w", err)
	}

	if time.Now().After(t) {
		// Clean up expired session
		_ = r.DeleteSession(token)
		return time.Time{}, false, nil
	}

	return t, true, nil
}

// DeleteSession removes a session token from SQLite kv table.
func (r *Repo) DeleteSession(token string) error {
	_, err := r.db.Exec(`DELETE FROM kv WHERE scope = 'dashboard_sessions' AND key = ?`, token)
	return err
}

// LoadAllActiveSessions preloads all non-expired dashboard sessions on startup.
func (r *Repo) LoadAllActiveSessions() (map[string]time.Time, error) {
	rows, err := r.db.Query(`SELECT key, value FROM kv WHERE scope = 'dashboard_sessions'`)
	if err != nil {
		return nil, fmt.Errorf("load active sessions: %w", err)
	}
	defer rows.Close()

	now := time.Now()
	res := make(map[string]time.Time)
	var expiredTokens []string

	for rows.Next() {
		var token, expiryStr string
		if err := rows.Scan(&token, &expiryStr); err == nil {
			if t, err := time.Parse(time.RFC3339, expiryStr); err == nil {
				if now.Before(t) {
					res[token] = t
				} else {
					expiredTokens = append(expiredTokens, token)
				}
			}
		}
	}

	// Purge expired sessions in background
	for _, tok := range expiredTokens {
		_ = r.DeleteSession(tok)
	}

	return res, nil
}
