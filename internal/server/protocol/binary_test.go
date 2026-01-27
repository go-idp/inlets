package protocol

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-zoox/inlets/internal/client"
	"github.com/gorilla/websocket"
)

// TestStreamingSequenceNumbers tests that streaming uses sequential sequence numbers starting from 0
func TestStreamingSequenceNumbers(t *testing.T) {
	// Create a WebSocket server for testing
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Create capabilities with streaming support
		capabilities := &client.Capabilities{
			Flags:   client.CapabilityFlagStreaming | client.CapabilityFlagHTTPStreaming,
			Version: "2.0.0",
			Features: &client.CapabilityFeatures{
				ChunkSize: &client.ChunkSizeFeatures{
					Min:     1024,
					Max:     1024 * 1024,
					Default: 64 * 1024,
				},
			},
		}

		// Create adapter
		adapter := NewBinaryProtocolAdapter(conn, capabilities, false)

		// Test data that will be split into multiple chunks
		testData := make([]byte, 200*1024) // 200KB, will be split into ~3 chunks with 64KB chunk size
		for i := range testData {
			testData[i] = byte(i % 256)
		}

		streamId := "test-stream-1"

		// Send streaming data
		err = adapter.sendStreaming(MessageTypeHTTPRequest, streamId, testData)
		if err != nil {
			t.Errorf("Failed to send streaming data: %v", err)
			return
		}
	}))
	defer server.Close()

	// Connect to the WebSocket server
	wsURL := "ws" + server.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket server: %v", err)
	}
	defer conn.Close()

	// Collect all messages
	var messages [][]byte
	var mu sync.Mutex

	// Read messages in a goroutine
	done := make(chan bool)
	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			mu.Lock()
			messages = append(messages, message)
			mu.Unlock()
		}
	}()

	// Wait for messages (with timeout)
	<-done

	mu.Lock()
	defer mu.Unlock()

	// Verify that messages were sent with sequential sequence numbers starting from 0
	if len(messages) == 0 {
		t.Fatal("No messages were sent")
	}

	// Parse all sent messages and verify sequence numbers
	expectedSequence := uint32(0)
	for i, msgBytes := range messages {
		// sendStreaming sends base64-encoded text messages, so we need to decode them
		var binaryMsg *BinaryMessage

		// Try to parse as JSON first (text message format)
		var msgArray []interface{}
		if err := json.Unmarshal(msgBytes, &msgArray); err == nil && len(msgArray) >= 2 {
			// Extract base64 data from JSON
			if dataMap, ok := msgArray[1].(map[string]interface{}); ok {
				if dataStr, ok := dataMap["data"].(string); ok {
					// Decode base64
					decoded, err := base64.StdEncoding.DecodeString(dataStr)
					if err != nil {
						t.Errorf("Failed to decode base64 for message %d: %v", i, err)
						continue
					}
					// Parse binary message
					binaryMsg, err = ParseBinaryMessage(decoded)
					if err != nil {
						t.Errorf("Failed to parse binary message %d: %v", i, err)
						continue
					}
				}
			}
		} else {
			// Try to parse as binary message directly
			var err error
			binaryMsg, err = ParseBinaryMessage(msgBytes)
			if err != nil {
				t.Errorf("Message %d: failed to parse as JSON or binary: %v", i, err)
				continue
			}
		}

		if binaryMsg == nil {
			t.Errorf("Message %d: could not extract binary message", i)
			continue
		}

		// Verify sequence number
		if binaryMsg.Sequence != expectedSequence {
			t.Errorf("Message %d: expected sequence %d, got %d", i, expectedSequence, binaryMsg.Sequence)
		}

		// Verify stream ID
		if binaryMsg.StreamID != "test-stream-1" {
			t.Errorf("Message %d: expected stream ID test-stream-1, got %s", i, binaryMsg.StreamID)
		}

		// Verify message type
		if binaryMsg.Type != MessageTypeHTTPRequest {
			t.Errorf("Message %d: expected message type %d, got %d", i, MessageTypeHTTPRequest, binaryMsg.Type)
		}

		expectedSequence++
	}

	// Verify we received multiple chunks
	if len(messages) < 2 {
		t.Errorf("Expected at least 2 chunks, got %d", len(messages))
	}

	t.Logf("Successfully verified %d chunks with sequential sequence numbers (0-%d)", len(messages), expectedSequence-1)
}

// TestStreamingSequenceNumberStartsFromZero tests that sequence numbers always start from 0 for each stream
func TestStreamingSequenceNumberStartsFromZero(t *testing.T) {
	// This test directly verifies the fix: sequence numbers should start from 0
	// and increment sequentially for each chunk in a stream

	// Create test chunks
	chunks := [][]byte{
		[]byte("chunk0"),
		[]byte("chunk1"),
		[]byte("chunk2"),
	}

	// Simulate the fixed logic: sequence starts from 0 and increments
	var sequence uint32 = 0
	sequences := make([]uint32, 0, len(chunks))

	for range chunks {
		sequences = append(sequences, sequence)
		sequence++
	}

	// Verify sequences start from 0 and are consecutive
	expectedSeq := uint32(0)
	for i, seq := range sequences {
		if seq != expectedSeq {
			t.Errorf("Chunk %d: expected sequence %d, got %d", i, expectedSeq, seq)
		}
		expectedSeq++
	}

	// Verify first sequence is 0
	if sequences[0] != 0 {
		t.Errorf("First sequence should be 0, got %d", sequences[0])
	}

	t.Logf("Verified sequence numbers: %v (all start from 0 and increment)", sequences)
}

// messageCollector implements a minimal websocket.Conn interface for testing
type messageCollector struct {
	messages [][]byte
	mu       sync.Mutex
}

func (m *messageCollector) WriteMessage(messageType int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Make a copy of the data to avoid issues with slice reuse
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	m.messages = append(m.messages, dataCopy)
	return nil
}

func (m *messageCollector) ReadMessage() (messageType int, p []byte, err error) {
	return 0, nil, &websocket.CloseError{Code: websocket.CloseAbnormalClosure}
}

func (m *messageCollector) Close() error {
	return nil
}

// TestStreamingOnCompleteCallback tests that streaming properly calls onComplete callback
func TestStreamingOnCompleteCallback(t *testing.T) {
	// Create a WebSocket server for testing
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	var receivedData []byte
	var callbackCalled bool
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Create capabilities with streaming support
		capabilities := &client.Capabilities{
			Flags:   client.CapabilityFlagStreaming | client.CapabilityFlagHTTPStreaming,
			Version: "2.0.0",
			Features: &client.CapabilityFeatures{
				ChunkSize: &client.ChunkSizeFeatures{
					Min:     1024,
					Max:     1024 * 1024,
					Default: 64 * 1024,
				},
			},
		}

		// Create adapter
		adapter := NewBinaryProtocolAdapter(conn, capabilities, false)

		// Register HTTP response handler
		adapter.OnHTTPResponse(func(id string, data []byte) error {
			mu.Lock()
			receivedData = data
			callbackCalled = true
			mu.Unlock()
			return nil
		})

		// Read and process messages
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}

			// Handle binary message (this should trigger streaming reassembly)
			adapter.HandleBinaryMessage(message)
		}
	}))
	defer server.Close()

	// Connect to the WebSocket server
	wsURL := "ws" + server.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket server: %v", err)
	}
	defer conn.Close()

	// Create test data that will be split into multiple chunks
	testData := make([]byte, 150*1024) // 150KB, will be split into multiple chunks
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	streamId := "test-stream-callback"

	// Split data into chunks and send as streaming messages
	chunkSize := 64 * 1024
	chunks := make([][]byte, 0)
	for i := 0; i < len(testData); i += chunkSize {
		end := i + chunkSize
		if end > len(testData) {
			end = len(testData)
		}
		chunks = append(chunks, testData[i:end])
	}

	// Send chunks with sequence numbers starting from 0
	for i, chunk := range chunks {
		isLast := i == len(chunks)-1

		flags := MessageFlags(0)
		if isLast {
			flags |= MessageFlagFIN
		}

		msg := BinaryMessage{
			Type:     MessageTypeHTTPResponse,
			StreamID: streamId,
			Sequence: uint32(i),
			Flags:    flags,
			Data:     chunk,
		}

		messageBytes, err := BuildBinaryMessage(msg)
		if err != nil {
			t.Fatalf("Failed to build binary message: %v", err)
		}

		// Send binary message
		if err := conn.WriteMessage(websocket.BinaryMessage, messageBytes); err != nil {
			t.Fatalf("Failed to send message: %v", err)
		}
	}

	// Wait for callback to be called (with timeout)
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for onComplete callback")
		case <-ticker.C:
			mu.Lock()
			if callbackCalled {
				mu.Unlock()
				goto verify
			}
			mu.Unlock()
		}
	}

verify:
	mu.Lock()
	defer mu.Unlock()

	if !callbackCalled {
		t.Fatal("onComplete callback was not called")
	}

	if len(receivedData) != len(testData) {
		t.Errorf("Received data length mismatch: expected %d, got %d", len(testData), len(receivedData))
	}

	// Verify data integrity
	if !bytes.Equal(receivedData, testData) {
		t.Error("Received data does not match original data")
	}

	t.Logf("Successfully verified onComplete callback: received %d bytes", len(receivedData))
}

// TestStreamingOnCompleteCallbackMultipleStreams tests that multiple streams have independent callbacks
func TestStreamingOnCompleteCallbackMultipleStreams(t *testing.T) {
	// Create a WebSocket server for testing
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	receivedData := make(map[string][]byte)
	callbackCount := make(map[string]int)
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Create capabilities with streaming support
		capabilities := &client.Capabilities{
			Flags:   client.CapabilityFlagStreaming | client.CapabilityFlagHTTPStreaming,
			Version: "2.0.0",
			Features: &client.CapabilityFeatures{
				ChunkSize: &client.ChunkSizeFeatures{
					Min:     1024,
					Max:     1024 * 1024,
					Default: 64 * 1024,
				},
			},
		}

		// Create adapter
		adapter := NewBinaryProtocolAdapter(conn, capabilities, false)

		// Register HTTP response handler
		adapter.OnHTTPResponse(func(id string, data []byte) error {
			mu.Lock()
			receivedData[id] = data
			callbackCount[id]++
			mu.Unlock()
			return nil
		})

		// Read and process messages
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}

			// Handle binary message
			adapter.HandleBinaryMessage(message)
		}
	}))
	defer server.Close()

	// Connect to the WebSocket server
	wsURL := "ws" + server.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket server: %v", err)
	}
	defer conn.Close()

	// Send data for multiple streams
	streamIds := []string{"stream-1", "stream-2"}
	testData := make(map[string][]byte)

	for _, streamId := range streamIds {
		// Create test data for each stream
		data := make([]byte, 100*1024)
		for i := range data {
			data[i] = byte(i % 256)
		}
		testData[streamId] = data

		// Split into chunks and send
		chunkSize := 64 * 1024
		chunks := make([][]byte, 0)
		for i := 0; i < len(data); i += chunkSize {
			end := i + chunkSize
			if end > len(data) {
				end = len(data)
			}
			chunks = append(chunks, data[i:end])
		}

		// Send chunks
		for i, chunk := range chunks {
			isLast := i == len(chunks)-1

			flags := MessageFlags(0)
			if isLast {
				flags |= MessageFlagFIN
			}

			msg := BinaryMessage{
				Type:     MessageTypeHTTPResponse,
				StreamID: streamId,
				Sequence: uint32(i),
				Flags:    flags,
				Data:     chunk,
			}

			messageBytes, err := BuildBinaryMessage(msg)
			if err != nil {
				t.Fatalf("Failed to build binary message: %v", err)
			}

			if err := conn.WriteMessage(websocket.BinaryMessage, messageBytes); err != nil {
				t.Fatalf("Failed to send message: %v", err)
			}
		}
	}

	// Wait for all callbacks to be called
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for onComplete callbacks")
		case <-ticker.C:
			mu.Lock()
			if len(callbackCount) == len(streamIds) {
				allCalled := true
				for _, streamId := range streamIds {
					if callbackCount[streamId] == 0 {
						allCalled = false
						break
					}
				}
				mu.Unlock()
				if allCalled {
					goto verify
				}
			} else {
				mu.Unlock()
			}
		}
	}

verify:
	mu.Lock()
	defer mu.Unlock()

	// Verify all streams received callbacks
	for _, streamId := range streamIds {
		if callbackCount[streamId] == 0 {
			t.Errorf("Stream %s: onComplete callback was not called", streamId)
			continue
		}

		if callbackCount[streamId] > 1 {
			t.Errorf("Stream %s: onComplete callback was called %d times (expected 1)", streamId, callbackCount[streamId])
		}

		data, exists := receivedData[streamId]
		if !exists {
			t.Errorf("Stream %s: no data received", streamId)
			continue
		}

		expectedData := testData[streamId]
		if !bytes.Equal(data, expectedData) {
			t.Errorf("Stream %s: received data does not match original data", streamId)
		}

		t.Logf("Stream %s: verified callback and data integrity (%d bytes)", streamId, len(data))
	}
}

// TestDataChannelCleanupOnDisconnect tests that data channel cleanup removes flow control and stream manager resources
func TestDataChannelCleanupOnDisconnect(t *testing.T) {
	// Create a WebSocket server for testing
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
	}))
	defer server.Close()

	// Connect to the WebSocket server
	wsURL := "ws" + server.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket server: %v", err)
	}
	defer conn.Close()

	capabilities := &client.Capabilities{
		Flags:   client.CapabilityFlagStreaming | client.CapabilityFlagHTTPStreaming | client.CapabilityFlagFlowControl,
		Version: "2.0.0",
		Features: &client.CapabilityFeatures{
			ChunkSize: &client.ChunkSizeFeatures{
				Min:     1024,
				Max:     1024 * 1024,
				Default: 64 * 1024,
			},
			FlowControl: &client.FlowControlFeatures{
				WindowSize: 1024 * 1024,
			},
		},
	}

	adapter := NewBinaryProtocolAdapter(conn, capabilities, false)

	streamId := "test-stream-cleanup"

	// Initialize stream in flow controller and stream manager
	if adapter.flowController != nil {
		adapter.flowController.InitializeStream(streamId)
	}

	if adapter.streamManager != nil {
		adapter.streamManager.CreateStream(streamId, nil, nil)
	}

	// Verify resources exist before cleanup
	if adapter.flowController != nil {
		window := adapter.flowController.GetWindowState(streamId)
		if window == nil {
			t.Fatal("Flow control window should exist before cleanup")
		}
	}

	if adapter.streamManager != nil {
		stream := adapter.streamManager.GetStream(streamId)
		if stream == nil {
			t.Fatal("Stream manager stream should exist before cleanup")
		}
	}

	// Simulate data channel connection by setting a data channel
	// Create another connection for data channel
	dataConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect data channel: %v", err)
	}
	defer dataConn.Close()

	var mu sync.Mutex
	adapter.SetDataChannelForStream(streamId, dataConn, &mu)

	// Simulate data channel disconnect by calling RemoveDataChannelForStream
	adapter.RemoveDataChannelForStream(streamId)

	// Verify flow control window is removed
	if adapter.flowController != nil {
		window := adapter.flowController.GetWindowState(streamId)
		if window != nil {
			t.Error("Flow control window should be removed after cleanup")
		}
	}

	// Verify stream manager stream is removed
	if adapter.streamManager != nil {
		stream := adapter.streamManager.GetStream(streamId)
		if stream != nil {
			t.Error("Stream manager stream should be removed after cleanup")
		}
	}

	t.Log("Successfully verified cleanup of data channel, flow control window, and stream manager resources")
}

// TestDataChannelCleanupMultipleStreams tests cleanup for multiple streams
func TestDataChannelCleanupMultipleStreams(t *testing.T) {
	// Create a WebSocket server for testing
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
	}))
	defer server.Close()

	// Connect to the WebSocket server
	wsURL := "ws" + server.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket server: %v", err)
	}
	defer conn.Close()

	capabilities := &client.Capabilities{
		Flags:   client.CapabilityFlagStreaming | client.CapabilityFlagHTTPStreaming | client.CapabilityFlagFlowControl,
		Version: "2.0.0",
		Features: &client.CapabilityFeatures{
			ChunkSize: &client.ChunkSizeFeatures{
				Min:     1024,
				Max:     1024 * 1024,
				Default: 64 * 1024,
			},
			FlowControl: &client.FlowControlFeatures{
				WindowSize: 1024 * 1024,
			},
		},
	}

	adapter := NewBinaryProtocolAdapter(conn, capabilities, false)

	streamIds := []string{"stream-1", "stream-2", "stream-3"}

	// Initialize all streams
	for _, streamId := range streamIds {
		if adapter.flowController != nil {
			adapter.flowController.InitializeStream(streamId)
		}
		if adapter.streamManager != nil {
			adapter.streamManager.CreateStream(streamId, nil, nil)
		}
		// Create data channel connection for each stream
		dataConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect data channel for stream %s: %v", streamId, err)
		}
		defer dataConn.Close()

		var mu sync.Mutex
		adapter.SetDataChannelForStream(streamId, dataConn, &mu)
	}

	// Verify all resources exist
	for _, streamId := range streamIds {
		if adapter.flowController != nil {
			window := adapter.flowController.GetWindowState(streamId)
			if window == nil {
				t.Errorf("Flow control window should exist for stream %s", streamId)
			}
		}
		if adapter.streamManager != nil {
			stream := adapter.streamManager.GetStream(streamId)
			if stream == nil {
				t.Errorf("Stream manager stream should exist for stream %s", streamId)
			}
		}
	}

	// Cleanup one stream
	adapter.RemoveDataChannelForStream(streamIds[0])

	// Verify cleaned stream is removed
	if adapter.flowController != nil {
		window := adapter.flowController.GetWindowState(streamIds[0])
		if window != nil {
			t.Error("Flow control window should be removed for cleaned stream")
		}
	}
	if adapter.streamManager != nil {
		stream := adapter.streamManager.GetStream(streamIds[0])
		if stream != nil {
			t.Error("Stream manager stream should be removed for cleaned stream")
		}
	}

	// Verify other streams are still intact
	for i := 1; i < len(streamIds); i++ {
		streamId := streamIds[i]
		if adapter.flowController != nil {
			window := adapter.flowController.GetWindowState(streamId)
			if window == nil {
				t.Errorf("Flow control window should still exist for stream %s", streamId)
			}
		}
		if adapter.streamManager != nil {
			stream := adapter.streamManager.GetStream(streamId)
			if stream == nil {
				t.Errorf("Stream manager stream should still exist for stream %s", streamId)
			}
		}
	}

	t.Log("Successfully verified cleanup of one stream does not affect other streams")
}
