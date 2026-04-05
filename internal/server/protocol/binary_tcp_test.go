package protocol

import (
	"sync/atomic"
	"testing"

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

