package protocol

import (
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

