package auditlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAuditLogger_RotationAndZeroDeletion(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "auditlog_test_*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	l := &Logger{
		logDir:    tempDir,
		entryChan: make(chan *AuditEntry, 100),
		doneChan:  make(chan struct{}),
	}
	if err := l.rotateLocked(); err != nil {
		t.Fatalf("rotate failed: %v", err)
	}

	entry := &AuditEntry{
		ID:       "req-1",
		Endpoint: "/v1/chat/completions",
		Provider: "openai",
		Model:    "gpt-4o",
		Status:   "ok",
		APIKey:   "sk-test-1234567890",
		ClientRequest: HTTPPayload{
			Method: "POST",
			URL:    "/v1/chat/completions",
			Body:   `{"model":"gpt-4o","messages":[{"role":"user","content":"Hello raw test"}]}`,
		},
		ClientResponse: HTTPPayload{
			Body: `{"id":"chatcmpl-1","choices":[{"message":{"content":"Hello back!"}}]}`,
		},
	}

	l.writeEntry(entry)

	files, err := l.ListLogFiles()
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least 1 log file")
	}

	fullPath, err := l.GetLogFilePath(files[0].Filename)
	if err != nil {
		t.Fatalf("get path: %v", err)
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty log file")
	}
	var persisted map[string]any
	if err := json.Unmarshal(content[:len(content)-1], &persisted); err != nil {
		t.Fatalf("compact record is not valid JSON: %v", err)
	}
	if persisted["apiKey"] != "sk-test...7890" {
		t.Fatalf("expected masked API key, got %v", persisted["apiKey"])
	}
	if persisted["api"] != nil {
		t.Fatalf("unexpected endpoint field, got %v", persisted["api"])
	}
	if _, exists := persisted["tokens"]; exists {
		t.Fatal("audit record must not persist token metadata")
	}
	if persisted["request"] == nil || persisted["response"] == nil {
		t.Fatalf("expected request and response training fields, got %v", persisted)
	}

	_ = filepath.Base(fullPath)
}

func TestAuditLogger_DeleteLogFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "auditlog_delete_test_*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	l := &Logger{
		logDir:    tempDir,
		entryChan: make(chan *AuditEntry, 10),
		doneChan:  make(chan struct{}),
	}
	if err := l.rotateLocked(); err != nil {
		t.Fatalf("rotate failed: %v", err)
	}
	activeName := filepath.Base(l.currentFile.Name())
	archivedName := "audit-2020-01-01-0001.jsonl"
	if err := os.WriteFile(filepath.Join(tempDir, archivedName), []byte("archived\n"), 0644); err != nil {
		t.Fatalf("write archived file: %v", err)
	}

	if err := l.DeleteLogFile(archivedName); err != nil {
		t.Fatalf("delete archived file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, archivedName)); !os.IsNotExist(err) {
		t.Fatalf("archived file still exists, stat error: %v", err)
	}

	if err := l.DeleteLogFile(activeName); err != nil {
		t.Fatalf("delete active file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, activeName)); !os.IsNotExist(err) {
		t.Fatalf("active file still exists, stat error: %v", err)
	}
	files, err := l.ListLogFiles()
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(files) != 1 || !files[0].IsActive || files[0].Filename == activeName {
		t.Fatalf("expected one fresh active file after deletion, got %+v", files)
	}
}

func TestAuditLogger_DeleteAllLogFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "auditlog_delete_all_test_*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	l := &Logger{
		logDir:    tempDir,
		entryChan: make(chan *AuditEntry, 10),
		doneChan:  make(chan struct{}),
	}
	if err := l.rotateLocked(); err != nil {
		t.Fatalf("rotate failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "audit-2020-01-01-0001.jsonl"), []byte("old\n"), 0644); err != nil {
		t.Fatalf("write old file: %v", err)
	}

	deleted, err := l.DeleteAllLogFiles()
	if err != nil {
		t.Fatalf("delete all files: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted files, got %d", deleted)
	}
	files, err := l.ListLogFiles()
	if err != nil {
		t.Fatalf("list files after batch deletion: %v", err)
	}
	if len(files) != 1 || !files[0].IsActive {
		t.Fatalf("expected one fresh active file after batch deletion, got %+v", files)
	}
	if info, err := os.Stat(filepath.Join(tempDir, files[0].Filename)); err != nil || info.Size() != 0 {
		t.Fatalf("expected empty active file, info=%v err=%v", info, err)
	}
}
