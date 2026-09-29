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

func TestUsageHistoryCursorIsBoundedAndStable(t *testing.T) {
	database, err := OpenDatabase(filepath.Join(t.TempDir(), "usage-cursor.sqlite"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	for i, ts := range []string{"2026-09-29T12:03:00Z", "2026-09-29T12:02:00Z", "2026-09-29T12:01:00Z"} {
		_, err := database.Exec(`INSERT INTO usageHistory (timestamp, provider, model, promptTokens, completionTokens, status, userId, meta) VALUES (?, 'provider', 'model', 1, 1, '200', 'user-cursor', ?)`, ts, `{"requestId":"cursor-`+string(rune('a'+i))+`","publicModel":"alias"}`)
		if err != nil {
			t.Fatalf("insert usage: %v", err)
		}
	}
	repo := NewRepo(database)
	first, err := repo.GetUserUsageLogsCursor("user-cursor", 2, "")
	if err != nil {
		t.Fatalf("first cursor page: %v", err)
	}
	if first["hasMore"] != true || first["nextCursor"] == "" || len(first["items"].([]map[string]any)) != 2 {
		t.Fatalf("expected bounded first page with cursor, got %#v", first)
	}
	second, err := repo.GetUserUsageLogsCursor("user-cursor", 2, first["nextCursor"].(string))
	if err != nil {
		t.Fatalf("second cursor page: %v", err)
	}
	items := second["items"].([]map[string]any)
	if second["hasMore"] != false || len(items) != 1 || items[0]["requestId"] != "cursor-c" {
		t.Fatalf("expected oldest item on second page, got %#v", second)
	}
}
