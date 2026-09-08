package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"zyrouter/backend/internal/models"
)

func HashUserSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (r *Repo) ListAccountTypes(activeOnly bool) ([]*models.AccountType, error) {
	query := `SELECT id, name, COALESCE(description,''), isSystem, isActive, quotaMode,
		COALESCE(quotaTokens,0), COALESCE(requestsPerMinute,0), COALESCE(dailyTokenLimit,0),
		COALESCE(monthlyTokenLimit,0), COALESCE(allowRTK,0), COALESCE(allowCaveman,0),
		COALESCE(allowPonytail,0), createdAt, updatedAt FROM accountTypes`
	if activeOnly {
		query += ` WHERE isActive = 1`
	}
	query += ` ORDER BY isSystem DESC, name`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*models.AccountType
	for rows.Next() {
		var item models.AccountType
		var rtk, caveman, ponytail int
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.IsSystem, &item.IsActive,
			&item.QuotaMode, &item.QuotaTokens, &item.RequestsPerMinute, &item.DailyTokenLimit,
			&item.MonthlyTokenLimit, &rtk, &caveman, &ponytail, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.AllowRTK, item.AllowCaveman, item.AllowPonytail = rtk == 1, caveman == 1, ponytail == 1
		result = append(result, &item)
	}
	return result, rows.Err()
}

func (r *Repo) GetAccountType(id string) (*models.AccountType, error) {
	items, err := r.ListAccountTypes(false)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, nil
}

func (r *Repo) UpsertAccountType(item *models.AccountType) error {
	if item == nil || strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Name) == "" {
		return fmt.Errorf("account type id and name are required")
	}
	if item.QuotaMode != "unlimited" && item.QuotaMode != "custom" {
		return fmt.Errorf("quotaMode must be unlimited or custom")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(`INSERT INTO accountTypes
		(id,name,description,isSystem,isActive,quotaMode,quotaTokens,requestsPerMinute,dailyTokenLimit,monthlyTokenLimit,allowRTK,allowCaveman,allowPonytail,createdAt,updatedAt)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, description=excluded.description, isActive=excluded.isActive,
		quotaMode=excluded.quotaMode, quotaTokens=excluded.quotaTokens, requestsPerMinute=excluded.requestsPerMinute,
		dailyTokenLimit=excluded.dailyTokenLimit, monthlyTokenLimit=excluded.monthlyTokenLimit,
		allowRTK=excluded.allowRTK, allowCaveman=excluded.allowCaveman, allowPonytail=excluded.allowPonytail, updatedAt=excluded.updatedAt`,
		item.ID, item.Name, item.Description, item.IsSystem, item.IsActive, item.QuotaMode, item.QuotaTokens,
		item.RequestsPerMinute, item.DailyTokenLimit, item.MonthlyTokenLimit, boolInt(item.AllowRTK), boolInt(item.AllowCaveman), boolInt(item.AllowPonytail), now, now)
	return err
}

func (r *Repo) DeleteAccountType(id string) error {
	var system int
	if err := r.db.QueryRow(`SELECT isSystem FROM accountTypes WHERE id = ?`, id).Scan(&system); err != nil {
		return err
	}
	if system == 1 {
		return fmt.Errorf("system account type cannot be deleted")
	}
	_, err := r.db.Exec(`DELETE FROM accountTypes WHERE id = ?`, id)
	return err
}

func (r *Repo) SetAccountTypeModels(accountTypeID string, aliases []string) error {
	if _, err := r.GetAccountType(accountTypeID); err != nil {
		return err
	}
	cleanAliases := make([]string, 0, len(aliases))
	seen := make(map[string]bool)
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || seen[strings.ToLower(alias)] {
			continue
		}
		if strings.Contains(alias, "/") {
			return fmt.Errorf("account models must use published bare aliases")
		}
		if target, err := r.GetModelAlias(alias); err != nil || strings.TrimSpace(target) == "" {
			if err != nil {
				return err
			}
			return fmt.Errorf("model alias %q is not published", alias)
		}
		seen[strings.ToLower(alias)] = true
		cleanAliases = append(cleanAliases, alias)
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM accountTypeModels WHERE accountTypeId = ?`, accountTypeID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, alias := range cleanAliases {
		if _, err := tx.Exec(`INSERT INTO accountTypeModels (accountTypeId, alias, createdAt) VALUES (?, ?, ?)`, accountTypeID, alias, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repo) GetAccountTypeModels(accountTypeID string) ([]string, error) {
	rows, err := r.db.Query(`SELECT alias FROM accountTypeModels WHERE accountTypeId = ? ORDER BY alias`, accountTypeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	aliases := make([]string, 0)
	for rows.Next() {
		var alias string
		if err := rows.Scan(&alias); err != nil {
			return nil, err
		}
		aliases = append(aliases, alias)
	}
	return aliases, rows.Err()
}

func (r *Repo) IsAliasAllowedForAccountType(accountTypeID, alias string) (bool, error) {
	typ, err := r.GetAccountType(accountTypeID)
	if err != nil || typ == nil || typ.IsActive != 1 {
		return false, err
	}
	if typ.ID == "administrator" {
		return true, nil
	}
	var count int
	err = r.db.QueryRow(`SELECT COUNT(*) FROM accountTypeModels WHERE accountTypeId = ? AND lower(alias) = lower(?)`, accountTypeID, alias).Scan(&count)
	return count > 0, err
}

func (r *Repo) CreateVerificationChallenge(ttl time.Duration) (string, time.Time, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	expires := now.Add(ttl)
	_, err := r.db.Exec(`INSERT INTO userVerificationChallenges (id, expiresAt, status, createdAt) VALUES (?, ?, 'pending', ?)`, id, expires.Format(time.RFC3339), now.Format(time.RFC3339))
	return id, expires, err
}

func (r *Repo) VerifyChallenge(challengeID, telegramID, username, displayName, defaultType string) (*models.User, error) {
	if strings.TrimSpace(challengeID) == "" || strings.TrimSpace(telegramID) == "" {
		return nil, fmt.Errorf("challenge and telegram id are required")
	}
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var expiresAt, status string
	if err := tx.QueryRow(`SELECT expiresAt, status FROM userVerificationChallenges WHERE id = ?`, challengeID).Scan(&expiresAt, &status); err != nil {
		return nil, err
	}
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().UTC().After(expires) || status != "pending" {
		return nil, fmt.Errorf("verification challenge expired or already used")
	}
	var existingID string
	if err := tx.QueryRow(`SELECT id FROM users WHERE telegramUserId = ?`, telegramID).Scan(&existingID); err == nil {
		return nil, fmt.Errorf("telegram account is already registered")
	} else if err != sql.ErrNoRows {
		return nil, err
	}
	if defaultType == "" {
		defaultType = "user"
	}
	var active int
	if err := tx.QueryRow(`SELECT isActive FROM accountTypes WHERE id = ?`, defaultType).Scan(&active); err != nil || active != 1 {
		return nil, fmt.Errorf("default account type is unavailable")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	userID := "usr_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := tx.Exec(`INSERT INTO users (id, telegramUserId, telegramUsername, displayName, accountTypeId, isActive, verifiedAt, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, 1, ?, ?, ?)`, userID, telegramID, username, displayName, defaultType, now, now, now); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE userVerificationChallenges SET telegramUserId=?, telegramUsername=?, telegramDisplayName=?, status='verified', verifiedAt=? WHERE id=? AND status='pending'`, telegramID, username, displayName, now, challengeID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetUserByID(userID)
}

func (r *Repo) GetVerificationStatus(challengeID string) (string, *models.User, error) {
	var status, telegramID sql.NullString
	var expiresAt string
	if err := r.db.QueryRow(`SELECT status, expiresAt, telegramUserId FROM userVerificationChallenges WHERE id = ?`, challengeID).Scan(&status, &expiresAt, &telegramID); err != nil {
		return "", nil, err
	}
	if time.Now().UTC().After(parseTimeOrZero(expiresAt)) && status.String == "pending" {
		_, _ = r.db.Exec(`UPDATE userVerificationChallenges SET status='expired' WHERE id=? AND status='pending'`, challengeID)
		return "expired", nil, nil
	}
	if status.String != "verified" || !telegramID.Valid {
		return status.String, nil, nil
	}
	user, err := r.GetUserByTelegramID(telegramID.String)
	return status.String, user, err
}

func (r *Repo) GetUserByID(id string) (*models.User, error) { return r.getUser(`id = ?`, id) }
func (r *Repo) GetUserByTelegramID(id string) (*models.User, error) {
	return r.getUser(`telegramUserId = ?`, id)
}

func (r *Repo) ListUsers() ([]*models.User, error) {
	rows, err := r.db.Query(`SELECT id, telegramUserId, telegramUsername, displayName, accountTypeId, isActive, verifiedAt, createdAt, updatedAt FROM users ORDER BY createdAt DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*models.User
	for rows.Next() {
		var user models.User
		var username, display sql.NullString
		if err := rows.Scan(&user.ID, &user.TelegramUserID, &username, &display, &user.AccountTypeID, &user.IsActive, &user.VerifiedAt, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		if username.Valid {
			user.TelegramUsername = &username.String
		}
		if display.Valid {
			user.DisplayName = &display.String
		}
		users = append(users, &user)
	}
	return users, rows.Err()
}

func (r *Repo) UpdateUserAccountType(userID, accountTypeID string) error {
	typ, err := r.GetAccountType(accountTypeID)
	if err != nil {
		return err
	}
	if typ == nil || typ.IsActive != 1 {
		return fmt.Errorf("account type not found or inactive")
	}
	result, err := r.db.Exec(`UPDATE users SET accountTypeId=?, updatedAt=? WHERE id=?`, accountTypeID, time.Now().UTC().Format(time.RFC3339), userID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	_, _ = r.db.Exec(`UPDATE apiKeys SET accountTypeId=? WHERE userId=?`, accountTypeID, userID)
	return nil
}

func (r *Repo) GetUserSettings(userID string) (*models.UserFeatureSettings, error) {
	var rtk, caveman, ponytail sql.NullInt64
	var cavemanLevel, ponytailLevel sql.NullString
	err := r.db.QueryRow(`SELECT rtkEnabled, cavemanEnabled, cavemanLevel, ponytailEnabled, ponytailLevel FROM userSettings WHERE userId = ?`, userID).
		Scan(&rtk, &caveman, &cavemanLevel, &ponytail, &ponytailLevel)
	if err == sql.ErrNoRows {
		return &models.UserFeatureSettings{}, nil
	}
	if err != nil {
		return nil, err
	}
	settings := &models.UserFeatureSettings{}
	if rtk.Valid {
		v := rtk.Int64 == 1
		settings.RTKEnabled = &v
	}
	if caveman.Valid {
		v := caveman.Int64 == 1
		settings.CavemanEnabled = &v
	}
	if ponytail.Valid {
		v := ponytail.Int64 == 1
		settings.PonytailEnabled = &v
	}
	if cavemanLevel.Valid {
		settings.CavemanLevel = cavemanLevel.String
	}
	if ponytailLevel.Valid {
		settings.PonytailLevel = ponytailLevel.String
	}
	return settings, nil
}

func (r *Repo) SaveUserSettings(userID string, settings *models.UserFeatureSettings) error {
	if settings == nil {
		return fmt.Errorf("settings are required")
	}
	var rtk, caveman, ponytail any
	if settings.RTKEnabled != nil {
		rtk = boolInt(*settings.RTKEnabled)
	}
	if settings.CavemanEnabled != nil {
		caveman = boolInt(*settings.CavemanEnabled)
	}
	if settings.PonytailEnabled != nil {
		ponytail = boolInt(*settings.PonytailEnabled)
	}
	_, err := r.db.Exec(`INSERT INTO userSettings (userId,rtkEnabled,cavemanEnabled,cavemanLevel,ponytailEnabled,ponytailLevel) VALUES (?,?,?,?,?,?)
		ON CONFLICT(userId) DO UPDATE SET rtkEnabled=excluded.rtkEnabled,cavemanEnabled=excluded.cavemanEnabled,cavemanLevel=excluded.cavemanLevel,
		ponytailEnabled=excluded.ponytailEnabled,ponytailLevel=excluded.ponytailLevel`, userID, rtk, caveman, settings.CavemanLevel, ponytail, settings.PonytailLevel)
	return err
}

func (r *Repo) getUser(where string, arg any) (*models.User, error) {
	var user models.User
	var username, display sql.NullString
	err := r.db.QueryRow(`SELECT id, telegramUserId, telegramUsername, displayName, accountTypeId, isActive, verifiedAt, createdAt, updatedAt FROM users WHERE `+where+` LIMIT 1`, arg).
		Scan(&user.ID, &user.TelegramUserID, &username, &display, &user.AccountTypeID, &user.IsActive, &user.VerifiedAt, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if username.Valid {
		user.TelegramUsername = &username.String
	}
	if display.Valid {
		user.DisplayName = &display.String
	}
	return &user, nil
}

func (r *Repo) CreateUserSession(userID, rawToken string, ttl time.Duration) error {
	now := time.Now().UTC()
	_, err := r.db.Exec(`INSERT INTO userSessions (hash, userId, expiresAt, createdAt) VALUES (?, ?, ?, ?)`, HashUserSecret(rawToken), userID, now.Add(ttl).Format(time.RFC3339), now.Format(time.RFC3339))
	return err
}

func (r *Repo) GetUserBySession(rawToken string) (*models.User, error) {
	var userID, expiresAt string
	err := r.db.QueryRow(`SELECT userId, expiresAt FROM userSessions WHERE hash = ?`, HashUserSecret(rawToken)).Scan(&userID, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if time.Now().UTC().After(parseTimeOrZero(expiresAt)) {
		return nil, nil
	}
	return r.GetUserByID(userID)
}

func (r *Repo) RevokeUserSession(rawToken string) error {
	_, err := r.db.Exec(`DELETE FROM userSessions WHERE hash = ?`, HashUserSecret(rawToken))
	return err
}

func (r *Repo) CreateUserApiKey(userID, accountTypeID, id, rawKey, name string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.Exec(`INSERT INTO apiKeys (id, key, keyHash, name, isActive, createdAt, userId, accountTypeId) VALUES (?, ?, ?, ?, 1, ?, ?, ?)`, id, keyPrefix(rawKey), HashUserSecret(rawKey), name, now, userID, accountTypeID)
	return err
}

func (r *Repo) GetActiveUserApiKey(userID string) (*models.APIKey, error) {
	var key models.APIKey
	var clientID, policyID, userIDValue, typeID, hash sql.NullString
	err := r.db.QueryRow(`SELECT id,key,keyHash,name,machineId,isActive,restrictions,createdAt,clientId,policyId,userId,accountTypeId FROM apiKeys WHERE userId=? AND isActive=1 LIMIT 1`, userID).
		Scan(&key.ID, &key.Key, &hash, &key.Name, &key.MachineID, &key.IsActive, &key.Restrictions, &key.CreatedAt, &clientID, &policyID, &userIDValue, &typeID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if clientID.Valid {
		key.ClientID = &clientID.String
	}
	if policyID.Valid {
		key.PolicyID = &policyID.String
	}
	if userIDValue.Valid {
		key.UserID = &userIDValue.String
	}
	if typeID.Valid {
		key.AccountTypeID = &typeID.String
	}
	if hash.Valid {
		key.KeyHash = &hash.String
	}
	return &key, nil
}

func (r *Repo) RotateUserApiKey(userID, accountTypeID, id, rawKey, name string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE apiKeys SET isActive=0 WHERE userId=? AND isActive=1`, userID); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.Exec(`INSERT INTO apiKeys (id,key,keyHash,name,isActive,createdAt,userId,accountTypeId) VALUES (?,?,?,?,1,?,?,?)`, id, keyPrefix(rawKey), HashUserSecret(rawKey), name, now, userID, accountTypeID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repo) RevokeUserApiKey(userID string) error {
	_, err := r.db.Exec(`UPDATE apiKeys SET isActive=0 WHERE userId=?`, userID)
	return err
}

// ReserveUserQuota atomically reserves an estimated token budget for one request.
// The reservation closes the race where many concurrent requests all pass a read-only quota check.
func (r *Repo) ReserveUserQuota(userID string, typ *models.AccountType, requestedTokens int64) (bool, error) {
	if typ == nil || typ.QuotaMode == "unlimited" {
		return true, nil
	}
	if requestedTokens < 1 {
		requestedTokens = 1
	}
	period := time.Now().UTC().Format("2006-01")
	limit := typ.QuotaTokens
	if typ.DailyTokenLimit > 0 {
		period = time.Now().UTC().Format("2006-01-02")
		limit = typ.DailyTokenLimit
	} else if typ.MonthlyTokenLimit > 0 {
		limit = typ.MonthlyTokenLimit
	}
	if limit <= 0 {
		return true, nil
	}
	tx, err := r.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO userQuotaUsage (userId,periodKey) VALUES (?,?)`, userID, period); err != nil {
		return false, err
	}
	var reserved, actual int64
	if err := tx.QueryRow(`SELECT reservedTokens,actualTokens FROM userQuotaUsage WHERE userId=? AND periodKey=?`, userID, period).Scan(&reserved, &actual); err != nil {
		return false, err
	}
	if actual+reserved+requestedTokens > limit {
		return false, nil
	}
	if _, err := tx.Exec(`UPDATE userQuotaUsage SET reservedTokens=reservedTokens+?, requestCount=requestCount+1 WHERE userId=? AND periodKey=?`, requestedTokens, userID, period); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (r *Repo) FinalizeUserQuota(userID string, reservedTokens, actualTokens int64) error {
	period := time.Now().UTC().Format("2006-01")
	var daily, monthly int64
	_ = r.db.QueryRow(`SELECT COALESCE(at.dailyTokenLimit,0), COALESCE(at.monthlyTokenLimit,0) FROM users u JOIN accountTypes at ON at.id=u.accountTypeId WHERE u.id=?`, userID).Scan(&daily, &monthly)
	if daily > 0 {
		period = time.Now().UTC().Format("2006-01-02")
	}
	if reservedTokens < 0 {
		reservedTokens = 0
	}
	if actualTokens < 0 {
		actualTokens = 0
	}
	_, err := r.db.Exec(`UPDATE userQuotaUsage SET reservedTokens=MAX(0,reservedTokens-?), actualTokens=actualTokens+? WHERE userId=? AND periodKey=?`, reservedTokens, actualTokens, userID, period)
	return err
}

func parseTimeOrZero(value string) time.Time { t, _ := time.Parse(time.RFC3339, value); return t }
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func keyPrefix(key string) string { return key[:minInt(12, len(key))] }
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
