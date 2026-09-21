package db

import (
	"sync"
	"time"
)

type proxyEventKind int

const (
	proxyEventSuccess proxyEventKind = iota
	proxyEventFailure
)

type proxyEvent struct {
	kind        proxyEventKind
	proxyPoolID string
	errorMsg    string
	timestamp   time.Time
}

var (
	proxyEventQueue     chan proxyEvent
	proxyEventQueueOnce sync.Once
)

// InitProxyEventQueue initializes the background non-blocking event worker.
func (r *Repo) InitProxyEventQueue() {
	proxyEventQueueOnce.Do(func() {
		proxyEventQueue = make(chan proxyEvent, 2048)
		go r.runProxyEventQueueWorker()
	})
}

// RecordProxySuccess queues a non-blocking success event for a proxy pool.
func (r *Repo) RecordProxySuccess(proxyPoolID string) {
	if proxyPoolID == "" || proxyPoolID == "__none__" || r == nil || r.db == nil {
		return
	}
	r.InitProxyEventQueue()
	select {
	case proxyEventQueue <- proxyEvent{
		kind:        proxyEventSuccess,
		proxyPoolID: proxyPoolID,
		timestamp:   time.Now().UTC(),
	}:
	default:
		// Channel buffer full; drop to protect request path latency
	}
}

// RecordProxyFailure queues a non-blocking failure event for a proxy pool.
func (r *Repo) RecordProxyFailure(proxyPoolID string, errorMsg string) {
	if proxyPoolID == "" || proxyPoolID == "__none__" || r == nil || r.db == nil {
		return
	}
	r.InitProxyEventQueue()
	select {
	case proxyEventQueue <- proxyEvent{
		kind:        proxyEventFailure,
		proxyPoolID: proxyPoolID,
		errorMsg:    errorMsg,
		timestamp:   time.Now().UTC(),
	}:
	default:
		// Channel buffer full; drop to protect request path latency
	}
}

func (r *Repo) runProxyEventQueueWorker() {
	for ev := range proxyEventQueue {
		r.processProxyEvent(ev)
	}
}

func (r *Repo) processProxyEvent(ev proxyEvent) {
	if r.db == nil {
		return
	}
	now := ev.timestamp.Format(time.RFC3339)

	if ev.kind == proxyEventSuccess {
		_, _ = r.db.Exec(`
			INSERT INTO proxyQuarantine (id, name, proxyUrl, proxyType, failCount, totalFailures, totalSuccess, lastSuccessAt, status, createdAt, updatedAt)
			SELECT id, json_extract(data, '$.name'), json_extract(data, '$.proxyUrl'), json_extract(data, '$.type'), 0, 0, 1, ?, 'healthy', ?, ?
			FROM proxyPools WHERE id = ?
			ON CONFLICT(id) DO UPDATE SET 
				failCount = 0, 
				totalSuccess = proxyQuarantine.totalSuccess + 1, 
				lastSuccessAt = excluded.lastSuccessAt, 
				status = 'healthy', 
				updatedAt = excluded.updatedAt
		`, now, now, now, ev.proxyPoolID)
		return
	}

	if ev.kind == proxyEventFailure {
		errMsg := ev.errorMsg
		if len(errMsg) > 500 {
			errMsg = errMsg[:500]
		}
		_, _ = r.db.Exec(`
			INSERT INTO proxyQuarantine (id, name, proxyUrl, proxyType, failCount, totalFailures, totalSuccess, lastError, lastFailedAt, status, createdAt, updatedAt)
			SELECT id, json_extract(data, '$.name'), json_extract(data, '$.proxyUrl'), json_extract(data, '$.type'), 1, 1, 0, ?, ?, 'healthy', ?, ?
			FROM proxyPools WHERE id = ?
			ON CONFLICT(id) DO UPDATE SET 
				failCount = proxyQuarantine.failCount + 1, 
				totalFailures = proxyQuarantine.totalFailures + 1, 
				lastError = excluded.lastError, 
				lastFailedAt = excluded.lastFailedAt, 
				status = CASE WHEN proxyQuarantine.failCount + 1 >= 3 THEN 'quarantined' ELSE proxyQuarantine.status END, 
				updatedAt = excluded.updatedAt
		`, errMsg, now, now, now, ev.proxyPoolID)
	}
}
