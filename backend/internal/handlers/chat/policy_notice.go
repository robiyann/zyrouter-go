package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"zyrouter/backend/internal/auth"
	"zyrouter/backend/internal/middleware"
)

const policyNotice = "Free tier usage has been exhausted or this API key does not have access to the requested model. For details, visit https://t.me/zyvenoxx or contact @robiyan."

// shouldWritePolicyNotice limits the assistant-shaped response to requests
// that already passed API-key authentication. Invalid keys must remain real
// 401 errors rather than becoming indistinguishable successful completions.
func shouldWritePolicyNotice(r *http.Request, err error) bool {
	if r == nil || middleware.GetAuthenticatedApiKey(r) == nil || err == nil {
		return false
	}
	return errors.Is(err, auth.ErrModelNotAllowed) || errors.Is(err, auth.ErrProviderNotAllowed) || errors.Is(err, auth.ErrRateLimitExceeded)
}

func writePolicyNotice(w http.ResponseWriter, model string, stream, claude bool) {
	if claude {
		if stream {
			writeClaudePolicyNoticeStream(w, model)
			return
		}
		writeClaudePolicyNotice(w, model)
		return
	}
	if stream {
		writeOpenAIPolicyNoticeStream(w, model)
		return
	}
	writeOpenAIPolicyNotice(w, model)
}

func policyNoticeID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func setPolicyNoticeHeaders(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Zyrouter-Policy-Notice", "true")
}

func writeOpenAIPolicyNotice(w http.ResponseWriter, model string) {
	id := policyNoticeID("chatcmpl-zyrouter-policy")
	response := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role":    "assistant",
				"content": policyNotice,
			},
			"finish_reason": "stop",
		}},
		"usage": map[string]int{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}
	writeJSONPolicyNotice(w, response)
}

func writeOpenAIPolicyNoticeStream(w http.ResponseWriter, model string) {
	id := policyNoticeID("chatcmpl-zyrouter-policy")
	setPolicyNoticeHeaders(w, "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	writeSSEJSON(w, map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{
			"index":         0,
			"delta":         map[string]any{"role": "assistant", "content": policyNotice},
			"finish_reason": nil,
		}},
	}, flusher)
	writeSSEJSON(w, map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model,
		"choices": []any{map[string]any{
			"index": 0, "delta": map[string]any{}, "finish_reason": "stop",
		}},
	}, flusher)
	fmt.Fprint(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}
}

func writeClaudePolicyNotice(w http.ResponseWriter, model string) {
	response := map[string]any{
		"id":            policyNoticeID("msg_zyrouter_policy"),
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       []any{map[string]any{"type": "text", "text": policyNotice}},
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage":         map[string]int{"input_tokens": 0, "output_tokens": 0},
	}
	writeJSONPolicyNotice(w, response)
}

func writeClaudePolicyNoticeStream(w http.ResponseWriter, model string) {
	id := policyNoticeID("msg_zyrouter_policy")
	setPolicyNoticeHeaders(w, "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	writeClaudeSSE(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": id, "type": "message", "role": "assistant", "model": model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": 0, "output_tokens": 0},
		},
	}, flusher)
	writeClaudeSSE(w, "content_block_start", map[string]any{
		"type": "content_block_start", "index": 0,
		"content_block": map[string]any{"type": "text", "text": ""},
	}, flusher)
	writeClaudeSSE(w, "content_block_delta", map[string]any{
		"type": "content_block_delta", "index": 0,
		"delta": map[string]any{"type": "text_delta", "text": policyNotice},
	}, flusher)
	writeClaudeSSE(w, "content_block_stop", map[string]any{
		"type": "content_block_stop", "index": 0,
	}, flusher)
	writeClaudeSSE(w, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": 0},
	}, flusher)
	writeClaudeSSE(w, "message_stop", map[string]any{"type": "message_stop"}, flusher)
}

func writeJSONPolicyNotice(w http.ResponseWriter, response map[string]any) {
	setPolicyNoticeHeaders(w, "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func writeSSEJSON(w http.ResponseWriter, payload map[string]any, flusher http.Flusher) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", encoded)
	if flusher != nil {
		flusher.Flush()
	}
}

func writeClaudeSSE(w http.ResponseWriter, event string, payload map[string]any, flusher http.Flusher) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", strings.TrimSpace(event), encoded)
	if flusher != nil {
		flusher.Flush()
	}
}
