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

// Regression: first hijacked frame for a WebSocket handshake must not use semantic request
// streaming (HTTPBodyStream). A typical GET upgrade has no body and ContentLength >= 0, so
// canSemanticStreamRequestBody is true — Attach must still treat it as non-streaming (see useStream in Attach).
func TestWebSocketHandshakeMatchesCanSemanticStreamRequestBody(t *testing.T) {
	r, _ := http.NewRequest("GET", "/terminal", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")
	if !isWebSocketUpgradeRequest(r) {
		t.Fatal("fixture must be a WS handshake")
	}
	if !canSemanticStreamRequestBody(r) {
		t.Fatal("GET with no body: expected canSemanticStreamRequestBody true (regression guard for Attach useStream)")
	}
}
