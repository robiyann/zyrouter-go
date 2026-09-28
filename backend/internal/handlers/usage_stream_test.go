package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/usagetracker"
)

func TestHandleUsageStream(t *testing.T) {
	tracker := usagetracker.GetTracker()

	handler := HandleUsageStream(nil)
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/usage/stream", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	reader := bufio.NewReader(resp.Body)

	// 1. Read initial state
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read line: %v", err)
	}
	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("expected data: prefix, got %s", line)
	}

	var initPayload usagetracker.StreamPayload
	dataBytes := strings.TrimPrefix(strings.TrimSpace(line), "data: ")
	if err := json.Unmarshal([]byte(dataBytes), &initPayload); err != nil {
		t.Fatalf("unmarshal init payload: %v", err)
	}

	// 2. Trigger pending request in tracker
	tracker.TrackPending("gemini-2.5-flash", "antigravity", "conn_test", true, false)

	// Read empty line
	_, _ = reader.ReadString('\n')

	// Read next data event
	line2, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read line2: %v", err)
	}
	if !strings.HasPrefix(line2, "data: ") {
		t.Fatalf("expected data: prefix on line2, got %s", line2)
	}

	var updatePayload usagetracker.StreamPayload
	dataBytes2 := strings.TrimPrefix(strings.TrimSpace(line2), "data: ")
	if err := json.Unmarshal([]byte(dataBytes2), &updatePayload); err != nil {
		t.Fatalf("unmarshal update payload: %v", err)
	}

	if len(updatePayload.ActiveRequests) == 0 {
		t.Fatalf("expected at least 1 active request in update payload")
	}

	tracker.TrackPending("gemini-2.5-flash", "antigravity", "conn_test", false, false)
}

func TestHandleUsageStatsOrdersRecentByTimestamp(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "usage-order-*.sqlite")
	if err != nil {
		t.Fatalf("create temp database: %v", err)
	}
	path := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(path)

	database, err := db.OpenDatabase(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	repo := db.NewRepo(database)
	if _, err := database.Exec(`INSERT INTO usageHistory (timestamp, provider, model, promptTokens, completionTokens, status) VALUES (?, ?, ?, 1, 1, '200'), (?, ?, ?, 1, 1, '200')`,
		"2026-09-28T23:25:22+07:00", "provider", "older", "2026-09-28T16:32:46Z", "provider", "newer"); err != nil {
		t.Fatalf("insert usage history: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/usage/stats?days=all", nil)
	rec := httptest.NewRecorder()
	HandleUsageStats(repo)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		RecentRequests []struct {
			Model string `json:"model"`
		} `json:"recentRequests"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode usage payload: %v", err)
	}
	if len(payload.RecentRequests) < 2 || payload.RecentRequests[0].Model != "provider/newer" {
		t.Fatalf("expected newest request first, got %+v", payload.RecentRequests)
	}
}
