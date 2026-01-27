package protocol

import (
	"github.com/go-idp/inlets/internal/client"
	"github.com/gorilla/websocket"
)

// ProtocolAdapterFactory creates protocol adapters based on capability negotiation
type ProtocolAdapterFactory struct{}

// Create creates a protocol adapter based on capabilities
// If capabilities is nil or has no flags, returns a LegacyProtocolAdapter
// Otherwise, returns a BinaryProtocolAdapter
func (f *ProtocolAdapterFactory) Create(conn *websocket.Conn, capabilities *client.Capabilities, isClient bool) ProtocolAdapter {
	if capabilities == nil || capabilities.Flags == 0 {
		// Legacy protocol: no capabilities or flags are 0
		return NewLegacyProtocolAdapter(conn, isClient)
	}

	// Binary protocol: capabilities are present
	return NewBinaryProtocolAdapter(conn, capabilities, isClient)
}

// NewProtocolAdapterFactory creates a new protocol adapter factory
func NewProtocolAdapterFactory() *ProtocolAdapterFactory {
	return &ProtocolAdapterFactory{}
}

// Create is a convenience function to create a protocol adapter
func Create(conn *websocket.Conn, capabilities *client.Capabilities, isClient bool) ProtocolAdapter {
	factory := NewProtocolAdapterFactory()
	return factory.Create(conn, capabilities, isClient)
}
