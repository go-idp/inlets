package monitor

import (
	"testing"

	"github.com/go-idp/inlets/internal/client"
)

func TestNegotiateCapabilities_TCPEarlyStreamRegister(t *testing.T) {
	modern := client.GetClientCapabilities("2.0.0")
	if modern == nil {
		t.Fatal("GetClientCapabilities(2.0.0) = nil")
	}
	n := negotiateCapabilities(modern)
	if n == nil {
		t.Fatal("negotiateCapabilities(modern) = nil")
	}
	if n.Flags&client.CapabilityFlagTCPEarlyStreamRegister == 0 {
		t.Fatalf("negotiated flags %#x missing CapabilityFlagTCPEarlyStreamRegister", n.Flags)
	}
	if n.Flags&client.CapabilityFlagTCPOverWS == 0 {
		t.Fatalf("negotiated flags %#x missing CapabilityFlagTCPOverWS", n.Flags)
	}

	legacyTCPClient := &client.Capabilities{
		Version:  modern.Version,
		Features: modern.Features,
		Flags:    modern.Flags &^ client.CapabilityFlagTCPEarlyStreamRegister,
	}
	nOld := negotiateCapabilities(legacyTCPClient)
	if nOld == nil {
		t.Fatal("negotiateCapabilities(legacyTCPClient) = nil")
	}
	if nOld.Flags&client.CapabilityFlagTCPEarlyStreamRegister != 0 {
		t.Fatalf("old-style client should not negotiate early register; got %#x", nOld.Flags)
	}
	if nOld.Flags&client.CapabilityFlagTCPOverWS == 0 {
		t.Fatalf("negotiated flags %#x missing CapabilityFlagTCPOverWS", nOld.Flags)
	}
}
