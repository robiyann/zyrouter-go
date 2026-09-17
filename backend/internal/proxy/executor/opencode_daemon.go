package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"zyrouter/backend/internal/constants"
)

const defaultOpencodeDaemonURL = "http://127.0.0.1:4096"

var (
	daemonSessionMu sync.RWMutex
	daemonSessions  = make(map[string]string)
	daemonClient    = &http.Client{Timeout: 120 * time.Second}
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

func extractPromptFromOpenAIBody(body []byte) string {
	var parsed struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Messages) == 0 {
		return "hello"
	}

	var b strings.Builder
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
		if content != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			if role == "system" {
				b.WriteString("[System Instructions]\n" + content)
			} else if role == "assistant" {
				b.WriteString("[Assistant]\n" + content)
			} else {
				b.WriteString(content)
			}
		}
	}
	res := b.String()
	if res == "" {
		return "hello"
	}
	return res
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

	promptText := extractPromptFromOpenAIBody(req.Body)

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

	finalText := textBuilder.String()
	reasoningText := reasoningBuilder.String()
	msgID := msgResp.Info.ID
	if msgID == "" {
		msgID = fmt.Sprintf("chatcmpl-%d", time.Now().Unix())
	}

	created := time.Now().Unix()

	if req.IsStream {
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeEventStream)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)

		// 1. send reasoning if present
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

		// 2. send text content
		textChunk := map[string]any{
			"id":      msgID,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   cleanModel,
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
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

		// 3. send finish
		finishReason := msgResp.Info.Finish
		if finishReason == "" {
			finishReason = "stop"
		}
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

	finishReason := msgResp.Info.Finish
	if finishReason == "" {
		finishReason = "stop"
	}

	respObj := map[string]any{
		"id":      msgID,
		"object":  "chat.completion",
		"created": created,
		"model":   cleanModel,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":              "assistant",
					"content":           finalText,
					"reasoning_content": reasoningText,
				},
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

	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(outBytes)
	return err
}
