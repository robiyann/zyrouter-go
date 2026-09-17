package proxy_test

import (
	"testing"

	"zyrouter/backend/internal/proxy"
)

func TestBuildOpenCodeHeaders(t *testing.T) {
	headers := proxy.BuildOpenCodeHeaders(nil, "my-session-123", true)
	if headers["User-Agent"] != proxy.DefaultOpenCodeUA {
		t.Errorf("expected User-Agent %s, got %s", proxy.DefaultOpenCodeUA, headers["User-Agent"])
	}
	if headers["x-opencode-client"] != "desktop" {
		t.Errorf("expected x-opencode-client desktop, got %s", headers["x-opencode-client"])
	}
	if headers["x-opencode-project"] != "global" {
		t.Errorf("expected x-opencode-project global, got %s", headers["x-opencode-project"])
	}
	if !proxy.OpenCodeSessionRegex.MatchString(headers["x-opencode-session"]) {
		t.Errorf("expected canonical x-opencode-session, got %s", headers["x-opencode-session"])
	}
	if !proxy.OpenCodeRequestRegex.MatchString(headers["x-opencode-request"]) {
		t.Errorf("expected canonical x-opencode-request, got %s", headers["x-opencode-request"])
	}
	if headers["Accept"] != "text/event-stream" {
		t.Errorf("expected Accept text/event-stream, got %s", headers["Accept"])
	}

	// Non-stream accepts */*
	headersNonStream := proxy.BuildOpenCodeHeaders(nil, "", false)
	if headersNonStream["Accept"] != "*/*" {
		t.Errorf("expected Accept */*, got %s", headersNonStream["Accept"])
	}
}

func TestOpenCodeSessionAndUaValidation(t *testing.T) {
	// Canonical session generation
	for i := 0; i < 10; i++ {
		ses := proxy.GenerateOpenCodeSessionID()
		if !proxy.OpenCodeSessionRegex.MatchString(ses) || len(ses) != 30 {
			t.Fatalf("invalid generated session id: %s (len=%d)", ses, len(ses))
		}
		msg := proxy.GenerateOpenCodeRequestID()
		if !proxy.OpenCodeRequestRegex.MatchString(msg) || len(msg) != 30 {
			t.Fatalf("invalid generated request id: %s (len=%d)", msg, len(msg))
		}
	}

	// Translation
	translated := proxy.TranslateOpenCodeSessionID("some-random-uuid-123")
	if !proxy.OpenCodeSessionRegex.MatchString(translated) || len(translated) != 30 {
		t.Fatalf("translation failed: %s", translated)
	}

	// Preserves valid session
	validSes := "ses_f534dfae8ffeCy4Ee4tLWNygDc"
	if proxy.TranslateOpenCodeSessionID(validSes) != validSes {
		t.Fatalf("expected valid session to be preserved")
	}

	// UA validation
	if !proxy.HasValidOpenCodeVersion("opencode/1.18.31") {
		t.Errorf("expected opencode/1.18.31 to be valid")
	}
	if !proxy.HasValidOpenCodeVersion("opencode/1.17.0") {
		t.Errorf("expected opencode/1.17.0 to be valid")
	}
	if proxy.HasValidOpenCodeVersion("opencode/1.15.0") {
		t.Errorf("expected opencode/1.15.0 to be invalid")
	}
	if proxy.HasValidOpenCodeVersion("opencode") {
		t.Errorf("expected bare opencode to be invalid")
	}
}
