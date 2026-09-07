package db

import (
	"database/sql"
	"fmt"
	"log"
)

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
			telegramUserId TEXT,
			telegramUsername TEXT,
			telegramDisplayName TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
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

	// Column migrations
	migrateColumnIfNotExists(db, "apiKeys", "restrictions", "TEXT")
	migrateColumnIfNotExists(db, "apiKeys", "clientId", "TEXT")
	migrateColumnIfNotExists(db, "apiKeys", "policyId", "TEXT")
	migrateColumnIfNotExists(db, "apiKeys", "userId", "TEXT")
	migrateColumnIfNotExists(db, "apiKeys", "accountTypeId", "TEXT")
	migrateColumnIfNotExists(db, "apiKeys", "keyHash", "TEXT")
	migrateColumnIfNotExists(db, "usageHistory", "userId", "TEXT")
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
	// Legacy gateway keys are operator/admin keys. Only keys that are not owned
	// by a verified user are backfilled; user-owned keys retain their tier.
	if _, err := db.Exec(`UPDATE apiKeys SET accountTypeId='administrator' WHERE accountTypeId IS NULL AND userId IS NULL`); err != nil {
		return fmt.Errorf("backfill legacy api key account type: %w", err)
	}

	log.Printf("[db] Database schema verified & up to date")
	return nil
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
