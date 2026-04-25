package client

import (
	"errors"
	"testing"

	"github.com/gorilla/websocket"
)

func TestShouldExitOnPublicHTTPNoAuthTimeoutClose(t *testing.T) {
	if !shouldExitOnPublicHTTPNoAuthTimeoutClose(&websocket.CloseError{
		Code: websocket.CloseNormalClosure,
		Text: "public monitor session timeout",
	}) {
		t.Fatalf("expected true for new close reason (websocket)")
	}
	if !shouldExitOnPublicHTTPNoAuthTimeoutClose(&websocket.CloseError{
		Code: websocket.CloseNormalClosure,
		Text: "public http no-auth timeout",
	}) {
		t.Fatalf("expected true for legacy close reason (websocket)")
	}
	if !shouldExitOnPublicHTTPNoAuthTimeoutClose(errors.New("read tcp: websocket: close 1000 (normal): public monitor session timeout")) {
		t.Fatalf("expected true for new reason in string error")
	}
	if !shouldExitOnPublicHTTPNoAuthTimeoutClose(errors.New("read tcp: websocket: close 1000 (normal): public http no-auth timeout")) {
		t.Fatalf("expected true for legacy reason in string error")
	}
	if shouldExitOnPublicHTTPNoAuthTimeoutClose(&websocket.CloseError{
		Code: websocket.CloseNormalClosure,
		Text: "normal shutdown",
	}) {
		t.Fatalf("expected false for unrelated close reason")
	}
}
