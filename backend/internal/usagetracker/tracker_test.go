package usagetracker

import (
	"os"
	"sync"
	"testing"
	"time"

	"zyrouter/backend/internal/db"
)

func TestTracker_Lifecycle(t *testing.T) {
	tracker := NewTracker()

	// Initially empty
	state := tracker.GetActiveState(nil)
	if len(state.ActiveRequests) != 0 {
		t.Errorf("expected 0 active requests, got %d", len(state.ActiveRequests))
	}

	// 1. Request starts
	tracker.TrackPending("claude-sonnet-4-6", "anthropic", "conn_123", true, false)
	state = tracker.GetActiveState(nil)
	if len(state.ActiveRequests) != 1 {
		t.Fatalf("expected 1 active request, got %d", len(state.ActiveRequests))
	}
	if state.ActiveRequests[0].Provider != "anthropic" {
		t.Errorf("expected provider anthropic, got %s", state.ActiveRequests[0].Provider)
	}

	// 2. Request finishes with success and push recent
	tracker.TrackPending("claude-sonnet-4-6", "anthropic", "conn_123", false, false)
	tracker.PushRecent(RecentRequest{
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Model:            "claude-sonnet-4-6",
		Provider:         "anthropic",
		PromptTokens:     100,
		CompletionTokens: 50,
		Status:           "success",
	}, nil)

	state = tracker.GetActiveState(nil)
	if len(state.ActiveRequests) != 0 {
		t.Errorf("expected 0 active requests after completion, got %d", len(state.ActiveRequests))
	}
	if len(state.RecentRequests) != 1 {
		t.Errorf("expected 1 recent request, got %d", len(state.RecentRequests))
	}
}

func TestTracker_Subscription(t *testing.T) {
	tracker := NewTracker()
	ch, unsub := tracker.Subscribe()
	defer unsub()

	tracker.TrackPending("gpt-4o", "openai", "conn_abc", true, false)

	select {
	case payload := <-ch:
		if len(payload) == 0 {
			t.Error("received empty payload")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("timed out waiting for subscription broadcast")
	}
}

func TestTracker_PreservesDistinctRequestsWithSameModelAndTimestamp(t *testing.T) {
	tracker := NewTracker()
	ts := time.Now().UTC().Format(time.RFC3339)
	for _, id := range []string{"req-1", "req-2", "req-3"} {
		tracker.PushRecent(RecentRequest{ID: id, Timestamp: ts, Model: "mimo-v2.5-free", Provider: "opencode", Status: "200"}, nil)
	}
	state := tracker.GetActiveState(nil)
	if len(state.RecentRequests) != 3 {
		t.Fatalf("expected all 3 distinct requests, got %d", len(state.RecentRequests))
	}
}

func TestTracker_OrdersMixedTimezoneTimestampsByInstant(t *testing.T) {
	tracker := NewTracker()
	tracker.PushRecent(RecentRequest{ID: "older-local", Timestamp: "2026-09-28T23:25:22+07:00", Model: "older", Provider: "provider"}, nil)
	tracker.PushRecent(RecentRequest{ID: "newer-utc", Timestamp: "2026-09-28T16:32:46Z", Model: "newer", Provider: "provider"}, nil)

	state := tracker.GetActiveState(nil)
	if len(state.RecentRequests) < 2 {
		t.Fatalf("expected two recent requests, got %d", len(state.RecentRequests))
	}
	if state.RecentRequests[0].ID != "newer-utc" {
		t.Fatalf("expected newest absolute timestamp first, got %+v", state.RecentRequests[:2])
	}
}

func TestTracker_HistoryLabelResolutionDoesNotHoldRowsConnection(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "tracker-history-*.sqlite")
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
	database.SetMaxOpenConns(1)

	if _, err := database.Exec(`INSERT INTO usageHistory (timestamp, provider, model, promptTokens, completionTokens, status, meta, tokens) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), "unknown-provider", "test-model", 1, 1, "200", `{"requestId":"history-1"}`, `{"prompt_tokens":1,"completion_tokens":1}`); err != nil {
		t.Fatalf("insert history row: %v", err)
	}

	done := make(chan struct{})
	go func() {
		NewTracker().GetActiveState(db.NewRepo(database))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("history label resolution blocked while rows cursor was open")
	}
}

func TestTracker_ConcurrentLoad(t *testing.T) {
	tracker := NewTracker()
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			model := "model-" + string(rune('a'+index%26))
			tracker.TrackPending(model, "provider", "connection", true, false)
			_ = tracker.GetActiveState(nil)
			tracker.TrackPending(model, "provider", "connection", false, false)
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tracker did not drain 1000 concurrent request updates")
	}
}
