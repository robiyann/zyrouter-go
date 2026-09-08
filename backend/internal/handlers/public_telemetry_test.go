package handlers

import "testing"

func TestFilterClientTelemetryUsesPublicModelFallback(t *testing.T) {
	data := map[string]any{
		"totalTokens": 10,
		"recent": []map[string]any{
			{"model": "mimo-v2.5", "publicModel": ""},
			{"model": "gemini-3.8-flash", "publicModel": "gemini-3.8-flash"},
			{"model": "unpublished", "publicModel": ""},
		},
	}
	filtered := filterClientTelemetry(data, map[string]bool{"mimo-v2.5": true})
	items := filtered["recent"].([]map[string]any)
	if len(items) != 1 || items[0]["model"] != "mimo-v2.5" {
		t.Fatalf("expected only allowed alias, got %#v", items)
	}
}
