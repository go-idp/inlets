package protocol

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-zoox/logger"
	"github.com/gorilla/websocket"
)

// MessageType represents the type of binary message
type MessageType uint8

const (
	MessageTypeControl          MessageType = 0x00 // Control message (auth, heartbeat, etc.)
	MessageTypeHTTPRequest      MessageType = 0x01 // HTTP request
	MessageTypeHTTPResponse     MessageType = 0x02 // HTTP response
	MessageTypeTCPData          MessageType = 0x03 // TCP data
	MessageTypeTCPOpen          MessageType = 0x04 // TCP connection open
	MessageTypeTCPClose         MessageType = 0x05 // TCP connection close
	MessageTypeFlowControl      MessageType = 0x06 // Flow control message
	MessageTypeHTTPRequestHead  MessageType = 0x07 // HTTP request headers only (semantic streaming)
	MessageTypeHTTPRequestBody  MessageType = 0x08 // HTTP request body chunk
	MessageTypeHTTPResponseHead MessageType = 0x09 // HTTP response headers only
	MessageTypeHTTPResponseBody MessageType = 0x0a // HTTP response body chunk
)

// MessageFlags represents message flags
type MessageFlags uint8

const (
	MessageFlagFIN          MessageFlags = 0x01 // Stream end
	MessageFlagACK          MessageFlags = 0x02 // Acknowledgment
	MessageFlagBackpressure MessageFlags = 0x04 // Backpressure
)

// BinaryMessage represents a binary message
// Format: [MessageType(1 byte)] + [StreamIDLength(1 byte)] + [StreamID(variable)] + [Sequence(4 bytes)] + [Flags(1 byte)] + [DataLength(4 bytes)] + [Data(variable)]
type BinaryMessage struct {
	Type     MessageType
	StreamID string
	Sequence uint32
	Flags    MessageFlags
	Data     []byte
}

// BuildBinaryMessage builds a binary message from the struct
func BuildBinaryMessage(msg BinaryMessage) ([]byte, error) {
	streamIDBytes := []byte(msg.StreamID)
	streamIDLength := len(streamIDBytes)

	if streamIDLength > 255 {
		return nil, errors.New("stream ID too long (max 255 bytes)")
	}

	headerSize := 1 + 1 + streamIDLength + 4 + 1 + 4 // type + streamIDLen + streamID + sequence + flags + dataLen
	totalSize := headerSize + len(msg.Data)

	buffer := make([]byte, totalSize)
	offset := 0

	// Message type (1 byte)
	buffer[offset] = byte(msg.Type)
	offset++

	// Stream ID length (1 byte)
	buffer[offset] = byte(streamIDLength)
	offset++

	// Stream ID (variable)
	copy(buffer[offset:], streamIDBytes)
	offset += streamIDLength

	// Sequence (4 bytes, big-endian)
	binary.BigEndian.PutUint32(buffer[offset:], msg.Sequence)
	offset += 4

	// Flags (1 byte)
	buffer[offset] = byte(msg.Flags)
	offset++

	// Data length (4 bytes, big-endian)
	binary.BigEndian.PutUint32(buffer[offset:], uint32(len(msg.Data)))
	offset += 4

	// Data (variable)
	copy(buffer[offset:], msg.Data)

	return buffer, nil
}

// ParseBinaryMessage parses a binary message from bytes
func ParseBinaryMessage(buffer []byte) (*BinaryMessage, error) {
	if len(buffer) < 11 { // Minimum message header length
		return nil, errors.New("message too short (min 11 bytes)")
	}

	offset := 0

	// Message type (1 byte)
	msgType := MessageType(buffer[offset])
	offset++

	// Stream ID length (1 byte)
	streamIDLength := int(buffer[offset])
	offset++

	if len(buffer) < offset+streamIDLength {
		return nil, errors.New("message too short for stream ID")
	}

	// Stream ID (variable)
	streamID := string(buffer[offset : offset+streamIDLength])
	offset += streamIDLength

	// Sequence (4 bytes, big-endian)
	sequence := binary.BigEndian.Uint32(buffer[offset:])
	offset += 4

	// Flags (1 byte)
	flags := MessageFlags(buffer[offset])
	offset++

	// Data length (4 bytes, big-endian)
	dataLength := binary.BigEndian.Uint32(buffer[offset:])
	offset += 4

	if len(buffer) < offset+int(dataLength) {
		return nil, errors.New("message too short for data")
	}

	// Data (variable)
	data := make([]byte, dataLength)
	copy(data, buffer[offset:offset+int(dataLength)])

	return &BinaryMessage{
		Type:     msgType,
		StreamID: streamID,
		Sequence: sequence,
		Flags:    flags,
		Data:     data,
	}, nil
}

// CompressionAlgorithm represents compression algorithm type
type CompressionAlgorithm string

const (
	CompressionBrotli CompressionAlgorithm = "brotli"
	CompressionGzip   CompressionAlgorithm = "gzip"
	CompressionNone   CompressionAlgorithm = "none"
)

// BinaryProtocolAdapter implements the binary protocol adapter
type BinaryProtocolAdapter struct {
	conn                    *websocket.Conn
	connWriteMu             *sync.Mutex             // monitor conn write serialization (server)
	dataConn                *websocket.Conn         // Optional data channel connection (legacy/shared)
	dataWriteMu             *sync.Mutex             // Mutex for shared data channel writes
	dataChannels            map[string]*dataChannel // Per-stream data channel connections
	dataChannelsMu          sync.RWMutex
	isClient                bool
	capabilities            *client.Capabilities
	httpRequestHandler      func(id string, data []byte) error
	httpResponseHandler     func(id string, data []byte) error
	httpRequestHeadHandler  func(id string, head []byte, fin bool) error
	httpRequestBodyHandler  func(id string, chunk []byte, fin bool) error
	httpResponseHeadHandler func(id string, head []byte, fin bool) error
	httpResponseBodyHandler func(id string, chunk []byte, fin bool) error
	tcpDataHandlers         map[int]func(streamId string, data []byte) error
	handlerMu               sync.RWMutex
	handlerIDCounter        int

	sequenceCounter      map[string]uint32
	sequenceCounterMu    sync.Mutex
	compressionAlgorithm CompressionAlgorithm
	streamManager        *StreamManager
	flowController       *FlowController
	chunkSize            int
}

type dataChannel struct {
	conn *websocket.Conn
	mu   *sync.Mutex
}

// NewBinaryProtocolAdapter creates a new binary protocol adapter
func NewBinaryProtocolAdapter(conn *websocket.Conn, capabilities *client.Capabilities, isClient bool) *BinaryProtocolAdapter {
	adapter := &BinaryProtocolAdapter{
		conn:                 conn,
		isClient:             isClient,
		capabilities:         capabilities,
		tcpDataHandlers:      make(map[int]func(streamId string, data []byte) error),
		sequenceCounter:      make(map[string]uint32),
		compressionAlgorithm: CompressionBrotli,
		chunkSize:            64 * 1024, // 64KB
		dataChannels:         make(map[string]*dataChannel),
	}

	// Determine compression algorithm
	if capabilities.Features != nil && capabilities.Features.Compression != nil {
		if capabilities.Features.Compression.Preferred != "" {
			adapter.compressionAlgorithm = CompressionAlgorithm(capabilities.Features.Compression.Preferred)
		} else if len(capabilities.Features.Compression.Algorithms) > 0 {
			adapter.compressionAlgorithm = CompressionAlgorithm(capabilities.Features.Compression.Algorithms[0])
		}
	}

	// Determine chunk size
	if capabilities.Features != nil && capabilities.Features.ChunkSize != nil {
		adapter.chunkSize = capabilities.Features.ChunkSize.Default
	}

	// Initialize stream manager if streaming is supported
	if capabilities.Flags&client.CapabilityFlagStreaming != 0 {
		adapter.streamManager = NewStreamManager(adapter.chunkSize)
	}

	// Initialize flow controller if flow control is supported
	if capabilities.Flags&client.CapabilityFlagFlowControl != 0 {
		windowSize := 1024 * 1024 // 1MB default
		if capabilities.Features != nil && capabilities.Features.FlowControl != nil {
			windowSize = capabilities.Features.FlowControl.WindowSize
		}
		adapter.flowController = NewFlowController(windowSize, func(streamId string, pause bool) {
			adapter.handleBackpressure(streamId, pause)
		})
	}

	// Don't start event listeners here - let WebSocketMonitor handle message reading
	// adapter.setupEventListeners()
	return adapter
}

// NegotiatedFlags returns capability flags negotiated for this adapter (0 if nil caps).
func (a *BinaryProtocolAdapter) NegotiatedFlags() int {
	if a == nil || a.capabilities == nil {
		return 0
	}
	return a.capabilities.Flags
}

// SetConnWriteMu sets the mutex used to serialize writes on the monitor connection.
func (a *BinaryProtocolAdapter) SetConnWriteMu(mu *sync.Mutex) {
	a.connWriteMu = mu
}

func isSemanticHTTPMessageType(t MessageType) bool {
	switch t {
	case MessageTypeHTTPRequestHead, MessageTypeHTTPRequestBody,
		MessageTypeHTTPResponseHead, MessageTypeHTTPResponseBody:
		return true
	default:
		return false
	}
}

func semanticHTTPHeadType(t MessageType) bool {
	return t == MessageTypeHTTPRequestHead || t == MessageTypeHTTPResponseHead
}

func (a *BinaryProtocolAdapter) shouldUseSemanticHeadCompression() bool {
	return a.capabilities != nil && a.capabilities.Flags&client.CapabilityFlagCompression != 0
}

func (a *BinaryProtocolAdapter) dispatchSemanticHTTPMessage(msg *BinaryMessage, fin bool) error {
	payload := msg.Data
	if semanticHTTPHeadType(msg.Type) && a.shouldUseSemanticHeadCompression() {
		var err error
		payload, err = a.decompressData(msg.Data)
		if err != nil {
			return err
		}
	}
	if a.flowController != nil {
		a.flowController.ReleaseReceiveWindow(msg.StreamID, len(msg.Data))
	}

	a.handlerMu.RLock()
	defer a.handlerMu.RUnlock()
	switch msg.Type {
	case MessageTypeHTTPRequestHead:
		if a.isClient && a.httpRequestHeadHandler != nil {
			return a.httpRequestHeadHandler(msg.StreamID, payload, fin)
		}
	case MessageTypeHTTPRequestBody:
		if a.isClient && a.httpRequestBodyHandler != nil {
			return a.httpRequestBodyHandler(msg.StreamID, payload, fin)
		}
	case MessageTypeHTTPResponseHead:
		if !a.isClient && a.httpResponseHeadHandler != nil {
			return a.httpResponseHeadHandler(msg.StreamID, payload, fin)
		}
	case MessageTypeHTTPResponseBody:
		if !a.isClient && a.httpResponseBodyHandler != nil {
			return a.httpResponseBodyHandler(msg.StreamID, payload, fin)
		}
	}
	return nil
}

func (a *BinaryProtocolAdapter) writeMonitorText(msg []byte) error {
	if a.connWriteMu != nil {
		a.connWriteMu.Lock()
		defer a.connWriteMu.Unlock()
	}
	return a.conn.WriteMessage(websocket.TextMessage, msg)
}

// getNextSequence gets the next sequence number for a stream
func (a *BinaryProtocolAdapter) getNextSequence(streamId string) uint32 {
	a.sequenceCounterMu.Lock()
	defer a.sequenceCounterMu.Unlock()

	current := a.sequenceCounter[streamId]
	next := current + 1
	a.sequenceCounter[streamId] = next
	return next
}

// handleBackpressure handles backpressure signals
func (a *BinaryProtocolAdapter) handleBackpressure(streamId string, pause bool) {
	if a.streamManager != nil {
		stream := a.streamManager.GetStream(streamId)
		if stream != nil {
			if pause {
				stream.Pause()
			} else {
				stream.Resume()
			}
		}
	}
}

// setupEventListeners sets up WebSocket event listeners
func (a *BinaryProtocolAdapter) setupEventListeners() {
	go func() {
		for {
			messageType, message, err := a.conn.ReadMessage()
			if err != nil {
				return
			}

			if messageType == websocket.BinaryMessage {
				// Parse binary message
				binaryMsg, err := ParseBinaryMessage(message)
				if err != nil {
					// Log error with message preview for debugging
					previewLen := 32 // Show first 32 bytes
					if len(message) < previewLen {
						previewLen = len(message)
					}
					previewHex := hex.EncodeToString(message[:previewLen])
					logger.Infof("[protocol:binary] Failed to parse binary message in message loop: %v, message length: %d bytes, first %d bytes (hex): %s", err, len(message), previewLen, previewHex)
					continue
				}

				a.handleBinaryMessage(binaryMsg)
			} else if messageType == websocket.TextMessage {
				// Handle text messages (for compatibility)
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

				// Skip control messages (ping, pong, authenticate, etc.) - these are handled by WebSocketMonitor
				if event == "ping" || event == "pong" || event == "authenticate" || event == "response" {
					continue
				}

				// Handle binary messages sent as base64-encoded text
				if event == "request" || event == "tcp:data" {
					if data, ok := payload.(map[string]interface{}); ok {
						dataStr, _ := data["data"].(string)
						if dataStr != "" {
							// Decode base64
							messageBuffer, err := base64.StdEncoding.DecodeString(dataStr)
							if err != nil {
								logger.Infof("[protocol:binary] Failed to decode base64 message: %v", err)
								continue
							}
							binaryMsg, err := ParseBinaryMessage(messageBuffer)
							if err != nil {
								// Log error with message preview for debugging
								previewLen := 32 // Show first 32 bytes
								if len(messageBuffer) < previewLen {
									previewLen = len(messageBuffer)
								}
								previewHex := hex.EncodeToString(messageBuffer[:previewLen])
								logger.Infof("[protocol:binary] Failed to parse binary message from base64 text: %v, message length: %d bytes, first %d bytes (hex): %s", err, len(messageBuffer), previewLen, previewHex)
								continue
							}
							a.handleBinaryMessage(binaryMsg)
						}
					}
				}
			}
		}
	}()
}

// HandleBinaryMessage handles a received binary message (public method for external callers)
func (a *BinaryProtocolAdapter) HandleBinaryMessage(message []byte) error {
	binaryMsg, err := ParseBinaryMessage(message)
	if err != nil {
		// Log error with message preview for debugging
		previewLen := 32 // Show first 32 bytes
		if len(message) < previewLen {
			previewLen = len(message)
		}
		previewHex := hex.EncodeToString(message[:previewLen])
		logger.Infof("[protocol:binary] Failed to parse binary message: %v, message length: %d bytes, first %d bytes (hex): %s", err, len(message), previewLen, previewHex)
		return fmt.Errorf("failed to parse binary message: %w", err)
	}
	return a.handleBinaryMessage(binaryMsg)
}

// handleBinaryMessage handles a received binary message
func (a *BinaryProtocolAdapter) handleBinaryMessage(msg *BinaryMessage) error {
	isLast := (msg.Flags & MessageFlagFIN) != 0

	// Check flow control
	if a.flowController != nil {
		if !a.flowController.Receive(msg.StreamID, len(msg.Data)) {
			// Receive window insufficient, wait
			time.Sleep(100 * time.Millisecond)
		}
	}

	if isSemanticHTTPMessageType(msg.Type) {
		return a.dispatchSemanticHTTPMessage(msg, isLast)
	}

	shouldStream := a.shouldUseStreaming(msg.Type)

	if shouldStream && a.streamManager != nil {
		// Streaming: need to reassemble
		// First, ensure stream exists with proper callbacks
		if a.streamManager.GetStream(msg.StreamID) == nil {
			// Create stream with appropriate callback based on message type
			var onComplete func(data []byte)
			var onError func(error error)

			// Decompress function for streaming data
			decompressFunc := func(data []byte) ([]byte, error) {
				if a.shouldUseCompression(msg.Type) {
					return a.decompressData(data)
				}
				return data, nil
			}

			// Set up onComplete callback based on message type
			a.handlerMu.RLock()
			if msg.Type == MessageTypeHTTPRequest && a.isClient && a.httpRequestHandler != nil {
				handler := a.httpRequestHandler
				onComplete = func(data []byte) {
					// Decompress and call handler
					payload, err := decompressFunc(data)
					if err != nil {
						if onError != nil {
							onError(err)
						}
						return
					}
					handler(msg.StreamID, payload)
					// Update flow control window
					if a.flowController != nil {
						a.flowController.ReleaseReceiveWindow(msg.StreamID, len(data))
					}
				}
			} else if msg.Type == MessageTypeHTTPResponse && !a.isClient && a.httpResponseHandler != nil {
				handler := a.httpResponseHandler
				onComplete = func(data []byte) {
					// Decompress and call handler
					payload, err := decompressFunc(data)
					if err != nil {
						if onError != nil {
							onError(err)
						}
						return
					}
					handler(msg.StreamID, payload)
					// Update flow control window
					if a.flowController != nil {
						a.flowController.ReleaseReceiveWindow(msg.StreamID, len(data))
					}
				}
			} else if msg.Type == MessageTypeTCPData {
				handlers := make([]func(streamId string, data []byte) error, 0)
				for _, handler := range a.tcpDataHandlers {
					handlers = append(handlers, handler)
				}
				onComplete = func(data []byte) {
					// TCP data is not compressed, call handlers directly
					for _, handler := range handlers {
						handler(msg.StreamID, data)
					}
					// Update flow control window
					if a.flowController != nil {
						a.flowController.ReleaseReceiveWindow(msg.StreamID, len(data))
					}
				}
			}
			a.handlerMu.RUnlock()

			// Create stream with callbacks
			a.streamManager.CreateStream(msg.StreamID, onComplete, onError)
		}

		// Add chunk to stream (stream will handle reassembly and call onComplete when done)
		a.streamManager.AddChunk(msg.StreamID, int(msg.Sequence), msg.Data, isLast)

		// If this is the last chunk, the stream manager will trigger callback when reassembly is complete
		if isLast {
			return nil
		}
	} else {
		// Direct processing: single message (non-streaming)
		payload := msg.Data

		// Decompress data (TCP data is not compressed)
		if a.shouldUseCompression(msg.Type) {
			var err error
			payload, err = a.decompressData(payload)
			if err != nil {
				return err
			}
		}

		// Route to corresponding handler
		a.handlerMu.RLock()
		if msg.Type == MessageTypeHTTPRequest && a.isClient && a.httpRequestHandler != nil {
			a.handlerMu.RUnlock()
			return a.httpRequestHandler(msg.StreamID, payload)
		} else if msg.Type == MessageTypeHTTPResponse && !a.isClient && a.httpResponseHandler != nil {
			a.handlerMu.RUnlock()
			return a.httpResponseHandler(msg.StreamID, payload)
		} else if msg.Type == MessageTypeTCPData {
			handlers := make([]func(streamId string, data []byte) error, 0)
			for _, handler := range a.tcpDataHandlers {
				handlers = append(handlers, handler)
			}
			a.handlerMu.RUnlock()
			for _, handler := range handlers {
				handler(msg.StreamID, payload)
			}
		} else {
			a.handlerMu.RUnlock()
		}

		// Update flow control window
		if a.flowController != nil {
			a.flowController.ReleaseReceiveWindow(msg.StreamID, len(msg.Data))
		}
	}

	return nil
}

// shouldUseCompression checks if compression should be used for the message type
func (a *BinaryProtocolAdapter) shouldUseCompression(msgType MessageType) bool {
	// TCP data is not compressed
	if msgType == MessageTypeTCPData {
		return false
	}
	// HTTP requests/responses use compression if supported
	return a.capabilities.Flags&client.CapabilityFlagCompression != 0
}

// shouldUseStreaming checks if streaming should be used for the message type
func (a *BinaryProtocolAdapter) shouldUseStreaming(msgType MessageType) bool {
	if msgType == MessageTypeHTTPRequest {
		return a.capabilities.Flags&client.CapabilityFlagHTTPStreaming != 0
	} else if msgType == MessageTypeHTTPResponse {
		return a.capabilities.Flags&client.CapabilityFlagHTTPStreaming != 0
	}
	return false
}

// compressData compresses data using the configured algorithm
func (a *BinaryProtocolAdapter) compressData(data []byte) ([]byte, error) {
	switch a.compressionAlgorithm {
	case CompressionBrotli:
		return compressBrotli(data)
	case CompressionGzip:
		return compressGzipBytes(data)
	case CompressionNone:
		return data, nil
	default:
		return data, nil
	}
}

// decompressData decompresses data using the configured algorithm
func (a *BinaryProtocolAdapter) decompressData(data []byte) ([]byte, error) {
	switch a.compressionAlgorithm {
	case CompressionBrotli:
		return decompressBrotli(data)
	case CompressionGzip:
		return decompressGzipBytes(data)
	case CompressionNone:
		return data, nil
	default:
		return data, nil
	}
}

// SendHTTPRequest sends an HTTP request
func (a *BinaryProtocolAdapter) SendHTTPRequest(id string, data []byte) error {
	return a.sendMessage(MessageTypeHTTPRequest, id, data)
}

// SendHTTPResponse sends an HTTP response
func (a *BinaryProtocolAdapter) SendHTTPResponse(id string, data []byte) error {
	return a.sendMessage(MessageTypeHTTPResponse, id, data)
}

// SetDataChannel sets the shared data channel connection (legacy fallback)
func (a *BinaryProtocolAdapter) SetDataChannel(dataConn *websocket.Conn, dataWriteMu *sync.Mutex) {
	a.dataConn = dataConn
	a.dataWriteMu = dataWriteMu
}

// SetDataChannelForStream sets the data channel connection for a specific stream (new protocol)
func (a *BinaryProtocolAdapter) SetDataChannelForStream(streamId string, dataConn *websocket.Conn, dataWriteMu *sync.Mutex) {
	a.dataChannelsMu.Lock()
	a.dataChannels[streamId] = &dataChannel{
		conn: dataConn,
		mu:   dataWriteMu,
	}
	a.dataChannelsMu.Unlock()
}

// RemoveDataChannelForStream removes the data channel mapping for a stream
// Also cleans up flow control window and stream manager resources for the stream
func (a *BinaryProtocolAdapter) RemoveDataChannelForStream(streamId string) {
	// Remove data channel mapping
	a.dataChannelsMu.Lock()
	delete(a.dataChannels, streamId)
	a.dataChannelsMu.Unlock()

	// Clean up flow control window for this stream
	if a.flowController != nil {
		a.flowController.RemoveStream(streamId)
	}

	// Clean up stream manager resources for this stream
	if a.streamManager != nil {
		a.streamManager.RemoveStream(streamId)
	}
}

// SendTCPData sends TCP data
// For new protocol, if data channel is available, sends via data channel; otherwise uses monitor channel
func (a *BinaryProtocolAdapter) SendTCPData(streamId string, data []byte) error {
	useTCPOverWS := a.capabilities != nil && (a.capabilities.Flags&client.CapabilityFlagTCPOverWS) != 0

	// Prefer per-stream data channel
	if dc := a.getDataChannel(streamId); dc != nil {
		return a.sendMessageViaSpecificDataChannel(dc, MessageTypeTCPData, streamId, data)
	}
	// Fallback to shared data channel
	if a.dataConn != nil {
		return a.sendMessageViaDataChannel(MessageTypeTCPData, streamId, data)
	}
	// For new protocol (TCP over WS), monitor channel does not carry tcp:data.
	// Failing fast prevents silent black-holing that can hang client connections.
	if useTCPOverWS {
		return fmt.Errorf("data channel not ready for stream %s", streamId)
	}
	// Otherwise, use monitor channel (legacy protocol or fallback)
	return a.sendMessage(MessageTypeTCPData, streamId, data)
}

// getDataChannel returns per-stream data channel if exists
func (a *BinaryProtocolAdapter) getDataChannel(streamId string) *dataChannel {
	a.dataChannelsMu.RLock()
	defer a.dataChannelsMu.RUnlock()
	return a.dataChannels[streamId]
}

// sendMessage sends a message (with streaming support if needed)
func (a *BinaryProtocolAdapter) sendMessage(msgType MessageType, streamId string, data []byte) error {
	shouldStream := a.shouldUseStreaming(msgType)

	if shouldStream && a.streamManager != nil {
		return a.sendStreaming(msgType, streamId, data)
	} else {
		return a.sendDirect(msgType, streamId, data)
	}
}

// sendDirect sends a message directly (non-streaming)
func (a *BinaryProtocolAdapter) sendDirect(msgType MessageType, streamId string, data []byte) error {
	// Compress data if needed
	processedData := data
	if a.shouldUseCompression(msgType) {
		var err error
		processedData, err = a.compressData(data)
		if err != nil {
			return err
		}
	}

	// Build binary message
	sequence := a.getNextSequence(streamId)
	msg := BinaryMessage{
		Type:     msgType,
		StreamID: streamId,
		Sequence: sequence,
		Flags:    0,
		Data:     processedData,
	}

	messageBytes, err := BuildBinaryMessage(msg)
	if err != nil {
		return err
	}

	// Send as base64-encoded text message (for compatibility with existing clients)
	// In the future, we can send as binary message directly
	base64Data := base64.StdEncoding.EncodeToString(messageBytes)

	var event string
	switch msgType {
	case MessageTypeHTTPRequest:
		event = "request"
	case MessageTypeHTTPResponse:
		event = "response"
	case MessageTypeTCPData:
		event = "tcp:data"
	default:
		event = "data"
	}

	msgJSON := []interface{}{
		event,
		map[string]interface{}{
			"id":   streamId,
			"data": base64Data,
		},
	}

	msgBytes, err := json.Marshal(msgJSON)
	if err != nil {
		return err
	}

	return a.writeMonitorText(msgBytes)
}

// sendMessageViaDataChannel sends a message via data channel (for new protocol)
func (a *BinaryProtocolAdapter) sendMessageViaDataChannel(msgType MessageType, streamId string, data []byte) error {
	shouldStream := a.shouldUseStreaming(msgType)

	if shouldStream && a.streamManager != nil {
		return a.sendStreamingViaDataChannel(msgType, streamId, data)
	} else {
		return a.sendDirectViaDataChannel(msgType, streamId, data)
	}
}

// sendDirectViaDataChannel sends a message directly via data channel (non-streaming)
func (a *BinaryProtocolAdapter) sendDirectViaDataChannel(msgType MessageType, streamId string, data []byte) error {
	// Compress data if needed
	processedData := data
	if a.shouldUseCompression(msgType) {
		var err error
		processedData, err = a.compressData(data)
		if err != nil {
			return err
		}
	}

	// Build binary message
	sequence := a.getNextSequence(streamId)
	msg := BinaryMessage{
		Type:     msgType,
		StreamID: streamId,
		Sequence: sequence,
		Flags:    0,
		Data:     processedData,
	}

	messageBytes, err := BuildBinaryMessage(msg)
	if err != nil {
		return err
	}

	// Send binary message directly via WebSocket BinaryMessage
	if a.dataWriteMu != nil {
		a.dataWriteMu.Lock()
		defer a.dataWriteMu.Unlock()
	}

	return a.dataConn.WriteMessage(websocket.BinaryMessage, messageBytes)
}

// sendMessageViaSpecificDataChannel sends message via a dedicated per-stream data channel
func (a *BinaryProtocolAdapter) sendMessageViaSpecificDataChannel(dc *dataChannel, msgType MessageType, streamId string, data []byte) error {
	// Compress data if needed
	processedData := data
	if a.shouldUseCompression(msgType) {
		var err error
		processedData, err = a.compressData(data)
		if err != nil {
			return err
		}
	}

	sequence := a.getNextSequence(streamId)
	msg := BinaryMessage{
		Type:     msgType,
		StreamID: streamId,
		Sequence: sequence,
		Flags:    0,
		Data:     processedData,
	}

	messageBytes, err := BuildBinaryMessage(msg)
	if err != nil {
		return err
	}

	if dc.mu != nil {
		dc.mu.Lock()
		defer dc.mu.Unlock()
	}
	return dc.conn.WriteMessage(websocket.BinaryMessage, messageBytes)
}

// sendStreamingViaDataChannel sends a message using streaming via data channel (chunked)
func (a *BinaryProtocolAdapter) sendStreamingViaDataChannel(msgType MessageType, streamId string, data []byte) error {
	if a.streamManager == nil {
		// Fallback to direct send if no stream manager
		return a.sendDirectViaDataChannel(msgType, streamId, data)
	}

	// Initialize flow control window
	if a.flowController != nil {
		a.flowController.InitializeStream(streamId)
	}

	// Compress data (before chunking for better compression efficiency)
	processedData := data
	if a.shouldUseCompression(msgType) {
		var err error
		processedData, err = a.compressData(data)
		if err != nil {
			return err
		}
	}

	// Split into chunks
	chunks := a.streamManager.SplitIntoChunks(processedData, a.chunkSize)

	// For streaming, sequence numbers start from 0 and increment for each chunk
	// This ensures proper reassembly on the receiver side
	var sequence uint32 = 0

	// Send each chunk
	for i, chunk := range chunks {
		isLast := i == len(chunks)-1

		// Check flow control (atomic check-and-update)
		if a.flowController != nil {
			// Wait for window to be available using atomic TrySend
			for !a.flowController.TrySend(streamId, len(chunk)) {
				time.Sleep(50 * time.Millisecond)
			}
		}

		// Build message with sequential numbering starting from 0
		// Each stream has its own sequence counter starting from 0
		flags := MessageFlags(0)
		if isLast {
			flags |= MessageFlagFIN
		}

		msg := BinaryMessage{
			Type:     msgType,
			StreamID: streamId,
			Sequence: sequence,
			Flags:    flags,
			Data:     chunk,
		}

		messageBytes, err := BuildBinaryMessage(msg)
		if err != nil {
			return err
		}

		// Send binary message directly via WebSocket BinaryMessage
		if a.dataWriteMu != nil {
			a.dataWriteMu.Lock()
		}
		if err := a.dataConn.WriteMessage(websocket.BinaryMessage, messageBytes); err != nil {
			if a.dataWriteMu != nil {
				a.dataWriteMu.Unlock()
			}
			return err
		}
		if a.dataWriteMu != nil {
			a.dataWriteMu.Unlock()
		}

		// Increment sequence for next chunk
		sequence++
	}

	return nil
}

// sendStreaming sends a message using streaming (chunked)
func (a *BinaryProtocolAdapter) sendStreaming(msgType MessageType, streamId string, data []byte) error {
	if a.streamManager == nil {
		// Fallback to direct send if no stream manager
		return a.sendDirect(msgType, streamId, data)
	}

	// Initialize flow control window
	if a.flowController != nil {
		a.flowController.InitializeStream(streamId)
	}

	// Compress data (before chunking for better compression efficiency)
	processedData := data
	if a.shouldUseCompression(msgType) {
		var err error
		processedData, err = a.compressData(data)
		if err != nil {
			return err
		}
	}

	// Split into chunks
	chunks := a.streamManager.SplitIntoChunks(processedData, a.chunkSize)

	// For streaming, sequence numbers start from 0 and increment for each chunk
	// This ensures proper reassembly on the receiver side
	var sequence uint32 = 0

	// Send each chunk
	for i, chunk := range chunks {
		isLast := i == len(chunks)-1

		// Check flow control (atomic check-and-update)
		if a.flowController != nil {
			// Wait for window to be available using atomic TrySend
			for !a.flowController.TrySend(streamId, len(chunk)) {
				time.Sleep(50 * time.Millisecond)
			}
		}

		// Build message with sequential numbering starting from 0
		// Each stream has its own sequence counter starting from 0
		flags := MessageFlags(0)
		if isLast {
			flags |= MessageFlagFIN
		}

		msg := BinaryMessage{
			Type:     msgType,
			StreamID: streamId,
			Sequence: sequence,
			Flags:    flags,
			Data:     chunk,
		}

		messageBytes, err := BuildBinaryMessage(msg)
		if err != nil {
			return err
		}

		// Send as base64-encoded text message
		base64Data := base64.StdEncoding.EncodeToString(messageBytes)

		var event string
		switch msgType {
		case MessageTypeHTTPRequest:
			event = "request"
		case MessageTypeHTTPResponse:
			event = "response"
		case MessageTypeTCPData:
			event = "tcp:data"
		default:
			event = "data"
		}

		msgJSON := []interface{}{
			event,
			map[string]interface{}{
				"id":   streamId,
				"data": base64Data,
			},
		}

		msgBytes, err := json.Marshal(msgJSON)
		if err != nil {
			return err
		}

		if err := a.writeMonitorText(msgBytes); err != nil {
			return err
		}

		// Increment sequence for next chunk
		sequence++
	}

	return nil
}

// OnHTTPRequest registers an HTTP request handler
func (a *BinaryProtocolAdapter) OnHTTPRequest(handler func(id string, data []byte) error) {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()
	a.httpRequestHandler = handler
}

// OnHTTPResponse registers an HTTP response handler
func (a *BinaryProtocolAdapter) OnHTTPResponse(handler func(id string, data []byte) error) {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()
	a.httpResponseHandler = handler
}

// OnHTTPRequestHead registers a handler for semantic-streaming HTTP request headers (server -> client).
func (a *BinaryProtocolAdapter) OnHTTPRequestHead(handler func(id string, head []byte, fin bool) error) {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()
	a.httpRequestHeadHandler = handler
}

// OnHTTPRequestBody registers a handler for semantic-streaming HTTP request body chunks.
func (a *BinaryProtocolAdapter) OnHTTPRequestBody(handler func(id string, chunk []byte, fin bool) error) {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()
	a.httpRequestBodyHandler = handler
}

// OnHTTPResponseHead registers a handler for semantic-streaming HTTP response headers (client -> server).
func (a *BinaryProtocolAdapter) OnHTTPResponseHead(handler func(id string, head []byte, fin bool) error) {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()
	a.httpResponseHeadHandler = handler
}

// OnHTTPResponseBody registers a handler for semantic-streaming HTTP response body chunks.
func (a *BinaryProtocolAdapter) OnHTTPResponseBody(handler func(id string, chunk []byte, fin bool) error) {
	a.handlerMu.Lock()
	defer a.handlerMu.Unlock()
	a.httpResponseBodyHandler = handler
}

func semanticMonitorEventForType(mt MessageType) string {
	switch mt {
	case MessageTypeHTTPRequestHead, MessageTypeHTTPRequestBody:
		return "request"
	case MessageTypeHTTPResponseHead, MessageTypeHTTPResponseBody:
		return "response"
	default:
		return "data"
	}
}

func (a *BinaryProtocolAdapter) sendSemanticHTTPMonitor(mt MessageType, streamId string, data []byte, flags MessageFlags) error {
	processed := data
	if semanticHTTPHeadType(mt) && a.shouldUseSemanticHeadCompression() {
		var err error
		processed, err = a.compressData(data)
		if err != nil {
			return err
		}
	}

	msg := BinaryMessage{
		Type:     mt,
		StreamID: streamId,
		Sequence: a.getNextSequence(streamId),
		Flags:    flags,
		Data:     processed,
	}
	messageBytes, err := BuildBinaryMessage(msg)
	if err != nil {
		return err
	}
	base64Data := base64.StdEncoding.EncodeToString(messageBytes)
	event := semanticMonitorEventForType(mt)
	msgJSON := []interface{}{
		event,
		map[string]interface{}{
			"id":   streamId,
			"data": base64Data,
		},
	}
	msgBytes, err := json.Marshal(msgJSON)
	if err != nil {
		return err
	}
	return a.writeMonitorText(msgBytes)
}

// SendHTTPRequestHead sends tunneled HTTP request headers (semantic streaming).
func (a *BinaryProtocolAdapter) SendHTTPRequestHead(id string, head []byte, fin bool) error {
	fl := MessageFlags(0)
	if fin {
		fl |= MessageFlagFIN
	}
	return a.sendSemanticHTTPMonitor(MessageTypeHTTPRequestHead, id, head, fl)
}

// SendHTTPRequestBody sends a tunneled HTTP request body chunk.
func (a *BinaryProtocolAdapter) SendHTTPRequestBody(id string, chunk []byte, fin bool) error {
	fl := MessageFlags(0)
	if fin {
		fl |= MessageFlagFIN
	}
	return a.sendSemanticHTTPMonitor(MessageTypeHTTPRequestBody, id, chunk, fl)
}

// SendHTTPResponseHead sends tunneled HTTP response headers (semantic streaming).
func (a *BinaryProtocolAdapter) SendHTTPResponseHead(id string, head []byte, fin bool) error {
	fl := MessageFlags(0)
	if fin {
		fl |= MessageFlagFIN
	}
	return a.sendSemanticHTTPMonitor(MessageTypeHTTPResponseHead, id, head, fl)
}

// SendHTTPResponseBody sends a tunneled HTTP response body chunk.
func (a *BinaryProtocolAdapter) SendHTTPResponseBody(id string, chunk []byte, fin bool) error {
	fl := MessageFlags(0)
	if fin {
		fl |= MessageFlagFIN
	}
	return a.sendSemanticHTTPMonitor(MessageTypeHTTPResponseBody, id, chunk, fl)
}

// OnTCPData registers a TCP data handler
func (a *BinaryProtocolAdapter) OnTCPData(handler func(streamId string, data []byte) error) func() {
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
func (a *BinaryProtocolAdapter) Destroy() {
	if a.streamManager != nil {
		a.streamManager.Destroy()
	}
	if a.flowController != nil {
		a.flowController.Clear()
	}
}
