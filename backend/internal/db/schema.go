package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

const legacyAPIKeyMigrationMetaKey = "migration.api_keys_hash.v1"

// LegacyAPIKeyMigrationStats describes the one-time gateway-key migration.
// Client-owned legacy rows are hashed but are deliberately not promoted to
// administrator because clientId is a separate trust boundary.
type LegacyAPIKeyMigrationStats struct {
	AlreadyApplied  bool
	Found           int
	Hashed          int
	Skipped         int
	ClientID        int
	UserKeyRetained int
}

// EnsureSchema creates all necessary tables and migrates missing columns.
func EnsureSchema(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS apiKeys (
			id           TEXT PRIMARY KEY,
			key          TEXT UNIQUE NOT NULL,
			name         TEXT,
			machineId    TEXT,
			isActive     INTEGER DEFAULT 1,
			restrictions TEXT,
			createdAt    TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_ak_key ON apiKeys(key);`,
		`CREATE INDEX IF NOT EXISTS idx_ak_active ON apiKeys(isActive);`,
		`CREATE TABLE IF NOT EXISTS accountTypes (
			id TEXT PRIMARY KEY,
			name TEXT UNIQUE NOT NULL,
			description TEXT,
			isSystem INTEGER DEFAULT 0,
			isActive INTEGER DEFAULT 1,
			quotaMode TEXT NOT NULL DEFAULT 'unlimited',
			quotaTokens INTEGER DEFAULT 0,
			requestsPerMinute INTEGER DEFAULT 0,
			dailyTokenLimit INTEGER DEFAULT 0,
			monthlyTokenLimit INTEGER DEFAULT 0,
			allowRTK INTEGER DEFAULT 0,
			allowCaveman INTEGER DEFAULT 0,
			allowPonytail INTEGER DEFAULT 0,
			createdAt TEXT NOT NULL,
			updatedAt TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			telegramUserId TEXT UNIQUE NOT NULL,
			telegramUsername TEXT,
			displayName TEXT,
			accountTypeId TEXT NOT NULL,
			isActive INTEGER DEFAULT 1,
			verifiedAt TEXT NOT NULL,
			createdAt TEXT NOT NULL,
			updatedAt TEXT NOT NULL,
			FOREIGN KEY(accountTypeId) REFERENCES accountTypes(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_users_account_type ON users(accountTypeId);`,
		`CREATE TABLE IF NOT EXISTS accountTypeModels (
			accountTypeId TEXT NOT NULL,
			alias TEXT NOT NULL,
			createdAt TEXT NOT NULL,
			PRIMARY KEY(accountTypeId, alias),
			FOREIGN KEY(accountTypeId) REFERENCES accountTypes(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS userVerificationChallenges (
			id TEXT PRIMARY KEY,
			expiresAt TEXT NOT NULL,
			browserKey TEXT,
			telegramUserId TEXT,
			telegramUsername TEXT,
			telegramDisplayName TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			confirmationHash TEXT,
			confirmationExpiresAt TEXT,
			confirmationAttempts INTEGER NOT NULL DEFAULT 0,
			createdAt TEXT NOT NULL,
			verifiedAt TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_verification_expiry ON userVerificationChallenges(expiresAt);`,
		`CREATE TABLE IF NOT EXISTS userSessions (
			hash TEXT PRIMARY KEY,
			userId TEXT NOT NULL,
			expiresAt TEXT NOT NULL,
			createdAt TEXT NOT NULL,
			FOREIGN KEY(userId) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS userQuotaUsage (
			userId TEXT NOT NULL,
			periodKey TEXT NOT NULL,
			reservedTokens INTEGER NOT NULL DEFAULT 0,
			actualTokens INTEGER NOT NULL DEFAULT 0,
			requestCount INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(userId, periodKey),
			FOREIGN KEY(userId) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS userSettings (
			userId TEXT PRIMARY KEY,
			rtkEnabled INTEGER,
			cavemanEnabled INTEGER,
			cavemanLevel TEXT,
			ponytailEnabled INTEGER,
			ponytailLevel TEXT,
			FOREIGN KEY(userId) REFERENCES users(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS clientPolicies (
			id        TEXT PRIMARY KEY,
			name      TEXT NOT NULL,
			isActive  INTEGER DEFAULT 1,
			data      TEXT NOT NULL,
			createdAt TEXT NOT NULL,
			updatedAt TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS clients (
			id              TEXT PRIMARY KEY,
			name            TEXT NOT NULL,
			email           TEXT,
			accessTokenHash TEXT UNIQUE NOT NULL,
			policyId        TEXT,
			isActive        INTEGER DEFAULT 1,
			createdAt       TEXT NOT NULL,
			updatedAt       TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_clients_token ON clients(accessTokenHash);`,
		`CREATE INDEX IF NOT EXISTS idx_clients_policy ON clients(policyId);`,

		`CREATE TABLE IF NOT EXISTS providerConnections (
			id                  TEXT PRIMARY KEY,
			provider            TEXT NOT NULL,
			authType            TEXT NOT NULL,
			name                TEXT,
			email               TEXT,
			priority            INTEGER DEFAULT 10,
			isActive            INTEGER DEFAULT 1,
			data                TEXT NOT NULL,
			lastUsedAt          TEXT,
			consecutiveUseCount INTEGER DEFAULT 0,
			createdAt           TEXT NOT NULL,
			updatedAt           TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pc_provider ON providerConnections(provider);`,
		`CREATE INDEX IF NOT EXISTS idx_pc_provider_active ON providerConnections(provider, isActive);`,
		`CREATE INDEX IF NOT EXISTS idx_pc_priority ON providerConnections(provider, priority);`,

		`CREATE TABLE IF NOT EXISTS combos (
			id        TEXT PRIMARY KEY,
			name      TEXT UNIQUE NOT NULL,
			kind      TEXT,
			models    TEXT NOT NULL,
			strategy  TEXT DEFAULT 'fallback',
			createdAt TEXT NOT NULL,
			updatedAt TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_combo_name ON combos(name);`,

		`CREATE TABLE IF NOT EXISTS providerNodes (
			id        TEXT PRIMARY KEY,
			type      TEXT,
			name      TEXT,
			data      TEXT NOT NULL,
			createdAt TEXT NOT NULL,
			updatedAt TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS proxyPools (
			id         TEXT PRIMARY KEY,
			isActive   INTEGER DEFAULT 1,
			testStatus TEXT,
			data       TEXT NOT NULL,
			createdAt  TEXT NOT NULL,
			updatedAt  TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS kv (
			scope TEXT NOT NULL,
			key   TEXT NOT NULL,
			value TEXT NOT NULL,
			PRIMARY KEY (scope, key)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_kv_scope ON kv(scope);`,
		`CREATE TABLE IF NOT EXISTS modelAliases (
			id            TEXT PRIMARY KEY,
			alias         TEXT UNIQUE NOT NULL,
			provider      TEXT NOT NULL,
			upstreamModel TEXT NOT NULL,
			connectionId  TEXT,
			isActive      INTEGER DEFAULT 1,
			capabilities  TEXT NOT NULL DEFAULT '[]',
			createdAt     TEXT NOT NULL,
			updatedAt     TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_model_aliases_provider ON modelAliases(provider);`,

		`CREATE TABLE IF NOT EXISTS settings (
			id   INTEGER PRIMARY KEY CHECK (id = 1),
			data TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS usageHistory (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp        TEXT NOT NULL,
			provider         TEXT,
			model            TEXT,
			connectionId     TEXT,
			apiKey           TEXT,
			endpoint         TEXT,
			promptTokens     INTEGER DEFAULT 0,
			completionTokens INTEGER DEFAULT 0,
			cost             REAL DEFAULT 0,
			status           TEXT,
			tokens           TEXT,
			meta             TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_uh_timestamp ON usageHistory(timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_uh_apikey ON usageHistory(apiKey);`,

		`CREATE TABLE IF NOT EXISTS usageDaily (
			dateKey TEXT PRIMARY KEY,
			data    TEXT NOT NULL
		);`,

		`CREATE TABLE IF NOT EXISTS requestDetails (
			id           TEXT PRIMARY KEY,
			timestamp    TEXT NOT NULL,
			provider     TEXT,
			model        TEXT,
			connectionId TEXT,
			status       TEXT,
			data         TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS authLogs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			event TEXT NOT NULL,
			ip TEXT,
			method TEXT,
			path TEXT,
			status INTEGER,
			requestId TEXT,
			userAgent TEXT,
			referer TEXT,
			detail TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_auth_logs_timestamp ON authLogs(timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_auth_logs_event ON authLogs(event);`,

		`CREATE TABLE IF NOT EXISTS _meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("ensure schema exec query failed: %w", err)
		}
	}
	if err := migrateModelAliasesFromKV(db); err != nil {
		return fmt.Errorf("migrate model aliases: %w", err)
	}

	// Column migrations
	migrateColumnIfNotExists(db, "apiKeys", "restrictions", "TEXT")
	migrateColumnIfNotExists(db, "apiKeys", "clientId", "TEXT")
	migrateColumnIfNotExists(db, "apiKeys", "policyId", "TEXT")
	migrateColumnIfNotExists(db, "apiKeys", "userId", "TEXT")
	migrateColumnIfNotExists(db, "apiKeys", "accountTypeId", "TEXT")
	migrateColumnIfNotExists(db, "apiKeys", "keyHash", "TEXT")
	migrateColumnIfNotExists(db, "usageHistory", "userId", "TEXT")
	migrateColumnIfNotExists(db, "userVerificationChallenges", "browserKey", "TEXT")
	migrateColumnIfNotExists(db, "userVerificationChallenges", "confirmationHash", "TEXT")
	migrateColumnIfNotExists(db, "userVerificationChallenges", "confirmationExpiresAt", "TEXT")
	migrateColumnIfNotExists(db, "userVerificationChallenges", "confirmationAttempts", "INTEGER NOT NULL DEFAULT 0")
	migrateColumnIfNotExists(db, "combos", "strategy", "TEXT DEFAULT 'fallback'")
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_one_active_user ON apiKeys(userId) WHERE userId IS NOT NULL AND isActive = 1`); err != nil {
		return fmt.Errorf("create user api key uniqueness index: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_usage_user ON usageHistory(userId)`); err != nil {
		return fmt.Errorf("create user usage index: %w", err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO accountTypes (id, name, description, isSystem, isActive, quotaMode, createdAt, updatedAt) VALUES
		('administrator', 'Administrator', 'Full access to all available models', 1, 1, 'unlimited', datetime('now'), datetime('now')),
		('user', 'User', 'Standard verified user', 1, 1, 'custom', datetime('now'), datetime('now')),
		('paid_user', 'Paid User', 'Paid user access tier', 1, 1, 'custom', datetime('now'), datetime('now'))`); err != nil {
		return fmt.Errorf("seed account types: %w", err)
	}
	// Legacy gateway keys are operator/admin keys. Client-owned rows are a
	// separate trust boundary and must not be promoted implicitly.
	if _, err := db.Exec(`UPDATE apiKeys SET accountTypeId='administrator' WHERE accountTypeId IS NULL AND userId IS NULL AND (clientId IS NULL OR trim(clientId) = '')`); err != nil {
		return fmt.Errorf("backfill legacy api key account type: %w", err)
	}
	if stats, err := MigrateLegacyGatewayKeys(db); err != nil {
		return fmt.Errorf("migrate legacy api keys: %w", err)
	} else if !stats.AlreadyApplied {
		log.Printf("[db] legacy api key migration: found=%d hashed=%d skipped=%d clientId=%d userKeysRetained=%d", stats.Found, stats.Hashed, stats.Skipped, stats.ClientID, stats.UserKeyRetained)
	}

	log.Printf("[db] Database schema verified & up to date")
	return nil
}

// migrateModelAliasesFromKV keeps aliases created by older builds while the
// structured registry becomes the source of truth for the management UI.
func migrateModelAliasesFromKV(db *sql.DB) error {
	rows, err := db.Query(`SELECT key, value FROM kv WHERE scope = 'modelAliases'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type aliasRow struct{ alias, target string }
	var candidates []aliasRow
	for rows.Next() {
		var alias, raw string
		if err := rows.Scan(&alias, &raw); err != nil {
			return err
		}
		target := parseJSONString(raw)
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(target)), "combo:") {
			comboName := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(target), "combo:"))
			if comboName != "" {
				candidates = append(candidates, aliasRow{alias: alias, target: "combo:" + comboName})
			}
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(target), "/", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			continue
		}
		candidates = append(candidates, aliasRow{alias: alias, target: target})
	}
	if err := rows.Close(); err != nil {
		return err
	}
	var now string
	for _, candidate := range candidates {
		alias, target := candidate.alias, candidate.target
		if strings.HasPrefix(target, "combo:") {
			if now == "" {
				now = time.Now().UTC().Format(time.RFC3339)
			}
			if _, err := db.Exec(`INSERT OR IGNORE INTO modelAliases (id, alias, provider, upstreamModel, isActive, capabilities, createdAt, updatedAt) VALUES (?, ?, '__combo__', ?, 1, '["combo"]', ?, ?)`, "alias_"+HashUserSecret(alias)[:24], alias, strings.TrimPrefix(target, "combo:"), now, now); err != nil {
				return err
			}
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(target), "/", 2)
		if now == "" {
			now = time.Now().UTC().Format(time.RFC3339)
		}
		if _, err := db.Exec(`INSERT OR IGNORE INTO modelAliases (id, alias, provider, upstreamModel, isActive, capabilities, createdAt, updatedAt) VALUES (?, ?, ?, ?, 1, '[]', ?, ?)`, "alias_"+HashUserSecret(alias)[:24], alias, parts[0], parts[1], now, now); err != nil {
			return err
		}
	}
	return rows.Err()
}

// MigrateLegacyGatewayKeys hashes legacy secrets without deleting any rows.
// The entire batch runs in one transaction and is guarded by _meta so a
// completed migration is not repeated on every startup.
func MigrateLegacyGatewayKeys(db *sql.DB) (LegacyAPIKeyMigrationStats, error) {
	var stats LegacyAPIKeyMigrationStats
	tx, err := db.Begin()
	if err != nil {
		return stats, err
	}
	defer tx.Rollback()

	var marker string
	err = tx.QueryRow(`SELECT value FROM _meta WHERE key = ?`, legacyAPIKeyMigrationMetaKey).Scan(&marker)
	if err == nil && marker == "complete" {
		stats.AlreadyApplied = true
		return stats, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return stats, err
	}

	if err := tx.QueryRow(`SELECT COUNT(*) FROM apiKeys WHERE userId IS NOT NULL`).Scan(&stats.UserKeyRetained); err != nil {
		return stats, err
	}
	rows, err := tx.Query(`SELECT id, key, clientId FROM apiKeys WHERE userId IS NULL AND keyHash IS NULL AND key IS NOT NULL AND length(trim(key)) > 0`)
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	type legacyRow struct {
		id       string
		secret   string
		clientID sql.NullString
	}
	var candidates []legacyRow
	for rows.Next() {
		var row legacyRow
		if err := rows.Scan(&row.id, &row.secret, &row.clientID); err != nil {
			return stats, err
		}
		candidates = append(candidates, row)
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	if err := rows.Close(); err != nil {
		return stats, err
	}
	stats.Found = len(candidates)

	for _, row := range candidates {
		masked, err := uniqueMaskedAPIKey(tx, row.id, row.secret)
		if err != nil {
			return stats, err
		}
		accountType := ""
		if strings.TrimSpace(row.clientID.String) == "" {
			accountType = "administrator"
		} else {
			// Hash the secret for safety, but do not change the client's tier.
			stats.ClientID++
			stats.Skipped++
		}
		query := `UPDATE apiKeys SET key = ?, keyHash = ?`
		args := []any{masked, HashUserSecret(row.secret)}
		if accountType != "" {
			query += `, accountTypeId = ?`
			args = append(args, accountType)
		}
		query += ` WHERE id = ? AND userId IS NULL AND keyHash IS NULL AND key = ?`
		args = append(args, row.id, row.secret)
		result, err := tx.Exec(query, args...)
		if err != nil {
			return stats, err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return stats, fmt.Errorf("legacy api key %s changed during migration", row.id)
		}
		stats.Hashed++
	}

	if _, err := tx.Exec(`INSERT INTO _meta (key, value) VALUES (?, 'complete') ON CONFLICT(key) DO UPDATE SET value = excluded.value`, legacyAPIKeyMigrationMetaKey); err != nil {
		return stats, err
	}
	if err := tx.Commit(); err != nil {
		return stats, err
	}
	return stats, nil
}

func uniqueMaskedAPIKey(tx *sql.Tx, id, secret string) (string, error) {
	base := keyPrefix(secret)
	if len(secret) <= 12 {
		base = "legacy_" + HashUserSecret(secret)[:12]
	}
	candidate := base
	for suffix := 0; ; suffix++ {
		var existingID string
		err := tx.QueryRow(`SELECT id FROM apiKeys WHERE key = ? AND id <> ? LIMIT 1`, candidate, id).Scan(&existingID)
		if err == sql.ErrNoRows {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s~%d", base, suffix+1)
	}
}

func migrateColumnIfNotExists(db *sql.DB, tableName, columnName, columnDef string) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return
	}
	defer rows.Close()

	exists := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err == nil {
			if name == columnName {
				exists = true
				break
			}
		}
	}

	if !exists {
		alterQuery := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableName, columnName, columnDef)
		if _, err := db.Exec(alterQuery); err != nil {
			log.Printf("[db] warning: failed to add column %s to table %s: %v", columnName, tableName, err)
		} else {
			log.Printf("[db] migrated: added column %s to table %s", columnName, tableName)
		}
	}
}
