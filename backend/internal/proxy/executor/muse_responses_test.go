package executor

import (
	"encoding/json"
	"testing"
)

func TestChatToResponsesBody(t *testing.T) {
	body, err := chatToResponsesBody([]byte(`{"model":"muse-spark-1.3-contributor-free","messages":[{"role":"system","content":"You are helpful."},{"role":"user","content":"hi"}],"max_tokens":8}`))
	if err != nil {
		t.Fatalf("chatToResponsesBody failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode transformed body: %v", err)
	}
	if got["model"] != "muse-spark-1.3-contributor-free" || got["max_output_tokens"] != float64(16) {
		t.Fatalf("unexpected transformed body: %s", body)
	}
	input := got["input"].([]any)
	if len(input) != 2 || input[0].(map[string]any)["role"] != "system" {
		t.Fatalf("expected system message to remain first: %s", body)
	}
}

func TestResponsesToChatBody(t *testing.T) {
	body, err := responsesToChatBody([]byte(`{"id":"resp_1","model":"muse-spark-1.3-contributor-free","output":[{"content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":4,"output_tokens":2,"total_tokens":6}}`))
	if err != nil {
		t.Fatalf("responsesToChatBody failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode chat body: %v", err)
	}
	choices := got["choices"].([]any)
	message := choices[0].(map[string]any)["message"].(map[string]any)
	if message["content"] != "OK" {
		t.Fatalf("expected converted content OK, got %v", message["content"])
	}
}
