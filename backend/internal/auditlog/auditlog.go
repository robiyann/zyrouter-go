package auditlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"zyrouter/backend/internal/config"
	"zyrouter/backend/internal/constants"
	"zyrouter/backend/internal/log"
)

const (
	// MaxFileSizeBytes is 50 Megabytes per log file.
	MaxFileSizeBytes int64 = 50 * 1024 * 1024
	// ChannelBufferSize allows non-blocking asynchronous log processing.
	ChannelBufferSize = 4096
)

// AuditEntry captures the complete, unredacted transaction between client, gateway, and upstream AI provider.
type AuditEntry struct {
	ID               string              `json:"id"`
	Timestamp        string              `json:"timestamp"`
	Endpoint         string              `json:"endpoint"`
	Provider         string              `json:"provider"`
	Model            string              `json:"model"`
	ConnectionID     string              `json:"connectionId"`
	APIKey           string              `json:"apiKey"`
	Status           string              `json:"status"`
	StatusCode       int                 `json:"statusCode"`
	DurationMs       int64               `json:"durationMs"`
	TTFTMs           int64               `json:"ttftMs"`
	ClientRequest    HTTPPayload         `json:"clientRequest"`
	ProviderRequest  HTTPPayload         `json:"providerRequest"`
	ProviderResponse HTTPPayload         `json:"providerResponse"`
	ClientResponse   HTTPPayload         `json:"clientResponse"`
	Tokens           map[string]int      `json:"tokens,omitempty"`
	Cost             float64             `json:"cost,omitempty"`
	Error            string              `json:"error,omitempty"`
}

// HTTPPayload contains raw, unredacted headers and body.
type HTTPPayload struct {
	URL     string            `json:"url,omitempty"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// Logger handles thread-safe, 50MB-rolling persistent logging without deleting historical files.
type Logger struct {
	mu           sync.Mutex
	logDir       string
	currentFile  *os.File
	currentSize  int64
	currentDate  string
	currentIndex int
	entryChan    chan *AuditEntry
	doneChan     chan struct{}
	closed       bool
}

var (
	globalLogger *Logger
	once         sync.Once
)

// InitGlobalLogger initializes the singleton audit logger.
func InitGlobalLogger(customDir string) *Logger {
	once.Do(func() {
		dir := customDir
		if dir == "" {
			dataDir := config.ResolveDataDir()
			dir = filepath.Join(dataDir, "logs", "audit")
		}
		_ = os.MkdirAll(dir, constants.FilePermDir)

		l := &Logger{
			logDir:    dir,
			entryChan: make(chan *AuditEntry, ChannelBufferSize),
			doneChan:  make(chan struct{}),
		}

		if err := l.rotateLocked(); err != nil {
			log.Error("auditlog", "initial file creation failed", "error", err)
		}

		go l.worker()
		globalLogger = l
		log.Info("auditlog", "initialized rotating audit logger (max 50MB/file)", "dir", dir)
	})
	return globalLogger
}

// Get returns the global audit logger instance.
func Get() *Logger {
	if globalLogger == nil {
		return InitGlobalLogger("")
	}
	return globalLogger
}

// Log enqueues an unredacted audit entry asynchronously.
func (l *Logger) Log(entry *AuditEntry) {
	if l == nil || entry == nil {
		return
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	select {
	case l.entryChan <- entry:
	default:
		// If queue is full, process synchronously to avoid dropping log data
		go l.writeEntry(entry)
	}
}

func (l *Logger) worker() {
	for entry := range l.entryChan {
		l.writeEntry(entry)
	}
	close(l.doneChan)
}

func (l *Logger) writeEntry(entry *AuditEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')
	entryLen := int64(len(data))

	l.mu.Lock()
	defer l.mu.Unlock()

	today := time.Now().UTC().Format("2006-01-02")
	if l.currentFile == nil || today != l.currentDate || (l.currentSize+entryLen) >= MaxFileSizeBytes {
		if err := l.rotateLocked(); err != nil {
			log.Error("auditlog", "rotate failed", "error", err)
			return
		}
	}

	if l.currentFile != nil {
		n, err := l.currentFile.Write(data)
		if err == nil {
			l.currentSize += int64(n)
		}
	}
}

// rotateLocked finds the next sequential file index for today and opens it.
func (l *Logger) rotateLocked() error {
	if l.currentFile != nil {
		_ = l.currentFile.Sync()
		_ = l.currentFile.Close()
		l.currentFile = nil
	}

	today := time.Now().UTC().Format("2006-01-02")
	if today != l.currentDate {
		l.currentDate = today
		l.currentIndex = 1
	}

	// Scan directory to find the highest index for today
	for {
		filename := fmt.Sprintf("audit-%s-%04d.jsonl", l.currentDate, l.currentIndex)
		filePath := filepath.Join(l.logDir, filename)
		info, err := os.Stat(filePath)
		if os.IsNotExist(err) {
			// New file to create
			f, createErr := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if createErr != nil {
				return createErr
			}
			l.currentFile = f
			l.currentSize = 0
			return nil
		} else if err == nil {
			if info.Size() < MaxFileSizeBytes {
				// Append to existing file that has not reached 50MB
				f, openErr := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0644)
				if openErr != nil {
					return openErr
				}
				l.currentFile = f
				l.currentSize = info.Size()
				return nil
			}
			// Current index is full (>= 50MB), increment index and check next
			l.currentIndex++
		} else {
			return err
		}
	}
}

// LogFileInfo represents metadata about an audit log file.
type LogFileInfo struct {
	Filename    string `json:"filename"`
	SizeBytes   int64  `json:"sizeBytes"`
	SizeMB      string `json:"sizeMB"`
	ModTime     string `json:"modTime"`
	IsActive    bool   `json:"isActive"`
}

// ListLogFiles returns all historical audit log files ordered by newest first.
func (l *Logger) ListLogFiles() ([]LogFileInfo, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return nil, err
	}

	var files []LogFileInfo
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}

		sizeMB := fmt.Sprintf("%.2f MB", float64(info.Size())/(1024*1024))
		isActive := false
		if l.currentFile != nil && filepath.Base(l.currentFile.Name()) == e.Name() {
			isActive = true
		}

		files = append(files, LogFileInfo{
			Filename:  e.Name(),
			SizeBytes: info.Size(),
			SizeMB:    sizeMB,
			ModTime:   info.ModTime().UTC().Format(time.RFC3339),
			IsActive:  isActive,
		})
	}
	return files, nil
}

// GetLogFilePath returns the absolute path to a specific audit log file.
func (l *Logger) GetLogFilePath(filename string) (string, error) {
	clean := filepath.Clean(filename)
	if filepath.Ext(clean) != ".jsonl" || filepath.Base(clean) != clean {
		return "", fmt.Errorf("invalid log filename")
	}
	fullPath := filepath.Join(l.logDir, clean)
	if _, err := os.Stat(fullPath); err != nil {
		return "", err
	}
	return fullPath, nil
}

// Close flushes and shuts down the audit logger.
func (l *Logger) Close() {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	l.closed = true
	close(l.entryChan)
	l.mu.Unlock()

	<-l.doneChan

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.currentFile != nil {
		_ = l.currentFile.Sync()
		_ = l.currentFile.Close()
		l.currentFile = nil
	}
}
