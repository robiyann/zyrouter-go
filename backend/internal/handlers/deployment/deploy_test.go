package deployment

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelayProjectName(t *testing.T) {
	if got := relayProjectName("  edge-relay  "); got != "edge-relay" {
		t.Fatalf("expected trimmed project name, got %q", got)
	}
	if got := relayProjectName(""); !strings.HasPrefix(got, "relay-") {
		t.Fatalf("expected generated relay name, got %q", got)
	}
}

func TestPlatformRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected request: method=%s auth=%q", r.Method, r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"ok":true}` {
			t.Errorf("unexpected body: %s", body)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"created":true}`))
	}))
	defer server.Close()

	status, body, err := platformRequest(context.Background(), server.Client(), http.MethodPost, server.URL, "test-token", []byte(`{"ok":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusCreated || string(body) != `{"created":true}` {
		t.Fatalf("unexpected response: status=%d body=%s", status, body)
	}
}
