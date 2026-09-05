package chat

import (
	"strings"
	"testing"
)

func TestSanitizeReasoningModelBody(t *testing.T) {
	got := string(sanitizeReasoningModelBody("gpt-6-astra", []byte(`{"temperature":0.7,"top_p":1,"max_tokens":5,"model":"gpt-6-astra"}`)))
	if strings.Contains(got, "temperature") || strings.Contains(got, "top_p") || !strings.Contains(got, `"max_tokens":16`) {
		t.Fatalf("reasoning parameters were not sanitized: %s", got)
	}
}

func TestSanitizeReasoningModelBodyLeavesNormalModelsUntouched(t *testing.T) {
	body := []byte(`{"temperature":0.7,"top_p":1,"max_tokens":5,"model":"gpt-5"}`)
	if got := string(sanitizeReasoningModelBody("gpt-5", body)); got != string(body) {
		t.Fatalf("normal model body changed: %s", got)
	}
}
