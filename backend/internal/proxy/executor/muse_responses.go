package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"zyrouter/backend/internal/proxy"
	"zyrouter/backend/internal/translator"
)

// Muse Spark is listed by Zen's chat catalog but only serves through the
// Responses API. Keep the public Zyrouter contract as chat completions and
// translate the request/response at the OpenCode executor boundary.
func isMuseSparkModel(body []byte) bool {
	var request struct {
		Model string `json:"model"`
	}
	return json.Unmarshal(body, &request) == nil && strings.HasPrefix(strings.ToLower(request.Model), "muse-spark-")
}

func forwardMuseSparkResponses(w http.ResponseWriter, req *Request, apiKey string) error {
	requestBody, err := chatToResponsesBody(req.Body)
	if err != nil {
		return fmt.Errorf("build Muse Spark Responses request: %w", err)
	}

	cfg := *req.Config
	if strings.Contains(cfg.BaseURL, "/chat/completions") {
		cfg.BaseURL = strings.Replace(cfg.BaseURL, "/chat/completions", "/responses", 1)
	}
	cfg.StaticHeaders = proxy.BuildOpenCodeHeaders(cfg.StaticHeaders, req.SessionID, false)
	// Edge relays keep their own URL and route by x-relay-path instead of the
	// upstream URL path.
	if _, ok := cfg.StaticHeaders["x-relay-path"]; ok {
		cfg.StaticHeaders["x-relay-path"] = "/v1/responses"
	}
	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := proxy.ForwardOpenAI(ctx, req.Client, &cfg, apiKey, requestBody, false)
	if err != nil {
		return fmt.Errorf("Muse Spark Responses request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, bodyErr := proxy.UpstreamBody(resp)
		return bodyErr
	}
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return fmt.Errorf("read Muse Spark Responses response: %w", err)
	}
	chatBody, err := responsesToChatBody(responseBody)
	if err != nil {
		return err
	}

	if req.IsStream {
		return writeMuseChatStream(w, chatBody, req)
	}
	return jsonResponse(ctx, w, bytes.NewReader(chatBody), req.TranslateResp, req.ResponseBuf)
}

func chatToResponsesBody(body []byte) ([]byte, error) {
	var source struct {
		Model           string           `json:"model"`
		Messages        []map[string]any `json:"messages"`
		MaxTokens       int              `json:"max_tokens"`
		ReasoningEffort string           `json:"reasoning_effort,omitempty"`
	}
	if err := json.Unmarshal(body, &source); err != nil {
		return nil, err
	}
	input := make([]map[string]any, 0, len(source.Messages))
	for _, message := range source.Messages {
		role, _ := message["role"].(string)
		content := message["content"]
		text := ""
		if value, ok := content.(string); ok {
			text = value
		} else if content != nil {
			encoded, _ := json.Marshal(content)
			text = string(encoded)
		}
		input = append(input, map[string]any{
			"role":    role,
			"content": []map[string]string{{"type": "input_text", "text": text}},
		})
	}
	maxTokens := source.MaxTokens
	if maxTokens < 16 {
		maxTokens = 16
	}
	result := map[string]any{
		"model":             source.Model,
		"input":             input,
		"max_output_tokens": maxTokens,
	}
	if source.ReasoningEffort != "" && source.ReasoningEffort != "none" && source.ReasoningEffort != "off" {
		result["reasoning"] = map[string]string{"effort": source.ReasoningEffort}
	}
	return json.Marshal(result)
}

func responsesToChatBody(body []byte) ([]byte, error) {
	var source struct {
		ID         string `json:"id"`
		Model      string `json:"model"`
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(body, &source); err != nil {
		return nil, fmt.Errorf("decode Muse Spark Responses response: %w", err)
	}
	content := source.OutputText
	if content == "" {
		for _, item := range source.Output {
			for _, part := range item.Content {
				if part.Type == "output_text" || part.Type == "text" {
					content += part.Text
				}
			}
		}
	}
	result := map[string]any{
		"id":      source.ID,
		"object":  "chat.completion",
		"model":   source.Model,
		"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}},
	}
	if source.Usage != nil {
		result["usage"] = map[string]int{"prompt_tokens": source.Usage.InputTokens, "completion_tokens": source.Usage.OutputTokens, "total_tokens": source.Usage.TotalTokens}
	}
	return json.Marshal(result)
}

func writeMuseChatStream(w http.ResponseWriter, body []byte, req *Request) error {
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}
	if req.ResponseBuf != nil {
		req.ResponseBuf.Write(body)
	}
	if usage := translator.ParseResponseUsage(body); usage != nil && req.Ctx != nil {
		translator.SetUsage(req.Ctx, usage)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	chunk := map[string]any{
		"id": response["id"], "object": "chat.completion.chunk", "model": response["model"],
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant", "content": response["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["content"]}, "finish_reason": nil}},
	}
	data, _ := json.Marshal(chunk)
	_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}
