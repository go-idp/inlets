package protocol

// ProtocolAdapter interface for protocol adaptation
// Used to unify message transmission handling across different protocol versions
type ProtocolAdapter interface {
	// SendHTTPRequest sends an HTTP request
	SendHTTPRequest(id string, data []byte) error

	// SendHTTPResponse sends an HTTP response
	SendHTTPResponse(id string, data []byte) error

	// SendTCPData sends TCP data
	SendTCPData(streamId string, data []byte) error

	// OnHTTPRequest registers an HTTP request handler
	OnHTTPRequest(handler func(id string, data []byte) error)

	// OnHTTPResponse registers an HTTP response handler
	OnHTTPResponse(handler func(id string, data []byte) error)

	// OnTCPData registers a TCP data handler
	// Returns an unsubscribe function
	OnTCPData(handler func(streamId string, data []byte) error) func()

	// Destroy cleans up resources
	Destroy()
}
