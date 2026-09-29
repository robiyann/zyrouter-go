package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const safeUpstreamErrorMessage = "Upstream service unavailable."

// SafeUpstreamErrorMessage is the public-safe message used for provider
// failures after the response has already entered streaming mode.
func SafeUpstreamErrorMessage() string { return safeUpstreamErrorMessage }

// SafeSSEErrorFrame returns a provider-agnostic terminal SSE error frame.
func SafeSSEErrorFrame() []byte {
	return []byte("data: {\"error\":{\"message\":\"Upstream service unavailable.\",\"type\":\"server_error\",\"code\":\"upstream_unavailable\"}}\n\ndata: [DONE]\n\n")
}

// IsErrorPayload reports whether a decoded upstream JSON payload represents an
// error response, including providers that incorrectly return it with HTTP 200.
func IsErrorPayload(payload []byte) bool {
	var obj map[string]any
	if json.Unmarshal(bytes.TrimSpace(payload), &obj) != nil || obj == nil {
		return false
	}
	if _, ok := obj["error"]; ok {
		return true
	}
	if nested, ok := obj["response"].(map[string]any); ok {
		if _, ok := nested["error"]; ok {
			return true
		}
	}
	if typ, ok := obj["type"].(string); ok && strings.EqualFold(typ, "error") {
		return true
	}
	return false
}

func sanitizeSSEEvent(lines []string) ([]byte, bool) {
	eventType := ""
	var dataLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			if strings.HasPrefix(data, " ") {
				data = data[1:]
			}
			dataLines = append(dataLines, data)
		}
	}
	if strings.EqualFold(eventType, "error") || IsErrorPayload([]byte(strings.Join(dataLines, "\n"))) {
		return SafeSSEErrorFrame(), true
	}
	return []byte(strings.Join(lines, "\n") + "\n\n"), false
}

// WriteSSEHeaders sets standard SSE headers on the response and writes HTTP 200.
// Returns the http.Flusher if the ResponseWriter supports it.
func WriteSSEHeaders(w http.ResponseWriter) http.Flusher {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	f, _ := w.(http.Flusher)
	return f
}

// SSECopy reads from upstream in a raw loop and writes each chunk to the client.
// A simplified passthrough that does NOT parse SSE framing — use when translation is not needed.
// onChunk is called for each chunk before writing (for metrics/TTFT tracking).
// Returns the first upstream or write error so a truncated stream is not
// reported as a successful completion.
func SSECopy(w http.ResponseWriter, upstream io.Reader, flusher http.Flusher, onChunk func([]byte)) error {
	buf := make([]byte, 4096)
	var sawDone bool
	var eventLines []string
	sawSSEField := false
	flushEvent := func() error {
		if len(eventLines) == 0 {
			return nil
		}
		out, isError := sanitizeSSEEvent(eventLines)
		eventLines = eventLines[:0]
		if bytes.Contains(out, []byte("[DONE]")) {
			sawDone = true
		}
		if onChunk != nil {
			onChunk(out)
		}
		if _, err := w.Write(out); err != nil {
			if sawDone {
				return nil
			}
			return fmt.Errorf("write stream to client: %w", err)
		}
		if flusher != nil {
			flusher.Flush()
		}
		if isError {
			return io.EOF
		}
		return nil
	}
	var pending []byte
	for {
		n, err := upstream.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			for {
				idx := bytes.IndexByte(pending, '\n')
				if idx < 0 {
					break
				}
				line := strings.TrimSuffix(string(pending[:idx]), "\r")
				pending = pending[idx+1:]
				if line == "" {
					flushErr := flushEvent()
					if flushErr == io.EOF || sawDone {
						return nil
					}
					if flushErr != nil {
						return flushErr
					}
					continue
				}
				if strings.HasPrefix(line, "data:") || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, ":") {
					sawSSEField = true
				}
				eventLines = append(eventLines, line)
			}
		}
		if err != nil {
			if !sawSSEField && len(pending) > 0 {
				raw := pending
				pending = nil
				if IsErrorPayload(raw) {
					raw = SafeSSEErrorFrame()
					sawDone = true
				}
				if onChunk != nil {
					onChunk(raw)
				}
				if _, writeErr := w.Write(raw); writeErr != nil && !sawDone {
					return fmt.Errorf("write stream to client: %w", writeErr)
				}
				if flusher != nil {
					flusher.Flush()
				}
				if sawDone || err == io.EOF {
					return nil
				}
				return fmt.Errorf("read upstream stream: %w", err)
			}
			if len(pending) > 0 {
				eventLines = append(eventLines, strings.TrimSuffix(string(pending), "\r"))
				pending = nil
			}
			if flushErr := flushEvent(); flushErr != nil && flushErr != io.EOF {
				return flushErr
			}
			if err == io.EOF || sawDone {
				return nil
			}
			return fmt.Errorf("read upstream stream: %w", err)
		}
	}
}
