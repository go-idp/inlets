package protocol

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"

	"github.com/gorilla/websocket"
)

// LegacyProtocolAdapter implements the legacy protocol (JSON + Base64)
// Maintains compatibility with old clients
type LegacyProtocolAdapter struct {
	conn                *websocket.Conn
	connWriteMu         *sync.Mutex // monitor conn write serialization (server)
	isClient            bool
	httpRequestHandler  func(id string, data []byte) error
	httpResponseHandler func(id string, data []byte) error
	tcpDataHandlers     map[int]func(streamId string, data []byte) error
	handlerMu           sync.RWMutex
	handlerIDCounter    int
}

// NewLegacyProtocolAdapter creates a new legacy protocol adapter
func NewLegacyProtocolAdapter(conn *websocket.Conn, isClient bool) *LegacyProtocolAdapter {
	adapter := &LegacyProtocolAdapter{
		conn:            conn,
		isClient:        isClient,
		tcpDataHandlers: make(map[int]func(streamId string, data []byte) error),
	}

	// Don't start event listeners here - let WebSocketMonitor handle message reading
	// adapter.setupEventListeners()
	return adapter
}

// SetConnWriteMu sets the mutex used to serialize writes on the monitor connection.
func (a *LegacyProtocolAdapter) SetConnWriteMu(mu *sync.Mutex) {
	a.connWriteMu = mu
}

func (a *LegacyProtocolAdapter) writeMonitorText(msg []byte) error {
	if a.connWriteMu != nil {
		a.connWriteMu.Lock()
		defer a.connWriteMu.Unlock()
	}
	return a.conn.WriteMessage(websocket.TextMessage, msg)
}

// setupEventListeners sets up WebSocket event listeners
func (a *LegacyProtocolAdapter) setupEventListeners() {
	// Start a goroutine to read messages
	go func() {
		for {
			messageType, message, err := a.conn.ReadMessage()
			if err != nil {
				// Connection closed or error
				return
			}

			if messageType == websocket.TextMessage {
				// Parse JSON message: ["event", payload]
				var msgArray []interface{}
				if err := json.Unmarshal(message, &msgArray); err != nil {
					continue
				}

				if len(msgArray) < 1 {
					continue
				}

				event, ok := msgArray[0].(string)
				if !ok {
					continue
				}

				var payload interface{}
				if len(msgArray) > 1 {
					payload = msgArray[1]
				}

				a.handleEvent(event, payload)
			}
		}
	}()
}

// HandleEvent handles incoming events (public method for external callers)
func (a *LegacyProtocolAdapter) HandleEvent(event string, payload interface{}) error {
	return a.handleEvent(event, payload)
}

// handleEvent handles incoming events
func (a *LegacyProtocolAdapter) handleEvent(event string, payload interface{}) error {
	switch event {
	case "request":
		if a.isClient && a.httpRequestHandler != nil {
			// Client: handle HTTP request from server
			if data, ok := payload.(map[string]interface{}); ok {
				id, _ := data["id"].(string)
				dataStr, _ := data["data"].(string)
				if id != "" && dataStr != "" {
					// Decode: decompress -> base64 decode
					decoded, err := a.decodeRequestData(dataStr)
					if err == nil {
						a.httpRequestHandler(id, decoded)
					}
				}
			}
		}
	case "response":
		if !a.isClient && a.httpResponseHandler != nil {
			// Server: handle HTTP response from client
			if data, ok := payload.(map[string]interface{}); ok {
				id, _ := data["id"].(string)
				dataStr, _ := data["data"].(string)
				if id != "" && dataStr != "" {
					// Decode: decompress -> base64 decode
					decoded, err := a.decodeResponseData(dataStr)
					if err == nil {
						a.httpResponseHandler(id, decoded)
					}
				}
			}
		}
	}
	return nil
}

// SendHTTPRequest sends an HTTP request
// Legacy protocol: Base64 encode -> compress -> JSON
func (a *LegacyProtocolAdapter) SendHTTPRequest(id string, data []byte) error {
	// Encode: base64 encode -> compress
	encoded, err := a.encodeRequestData(data)
	if err != nil {
		return err
	}

	// Send as JSON: ["request", {id, data}]
	msg := []interface{}{
		"request",
		map[string]interface{}{
			"id":   id,
			"data": encoded,
		},
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return a.writeMonitorText(msgBytes)
}

// SendHTTPResponse sends an HTTP response
// Legacy protocol: Base64 encode -> compress -> JSON
func (a *LegacyProtocolAdapter) SendHTTPResponse(id string, data []byte) error {
	// Encode: base64 encode -> compress
	encoded, err := a.encodeResponseData(data)
	if err != nil {
		return err
	}

	// Send as JSON: ["response", {id, data}]
	msg := []interface{}{
		"response",
		map[string]interface{}{
			"id":   id,
			"data": encoded,
		},
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return a.writeMonitorText(msgBytes)
}

// SendTCPData sends TCP data
// Legacy protocol doesn't support TCP over WebSocket
func (a *LegacyProtocolAdapter) SendTCPData(streamId string, data []byte) error {
	return errors.New("TCP data transmission not supported in legacy protocol")
}

// OnHTTPRequest registers an HTTP request handler
func (a *LegacyProtocolAdapter) OnHTTPRequest(handler func(id string, data []byte) error) {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()
	a.httpRequestHandler = handler
}

// OnHTTPResponse registers an HTTP response handler
func (a *LegacyProtocolAdapter) OnHTTPResponse(handler func(id string, data []byte) error) {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()
	a.httpResponseHandler = handler
}

// OnTCPData registers a TCP data handler
func (a *LegacyProtocolAdapter) OnTCPData(handler func(streamId string, data []byte) error) func() {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()

	handlerID := a.handlerIDCounter
	a.handlerIDCounter++
	a.tcpDataHandlers[handlerID] = handler

	// Return unsubscribe function
	return func() {
		a.handlerMu.Lock()
		defer a.handlerMu.Unlock()
		delete(a.tcpDataHandlers, handlerID)
	}
}

// Destroy cleans up resources
func (a *LegacyProtocolAdapter) Destroy() {
	// Close connection if needed
	// Note: We don't close the connection here as it might be managed elsewhere
}

// encodeRequestData encodes data for sending request (server side)
// Process: base64 encode -> compress
func (a *LegacyProtocolAdapter) encodeRequestData(data []byte) (string, error) {
	// Base64 encode
	base64Data := base64.StdEncoding.EncodeToString(data)

	// Compress (using gzip for legacy protocol)
	compressed, err := compressGzip(base64Data)
	if err != nil {
		return "", err
	}

	return compressed, nil
}

// decodeRequestData decodes data received as request (client side)
// Process: decompress -> base64 decode
func (a *LegacyProtocolAdapter) decodeRequestData(data string) ([]byte, error) {
	// Decompress
	decompressed, err := decompressGzip(data)
	if err != nil {
		return nil, err
	}

	// Base64 decode
	decoded, err := base64.StdEncoding.DecodeString(decompressed)
	if err != nil {
		return nil, err
	}

	return decoded, nil
}

// encodeResponseData encodes data for sending response (client side)
// Process: base64 encode -> compress
func (a *LegacyProtocolAdapter) encodeResponseData(data []byte) (string, error) {
	// Base64 encode
	base64Data := base64.StdEncoding.EncodeToString(data)

	// Compress (using gzip for legacy protocol)
	compressed, err := compressGzip(base64Data)
	if err != nil {
		return "", err
	}

	return compressed, nil
}

// decodeResponseData decodes data received as response (server side)
// Process: decompress -> base64 decode
func (a *LegacyProtocolAdapter) decodeResponseData(data string) ([]byte, error) {
	// Decompress
	decompressed, err := decompressGzip(data)
	if err != nil {
		return nil, err
	}

	// Base64 decode
	decoded, err := base64.StdEncoding.DecodeString(decompressed)
	if err != nil {
		return nil, err
	}

	return decoded, nil
}

