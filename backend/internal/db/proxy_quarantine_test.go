package db

import (
	"testing"
	"time"
)

func TestProxyQuarantine_EventFlow(t *testing.T) {
	db, err := OpenDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	repo := NewRepo(db)

	// Insert test proxy
	poolData := `{"name":"test-proxy-1","proxyUrl":"https://test-relay.vercel.app","type":"vercel"}`
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO proxyPools (id, isActive, testStatus, data, createdAt, updatedAt) VALUES (?, 1, 'active', ?, ?, ?)`,
		"test-p1", poolData, now, now); err != nil {
		t.Fatalf("failed to insert test proxy: %v", err)
	}

	// 1. Process 1st failure
	repo.processProxyEvent(proxyEvent{kind: proxyEventFailure, proxyPoolID: "test-p1", errorMsg: "timeout 1", timestamp: time.Now().UTC()})

	var failCount int
	var status string
	err = db.QueryRow(`SELECT failCount, status FROM proxyQuarantine WHERE id = ?`, "test-p1").Scan(&failCount, &status)
	if err != nil {
		t.Fatalf("query quarantine failed: %v", err)
	}
	if failCount != 1 || status != "healthy" {
		t.Errorf("expected failCount=1, status=healthy, got failCount=%d, status=%s", failCount, status)
	}

	// 2. Process 2nd failure
	repo.processProxyEvent(proxyEvent{kind: proxyEventFailure, proxyPoolID: "test-p1", errorMsg: "timeout 2", timestamp: time.Now().UTC()})
	_ = db.QueryRow(`SELECT failCount, status FROM proxyQuarantine WHERE id = ?`, "test-p1").Scan(&failCount, &status)
	if failCount != 2 || status != "healthy" {
		t.Errorf("expected failCount=2, status=healthy, got failCount=%d, status=%s", failCount, status)
	}

	// 3. Process 3rd failure -> Should trigger 'quarantined'
	repo.processProxyEvent(proxyEvent{kind: proxyEventFailure, proxyPoolID: "test-p1", errorMsg: "timeout 3", timestamp: time.Now().UTC()})
	_ = db.QueryRow(`SELECT failCount, status FROM proxyQuarantine WHERE id = ?`, "test-p1").Scan(&failCount, &status)
	if failCount != 3 || status != "quarantined" {
		t.Errorf("expected failCount=3, status=quarantined, got failCount=%d, status=%s", failCount, status)
	}

	// 4. Process success -> Should reset failCount to 0 and status to 'healthy'
	repo.processProxyEvent(proxyEvent{kind: proxyEventSuccess, proxyPoolID: "test-p1", timestamp: time.Now().UTC()})
	_ = db.QueryRow(`SELECT failCount, status FROM proxyQuarantine WHERE id = ?`, "test-p1").Scan(&failCount, &status)
	if failCount != 0 || status != "healthy" {
		t.Errorf("expected failCount=0, status=healthy after success, got failCount=%d, status=%s", failCount, status)
	}
}
