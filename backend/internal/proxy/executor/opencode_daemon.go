package executor

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"zyrouter/backend/internal/constants"
)

const defaultOpencodeDaemonURL = "http://127.0.0.1:4096"

var (
	daemonSessionMu sync.RWMutex
	daemonSessions  = make(map[string]string)
	daemonClient    = &http.Client{Timeout: 180 * time.Second}
	toolCallRegex   = regexp.MustCompile(`(?s)\{\s*"tool_calls"\s*:\s*\[(.*?)\]\s*\}`)
	singleToolRegex = regexp.MustCompile(`(?s)\{\s*"name"\s*:\s*"([^"]+)"\s*,\s*"arguments"\s*:\s*(\{.*?\})\s*\}`)
)

type opencodeSessionResp struct {
	ID string `json:"id"`
}

type opencodeMessagePart struct {
	Type   string `json:"type"`
	Text   string `json:"text,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type opencodeMessageInfo struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Finish string `json:"finish"`
	Tokens struct {
		Input     int `json:"input"`
		Output    int `json:"output"`
		Reasoning int `json:"reasoning"`
		Total     int `json:"total"`
	} `json:"tokens"`
	Error *struct {
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	} `json:"error,omitempty"`
}

type opencodeMessageResp struct {
	Info  opencodeMessageInfo   `json:"info"`
	Parts []opencodeMessagePart `json:"parts"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func genToolCallID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return "call_" + hex.EncodeToString(b)
}

func getOrCreateDaemonSession(ctx context.Context, sessionKey string) (string, error) {
	if sessionKey != "" {
		daemonSessionMu.RLock()
		if sid, ok := daemonSessions[sessionKey]; ok {
			daemonSessionMu.RUnlock()
			return sid, nil
		}
		daemonSessionMu.RUnlock()
	}

	reqBody, _ := json.Marshal(map[string]string{"directory": "/root"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, defaultOpencodeDaemonURL+"/session", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

	resp, err := daemonClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create session failed (status %d): %s", resp.StatusCode, string(b))
	}

	var sResp opencodeSessionResp
	if err := json.NewDecoder(resp.Body).Decode(&sResp); err != nil {
		return "", err
	}

	if sessionKey != "" && sResp.ID != "" {
		daemonSessionMu.Lock()
		if len(daemonSessions) >= 1000 {
			for k := range daemonSessions {
				delete(daemonSessions, k)
				break
			}
		}
		daemonSessions[sessionKey] = sResp.ID
		daemonSessionMu.Unlock()
	}

	return sResp.ID, nil
}

func extractPromptFromOpenAIBody(body []byte) (string, bool) {
	var parsed struct {
		Messages []struct {
			Role       string           `json:"role"`
			Content    any              `json:"content"`
			ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
			ToolCallID string           `json:"tool_call_id,omitempty"`
			Name       string           `json:"name,omitempty"`
		} `json:"messages"`
		Tools      []any `json:"tools,omitempty"`
		ToolChoice any   `json:"tool_choice,omitempty"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Messages) == 0 {
		return "hello", false
	}

	hasTools := len(parsed.Tools) > 0
	var b strings.Builder

	if hasTools {
		toolsJSON, _ := json.MarshalIndent(parsed.Tools, "", "  ")
		b.WriteString("# Available Tools\n")
		b.WriteString("You have access to the following tools:\n```json\n")
		b.WriteString(string(toolsJSON))
		b.WriteString("\n```\n\n")
		b.WriteString("INSTRUCTIONS FOR TOOL CALLING:\n")
		b.WriteString("When you need to execute a tool, respond ONLY with a valid JSON object in this exact format:\n")
		b.WriteString("```json\n{\n  \"tool_calls\": [\n    {\n      \"name\": \"function_name\",\n      \"arguments\": {\"param_name\": \"value\"}\n    }\n  ]\n}\n```\n")
		b.WriteString("If you do not need to call any tool, answer directly in standard text.\n\n---\n\n")
	}

	for _, m := range parsed.Messages {
		role := strings.ToLower(m.Role)
		content := ""
		if str, ok := m.Content.(string); ok {
			content = str
		} else if arr, ok := m.Content.([]any); ok {
			for _, item := range arr {
				if obj, ok := item.(map[string]any); ok {
					if text, ok := obj["text"].(string); ok {
						content += text + " "
					}
				}
			}
		}
		content = strings.TrimSpace(content)

		if b.Len() > 0 {
			b.WriteString("\n\n")
		}

		if role == "system" {
			b.WriteString("[System Instructions]\n" + content)
		} else if role == "assistant" {
			if len(m.ToolCalls) > 0 {
				tcJSON, _ := json.Marshal(map[string]any{"tool_calls": m.ToolCalls})
				b.WriteString("[Assistant Tool Call]\n" + string(tcJSON))
			} else {
				b.WriteString("[Assistant]\n" + content)
			}
		} else if role == "tool" {
			b.WriteString(fmt.Sprintf("[Tool Result for %s]\n%s", m.ToolCallID, content))
		} else {
			b.WriteString(content)
		}
	}

	res := b.String()
	if res == "" {
		return "hello", hasTools
	}
	return res, hasTools
}

func parseToolCallsFromOutput(text string) ([]openAIToolCall, string) {
	clean := strings.TrimSpace(text)
	
	// Strip code block fences if present
	if strings.HasPrefix(clean, "```json") && strings.HasSuffix(clean, "```") {
		clean = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(clean, "```json"), "```"))
	} else if strings.HasPrefix(clean, "```") && strings.HasSuffix(clean, "```") {
		clean = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(clean, "```"), "```"))
	}

	// Try direct unmarshal of {"tool_calls": [...]}
	var parsed struct {
		ToolCalls []struct {
			Name      string `json:"name"`
			Arguments any    `json:"arguments"`
		} `json:"tool_calls"`
	}

	if err := json.Unmarshal([]byte(clean), &parsed); err == nil && len(parsed.ToolCalls) > 0 {
		var out []openAIToolCall
		for _, tc := range parsed.ToolCalls {
			if tc.Name == "" {
				continue
			}
			argsStr := "{}"
			if str, ok := tc.Arguments.(string); ok {
				argsStr = str
			} else if tc.Arguments != nil {
				if b, mErr := json.Marshal(tc.Arguments); mErr == nil {
					argsStr = string(b)
				}
			}
			out = append(out, openAIToolCall{
				ID:   genToolCallID(),
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      tc.Name,
					Arguments: argsStr,
				},
			})
		}
		if len(out) > 0 {
			return out, ""
		}
	}

	// Try regex extraction of tool_calls block
	if match := toolCallRegex.FindString(clean); match != "" {
		if err := json.Unmarshal([]byte(match), &parsed); err == nil && len(parsed.ToolCalls) > 0 {
			var out []openAIToolCall
			for _, tc := range parsed.ToolCalls {
				if tc.Name == "" {
					continue
				}
				argsStr := "{}"
				if str, ok := tc.Arguments.(string); ok {
					argsStr = str
				} else if tc.Arguments != nil {
					if b, mErr := json.Marshal(tc.Arguments); mErr == nil {
						argsStr = string(b)
					}
				}
				out = append(out, openAIToolCall{
					ID:   genToolCallID(),
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{
						Name:      tc.Name,
						Arguments: argsStr,
					},
				})
			}
			if len(out) > 0 {
				remText := strings.TrimSpace(strings.Replace(clean, match, "", 1))
				return out, remText
			}
		}
	}

	return nil, text
}

func ForwardOpencodeDaemon(w http.ResponseWriter, req *Request, cleanModel string) error {
	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	sessionID, err := getOrCreateDaemonSession(ctx, req.SessionID)
	if err != nil {
		return fmt.Errorf("opencode daemon session: %w", err)
	}

	promptText, hasTools := extractPromptFromOpenAIBody(req.Body)

	msgPayload := map[string]any{
		"model": map[string]string{
			"providerID": "opencode",
			"modelID":    cleanModel,
		},
		"parts": []map[string]string{
			{
				"type": "text",
				"text": promptText,
			},
		},
	}

	payloadBytes, err := json.Marshal(msgPayload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/session/%s/message", defaultOpencodeDaemonURL, sessionID)
	daemonReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	daemonReq.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)

	resp, err := daemonClient.Do(daemonReq)
	if err != nil {
		return fmt.Errorf("opencode daemon message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("opencode daemon returned status %d: %s", resp.StatusCode, string(b))
	}

	var msgResp opencodeMessageResp
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return err
	}

	if msgResp.Info.Error != nil && msgResp.Info.Error.Data.Message != "" {
		return fmt.Errorf("opencode upstream error: %s", msgResp.Info.Error.Data.Message)
	}

	var textBuilder strings.Builder
	var reasoningBuilder strings.Builder

	for _, p := range msgResp.Parts {
		if p.Type == "text" && p.Text != "" {
			textBuilder.WriteString(p.Text)
		} else if p.Type == "reasoning" && p.Text != "" {
			reasoningBuilder.WriteString(p.Text)
		}
	}

	rawText := textBuilder.String()
	reasoningText := reasoningBuilder.String()
	msgID := msgResp.Info.ID
	if msgID == "" {
		msgID = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
	}

	created := time.Now().Unix()

	var toolCalls []openAIToolCall
	finalText := rawText
	if hasTools {
		toolCalls, finalText = parseToolCallsFromOutput(rawText)
	}

	finishReason := msgResp.Info.Finish
	if finishReason == "" {
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
		} else {
			finishReason = "stop"
		}
	}

	if req.IsStream {
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeEventStream)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)

		// 1. Send reasoning if present
		if reasoningText != "" {
			reasoningChunk := map[string]any{
				"id":      msgID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   cleanModel,
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role":              "assistant",
							"reasoning_content": reasoningText,
						},
						"finish_reason": nil,
					},
				},
			}
			chunkBytes, _ := json.Marshal(reasoningChunk)
			fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
			if ok {
				flusher.Flush()
			}
		}

		// 2. Send content or tool_calls
		if len(toolCalls) > 0 {
			tcChunk := map[string]any{
				"id":      msgID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   cleanModel,
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role":       "assistant",
							"tool_calls": toolCalls,
						},
						"finish_reason": nil,
					},
				},
			}
			chunkBytes, _ := json.Marshal(tcChunk)
			fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
			if ok {
				flusher.Flush()
			}
		} else {
			textChunk := map[string]any{
				"id":      msgID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   cleanModel,
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role":    "assistant",
							"content": finalText,
						},
						"finish_reason": nil,
					},
				},
			}
			chunkBytes, _ := json.Marshal(textChunk)
			fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
			if ok {
				flusher.Flush()
			}
		}

		// 3. Send finish chunk
		finishChunk := map[string]any{
			"id":      msgID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   cleanModel,
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         map[string]any{},
					"finish_reason": finishReason,
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     msgResp.Info.Tokens.Input,
				"completion_tokens": msgResp.Info.Tokens.Output,
				"total_tokens":      msgResp.Info.Tokens.Total,
			},
		}
		finishBytes, _ := json.Marshal(finishChunk)
		fmt.Fprintf(w, "data: %s\n\n", string(finishBytes))
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if ok {
			flusher.Flush()
		}
		return nil
	}

	choiceMsg := map[string]any{
		"role":              "assistant",
		"content":           finalText,
		"reasoning_content": reasoningText,
	}
	if len(toolCalls) > 0 {
		choiceMsg["tool_calls"] = toolCalls
		if finalText == "" {
			choiceMsg["content"] = nil
		}
	}

	respObj := map[string]any{
		"id":      msgID,
		"object":  "chat.completion",
		"created": created,
		"model":   cleanModel,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       choiceMsg,
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     msgResp.Info.Tokens.Input,
			"completion_tokens": msgResp.Info.Tokens.Output,
			"total_tokens":      msgResp.Info.Tokens.Total,
		},
	}

	outBytes, err := json.Marshal(respObj)
	if err != nil {
		return err
	}

	if req.ResponseBuf != nil {
		req.ResponseBuf.Write(outBytes)
	}

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(outBytes)
	return err
}
