package db

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CustomModelEntry represents a custom model registered for a specific provider.
type CustomModelEntry struct {
	ProviderAlias string `json:"providerAlias"`
	ID            string `json:"id"`
	Type          string `json:"type"`
	Name          string `json:"name"`
}

// GetCustomModels retrieves all custom model entries from the kv table.
func (r *Repo) GetCustomModels() ([]CustomModelEntry, error) {
	rows, err := r.db.Query("SELECT value FROM kv WHERE scope = 'customModels'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []CustomModelEntry
	for rows.Next() {
		var rawVal string
		if err := rows.Scan(&rawVal); err == nil {
			var entry CustomModelEntry
			if json.Unmarshal([]byte(rawVal), &entry) == nil && entry.ID != "" {
				list = append(list, entry)
			}
		}
	}
	return list, nil
}

// GetCustomModelsByProvider returns all custom model IDs for a provider.
func (r *Repo) GetCustomModelsByProvider(provider string) ([]string, error) {
	all, err := r.GetCustomModels()
	if err != nil {
		return nil, err
	}
	var res []string
	pLower := strings.ToLower(provider)
	for _, m := range all {
		if strings.ToLower(m.ProviderAlias) == pLower {
			res = append(res, m.ID)
		}
	}
	return res, nil
}

// AddCustomModel saves a custom model under scope 'customModels'.
func (r *Repo) AddCustomModel(provider, modelID, modelType, name string) error {
	if modelType == "" {
		modelType = "llm"
	}
	if name == "" {
		name = modelID
	}
	k := fmt.Sprintf("%s|%s|%s", provider, modelID, modelType)
	entry := CustomModelEntry{
		ProviderAlias: provider,
		ID:            modelID,
		Type:          modelType,
		Name:          name,
	}
	valBytes, _ := json.Marshal(entry)
	_, err := r.db.Exec(
		`INSERT INTO kv (scope, key, value) VALUES ('customModels', ?, ?)
		 ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`,
		k, string(valBytes),
	)
	return err
}

// DeleteCustomModel removes a custom model from scope 'customModels'.
func (r *Repo) DeleteCustomModel(provider, modelID, modelType string) error {
	if modelType == "" {
		modelType = "llm"
	}
	k := fmt.Sprintf("%s|%s|%s", provider, modelID, modelType)
	_, err := r.db.Exec("DELETE FROM kv WHERE scope = 'customModels' AND key = ?", k)
	return err
}
