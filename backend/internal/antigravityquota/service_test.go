package antigravityquota

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"zyrouter/backend/internal/db"
)

func TestParseSummaryIncludesWeeklyAndFiveHourBuckets(t *testing.T) {
	body := []byte(`{"groups":[
        {"displayName":"Gemini Models","buckets":[
            {"bucketId":"gemini-5h","window":"5h","remainingFraction":0.84,"resetTime":"2030-01-01T00:00:00Z"},
            {"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.67,"resetTime":"2030-01-05T00:00:00Z"}
        ]},
        {"displayName":"Claude and GPT models","buckets":[
            {"bucketId":"3p-5h","window":"5h","remainingFraction":0.91},
            {"bucketId":"3p-weekly","window":"weekly","remainingFraction":0.78}
        ]}
    ]}`)

	windows := parseSummary(body)
	if len(windows) != 4 {
		t.Fatalf("expected four quota windows, got %d: %+v", len(windows), windows)
	}
	if windows[1].ID != "gemini-weekly" || windows[1].RemainingPercentage != 67 {
		t.Fatalf("unexpected Gemini weekly window: %+v", windows[1])
	}
	if windows[3].ID != "3p-weekly" || windows[3].RemainingPercentage != 78 {
		t.Fatalf("unexpected Claude/GPT weekly window: %+v", windows[3])
	}
}

func TestSnapshotReadsDatabaseAndMergesSummaryWithModelQuota(t *testing.T) {
	file, err := os.CreateTemp("", "ag-quota-*.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	defer os.Remove(file.Name())

	database, err := db.OpenDatabase(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	_, err = database.Exec(`INSERT INTO providerConnections
		(id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt)
		VALUES ('ag-1', 'antigravity', 'oauth', 'Primary', 'user@example.com', 1, 1, ?, '2030-01-01T00:00:00Z', '2030-01-01T00:00:00Z')`,
		`{"accessToken":"access-token","expiresAt":"2030-01-01T00:00:00Z","projectId":"project-1"}`)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-token" {
			t.Errorf("missing or incorrect authorization header: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("User-Agent") != DefaultUserAgent {
			t.Errorf("unexpected User-Agent: %q", r.Header.Get("User-Agent"))
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if payload["project"] != "project-1" {
			t.Errorf("expected project-1, got %#v", payload)
		}
		switch r.URL.Path {
		case "/v1internal:retrieveUserQuotaSummary":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini Models","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.72,"resetTime":"2030-01-05T00:00:00Z"},{"bucketId":"gemini-5h","window":"5h","remainingFraction":0.88}]}]}`))
		case "/v1internal:fetchAvailableModels":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"models":{"gemini-3.8-flash-high":{"displayName":"Gemini 3.8 Flash High","quotaInfo":{"remainingFraction":0.81,"resetTime":"2030-01-01T01:00:00Z"}},"internal":{"isInternal":true,"quotaInfo":{"remainingFraction":0}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewServiceWithClient(db.NewRepo(database), server.Client(), []string{server.URL})
	snapshot, err := service.Snapshot(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Accounts) != 1 {
		t.Fatalf("expected one account, got %+v", snapshot.Accounts)
	}
	account := snapshot.Accounts[0]
	if len(account.Windows) != 2 || account.Windows[0].ID != "gemini-5h" || account.Windows[1].ID != "gemini-weekly" {
		t.Fatalf("summary windows were not merged/sorted: %+v", account.Windows)
	}
	if account.Windows[1].RemainingPercentage != 72 {
		t.Fatalf("unexpected weekly percentage: %+v", account.Windows[1])
	}
	if len(account.Models) != 1 || account.Models[0].RemainingPercentage != 81 {
		t.Fatalf("unexpected model quota: %+v", account.Models)
	}
	if strings.Contains(account.Error, "weekly") {
		t.Fatalf("unexpected weekly error: %q", account.Error)
	}
}
