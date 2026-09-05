package chat

import (
	"encoding/json"
	"strings"
)

// sanitizeReasoningModelBody applies the narrow compatibility rules required
// by next-generation reasoning routes without changing normal provider bodies.
func sanitizeReasoningModelBody(model string, body []byte) []byte {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-6-astra") {
		return body
	}
	var request map[string]any
	if json.Unmarshal(body, &request) != nil {
		return body
	}
	delete(request, "temperature")
	delete(request, "top_p")
	if maxTokens, ok := request["max_tokens"].(float64); ok && maxTokens > 0 && maxTokens < 16 {
		request["max_tokens"] = 16
	}
	out, err := json.Marshal(request)
	if err != nil {
		return body
	}
	return out
}
