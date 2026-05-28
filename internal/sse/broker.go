// Package sse implements a server-sent event broker for fan-out to HTTP clients.
package sse

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Broker fans out server-sent events to subscribed HTTP clients per topic (node ID).
type Broker struct {
	mu     sync.RWMutex
	topics map[int64]map[chan []byte]struct{}
}

// NewBroker creates a new Broker.
func NewBroker() *Broker {
	return &Broker{topics: make(map[int64]map[chan []byte]struct{})}
}

// Subscribe registers a receive channel for the given topicID.
// Call the returned cancel function to unsubscribe and close the channel.
func (b *Broker) Subscribe(topicID int64) (<-chan []byte, func()) {
	ch := make(chan []byte, 16)
	b.mu.Lock()
	if b.topics[topicID] == nil {
		b.topics[topicID] = make(map[chan []byte]struct{})
	}
	b.topics[topicID][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.topics[topicID], ch)
		if len(b.topics[topicID]) == 0 {
			delete(b.topics, topicID)
		}
		b.mu.Unlock()
		close(ch)
	}
}

// HasSubscribers reports whether topicID has at least one active subscriber.
func (b *Broker) HasSubscribers(topicID int64) bool {
	b.mu.RLock()
	n := len(b.topics[topicID])
	b.mu.RUnlock()
	return n > 0
}

// Publish sends data to all current subscribers of topicID (non-blocking; slow clients are dropped).
func (b *Broker) Publish(topicID int64, data []byte) {
	b.mu.RLock()
	subs := b.topics[topicID]
	b.mu.RUnlock()
	for ch := range subs {
		select {
		case ch <- data:
		default:
		}
	}
}

// StreamFrom writes SSE events from an already-subscribed channel to w until
// r.Context() is cancelled. Any non-nil initial payloads are sent first.
// Use this when you need to subscribe before triggering the first data fetch.
func (b *Broker) StreamFrom(w http.ResponseWriter, r *http.Request, ch <-chan []byte, initial ...[]byte) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	for _, msg := range initial {
		if len(msg) > 0 {
			fmt.Fprintf(w, "data: %s\n\n", msg)
		}
	}
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// Stream writes SSE events from topicID to w until r.Context() is cancelled.
// Any non-nil initial payloads are sent immediately before waiting for published events.
func (b *Broker) Stream(w http.ResponseWriter, r *http.Request, topicID int64, initial ...[]byte) {
	ch, cancel := b.Subscribe(topicID)
	defer cancel()
	b.StreamFrom(w, r, ch, initial...)
}
