package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"zyrouter/backend/internal/handlerutil"
)

// HandleResponses exposes a small OpenAI Responses API compatibility layer.
// Requests are normalized to the existing chat router so account selection,
// proxy policy, fallback, usage logging, and OAuth refresh stay centralized.
func (h *ChatHandler) HandleResponses(w http.ResponseWriter, r *http.Request) {
	body, err := readResponsesBody(r)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var input struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &input); err != nil || input.Model == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing model")
		return
	}
	modelInfo, err := h.resolveModel(input.Model)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(modelInfo.ComboModels) > 0 {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "Responses API does not support combo models")
		return
	}
	if err := h.validateRequestPolicy(r, input.Model, modelInfo); err != nil {
		handlerutil.WriteJSONError(w, http.StatusForbidden, fmt.Sprintf("Forbidden: %v", err))
		return
	}
	if err := h.validateRequestRateLimit(r); err != nil {
		handlerutil.WriteJSONError(w, http.StatusTooManyRequests, err.Error())
		return
	}

	chatBody, err := responsesToChatRequest(body, modelInfo.Model)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	recorder := httptest.NewRecorder()
	forwardErr := h.handleAccountFallback(r.Context(), recorder, modelInfo.Provider, modelInfo.Model, modelInfo.ConnectionID, chatBody, false, false, "/v1/responses")
	if forwardErr != nil {
		if recorder.Code > 0 {
			w.WriteHeader(recorder.Code)
			_, _ = w.Write(recorder.Body.Bytes())
			return
		}
		handlerutil.WriteJSONError(w, http.StatusBadGateway, forwardErr.Error())
		return
	}
	responseBody, err := chatToResponsesResponse(recorder.Body.Bytes(), input.Model)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	if input.Stream {
		writeResponsesStream(w, responseBody)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseBody)
}

func readResponsesBody(r *http.Request) ([]byte, error) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("invalid JSON body")
	}
	encoded, err := json.Marshal(body)
	return encoded, err
}

func responsesToChatRequest(body []byte, resolvedModel string) ([]byte, error) {
	var source struct {
		Instructions any               `json:"instructions"`
		Input        any               `json:"input"`
		MaxTokens    int               `json:"max_output_tokens"`
		Reasoning    map[string]string `json:"reasoning,omitempty"`
	}
	if err := json.Unmarshal(body, &source); err != nil {
		return nil, err
	}
	messages := make([]map[string]any, 0)
	if instruction, ok := source.Instructions.(string); ok && strings.TrimSpace(instruction) != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instruction})
	}
	if inputText, ok := source.Input.(string); ok {
		messages = append(messages, map[string]any{"role": "user", "content": inputText})
	} else if items, ok := source.Input.([]any); ok {
		for _, item := range items {
			message, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := message["role"].(string)
			content := message["content"]
			text := contentText(content)
			messages = append(messages, map[string]any{"role": role, "content": text})
		}
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("missing input")
	}
	request := map[string]any{
		"model":    resolvedModel,
		"messages": messages,
		"stream":   false,
	}
	if source.MaxTokens > 0 {
		request["max_tokens"] = source.MaxTokens
	}
	if source.Reasoning != nil {
		if effort := source.Reasoning["effort"]; effort != "" {
			request["reasoning_effort"] = effort
		}
	}
	return json.Marshal(request)
}

func contentText(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	parts, _ := content.([]any)
	var result strings.Builder
	for _, part := range parts {
		block, _ := part.(map[string]any)
		text, _ := block["text"].(string)
		if text != "" {
			result.WriteString(text)
		}
	}
	return result.String()
}

func chatToResponsesResponse(body []byte, requestedModel string) ([]byte, error) {
	var chat struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage any `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(body, &chat); err != nil {
		return nil, fmt.Errorf("decode chat response: %w", err)
	}
	content := ""
	if len(chat.Choices) > 0 {
		content = chat.Choices[0].Message.Content
	}
	model := chat.Model
	if model == "" {
		model = requestedModel
	}
	result := map[string]any{
		"id":          "resp_" + strings.TrimPrefix(chat.ID, "chatcmpl-"),
		"object":      "response",
		"created_at":  time.Now().Unix(),
		"status":      "completed",
		"model":       model,
		"output_text": content,
		"output": []map[string]any{{
			"type": "message", "id": "msg_" + strings.TrimPrefix(chat.ID, "chatcmpl-"), "role": "assistant", "status": "completed",
			"content": []map[string]any{{"type": "output_text", "text": content, "annotations": []any{}}},
		}},
		"error": nil,
	}
	if chat.Usage != nil {
		result["usage"] = chat.Usage
	}
	return json.Marshal(result)
}

func writeResponsesStream(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	var response map[string]any
	_ = json.Unmarshal(body, &response)
	id, _ := response["id"].(string)
	model, _ := response["model"].(string)
	created := response["created_at"]
	write := func(event string, data any) {
		encoded, _ := json.Marshal(data)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, encoded)
	}
	write("response.created", map[string]any{"type": "response.created", "response": response})
	if outputText, ok := response["output_text"].(string); ok && outputText != "" {
		write("response.output_text.delta", map[string]any{"type": "response.output_text.delta", "delta": outputText, "item_id": id, "output_index": 0, "content_index": 0})
	}
	write("response.completed", map[string]any{"type": "response.completed", "response": map[string]any{"id": id, "object": "response", "created_at": created, "status": "completed", "model": model}})
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
