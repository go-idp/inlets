package tunnel

import (
	"testing"
	"time"

	"github.com/go-idp/inlets/internal/client"
)

func TestTCPRelaySetupDelay(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		flags int
		want  time.Duration
	}{
		{"no flags", 0, 0},
		{"tcp over ws only", client.CapabilityFlagTCPOverWS, 75 * time.Millisecond},
		{"early register only", client.CapabilityFlagTCPEarlyStreamRegister, 0},
		{
			"tcp over ws and early register",
			client.CapabilityFlagTCPOverWS | client.CapabilityFlagTCPEarlyStreamRegister,
			0,
		},
		{
			"tcp over ws with unrelated bits still delays",
			client.CapabilityFlagTCPOverWS | client.CapabilityFlagBinaryProtocol,
			75 * time.Millisecond,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tcpRelaySetupDelay(tt.flags)
			if got != tt.want {
				t.Fatalf("tcpRelaySetupDelay(%#x) = %v, want %v", tt.flags, got, tt.want)
			}
		})
	}
}
