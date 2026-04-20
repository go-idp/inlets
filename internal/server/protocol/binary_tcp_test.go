package protocol

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-idp/inlets/internal/client"
)

func TestSendTCPData_ReturnsErrorWhenTCPOverWSHasNoDataChannel(t *testing.T) {
	capabilities := &client.Capabilities{
		Flags:   client.CapabilityFlagTCPOverWS,
		Version: "2.0.0",
	}

	adapter := NewBinaryProtocolAdapter(nil, capabilities, false)
	err := adapter.SendTCPData("stream-1", []byte("hello"))
	if err == nil {
		t.Fatal("expected error when TCP over WS stream has no data channel")
	}
}

// TestHandleBinaryMessage_TCPDataIgnoresSemanticHTTPRouting ensures TCP tunnel traffic (0x03) is not
// affected by semantic HTTP streaming (0x07–0x0a): it must still reach OnTCPData even when
// HTTPStreaming and HTTPBodyStream are negotiated (TCP must not use stream reassembly or semantic dispatch).
func TestHandleBinaryMessage_TCPDataIgnoresSemanticHTTPRouting(t *testing.T) {
	capabilities := &client.Capabilities{
		Flags: client.CapabilityFlagBinaryProtocol |
			client.CapabilityFlagHTTPStreaming |
			client.CapabilityFlagHTTPBodyStream |
			client.CapabilityFlagCompression,
		Version: "2.0.0",
	}

	adapter := NewBinaryProtocolAdapter(nil, capabilities, false)

	var streamID string
	var payload []byte
	adapter.OnTCPData(func(sid string, data []byte) error {
		streamID = sid
		payload = append([]byte(nil), data...)
		return nil
	})

	raw, err := BuildBinaryMessage(BinaryMessage{
		Type:     MessageTypeTCPData,
		StreamID: "tcp-stream-id",
		Sequence: 0,
		Flags:    MessageFlagFIN,
		Data:     []byte("payload"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.HandleBinaryMessage(raw); err != nil {
		t.Fatal(err)
	}
	if streamID != "tcp-stream-id" {
		t.Fatalf("OnTCPData streamId = %q, want tcp-stream-id", streamID)
	}
	if string(payload) != "payload" {
		t.Fatalf("OnTCPData payload = %q", payload)
	}
}

// TestHandleBinaryMessage_SemanticHTTPDoesNotInvokeTCPHandler guards against accidentally routing
// semantic HTTP frames to TCP handlers.
func TestHandleBinaryMessage_SemanticHTTPDoesNotInvokeTCPHandler(t *testing.T) {
	capabilities := &client.Capabilities{
		Flags:   client.CapabilityFlagBinaryProtocol | client.CapabilityFlagHTTPBodyStream,
		Version: "2.0.0",
	}
	adapter := NewBinaryProtocolAdapter(nil, capabilities, false)

	var tcpCalls atomic.Int32
	adapter.OnTCPData(func(string, []byte) error {
		tcpCalls.Add(1)
		return nil
	})

	raw, err := BuildBinaryMessage(BinaryMessage{
		Type:     MessageTypeHTTPResponseHead,
		StreamID: "http-req-id",
		Sequence: 0,
		Flags:    MessageFlagFIN,
		Data:     []byte("HTTP/1.1 200 OK\r\n\r\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.HandleBinaryMessage(raw); err != nil {
		t.Fatal(err)
	}
	if tcpCalls.Load() != 0 {
		t.Fatalf("OnTCPData invoked %d times for semantic HTTP head frame", tcpCalls.Load())
	}
}

// TestHandleBinaryMessage_StreamingHTTPClientRequiresHandler ensures EnsureStream path rejects
// streaming HTTP requests when no OnHTTPRequest handler is registered (regression: nil onComplete).
func TestHandleBinaryMessage_StreamingHTTPClientRequiresHandler(t *testing.T) {
	caps := &client.Capabilities{
		Flags:   client.CapabilityFlagBinaryProtocol | client.CapabilityFlagStreaming | client.CapabilityFlagHTTPStreaming,
		Version: "2.0.0",
		Features: &client.CapabilityFeatures{
			ChunkSize: &client.ChunkSizeFeatures{Min: 256, Max: 65536, Default: 1024},
		},
	}
	adapter := NewBinaryProtocolAdapter(nil, caps, true)

	raw, err := BuildBinaryMessage(BinaryMessage{
		Type:     MessageTypeHTTPRequest,
		StreamID: "req-1",
		Sequence: 0,
		Flags:    MessageFlagFIN,
		Data:     []byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.HandleBinaryMessage(raw); err == nil {
		t.Fatal("expected error when streaming HTTP client has no request handler")
	}
}

// TestHandleBinaryMessage_PropagatesHTTPResponseHandlerError checks that handler errors surface from HandleBinaryMessage.
func TestHandleBinaryMessage_PropagatesHTTPResponseHandlerError(t *testing.T) {
	caps := &client.Capabilities{
		Flags:   client.CapabilityFlagBinaryProtocol,
		Version: "2.0.0",
	}
	adapter := NewBinaryProtocolAdapter(nil, caps, false)
	want := errors.New("handler failed")
	adapter.OnHTTPResponse(func(id string, data []byte) error {
		if id != "cid" {
			t.Errorf("stream id = %q", id)
		}
		return want
	})

	raw, err := BuildBinaryMessage(BinaryMessage{
		Type:     MessageTypeHTTPResponse,
		StreamID: "cid",
		Sequence: 1,
		Flags:    MessageFlagFIN,
		Data:     []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = adapter.HandleBinaryMessage(raw)
	if !errors.Is(err, want) {
		t.Fatalf("HandleBinaryMessage err = %v, want %v", err, want)
	}
}

// TestHandleBinaryMessage_InvalidWireReturnsError ensures parse failures return a wrapped error from HandleBinaryMessage.
func TestHandleBinaryMessage_InvalidWireReturnsError(t *testing.T) {
	adapter := NewBinaryProtocolAdapter(nil, &client.Capabilities{
		Flags:   client.CapabilityFlagBinaryProtocol,
		Version: "2.0.0",
	}, false)
	if err := adapter.HandleBinaryMessage([]byte{0x00}); err == nil {
		t.Fatal("expected parse error for truncated message")
	}
}

// TestWaitFlowSendSlotUnblocksAfterOnAck verifies flow-control wait does not deadlock when the window is refilled.
func TestWaitFlowSendSlotUnblocksAfterOnAck(t *testing.T) {
	fc := NewFlowController(100, nil)
	a := &BinaryProtocolAdapter{flowController: fc}
	fc.InitializeStream("sid", 100)
	if !fc.TrySend("sid", 90) {
		t.Fatal("TrySend should admit 90 bytes in 100-byte window")
	}

	go func() {
		time.Sleep(30 * time.Millisecond)
		fc.OnAck("sid", 50)
	}()

	start := time.Now()
	if err := a.waitFlowSendSlot("sid", 20); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 20*time.Millisecond {
		t.Fatalf("wait returned too fast (OnAck likely not applied): %v", time.Since(start))
	}
	if !fc.TrySend("sid", 1) {
		t.Fatal("expected spare capacity after waitFlowSendSlot + prior OnAck")
	}
}

// TestWaitFlowSendSlotTimeout verifies the wait does not run forever when the window never opens.
func TestWaitFlowSendSlotTimeout(t *testing.T) {
	old := flowSendWaitTimeout
	flowSendWaitTimeout = 120 * time.Millisecond
	t.Cleanup(func() { flowSendWaitTimeout = old })

	fc := NewFlowController(100, nil)
	a := &BinaryProtocolAdapter{flowController: fc}
	fc.InitializeStream("sid", 100)
	if !fc.TrySend("sid", 100) {
		t.Fatal("TrySend should fill 100-byte window")
	}

	start := time.Now()
	err := a.waitFlowSendSlot("sid", 1)
	if err == nil {
		t.Fatal("expected timeout when window stays full")
	}
	if !strings.Contains(err.Error(), "flow control send wait timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
	if d := time.Since(start); d < 100*time.Millisecond || d > 400*time.Millisecond {
		t.Fatalf("elapsed %v, want ~flowSendWaitTimeout (with slack)", d)
	}
}
