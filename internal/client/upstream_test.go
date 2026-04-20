package client

import "testing"

func TestParseUpstream(t *testing.T) {
	h, p, err := ParseUpstream("8080")
	if err != nil || h != "127.0.0.1" || p != 8080 {
		t.Fatalf("port-only: got %s %d %v", h, p, err)
	}
	h, p, err = ParseUpstream("10.0.0.5:3000")
	if err != nil || h != "10.0.0.5" || p != 3000 {
		t.Fatalf("host:port: got %s %d %v", h, p, err)
	}
	_, _, err = ParseUpstream("")
	if err == nil {
		t.Fatal("expected error for empty")
	}
}
