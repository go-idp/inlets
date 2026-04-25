package tunnel

import (
	"net/http"
	"testing"
)

func TestIsWebSocketUpgradeRequest(t *testing.T) {
	r, _ := http.NewRequest("GET", "/terminal", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")
	if !isWebSocketUpgradeRequest(r) {
		t.Fatal("expected true for standard handshake")
	}
	r2, _ := http.NewRequest("GET", "/", nil)
	if isWebSocketUpgradeRequest(r2) {
		t.Fatal("expected false without headers")
	}
}
