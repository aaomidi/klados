//go:build !release

package cluster

import (
	"context"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

// NewTestConnection builds a Connection with a live connCtx for unit tests.
// disc may be nil when the test doesn't exercise discovery.
func NewTestConnection(dyn dynamic.Interface, disc discovery.DiscoveryInterface) *Connection {
	ctx, cancel := context.WithCancel(context.Background())
	return &Connection{Dynamic: dyn, Discovery: disc, connCtx: ctx, cancel: cancel}
}

// CloseForTest cancels the test connection's context (simulates Disconnect).
func (c *Connection) CloseForTest() {
	if c.cancel != nil {
		c.cancel()
	}
}

// SetConnectionForTest directly injects a Connection into the Manager for unit testing.
func (m *Manager) SetConnectionForTest(contextName string, conn *Connection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections[contextName] = conn
}

// SetDiscoveredResourcesForTest pre-populates the discoveredResources map for unit testing.
func (m *Manager) SetDiscoveredResourcesForTest(contextName string, resources []APIResource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.discoveredResources == nil {
		m.discoveredResources = map[string][]APIResource{}
	}
	m.discoveredResources[contextName] = resources
}
