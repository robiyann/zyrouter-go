package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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
	return json.Unmarshal(body, &request) == nil && strings.Contains(strings.ToLower(request.Model), "muse-spark")
}

func forwardMuseSparkResponses(w http.ResponseWriter, req *Request, apiKey string) error {
	requestBody, err := chatToResponsesBody(req.Body)
	if err != nil {
		return fmt.Errorf("build Muse Spark Responses request: %w", err)
	}

	cloakedBody := proxy.CloakOpenCodeTools(requestBody, true)

	cfg := *req.Config
	relayPath, usingEdgeRelay := cfg.StaticHeaders["x-relay-path"]
	cfg.StaticHeaders = proxy.BuildOpenCodeHeaders(cfg.StaticHeaders, req.SessionID, true)
	// Edge relays are generic; preserve the upstream path prefix (OpenCode uses
	// /zen/v1, not just /v1) and swap only the final endpoint.
	if usingEdgeRelay {
		cfg.StaticHeaders["x-relay-path"] = museResponsesPath(relayPath)
	} else {
		cfg.BaseURL = museResponsesURL(cfg.BaseURL)
	}
	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	resp, err := proxy.ForwardOpenAI(ctx, req.Client, &cfg, apiKey, cloakedBody, true)
	if err != nil {
		return fmt.Errorf("Muse Spark Responses request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, bodyErr := proxy.UpstreamBody(resp)
		return bodyErr
	}

	if req.IsStream {
		return streamMuseResponsesToChatSSE(ctx, w, resp.Body, req)
	}
	return aggregateMuseResponsesSSEToJSON(ctx, w, resp.Body, req)
}

func museResponsesURL(baseURL string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return strings.TrimSuffix(baseURL, "/chat/completions") + "/responses"
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL + "/responses"
	}
	if strings.HasSuffix(baseURL, "/responses") {
		return baseURL
	}
	return baseURL + "/responses"
}

func museResponsesPath(path string) string {
	path = strings.TrimRight(path, "/")
	if strings.HasSuffix(path, "/chat/completions") {
		return strings.TrimSuffix(path, "/chat/completions") + "/responses"
	}
	if strings.HasSuffix(path, "/v1") {
		return path + "/responses"
	}
	if strings.HasSuffix(path, "/responses") {
		return path
	}
	return path + "/responses"
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
	if maxTokens < 2048 {
		maxTokens = 4096
	}
	result := map[string]any{
		"model":             source.Model,
		"input":             input,
		"max_output_tokens": maxTokens,
		"stream":            true,
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

func streamMuseResponsesToChatSSE(ctx context.Context, w http.ResponseWriter, upstream io.Reader, req *Request) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush()
	}

	var (
		respID     = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
		created    = time.Now().Unix()
		modelName  = "muse-spark-1.3-contributor-free"
		contentBuf strings.Builder
		promptToks int
		compToks   int
		totToks    int
	)

	scanner := bufio.NewScanner(upstream)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataStr == "[DONE]" {
			break
		}

		var eventData map[string]any
		if err := json.Unmarshal([]byte(dataStr), &eventData); err != nil {
			continue
		}

		eventType, _ := eventData["type"].(string)

		if eventType == "response.created" || eventType == "response.in_progress" {
			if respObj, ok := eventData["response"].(map[string]any); ok {
				if id, ok := respObj["id"].(string); ok && id != "" {
					respID = id
				}
				if m, ok := respObj["model"].(string); ok && m != "" {
					modelName = m
				}
			}
		} else if eventType == "response.output_text.delta" {
			if deltaStr, ok := eventData["delta"].(string); ok && deltaStr != "" {
				contentBuf.WriteString(deltaStr)
				chunk := map[string]any{
					"id":      respID,
					"object":  "chat.completion.chunk",
					"created": created,
					"model":   modelName,
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"content": deltaStr,
							},
							"finish_reason": nil,
						},
					},
				}
				chunkBytes, _ := json.Marshal(chunk)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", chunkBytes)
				if flusher != nil {
					flusher.Flush()
				}
			}
		} else if eventType == "response.completed" {
			if respObj, ok := eventData["response"].(map[string]any); ok {
				if usageObj, ok := respObj["usage"].(map[string]any); ok {
					if inTok, ok := usageObj["input_tokens"].(float64); ok {
						promptToks = int(inTok)
					}
					if outTok, ok := usageObj["output_tokens"].(float64); ok {
						compToks = int(outTok)
					}
					if totTok, ok := usageObj["total_tokens"].(float64); ok {
						totToks = int(totTok)
					}
				}
			}
		}
	}

	finalChunk := map[string]any{
		"id":      respID,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   modelName,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "stop",
			},
		},
	}
	if totToks > 0 {
		finalChunk["usage"] = map[string]int{
			"prompt_tokens":     promptToks,
			"completion_tokens": compToks,
			"total_tokens":      totToks,
		}
	}
	finalBytes, _ := json.Marshal(finalChunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", finalBytes)
	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
	if flusher != nil {
		flusher.Flush()
	}

	fullResponse := map[string]any{
		"id":      respID,
		"object":  "chat.completion",
		"created": created,
		"model":   modelName,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": contentBuf.String(),
				},
				"finish_reason": "stop",
			},
		},
	}
	if totToks > 0 {
		fullResponse["usage"] = map[string]int{
			"prompt_tokens":     promptToks,
			"completion_tokens": compToks,
			"total_tokens":      totToks,
		}
	}
	fullBytes, _ := json.Marshal(fullResponse)
	if req.ResponseBuf != nil {
		req.ResponseBuf.Write(fullBytes)
	}
	if usage := translator.ParseResponseUsage(fullBytes); usage != nil && req.Ctx != nil {
		translator.SetUsage(req.Ctx, usage)
	}

	return nil
}

func aggregateMuseResponsesSSEToJSON(ctx context.Context, w http.ResponseWriter, upstream io.Reader, req *Request) error {
	var (
		respID     = fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
		created    = time.Now().Unix()
		modelName  = "muse-spark-1.3-contributor-free"
		contentBuf strings.Builder
		promptToks int
		compToks   int
		totToks    int
	)

	scanner := bufio.NewScanner(upstream)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataStr == "[DONE]" {
			break
		}

		var eventData map[string]any
		if err := json.Unmarshal([]byte(dataStr), &eventData); err != nil {
			continue
		}

		eventType, _ := eventData["type"].(string)

		if eventType == "response.created" || eventType == "response.in_progress" {
			if respObj, ok := eventData["response"].(map[string]any); ok {
				if id, ok := respObj["id"].(string); ok && id != "" {
					respID = id
				}
				if m, ok := respObj["model"].(string); ok && m != "" {
					modelName = m
				}
			}
		} else if eventType == "response.output_text.delta" {
			if deltaStr, ok := eventData["delta"].(string); ok && deltaStr != "" {
				contentBuf.WriteString(deltaStr)
			}
		} else if eventType == "response.completed" {
			if respObj, ok := eventData["response"].(map[string]any); ok {
				if id, ok := respObj["id"].(string); ok && id != "" {
					respID = id
				}
				if m, ok := respObj["model"].(string); ok && m != "" {
					modelName = m
				}
				if contentBuf.Len() == 0 {
					if outputArr, ok := respObj["output"].([]any); ok {
						for _, outItem := range outputArr {
							if outMap, ok := outItem.(map[string]any); ok {
								if cArr, ok := outMap["content"].([]any); ok {
									for _, cPart := range cArr {
										if cpMap, ok := cPart.(map[string]any); ok {
											if tStr, ok := cpMap["text"].(string); ok {
												contentBuf.WriteString(tStr)
											}
										}
									}
								}
							}
						}
					}
				}
				if usageObj, ok := respObj["usage"].(map[string]any); ok {
					if inTok, ok := usageObj["input_tokens"].(float64); ok {
						promptToks = int(inTok)
					}
					if outTok, ok := usageObj["output_tokens"].(float64); ok {
						compToks = int(outTok)
					}
					if totTok, ok := usageObj["total_tokens"].(float64); ok {
						totToks = int(totTok)
					}
				}
			}
		}
	}

	result := map[string]any{
		"id":      respID,
		"object":  "chat.completion",
		"created": created,
		"model":   modelName,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": contentBuf.String(),
				},
				"finish_reason": "stop",
			},
		},
	}
	if totToks > 0 {
		result["usage"] = map[string]int{
			"prompt_tokens":     promptToks,
			"completion_tokens": compToks,
			"total_tokens":      totToks,
		}
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return jsonResponse(ctx, w, bytes.NewReader(jsonBytes), req.TranslateResp, req.ResponseBuf)
}
