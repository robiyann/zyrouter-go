package clientstream

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Event is the sanitized request lifecycle event exposed to one client user.
// It deliberately contains no provider, connection, prompt, response, or key data.
type Event struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	Timestamp        string `json:"timestamp"`
	RequestID        string `json:"requestId,omitempty"`
	Model            string `json:"model"`
	Status           string `json:"status"`
	HTTPStatus       int    `json:"httpStatus,omitempty"`
	DurationMs       int64  `json:"durationMs,omitempty"`
	PromptTokens     int    `json:"promptTokens,omitempty"`
	CompletionTokens int    `json:"completionTokens,omitempty"`
	ErrorCode        string `json:"errorCode,omitempty"`
}

type Tracker struct {
	mu          sync.Mutex
	ring        map[string][]Event
	subscribers map[string]map[chan []byte]struct{}
}

var global = &Tracker{
	ring:        make(map[string][]Event),
	subscribers: make(map[string]map[chan []byte]struct{}),
}

func Get() *Tracker { return global }

func (t *Tracker) Publish(userID string, event Event) {
	if userID == "" || event.Model == "" {
		return
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if event.ID == "" {
		event.ID = fmt.Sprintf("client-%d", time.Now().UnixNano())
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	frame := append([]byte(nil), payload...)

	t.mu.Lock()
	items := append(t.ring[userID], event)
	if len(items) > 100 {
		items = items[len(items)-100:]
	}
	t.ring[userID] = items
	for ch := range t.subscribers[userID] {
		select {
		case ch <- frame:
		default:
			// A slow browser must not block request processing.
		}
	}
	t.mu.Unlock()
}

func (t *Tracker) Snapshot(userID string) []Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	items := append([]Event(nil), t.ring[userID]...)
	return items
}

func (t *Tracker) Subscribe(userID string) (chan []byte, func()) {
	ch := make(chan []byte, 32)
	t.mu.Lock()
	if t.subscribers[userID] == nil {
		t.subscribers[userID] = make(map[chan []byte]struct{})
	}
	t.subscribers[userID][ch] = struct{}{}
	t.mu.Unlock()

	return ch, func() {
		t.mu.Lock()
		if set := t.subscribers[userID]; set != nil {
			delete(set, ch)
			if len(set) == 0 {
				delete(t.subscribers, userID)
			}
		}
		close(ch)
		t.mu.Unlock()
	}
}
