package protocol

import (
	"errors"
	"sync"
)

// ProtocolAdapter interface for protocol adaptation
// Used to unify message transmission handling across different protocol versions
type ProtocolAdapter interface {
	// SendHTTPRequest sends an HTTP request
	SendHTTPRequest(id string, data []byte) error

	// SendHTTPResponse sends an HTTP response
	SendHTTPResponse(id string, data []byte) error

	// SendHTTPRequestHead sends HTTP request headers only (semantic body streaming).
	SendHTTPRequestHead(id string, head []byte, fin bool) error
	// SendHTTPRequestBody sends one HTTP request body chunk (last chunk sets fin).
	SendHTTPRequestBody(id string, chunk []byte, fin bool) error
	// SendHTTPResponseHead sends HTTP response headers only.
	SendHTTPResponseHead(id string, head []byte, fin bool) error
	// SendHTTPResponseBody sends one HTTP response body chunk.
	SendHTTPResponseBody(id string, chunk []byte, fin bool) error

	// SendTCPData sends TCP data
	SendTCPData(streamId string, data []byte) error

	// OnHTTPRequest registers an HTTP request handler
	OnHTTPRequest(handler func(id string, data []byte) error)

	// OnHTTPResponse registers an HTTP response handler
	OnHTTPResponse(handler func(id string, data []byte) error)

	OnHTTPRequestHead(handler func(id string, head []byte, fin bool) error)
	OnHTTPRequestBody(handler func(id string, chunk []byte, fin bool) error)
	OnHTTPResponseHead(handler func(id string, head []byte, fin bool) error)
	OnHTTPResponseBody(handler func(id string, chunk []byte, fin bool) error)

	// OnTCPData registers a TCP data handler
	// Returns an unsubscribe function
	OnTCPData(handler func(streamId string, data []byte) error) func()

	// OnTCPClose registers a handler for client-side TCP stream teardown (e.g. upstream dial failed).
	// Legacy adapters return a no-op unsubscribe.
	OnTCPClose(handler func(streamId string) error) func()

	// Destroy cleans up resources
	Destroy()

	// SetConnWriteMu serializes WriteMessage on the monitor WebSocket when non-nil (server-side; required by gorilla/websocket).
	SetConnWriteMu(mu *sync.Mutex)

	// NegotiatedFlags returns the bitmask from capability negotiation (0 for legacy).
	NegotiatedFlags() int
}

// ErrSemanticHTTPNotSupported is returned by legacy adapters for semantic streaming sends.
var ErrSemanticHTTPNotSupported = errors.New("semantic HTTP streaming not supported")
