package container

import (
	"sync"

	"github.com/go-idp/inlets/internal/server/types"
)

// callbackContainer implements CallbackContainer
type callbackContainer struct {
	mu        sync.RWMutex
	callbacks map[string]map[string]types.CallbackFunc
}

// NewCallbackContainer creates a new callback container
func NewCallbackContainer() types.CallbackContainer {
	return &callbackContainer{
		callbacks: make(map[string]map[string]types.CallbackFunc),
	}
}

// Get retrieves a callback function by tcpId and requestId
func (c *callbackContainer) Get(tcpId string, requestId string) types.CallbackFunc {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if tcpCallbacks, exists := c.callbacks[tcpId]; exists {
		if callback, exists := tcpCallbacks[requestId]; exists {
			return callback
		}
	}
	return nil
}

// Set stores a callback function by tcpId and requestId
func (c *callbackContainer) Set(tcpId string, requestId string, callback types.CallbackFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.callbacks[tcpId] == nil {
		c.callbacks[tcpId] = make(map[string]types.CallbackFunc)
	}
	c.callbacks[tcpId][requestId] = callback
}

// Remove removes all callbacks for a given tcpId
func (c *callbackContainer) Remove(tcpId string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.callbacks, tcpId)
}
