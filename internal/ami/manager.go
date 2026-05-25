package ami

import (
	"context"
	"fmt"
	"sync"

	"allstar-yaamon/internal/db"
)

// Manager maintains one AMI Client per enabled node.
type Manager struct {
	mu      sync.RWMutex
	clients map[int64]*Client
	ctx     context.Context
	cancel  context.CancelFunc
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

	c := NewClient(n.ID, host, port, n.AMIUser, n.AMIPass)
	c.Start(m.ctx)
	m.clients[n.ID] = c
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

// SendAction sends an AMI action to the specified node.
func (m *Manager) SendAction(nodeID int64, headers map[string]string) error {
	m.mu.RLock()
	c, ok := m.clients[nodeID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no AMI client for node %d", nodeID)
	}
	return c.SendAction(headers)
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
