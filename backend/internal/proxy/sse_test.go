package proxy

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScanStreamChunks(t *testing.T) {
	streamData := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n")
	buf := bytes.NewBuffer(streamData)

	var chunks [][]byte
	err := ScanStream(buf, func(chunk []byte) {
		chunks = append(chunks, chunk)
	})

	if err != nil {
		t.Fatalf("ScanStream failed: %v", err)
	}

	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}
	if !bytes.Equal(chunks[1], []byte("[DONE]")) {
		t.Errorf("expected last chunk to be [DONE], got %s", string(chunks[1]))
	}
}

func TestScanStreamAccumulatesEvents(t *testing.T) {
	// Multi-line data payload, keep-alive, comment, and CRLF endings.
	streamData := []byte(": keep-alive\r\n" +
		"data: {\"a\":1,\r\n" +
		"data: \"b\":2}\r\n" +
		"\r\n" +
		"data: \r\n" +
		"\r\n" +
		"data: [DONE]\r\n")
	buf := bytes.NewBuffer(streamData)

	var chunks [][]byte
	err := ScanStream(buf, func(chunk []byte) {
		chunks = append(chunks, chunk)
	})

	if err != nil {
		t.Fatalf("ScanStream failed: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %q", len(chunks), chunks)
	}
	// Multi-line payload is joined with a newline.
	want := "{\"a\":1,\n\"b\":2}"
	if string(chunks[0]) != want {
		t.Errorf("expected %q, got %q", want, string(chunks[0]))
	}
	if string(chunks[1]) != "[DONE]" {
		t.Errorf("expected last chunk [DONE], got %q", string(chunks[1]))
	}
}

type mockFlusher struct {
	bytes.Buffer
	flushed bool
}

func (f *mockFlusher) Flush() {
	f.flushed = true
}

func TestStreamWriterAndWriteChunk(t *testing.T) {
	// Test StreamWriter with a flusher
	flusher := &mockFlusher{}
	sw := NewStreamWriter(flusher)

	n, err := sw.WriteChunk([]byte("hello"))
	if err != nil {
		t.Fatalf("WriteChunk failed: %v", err)
	}
	expected := "data: hello\n\n"
	if flusher.String() != expected {
		t.Errorf("expected output %q, got %q", expected, flusher.String())
	}
	if n != len(expected) {
		t.Errorf("expected length %d, got %d", len(expected), n)
	}
	if !flusher.flushed {
		t.Errorf("expected flusher to be called")
	}

	// Test WriteChunk directly
	flusher2 := &mockFlusher{}
	n2, err2 := WriteChunk(flusher2, []byte("world"))
	if err2 != nil {
		t.Fatalf("WriteChunk failed: %v", err2)
	}
	expected2 := "data: world\n\n"
	if flusher2.String() != expected2 {
		t.Errorf("expected output %q, got %q", expected2, flusher2.String())
	}
	if n2 != len(expected2) {
		t.Errorf("expected length %d, got %d", len(expected2), n2)
	}
	if !flusher2.flushed {
		t.Errorf("expected flusher to be called")
	}
}

type errorWriter struct{}

func (ew *errorWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("write error")
}

func TestStreamWriterErrors(t *testing.T) {
	ew := &errorWriter{}
	sw := NewStreamWriter(ew)

	_, err := sw.WriteChunk([]byte("hello"))
	if err == nil {
		t.Error("expected error, got nil")
	}

	_, err2 := WriteChunk(ew, []byte("hello"))
	if err2 == nil {
		t.Error("expected error, got nil")
	}
}

type trailingErrorReader struct {
	data   []byte
	offset int
	err    error
}

func (r *trailingErrorReader) Read(p []byte) (n int, err error) {
	if r.offset < len(r.data) {
		n = copy(p, r.data[r.offset:])
		r.offset += n
		return n, nil
	}
	return 0, r.err
}

func TestSSECopy_CompletesOnDone(t *testing.T) {
	// Stream contains data: [DONE], followed by trailing context.Canceled error.
	// SSECopy should return nil because [DONE] marks clean completion.
	streamData := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"test\"}}]}\n\ndata: [DONE]\n\n")
	upstream := &trailingErrorReader{
		data: streamData,
		err:  context.Canceled,
	}
	rec := httptest.NewRecorder()
	var collected []byte
	err := SSECopy(rec, upstream, rec, func(chunk []byte) {
		collected = append(collected, chunk...)
	})
	if err != nil {
		t.Fatalf("expected nil error on [DONE] stream, got: %v", err)
	}
	if !bytes.Contains(collected, []byte("[DONE]")) {
		t.Fatalf("expected collected bytes to contain [DONE]")
	}
}

func TestSSECopy_ReturnsErrorWhenInterruptedBeforeDone(t *testing.T) {
	// Stream interrupted before [DONE] is reached.
	streamData := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
	upstream := &trailingErrorReader{
		data: streamData,
		err:  context.Canceled,
	}
	rec := httptest.NewRecorder()
	err := SSECopy(rec, upstream, rec, nil)
	if err == nil {
		t.Fatalf("expected error when stream interrupted before [DONE], got nil")
	}
}

func TestSSECopy_SanitizesProviderErrorEvent(t *testing.T) {
	upstream := strings.NewReader("data: {\"error\":{\"message\":\"account banned\",\"type\":\"auth_error\"}}\n\n")
	rec := httptest.NewRecorder()
	if err := SSECopy(rec, upstream, rec, nil); err != nil {
		t.Fatalf("SSECopy: %v", err)
	}
	if strings.Contains(rec.Body.String(), "account banned") || strings.Contains(rec.Body.String(), "auth_error") {
		t.Fatalf("provider error leaked through SSE: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "upstream_unavailable") {
		t.Fatalf("missing safe SSE error: %s", rec.Body.String())
	}
}
