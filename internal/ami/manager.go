package ami

import (
	"context"
	"fmt"
	"sync"
	"time"

	"allstar-yaamon/internal/db"
)

// NodeEvent pairs an AMI event with the ID of the node it came from.
type NodeEvent struct {
	NodeID int64
	Event  Event
}

// Manager maintains one AMI Client per enabled node.
type Manager struct {
	mu          sync.RWMutex
	clients     map[int64]*Client
	ctx         context.Context
	cancel      context.CancelFunc
	subscribers []chan NodeEvent
	subMu       sync.RWMutex
}

// NewManager creates a new Manager.
func NewManager() *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		clients: make(map[int64]*Client),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// LoadNodes starts AMI clients for all enabled nodes.
func (m *Manager) LoadNodes(nodes []db.Node) {
	for _, n := range nodes {
		if n.Enabled {
			m.Add(n)
		}
	}
}

// Add starts (or restarts) an AMI client for the given node.
// If the node is disabled, any existing client is stopped and removed.
func (m *Manager) Add(n db.Node) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.clients[n.ID]; ok {
		existing.Stop()
		delete(m.clients, n.ID)
	}

	if !n.Enabled {
		return
	}

	host := n.AMIHost
	if host == "" {
		host = "localhost"
	}
	port := n.AMIPort
	if port == 0 {
		port = 5038
	}

	c := NewClient(n.ID, n.Name, n.NodeNumber, host, port, n.AMIUser, n.AMIPass)
	c.Start(m.ctx)
	m.clients[n.ID] = c

	// Drain this client's events and fan them out to all subscribers.
	nodeID := n.ID
	go func() {
		for evt := range c.Events() {
			m.publish(nodeID, evt)
		}
	}()

}

// Remove stops and removes the client for nodeID.
func (m *Manager) Remove(nodeID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.clients[nodeID]; ok {
		c.Stop()
		delete(m.clients, nodeID)
	}
}

// IsConnected reports whether the specified node's AMI client is authenticated.
func (m *Manager) IsConnected(nodeID int64) bool {
	m.mu.RLock()
	c, ok := m.clients[nodeID]
	m.mu.RUnlock()
	return ok && c.IsConnected()
}

// SendAction sends an AMI action to the specified node without waiting for a response.
func (m *Manager) SendAction(nodeID int64, headers map[string]string) error {
	m.mu.RLock()
	c, ok := m.clients[nodeID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no AMI client for node %d", nodeID)
	}
	return c.SendAction(headers)
}

// SendActionWait sends an AMI action and waits up to timeout for the response.
func (m *Manager) SendActionWait(nodeID int64, headers map[string]string, timeout time.Duration) (Event, error) {
	m.mu.RLock()
	c, ok := m.clients[nodeID]
	m.mu.RUnlock()
	if !ok {
		return Event{}, fmt.Errorf("no AMI client for node %d", nodeID)
	}
	return c.SendActionWait(headers, timeout)
}

// Subscribe returns a channel that receives events from all managed nodes.
// The caller must read from the channel promptly; slow consumers cause events to be dropped.
func (m *Manager) Subscribe() <-chan NodeEvent {
	ch := make(chan NodeEvent, 256)
	m.subMu.Lock()
	m.subscribers = append(m.subscribers, ch)
	m.subMu.Unlock()
	return ch
}

// publish fans out an event to all subscribers (non-blocking, drops on slow consumers).
func (m *Manager) publish(nodeID int64, evt Event) {
	m.subMu.RLock()
	subs := m.subscribers
	m.subMu.RUnlock()
	ne := NodeEvent{NodeID: nodeID, Event: evt}
	for _, ch := range subs {
		select {
		case ch <- ne:
		default:
		}
	}
}

// Shutdown stops all managed clients.
func (m *Manager) Shutdown() {
	m.cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.clients {
		c.Stop()
		delete(m.clients, id)
	}
}
