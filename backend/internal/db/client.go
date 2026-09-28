package db

import (
	"database/sql"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	dbInstance *sql.DB
	dbOnce     sync.Once
	initErr    error
)

// OpenDatabase opens a SQLite database and configures it with WAL mode, normal synchronous mode,
// and other safe concurrency / performance defaults matching the Node/Bun implementation.
func OpenDatabase(path string) (*sql.DB, error) {
	if path != ":memory:" && !strings.Contains(path, ":memory:") {
		// Ensure the parent directory of the database file exists
		dbDir := filepath.Dir(path)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return nil, fmt.Errorf("create db dir %s: %w", dbDir, err)
		}
		// The DB stores provider API keys/tokens in plaintext, so keep the file
		// and its directory private to the owning user.
		_ = os.Chmod(dbDir, 0700)
	}
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("sql.Open(%s): %w", path, err)
	}

	// Configure PRAGMAs for performance, safety, and concurrency.
	// busy_timeout is critical in WAL mode to prevent immediate "database is locked" errors during concurrent writes.
	// foreign_keys is required to enforce relational database integrity.
	pragmas := `
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA temp_store = MEMORY;
PRAGMA mmap_size = 30000000;
PRAGMA cache_size = -64000;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
`
	if _, err = db.Exec(pragmas); err != nil {
		db.Close()
		return nil, fmt.Errorf("pragma exec: %w", err)
	}

	if err := EnsureSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}

	// Restrict the DB file to the owning user (it stores plaintext keys).
	// The file is created by the driver on first open; chmod it now and
	// again on every open to re-assert the permission.
	if path != ":memory:" && !strings.Contains(path, ":memory:") {
		if err := os.Chmod(path, 0600); err != nil && !errors.Is(err, os.ErrNotExist) {
			db.Close()
			return nil, fmt.Errorf("chmod db file %s: %w", path, err)
		}
	}

	// SQLite readers can run concurrently in WAL mode. Keep enough connections
	// for the HTTP/SSE fan-out, while the busy timeout serializes short writes
	// instead of letting a small pool deadlock behind an open Rows cursor.
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(time.Hour)

	return db, nil
}

// sqliteDSN applies connection-local pragmas to every connection created by
// database/sql. Executing PRAGMA busy_timeout once through db.Exec only
// configures the one connection that happened to be selected at that moment.
func sqliteDSN(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// InitGlobalDatabase initializes the global database connection instance.
func InitGlobalDatabase(path string) error {
	dbOnce.Do(func() {
		dbInstance, initErr = OpenDatabase(path)
	})
	return initErr
}

// GetConnection returns the global database connection.
func GetConnection() (*sql.DB, error) {
	if dbInstance == nil {
		return nil, errors.New("database not initialized, call InitGlobalDatabase first")
	}
	return dbInstance, nil
}
