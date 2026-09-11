package antigravitybot

import (
	"strings"
	"testing"
	"time"

	"zyrouter/backend/internal/antigravityquota"
)

func TestParseAllowedUserIDs(t *testing.T) {
	allowed, err := ParseAllowedUserIDs("123, 456,123")
	if err != nil {
		t.Fatal(err)
	}
	if len(allowed) != 2 {
		t.Fatalf("expected two unique IDs, got %d", len(allowed))
	}
	if _, err := ParseAllowedUserIDs("not-a-number"); err == nil {
		t.Fatal("expected invalid user ID error")
	}
}

func TestRenderSnapshotIncludesWeeklyAndFiveHour(t *testing.T) {
	snapshot := &antigravityquota.Snapshot{
		FetchedAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
		Accounts: []antigravityquota.AccountQuota{{
			Email: "user@example.com",
			Windows: []antigravityquota.Window{
				{ID: "gemini-5h", Label: "Gemini 5-hour", RemainingPercentage: 84},
				{ID: "gemini-weekly", Label: "Gemini Weekly", RemainingPercentage: 67},
			},
			Models: []antigravityquota.ModelQuota{
				{ID: "model-a", DisplayName: "Same Model", RemainingPercentage: 80, ResetAt: "2030-01-01T01:00:00Z"},
				{ID: "model-b", DisplayName: "Same Model Alias", RemainingPercentage: 80, ResetAt: "2030-01-01T01:00:00Z"},
			},
		}},
	}
	full := renderSnapshot(snapshot, false, "")
	if !containsAll(full, "Gemini Models", "Gemini 5-hour: 84%", "Gemini Weekly: 67%", "Same Model, Same Model Alias") {
		t.Fatalf("full quota output missing windows: %s", full)
	}
	weekly := renderSnapshot(snapshot, true, "")
	if !containsAll(weekly, "Gemini Weekly: 67%") || containsAll(weekly, "Gemini 5-hour: 84%") {
		t.Fatalf("weekly-only output is incorrect: %s", weekly)
	}
	filtered := renderSnapshot(snapshot, false, "user@example.com")
	if !containsAll(filtered, "user@example.com") {
		t.Fatalf("account filter removed the matching account: %s", filtered)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
