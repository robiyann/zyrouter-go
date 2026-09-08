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
	err := r.db.QueryRow("SELECT isActive FROM apiKeys WHERE (keyHash IS NOT NULL AND keyHash = ?) OR (keyHash IS NULL AND key = ?) LIMIT 1", HashUserSecret(key), key).Scan(&active)
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
	var userID, typeID, keyHash sql.NullString
	err := r.db.QueryRow(
		"SELECT id, key, keyHash, name, machineId, isActive, restrictions, createdAt, clientId, policyId, userId, accountTypeId FROM apiKeys WHERE (keyHash IS NOT NULL AND keyHash = ?) OR (keyHash IS NULL AND key = ?) LIMIT 1",
		HashUserSecret(key), key,
	).Scan(&apiKey.ID, &apiKey.Key, &keyHash, &apiKey.Name, &apiKey.MachineID, &apiKey.IsActive, &apiKey.Restrictions, &apiKey.CreatedAt, &apiKey.ClientID, &apiKey.PolicyID, &userID, &typeID)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if keyHash.Valid {
		apiKey.KeyHash = &keyHash.String
	}
	if userID.Valid {
		apiKey.UserID = &userID.String
	}
	if typeID.Valid {
		apiKey.AccountTypeID = &typeID.String
	}
	return &apiKey, nil
}

// GetApiKeyByID retrieves APIKey information by primary key ID.
func (r *Repo) GetApiKeyByID(id string) (*models.APIKey, error) {
	var apiKey models.APIKey
	var userID, typeID, keyHash sql.NullString
	err := r.db.QueryRow(
		"SELECT id, key, keyHash, name, machineId, isActive, restrictions, createdAt, clientId, policyId, userId, accountTypeId FROM apiKeys WHERE id = ? LIMIT 1",
		id,
	).Scan(&apiKey.ID, &apiKey.Key, &keyHash, &apiKey.Name, &apiKey.MachineID, &apiKey.IsActive, &apiKey.Restrictions, &apiKey.CreatedAt, &apiKey.ClientID, &apiKey.PolicyID, &userID, &typeID)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if keyHash.Valid {
		apiKey.KeyHash = &keyHash.String
	}
	if userID.Valid {
		apiKey.UserID = &userID.String
	}
	if typeID.Valid {
		apiKey.AccountTypeID = &typeID.String
	}
	return &apiKey, nil
}

// GetApiKeys retrieves all API keys.
func (r *Repo) GetApiKeys() ([]*models.APIKey, error) {
	rows, err := r.db.Query("SELECT id, key, keyHash, name, machineId, isActive, restrictions, createdAt, clientId, policyId, userId, accountTypeId FROM apiKeys ORDER BY createdAt DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*models.APIKey
	for rows.Next() {
		var k models.APIKey
		var userID, typeID, keyHash sql.NullString
		if err := rows.Scan(&k.ID, &k.Key, &keyHash, &k.Name, &k.MachineID, &k.IsActive, &k.Restrictions, &k.CreatedAt, &k.ClientID, &k.PolicyID, &userID, &typeID); err != nil {
			return nil, err
		}
		if keyHash.Valid {
			k.KeyHash = &keyHash.String
		}
		if userID.Valid {
			k.UserID = &userID.String
		}
		if typeID.Valid {
			k.AccountTypeID = &typeID.String
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

// CreateApiKey inserts a new API key record with optional restrictions. The
// raw secret is returned to the caller but only its prefix and one-way hash
// are persisted.
func (r *Repo) CreateApiKey(id, rawKey, name, machineID, accountTypeID string, restrictions *string) (*models.APIKey, error) {
	if err := validateAliasRestrictions(restrictions); err != nil {
		return nil, err
	}
	accountTypeID = strings.TrimSpace(accountTypeID)
	if accountTypeID == "" {
		accountTypeID = "administrator"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(
		`INSERT INTO apiKeys (id, key, keyHash, name, machineId, isActive, restrictions, createdAt, accountTypeId) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		id, keyPrefix(rawKey), HashUserSecret(rawKey), name, machineID, restrictions, now, accountTypeID,
	)
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}
	return &models.APIKey{
		ID:            id,
		Key:           rawKey,
		Name:          &name,
		MachineID:     &machineID,
		IsActive:      1,
		Restrictions:  restrictions,
		AccountTypeID: &accountTypeID,
		CreatedAt:     now,
	}, nil
}

// UpdateApiKey updates an existing API key's name, active state, account type, or restrictions.
func (r *Repo) UpdateApiKey(id string, name *string, isActive *int, accountTypeID *string, restrictions *string) error {
	if err := validateAliasRestrictions(restrictions); err != nil {
		return err
	}
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
	if accountTypeID != nil && strings.TrimSpace(*accountTypeID) != "" {
		sets = append(sets, "accountTypeId = ?")
		args = append(args, strings.TrimSpace(*accountTypeID))
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

func validateAliasRestrictions(raw *string) error {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	var policy models.KeyRestrictions
	if err := json.Unmarshal([]byte(*raw), &policy); err != nil {
		return fmt.Errorf("invalid key restrictions: %w", err)
	}
	for _, value := range append(append([]string{}, policy.AllowedModels...), policy.BlockedModels...) {
		if strings.Contains(value, "/") {
			return fmt.Errorf("model restrictions must use public aliases; provider prefixes are forbidden")
		}
	}
	for _, value := range policy.AllowedPrefixes {
		if strings.Contains(value, "/") {
			return fmt.Errorf("alias family restrictions cannot contain provider prefixes")
		}
	}
	return nil
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
	var provider, upstream string
	var active int
	err := r.db.QueryRow(`SELECT provider, upstreamModel, isActive FROM modelAliases WHERE alias = ? LIMIT 1`, alias).Scan(&provider, &upstream, &active)
	if err == nil {
		if active != 1 {
			return "", nil
		}
		if provider == "__combo__" {
			return "combo:" + upstream, nil
		}
		return provider + "/" + upstream, nil
	}
	if err == sql.ErrNoRows {
		err = r.db.QueryRow("SELECT value FROM kv WHERE scope = 'modelAliases' AND key = ? LIMIT 1", alias).Scan(&rawVal)
		if err == sql.ErrNoRows {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("get model alias %s: %w", alias, err)
		}
		return parseJSONString(rawVal), nil
	}
	return "", fmt.Errorf("get model alias %s: %w", alias, err)
}

// GetModelAliases returns all model aliases as a key-value map.
func (r *Repo) GetModelAliases() (map[mapKey]string, error) {
	rows, err := r.db.Query(`SELECT alias, provider, upstreamModel FROM modelAliases WHERE isActive = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := make(map[mapKey]string)
	for rows.Next() {
		var alias, provider, upstream string
		if err := rows.Scan(&alias, &provider, &upstream); err != nil {
			return nil, err
		}
		if provider == "__combo__" {
			aliases[alias] = "combo:" + upstream
		} else {
			aliases[alias] = provider + "/" + upstream
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	knownRows, err := r.db.Query(`SELECT alias FROM modelAliases`)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool)
	for knownRows.Next() {
		var alias string
		if err := knownRows.Scan(&alias); err != nil {
			knownRows.Close()
			return nil, err
		}
		known[alias] = true
	}
	if err := knownRows.Close(); err != nil {
		return nil, err
	}
	// Keep test fixtures and older databases readable if their compatibility
	// rows have not yet been copied into the structured registry.
	legacyRows, err := r.db.Query(`SELECT key, value FROM kv WHERE scope = 'modelAliases'`)
	if err != nil {
		return nil, err
	}
	defer legacyRows.Close()
	for legacyRows.Next() {
		var alias, rawVal string
		if err := legacyRows.Scan(&alias, &rawVal); err != nil {
			return nil, err
		}
		if !known[alias] {
			target := parseJSONString(rawVal)
			if strings.Contains(target, "/") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(target)), "combo:") {
				aliases[alias] = target
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return aliases, legacyRows.Err()
}

// GetModelAliasRecords returns the structured admin model registry, including
// disabled aliases. Legacy KV aliases are represented as active records until
// the startup compatibility migration has copied them into modelAliases.
func (r *Repo) GetModelAliasRecords() ([]*models.ModelAlias, error) {
	rows, err := r.db.Query(`SELECT id, alias, provider, upstreamModel, connectionId, isActive, capabilities, createdAt, updatedAt FROM modelAliases ORDER BY alias`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*models.ModelAlias, 0)
	for rows.Next() {
		var item models.ModelAlias
		var connectionID sql.NullString
		var capabilities string
		if err := rows.Scan(&item.ID, &item.Alias, &item.Provider, &item.UpstreamModel, &connectionID, &item.IsActive, &capabilities, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if connectionID.Valid && connectionID.String != "" {
			item.ConnectionID = &connectionID.String
		}
		if capabilities != "" {
			_ = json.Unmarshal([]byte(capabilities), &item.Capabilities)
		}
		result = append(result, &item)
	}
	return result, rows.Err()
}

func (r *Repo) GetModelAliasRecord(alias string) (*models.ModelAlias, error) {
	var item models.ModelAlias
	var connectionID sql.NullString
	var capabilities string
	err := r.db.QueryRow(`SELECT id, alias, provider, upstreamModel, connectionId, isActive, capabilities, createdAt, updatedAt FROM modelAliases WHERE alias = ? LIMIT 1`, alias).
		Scan(&item.ID, &item.Alias, &item.Provider, &item.UpstreamModel, &connectionID, &item.IsActive, &capabilities, &item.CreatedAt, &item.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if connectionID.Valid && connectionID.String != "" {
		item.ConnectionID = &connectionID.String
	}
	if capabilities != "" {
		_ = json.Unmarshal([]byte(capabilities), &item.Capabilities)
	}
	return &item, nil
}

// SetModelAlias stores or updates a model alias in the structured registry.
func (r *Repo) SetModelAlias(alias, target string) error {
	alias = strings.TrimSpace(alias)
	target = strings.TrimSpace(target)
	if alias == "" || strings.ContainsAny(alias, "/ \t\r\n") {
		return fmt.Errorf("model alias must be a non-empty bare ID")
	}
	if strings.HasPrefix(strings.ToLower(target), "combo:") {
		comboName := strings.TrimSpace(target[len("combo:"):])
		if comboName == "" {
			return fmt.Errorf("combo alias target must name a combo")
		}
		return r.SetModelAliasRecord(alias, "__combo__", comboName, nil, []string{"combo"}, 1)
	}
	parts := strings.SplitN(target, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("model alias target must map to one provider and one upstream model")
	}
	return r.SetModelAliasRecord(alias, parts[0], parts[1], nil, nil, 1)
}

// SetModelAliasRecord creates or updates one public alias and its internal
// provider target. The public alias remains bare and unique.
func (r *Repo) SetModelAliasRecord(alias, provider, upstreamModel string, connectionID *string, capabilities []string, isActive int) error {
	alias = strings.TrimSpace(alias)
	provider = strings.TrimSpace(provider)
	upstreamModel = strings.TrimSpace(upstreamModel)
	if alias == "" || strings.ContainsAny(alias, "/ \t\r\n") {
		return fmt.Errorf("model alias must be a non-empty bare ID")
	}
	if provider == "" || upstreamModel == "" {
		return fmt.Errorf("model alias target must map to one provider and one upstream model")
	}
	if isActive != 0 {
		isActive = 1
	}
	if capabilities == nil {
		capabilities = []string{}
	}
	capJSON, _ := json.Marshal(capabilities)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(`INSERT INTO modelAliases (id, alias, provider, upstreamModel, connectionId, isActive, capabilities, createdAt, updatedAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(alias) DO UPDATE SET provider=excluded.provider, upstreamModel=excluded.upstreamModel, connectionId=excluded.connectionId, isActive=excluded.isActive, capabilities=excluded.capabilities, updatedAt=excluded.updatedAt`,
		"alias_"+HashUserSecret(alias)[:24], alias, provider, upstreamModel, connectionID, isActive, string(capJSON), now, now)
	if err != nil {
		return err
	}
	// Preserve the legacy export format while all runtime reads use the table.
	legacyTarget := provider + "/" + upstreamModel
	if provider == "__combo__" {
		legacyTarget = "combo:" + upstreamModel
	}
	valBytes, _ := json.Marshal(legacyTarget)
	_, err = r.db.Exec(`INSERT INTO kv (scope, key, value) VALUES ('modelAliases', ?, ?)
		ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`, alias, string(valBytes))
	return err
}

// CreateCompositeAlias atomically stores an admin-only combo definition and
// its single public composite alias. Combo members remain internal routing
// targets; only the outer alias belongs in the public model registry.
func (r *Repo) CreateCompositeAlias(alias string, combo *models.Combo, isActive int) error {
	alias = strings.TrimSpace(alias)
	if alias == "" || strings.ContainsAny(alias, "/ \t\r\n") {
		return fmt.Errorf("model alias must be a non-empty bare ID")
	}
	if combo == nil || strings.TrimSpace(combo.ID) == "" || strings.TrimSpace(combo.Name) == "" || strings.TrimSpace(combo.Models) == "" {
		return fmt.Errorf("composite alias requires a combo definition")
	}
	if isActive != 0 {
		isActive = 1
	}
	if combo.Strategy == "" {
		combo.Strategy = "fallback"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	combo.CreatedAt = now
	combo.UpdatedAt = now
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO combos (id, name, kind, models, strategy, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?, ?)`, combo.ID, combo.Name, combo.Kind, combo.Models, combo.Strategy, combo.CreatedAt, combo.UpdatedAt); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO modelAliases (id, alias, provider, upstreamModel, isActive, capabilities, createdAt, updatedAt)
		VALUES (?, ?, '__combo__', ?, ?, '["combo"]', ?, ?)`,
		"alias_"+HashUserSecret(alias)[:24], alias, combo.Name, isActive, now, now); err != nil {
		return err
	}
	legacyTarget, _ := json.Marshal("combo:" + combo.Name)
	if _, err := tx.Exec(`INSERT INTO kv (scope, key, value) VALUES ('modelAliases', ?, ?)
		ON CONFLICT(scope, key) DO UPDATE SET value = excluded.value`, alias, string(legacyTarget)); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteModelAlias removes a model alias.
func (r *Repo) DeleteModelAlias(alias string) error {
	if _, err := r.db.Exec("DELETE FROM modelAliases WHERE alias = ?", alias); err != nil {
		return err
	}
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
