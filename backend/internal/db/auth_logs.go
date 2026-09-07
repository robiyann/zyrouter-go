package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// AuthLogEntry contains metadata only; credentials and request bodies must never enter this table.
type AuthLogEntry struct {
	Timestamp string
	Event     string
	IP        string
	Method    string
	Path      string
	Status    int
	RequestID string
	UserAgent string
	Referer   string
	Detail    string
}

func (r *Repo) InsertAuthLogs(entries []AuthLogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		err := r.insertAuthLogsTx(entries)
		if err == nil {
			return nil
		}
		lastErr = err
		errStr := strings.ToLower(err.Error())
		if !strings.Contains(errStr, "locked") && !strings.Contains(errStr, "busy") {
			return err
		}
		time.Sleep(time.Duration(25*(attempt+1)) * time.Millisecond)
	}
	return lastErr
}

func (r *Repo) insertAuthLogsTx(entries []AuthLogEntry) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO authLogs (timestamp,event,ip,method,path,status,requestId,userAgent,referer,detail) VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, entry := range entries {
		if entry.Timestamp == "" {
			entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if _, err := stmt.Exec(entry.Timestamp, entry.Event, entry.IP, entry.Method, entry.Path, entry.Status, entry.RequestID, entry.UserAgent, entry.Referer, entry.Detail); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM authLogs WHERE id NOT IN (SELECT id FROM authLogs ORDER BY id DESC LIMIT 10000)`); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repo) ListAuthLogs(limit, offset int) ([]map[string]any, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.Query(`SELECT id,timestamp,event,ip,method,path,status,requestId,userAgent,referer,detail FROM authLogs ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var id, status int
		var ts, event, ip, method, path, requestID, ua, referer, detail sql.NullString
		if err := rows.Scan(&id, &ts, &event, &ip, &method, &path, &status, &requestID, &ua, &referer, &detail); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{"id": id, "timestamp": ts.String, "event": event.String, "ip": ip.String, "method": method.String, "path": path.String, "status": status, "requestId": requestID.String, "userAgent": ua.String, "referer": referer.String, "detail": detail.String})
	}
	return result, rows.Err()
}

func (r *Repo) CountAuthLogs() (int, error) {
	var count int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM authLogs`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count auth logs: %w", err)
	}
	return count, nil
}
