package container

import (
	"net"
	"strings"
	"sync"

	"github.com/go-zoox/inlets/internal/server/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// domainContainer implements DomainMappingContainer
type domainContainer struct {
	mu    sync.RWMutex
	hosts map[string]*types.DomainMapping
}

// DomainContainer is an alias for domainContainer to allow external access
type DomainContainer = domainContainer

// NewDomainContainer creates a new domain mapping container
func NewDomainContainer() types.DomainMappingContainer {
	return &domainContainer{
		hosts: make(map[string]*types.DomainMapping),
	}
}

// Get returns a domain mapping by id
func (c *domainContainer) Get(id string) *types.DomainMapping {
	c.mu.RLock()
	defer c.mu.RUnlock()

	id = strings.ToLower(id)
	return c.hosts[id]
}

// GetAll returns all domain mappings
func (c *domainContainer) GetAll() map[string]*types.DomainMapping {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*types.DomainMapping)
	for k, v := range c.hosts {
		result[k] = v
	}
	return result
}

// Has checks if a domain mapping exists
func (c *domainContainer) Has(id string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	id = strings.ToLower(id)
	_, exists := c.hosts[id]
	return exists
}

// BindWS binds a WebSocket connection to a subdomain
// If subDomain is empty, generates a short UUID
func (c *domainContainer) BindWS(wsSocket *websocket.Conn, subDomain string) string {
	return c.BindWSWithMetadata(wsSocket, subDomain, "", nil, false)
}

// BindWSWithMetadata binds a WebSocket connection to a subdomain with metadata
func (c *domainContainer) BindWSWithMetadata(wsSocket *websocket.Conn, subDomain string, clientID string, adapter interface{}, useNewProtocol bool) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var uid string
	if subDomain == "" {
		// Generate short UUID (similar to TypeScript shorturl)
		uid = shortUUID()
	} else {
		uid = subDomain
	}
	uid = strings.ToLower(uid)

	c.hosts[uid] = &types.DomainMapping{
		WSSocket:       wsSocket,
		ClientID:       clientID,
		Adapter:        adapter,
		UseNewProtocol: useNewProtocol,
	}

	return uid
}

// UnbindWS removes a domain mapping
func (c *domainContainer) UnbindWS(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id = strings.ToLower(id)
	delete(c.hosts, id)
}

// BindTCP binds a TCP socket to a domain mapping
func (c *domainContainer) BindTCP(id string, tcpSocket *net.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()

	id = strings.ToLower(id)
	if mapping, exists := c.hosts[id]; exists {
		mapping.TCPSocket = tcpSocket
	}
}

// shortUUID generates a short UUID (first 8 characters)
func shortUUID() string {
	return uuid.New().String()[:8]
}
