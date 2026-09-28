package chat

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/middleware"
	"zyrouter/backend/internal/models"
)

func authenticatedPolicyRequest() *http.Request {
	key := &models.APIKey{IsActive: 1}
	return httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(
		context.WithValue(context.Background(), middleware.ApiKeyContextKey, key),
	)
}

func TestShouldWritePolicyNoticeRequiresAuthenticatedKey(t *testing.T) {
	if shouldWritePolicyNotice(httptest.NewRequest(http.MethodPost, "/", nil), auth.ErrModelNotAllowed) {
		t.Fatal("invalid request unexpectedly qualified for assistant policy notice")
	}
	if !shouldWritePolicyNotice(authenticatedPolicyRequest(), auth.ErrModelNotAllowed) {
		t.Fatal("authenticated model denial did not qualify for assistant policy notice")
	}
	if shouldWritePolicyNotice(authenticatedPolicyRequest(), errors.New("database failure")) {
		t.Fatal("internal error unexpectedly qualified for assistant policy notice")
	}
}

func TestWriteOpenAIPolicyNotice(t *testing.T) {
	rec := httptest.NewRecorder()
	writePolicyNotice(rec, "premium-model", false, false)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected synthetic policy notice to return 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"\"role\":\"assistant\"", policyNotice, "premium-model"} {
		if !strings.Contains(body, want) {
			t.Fatalf("policy notice missing %q: %s", want, body)
		}
	}
	if got := rec.Header().Get("X-Zyrouter-Policy-Notice"); got != "true" {
		t.Fatalf("expected policy marker header, got %q", got)
	}
}

func TestWriteClaudePolicyNoticeStream(t *testing.T) {
	rec := httptest.NewRecorder()
	writePolicyNotice(rec, "premium-model", true, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected synthetic Claude stream to return 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"event: message_start", "event: content_block_delta", "event: message_stop", policyNotice} {
		if !strings.Contains(body, want) {
			t.Fatalf("Claude policy stream missing %q: %s", want, body)
		}
	}
}
