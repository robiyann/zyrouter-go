package db

import (
	"encoding/json"
	"strings"
)

// GetProviderPrefixes returns all custom provider prefix mappings from kv table.
func (r *Repo) GetProviderPrefixes() (map[string]string, error) {
	rows, err := r.db.Query("SELECT key, value FROM kv WHERE scope = 'providerPrefixes'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			var decoded string
			if json.Unmarshal([]byte(v), &decoded) == nil {
				res[k] = decoded
			} else {
				res[k] = v
			}
		}
	}
	return res, nil
}

// SetProviderPrefix stores a custom prefix for a provider in the kv table.
func (r *Repo) SetProviderPrefix(provider, prefix string) error {
	provider = strings.TrimSpace(strings.ToLower(provider))
	prefix = strings.TrimSpace(strings.ToLower(prefix))
	valBytes, _ := json.Marshal(prefix)
	_, err := r.db.Exec(
		`INSERT INTO kv (scope, key, value) VALUES ('providerPrefixes', ?, ?)
		 ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`,
		provider, string(valBytes),
	)
	return err
}

// DeleteProviderPrefix removes a custom prefix for a provider.
func (r *Repo) DeleteProviderPrefix(provider string) error {
	provider = strings.TrimSpace(strings.ToLower(provider))
	_, err := r.db.Exec("DELETE FROM kv WHERE scope = 'providerPrefixes' AND key = ?", provider)
	return err
}
