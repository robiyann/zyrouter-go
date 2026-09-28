package db

import (
	"path/filepath"
	"testing"
)

func TestGetRecentUsageByAliasesReturnsNewestTimestamp(t *testing.T) {
	database, err := OpenDatabase(filepath.Join(t.TempDir(), "usage-order.sqlite"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()

	// The later insert has an older timestamp: insertion ID is not request time.
	for _, entry := range []struct{ timestamp, requestID string }{
		{"2026-09-28T12:46:40Z", "newest"},
		{"2026-09-28T12:44:13Z", "older"},
	} {
		_, err := database.Exec(`INSERT INTO usageHistory (timestamp, provider, model, promptTokens, completionTokens, status, meta) VALUES (?, 'opencode', 'test-model', 1, 1, '200', ?)`, entry.timestamp, `{"requestId":"`+entry.requestID+`","publicModel":"test-alias"}`)
		if err != nil {
			t.Fatalf("insert usage: %v", err)
		}
	}

	rows, err := NewRepo(database).GetRecentUsageByAliases(map[string]string{"test-alias": "opencode/test-model"}, 1)
	if err != nil {
		t.Fatalf("get recent usage: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "newest" {
		t.Fatalf("wanted latest request by timestamp, got %+v", rows)
	}
}
