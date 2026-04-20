package container

import (
	"errors"
	"net"
	"sync"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// tunnelContainer implements TunnelContainer
type tunnelContainer struct {
	mu         sync.RWMutex
	containers map[string]*types.TunnelMapping
}

// NewTunnelContainer creates a new tunnel container
func NewTunnelContainer() types.TunnelContainer {
	return &tunnelContainer{
		containers: make(map[string]*types.TunnelMapping),
	}
}

// Create creates a new tunnel mapping
func (c *tunnelContainer) Create(id string, token types.GetToken, wsSocket *websocket.Conn, auth *client.Authentication, writeMu *sync.Mutex) {
	c.mu.Lock()
	defer c.mu.Unlock()

	destroy := func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		mapping := c.containers[id]
		if mapping == nil {
			return
		}

		// Close source server if exists
		if mapping.SourceServer != nil {
			mapping.SourceServer.Close()
		}

		// Remove from containers
		delete(c.containers, id)
	}

	tunnelPort := auth.TunnelPort
	// Don't set SourcePort here - it will be set when CreateServer is called
	// SourcePort should only be set when the TCP server is actually created,
	// not when the container is created, to avoid port conflicts

	mapping := &types.TunnelMapping{
		WSSocket:        wsSocket,
		WriteMu:         writeMu, // Use the provided mutex from WebSocketConnection
		DataSockets:     make(map[string]*websocket.Conn),
		DataWriteMu:     make(map[string]*sync.Mutex),
		DataMu:          &sync.RWMutex{},
		Token:           token,
		Type:            types.TunnelType(auth.Type),
		Port:            auth.Port,
		SourcePort:      nil, // Will be set when TCP server is created
		SourceServer:    nil,
		Version:         auth.Version,
		AuthType:        types.AuthType(auth.AuthType),
		ClientId:        auth.ClientId,
		Signature:       auth.Signature,
		ClientTimestamp: auth.Timestamp,
		TunnelPort:      &tunnelPort, // Store the requested tunnel port
		Requests:        make(map[string]*net.Conn),
		Destroy:         destroy,
	}

	c.containers[id] = mapping
}

// Get retrieves a tunnel mapping by id
func (c *tunnelContainer) Get(id string) *types.TunnelMapping {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.containers[id]
}

// Set sets a field value in a tunnel mapping
func (c *tunnelContainer) Set(id string, key string, value interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	mapping := c.containers[id]
	if mapping == nil {
		return errors.New("invalid id for container")
	}

	switch key {
	case "sourcePort":
		if port, ok := value.(int); ok {
			mapping.SourcePort = &port
		} else if port, ok := value.(*int); ok {
			mapping.SourcePort = port
		}
	case "sourceServer":
		if value == nil {
			mapping.SourceServer = nil
		} else if srv, ok := value.(net.Listener); ok {
			mapping.SourceServer = srv
		}
	case "targetPort":
		if port, ok := value.(int); ok {
			mapping.TargetPort = &port
		} else if port, ok := value.(*int); ok {
			mapping.TargetPort = port
		}
	case "adapter":
		mapping.Adapter = value
	case "useNewProtocol":
		if useNew, ok := value.(bool); ok {
			mapping.UseNewProtocol = useNew
		}
	default:
		return errors.New("unknown key: " + key)
	}

	return nil
}

// Remove removes a tunnel mapping
func (c *tunnelContainer) Remove(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.containers, id)
}

// RegisterRequest registers a request socket for a tunnel
func (c *tunnelContainer) RegisterRequest(containerId string, requestId string, sourceSocket *net.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()

	mapping := c.containers[containerId]
	if mapping == nil {
		return
	}

	if mapping.Requests == nil {
		mapping.Requests = make(map[string]*net.Conn)
	}

	mapping.Requests[requestId] = sourceSocket
}

// ConnectRequest connects a target socket to a source socket
func (c *tunnelContainer) ConnectRequest(containerId string, requestId string, targetSocket *net.Conn) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	mapping := c.containers[containerId]
	if mapping == nil {
		return errors.New("container not found")
	}

	sourceSocket := mapping.Requests[requestId]
	if sourceSocket == nil {
		return errors.New("source socket not found")
	}

	// Remove from requests map
	delete(mapping.Requests, requestId)

	// Create bidirectional pipe between source and target sockets
	go pipeConnections(*sourceSocket, *targetSocket)

	return nil
}

// pipeConnections pipes data bidirectionally between two connections
func pipeConnections(src, dst net.Conn) {
	// Pipe src -> dst
	go func() {
		defer dst.Close()
		buf := make([]byte, 32*1024)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Pipe dst -> src
	defer src.Close()
	buf := make([]byte, 32*1024)
	for {
		n, err := dst.Read(buf)
		if n > 0 {
			if _, writeErr := src.Write(buf[:n]); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// generateContainerID generates a new container ID
func generateContainerID() string {
	return uuid.New().String()
}
