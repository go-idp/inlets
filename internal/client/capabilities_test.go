package client

import "testing"

func TestGetClientCapabilities_IncludesTCPEarlyStreamRegister(t *testing.T) {
	c := GetClientCapabilities("2.0.0")
	if c == nil {
		t.Fatal("GetClientCapabilities = nil")
	}
	if c.Flags&CapabilityFlagTCPEarlyStreamRegister == 0 {
		t.Fatalf("flags %#x should include CapabilityFlagTCPEarlyStreamRegister", c.Flags)
	}
}

func TestGetClientCapabilities_LegacyNoCapabilities(t *testing.T) {
	if c := GetClientCapabilities("1.9.0"); c != nil {
		t.Fatalf("expected nil for pre-2.0, got %#v", c)
	}
}
