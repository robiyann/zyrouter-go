package authlog

import (
	"sync"
	"time"

	"zyrouter/backend/internal/db"
	"zyrouter/backend/internal/log"
)

type Logger struct {
	repo  *db.Repo
	queue chan db.AuthLogEntry
	stop  chan struct{}
	once  sync.Once
}

var global *Logger

func Init(repo *db.Repo) {
	global = &Logger{repo: repo, queue: make(chan db.AuthLogEntry, 4096), stop: make(chan struct{})}
	go global.run()
}

func Record(entry db.AuthLogEntry) {
	if global == nil {
		return
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	// Backpressure is deliberate: security events must not be silently dropped.
	global.queue <- entry
}

func Shutdown() {
	if global == nil {
		return
	}
	global.once.Do(func() { close(global.stop) })
	global = nil
}

func (l *Logger) run() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	batch := make([]db.AuthLogEntry, 0, 100)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := l.repo.InsertAuthLogs(batch); err != nil {
			log.Error("authlog", "flush failed", "error", err)
		}
		batch = batch[:0]
	}
	for {
		select {
		case entry := <-l.queue:
			batch = append(batch, entry)
			if len(batch) >= 100 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-l.stop:
			for {
				select {
				case entry := <-l.queue:
					batch = append(batch, entry)
				default:
					flush()
					return
				}
			}
		}
	}
}
