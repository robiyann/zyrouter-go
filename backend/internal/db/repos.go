package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"zyrouter/backend/internal/models"
)

type Repo struct {
	db *sql.DB
}

// NewRepo creates a new repository instance using the provided SQL database connection.
func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// RawDB returns the underlying *sql.DB connection for direct queries.
func (r *Repo) RawDB() *sql.DB {
	return r.db
}

// ==========================================
// API Key Methods (With Granular Restrictions)
// ==========================================

// ValidateApiKey checks if the given API key exists and is active.
func (r *Repo) ValidateApiKey(key string) (bool, error) {
	var active int
	err := r.db.QueryRow("SELECT isActive FROM apiKeys WHERE key = ? LIMIT 1", key).Scan(&active)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return active == 1, nil
}

// GetApiKeyByKey retrieves detailed APIKey information by key string.
func (r *Repo) GetApiKeyByKey(key string) (*models.APIKey, error) {
	var apiKey models.APIKey
	err := r.db.QueryRow(
		"SELECT id, key, name, machineId, isActive, restrictions, createdAt, clientId, policyId FROM apiKeys WHERE key = ? LIMIT 1",
		key,
	).Scan(&apiKey.ID, &apiKey.Key, &apiKey.Name, &apiKey.MachineID, &apiKey.IsActive, &apiKey.Restrictions, &apiKey.CreatedAt, &apiKey.ClientID, &apiKey.PolicyID)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

// GetApiKeyByID retrieves APIKey information by primary key ID.
func (r *Repo) GetApiKeyByID(id string) (*models.APIKey, error) {
	var apiKey models.APIKey
	err := r.db.QueryRow(
		"SELECT id, key, name, machineId, isActive, restrictions, createdAt, clientId, policyId FROM apiKeys WHERE id = ? LIMIT 1",
		id,
	).Scan(&apiKey.ID, &apiKey.Key, &apiKey.Name, &apiKey.MachineID, &apiKey.IsActive, &apiKey.Restrictions, &apiKey.CreatedAt, &apiKey.ClientID, &apiKey.PolicyID)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

// GetApiKeys retrieves all API keys.
func (r *Repo) GetApiKeys() ([]*models.APIKey, error) {
	rows, err := r.db.Query("SELECT id, key, name, machineId, isActive, restrictions, createdAt, clientId, policyId FROM apiKeys ORDER BY createdAt DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*models.APIKey
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(&k.ID, &k.Key, &k.Name, &k.MachineID, &k.IsActive, &k.Restrictions, &k.CreatedAt, &k.ClientID, &k.PolicyID); err != nil {
			return nil, err
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

// CreateApiKey inserts a new API key record with optional restrictions.
func (r *Repo) CreateApiKey(id, key, name, machineID string, restrictions *string) (*models.APIKey, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(
		`INSERT INTO apiKeys (id, key, name, machineId, isActive, restrictions, createdAt) VALUES (?, ?, ?, ?, 1, ?, ?)`,
		id, key, name, machineID, restrictions, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}
	return &models.APIKey{
		ID:           id,
		Key:          key,
		Name:         &name,
		MachineID:    &machineID,
		IsActive:     1,
		Restrictions: restrictions,
		CreatedAt:    now,
	}, nil
}

// UpdateApiKey updates an existing API key's name, active state, or restrictions.
func (r *Repo) UpdateApiKey(id string, name *string, isActive *int, restrictions *string) error {
	query := "UPDATE apiKeys SET "
	var args []any
	var sets []string

	if name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *name)
	}
	if isActive != nil {
		sets = append(sets, "isActive = ?")
		args = append(args, *isActive)
	}
	if restrictions != nil {
		sets = append(sets, "restrictions = ?")
		args = append(args, *restrictions)
	}

	if len(sets) == 0 {
		return nil
	}

	query += strings.Join(sets, ", ") + " WHERE id = ?"
	args = append(args, id)

	_, err := r.db.Exec(query, args...)
	return err
}

// DeleteApiKey removes an API key by primary key ID.
func (r *Repo) DeleteApiKey(id string) error {
	_, err := r.db.Exec("DELETE FROM apiKeys WHERE id = ?", id)
	return err
}

// ==========================================
// Provider Connection Methods
// ==========================================

// CreateProviderConnection inserts a new provider connection.
func (r *Repo) CreateProviderConnection(id, provider, authType, name string, apiKey string) error {
	data, err := json.Marshal(map[string]string{"apiKey": apiKey})
	if err != nil {
		return fmt.Errorf("marshal provider connection data: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = r.db.Exec(
		`INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, ?, ?, ?, 1, ?, ?, ?)`,
		id, provider, authType, name, string(data), now, now,
	)
	if err != nil {
		return fmt.Errorf("create provider connection: %w", err)
	}
	return nil
}

// CreateProviderConnectionFull inserts a full provider connection record.
func (r *Repo) CreateProviderConnectionFull(conn *models.ProviderConnection) error {
	now := time.Now().UTC().Format(time.RFC3339)
	conn.CreatedAt = now
	conn.UpdatedAt = now
	_, err := r.db.Exec(
		`INSERT INTO providerConnections (id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		conn.ID, conn.Provider, conn.AuthType, conn.Name, conn.Email, conn.Priority, conn.IsActive, conn.Data, conn.CreatedAt, conn.UpdatedAt,
	)
	return err
}

// GetProviderConnectionByID retrieves a single provider connection by primary key.
func (r *Repo) GetProviderConnectionByID(id string) (*models.ProviderConnection, error) {
	var conn models.ProviderConnection
	err := r.db.QueryRow(
		"SELECT id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt FROM providerConnections WHERE id = ? LIMIT 1",
		id,
	).Scan(&conn.ID, &conn.Provider, &conn.AuthType, &conn.Name, &conn.Email,
		&conn.Priority, &conn.IsActive, &conn.Data, &conn.CreatedAt, &conn.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &conn, nil
}

// GetProviderConnections retrieves provider connections. If activeOnly is true, only returns active ones.
func (r *Repo) GetProviderConnections(provider string, activeOnly bool) ([]*models.ProviderConnection, error) {
	var query string
	var args []any

	if provider != "" {
		if activeOnly {
			query = `SELECT id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt
				FROM providerConnections
				WHERE provider = ? AND isActive = 1
				ORDER BY CASE WHEN priority IS NULL THEN 999999 ELSE priority END ASC, updatedAt DESC`
		} else {
			query = `SELECT id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt
				FROM providerConnections
				WHERE provider = ?
				ORDER BY CASE WHEN priority IS NULL THEN 999999 ELSE priority END ASC, updatedAt DESC`
		}
		args = append(args, provider)
	} else {
		if activeOnly {
			query = `SELECT id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt
				FROM providerConnections
				WHERE isActive = 1
				ORDER BY CASE WHEN priority IS NULL THEN 999999 ELSE priority END ASC, updatedAt DESC`
		} else {
			query = `SELECT id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt
				FROM providerConnections
				ORDER BY CASE WHEN priority IS NULL THEN 999999 ELSE priority END ASC, updatedAt DESC`
		}
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var connections []*models.ProviderConnection
	for rows.Next() {
		var conn models.ProviderConnection
		err := rows.Scan(
			&conn.ID, &conn.Provider, &conn.AuthType, &conn.Name, &conn.Email,
			&conn.Priority, &conn.IsActive, &conn.Data, &conn.CreatedAt, &conn.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		connections = append(connections, &conn)
	}

	return connections, rows.Err()
}

// UpdateProviderConnection updates an existing provider connection.
func (r *Repo) UpdateProviderConnection(conn *models.ProviderConnection) error {
	now := time.Now().UTC().Format(time.RFC3339)
	conn.UpdatedAt = now
	_, err := r.db.Exec(
		`UPDATE providerConnections
		 SET provider = ?, authType = ?, name = ?, email = ?, priority = ?, isActive = ?, data = ?, updatedAt = ?
		 WHERE id = ?`,
		conn.Provider, conn.AuthType, conn.Name, conn.Email, conn.Priority, conn.IsActive, conn.Data, conn.UpdatedAt, conn.ID,
	)
	return err
}

// DeleteProviderConnection removes a provider connection by ID.
func (r *Repo) DeleteProviderConnection(id string) error {
	_, err := r.db.Exec("DELETE FROM providerConnections WHERE id = ?", id)
	return err
}

// ==========================================
// Combo Methods
// ==========================================

// CreateCombo inserts a new combo.
func (r *Repo) CreateCombo(combo *models.Combo) error {
	now := time.Now().UTC().Format(time.RFC3339)
	combo.CreatedAt = now
	combo.UpdatedAt = now
	if combo.Strategy == "" {
		combo.Strategy = "fallback"
	}
	_, err := r.db.Exec(
		`INSERT INTO combos (id, name, kind, models, strategy, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		combo.ID, combo.Name, combo.Kind, combo.Models, combo.Strategy, combo.CreatedAt, combo.UpdatedAt,
	)
	return err
}

// GetComboByName retrieves a combo configuration by its name.
func (r *Repo) GetComboByName(name string) (*models.Combo, error) {
	var combo models.Combo
	err := r.db.QueryRow(
		"SELECT id, name, kind, models, strategy, createdAt, updatedAt FROM combos WHERE name = ? LIMIT 1",
		name,
	).Scan(&combo.ID, &combo.Name, &combo.Kind, &combo.Models, &combo.Strategy, &combo.CreatedAt, &combo.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &combo, nil
}

// GetComboById retrieves a combo configuration by its ID.
func (r *Repo) GetComboById(id string) (*models.Combo, error) {
	var combo models.Combo
	err := r.db.QueryRow(
		"SELECT id, name, kind, models, strategy, createdAt, updatedAt FROM combos WHERE id = ? LIMIT 1",
		id,
	).Scan(&combo.ID, &combo.Name, &combo.Kind, &combo.Models, &combo.Strategy, &combo.CreatedAt, &combo.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &combo, nil
}

// GetCombos retrieves all combos from the database.
func (r *Repo) GetCombos() ([]*models.Combo, error) {
	rows, err := r.db.Query("SELECT id, name, kind, models, strategy, createdAt, updatedAt FROM combos ORDER BY createdAt ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var combos []*models.Combo
	for rows.Next() {
		var combo models.Combo
		err := rows.Scan(&combo.ID, &combo.Name, &combo.Kind, &combo.Models, &combo.Strategy, &combo.CreatedAt, &combo.UpdatedAt)
		if err != nil {
			return nil, err
		}
		combos = append(combos, &combo)
	}

	return combos, rows.Err()
}

// UpdateCombo updates an existing combo.
func (r *Repo) UpdateCombo(combo *models.Combo) error {
	now := time.Now().UTC().Format(time.RFC3339)
	combo.UpdatedAt = now
	_, err := r.db.Exec(
		`UPDATE combos SET name = ?, kind = ?, models = ?, strategy = ?, updatedAt = ? WHERE id = ?`,
		combo.Name, combo.Kind, combo.Models, combo.Strategy, combo.UpdatedAt, combo.ID,
	)
	return err
}

// DeleteCombo deletes a combo by ID.
func (r *Repo) DeleteCombo(id string) error {
	_, err := r.db.Exec("DELETE FROM combos WHERE id = ?", id)
	return err
}

// ==========================================
// KV & Aliases Methods
// ==========================================

// GetModelAlias retrieves the target model string for a given alias.
func (r *Repo) GetModelAlias(alias string) (string, error) {
	var rawVal string
	err := r.db.QueryRow(
		"SELECT value FROM kv WHERE scope = 'modelAliases' AND key = ? LIMIT 1",
		alias,
	).Scan(&rawVal)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get model alias %s: %w", alias, err)
	}
	return parseJSONString(rawVal), nil
}

// GetModelAliases returns all model aliases as a key-value map.
func (r *Repo) GetModelAliases() (map[mapKey]string, error) {
	rows, err := r.db.Query("SELECT key, value FROM kv WHERE scope = 'modelAliases'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := make(map[mapKey]string)
	for rows.Next() {
		var key, rawVal string
		if err := rows.Scan(&key, &rawVal); err != nil {
			return nil, err
		}
		aliases[key] = parseJSONString(rawVal)
	}

	return aliases, rows.Err()
}

// SetModelAlias stores or updates a model alias in the KV table.
func (r *Repo) SetModelAlias(alias, target string) error {
	valBytes, _ := json.Marshal(target)
	_, err := r.db.Exec(
		`INSERT INTO kv (scope, key, value) VALUES ('modelAliases', ?, ?)
		 ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`,
		alias, string(valBytes),
	)
	return err
}

// DeleteModelAlias removes a model alias.
func (r *Repo) DeleteModelAlias(alias string) error {
	_, err := r.db.Exec("DELETE FROM kv WHERE scope = 'modelAliases' AND key = ?", alias)
	return err
}

// ==========================================
// Provider Nodes & Helpers
// ==========================================

type ProviderNodeData struct {
	Prefix  string `json:"prefix"`
	APIType string `json:"apiType"`
	BaseURL string `json:"baseUrl"`
}

func (r *Repo) GetProviderNodeByID(id string) (*models.ProviderNode, *ProviderNodeData, error) {
	var node models.ProviderNode
	err := r.db.QueryRow(
		"SELECT id, type, name, data, createdAt, updatedAt FROM providerNodes WHERE id = ? LIMIT 1",
		id,
	).Scan(&node.ID, &node.Type, &node.Name, &node.Data, &node.CreatedAt, &node.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}

	nodeData := parseProviderNodeData(node.Data)
	return &node, nodeData, nil
}

func (r *Repo) GetProviderNodeByPrefix(prefix string) (*models.ProviderNode, *ProviderNodeData, error) {
	rows, err := r.db.Query("SELECT id, type, name, data, createdAt, updatedAt FROM providerNodes")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var node models.ProviderNode
		if err := rows.Scan(&node.ID, &node.Type, &node.Name, &node.Data, &node.CreatedAt, &node.UpdatedAt); err != nil {
			return nil, nil, err
		}
		nodeData := parseProviderNodeData(node.Data)
		if nodeData != nil && nodeData.Prefix == prefix {
			return &node, nodeData, nil
		}
	}

	return nil, nil, rows.Err()
}

func (r *Repo) GetProviderNodes() ([]*models.ProviderNode, error) {
	rows, err := r.db.Query("SELECT id, type, name, data, createdAt, updatedAt FROM providerNodes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*models.ProviderNode
	for rows.Next() {
		var node models.ProviderNode
		if err := rows.Scan(&node.ID, &node.Type, &node.Name, &node.Data, &node.CreatedAt, &node.UpdatedAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, &node)
	}
	return nodes, rows.Err()
}

func (r *Repo) CreateProviderNode(id, nodeType, name string, data map[string]any) (*models.ProviderNode, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	dataBytes, _ := json.Marshal(data)
	dataStr := string(dataBytes)
	if id == "" {
		id = fmt.Sprintf("%s-%d", nodeType, time.Now().UnixNano())
	}
	_, err := r.db.Exec(
		`INSERT INTO providerNodes (id, type, name, data, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?)`,
		id, nodeType, name, dataStr, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("create provider node: %w", err)
	}
	return &models.ProviderNode{
		ID:        id,
		Type:      &nodeType,
		Name:      &name,
		Data:      dataStr,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (r *Repo) UpdateProviderNode(id, name string, data map[string]any) error {
	now := time.Now().UTC().Format(time.RFC3339)
	dataBytes, _ := json.Marshal(data)
	dataStr := string(dataBytes)
	_, err := r.db.Exec(
		`UPDATE providerNodes SET name = ?, data = ?, updatedAt = ? WHERE id = ?`,
		name, dataStr, now, id,
	)
	return err
}

func (r *Repo) DeleteProviderNode(id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Custom provider connections use the node ID as their provider value.
	// Remove them with the node so deleted nodes cannot remain routable/orphaned.
	if _, err := tx.Exec("DELETE FROM providerConnections WHERE provider = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM providerNodes WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func parseProviderNodeData(raw string) *ProviderNodeData {
	if raw == "" {
		return nil
	}
	var d ProviderNodeData
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return nil
	}
	return &d
}

type mapKey = string

func parseJSONString(raw string) string {
	var val string
	if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		if err := json.Unmarshal([]byte(raw), &val); err == nil {
			return val
		}
	}
	return raw
}
