package monitor

import (
	"encoding/base64"
	"testing"

	servercontainer "github.com/go-idp/inlets/internal/server/container"
	"github.com/go-idp/inlets/internal/server/types"
)

func TestHandleResponseConsumesCallbackOnce(t *testing.T) {
	ctx := &types.Context{
		CallbackContainer: servercontainer.NewCallbackContainer(),
	}

	tcpID := "tcp-1"
	requestID := "req-1"
	payloadData := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"

	callCount := 0
	var got string
	ctx.CallbackContainer.Set(tcpID, requestID, func(data string) {
		callCount++
		got = data
	})

	// New protocol also uses the same TextMessage ["response", ...] path on the monitor channel.
	wsConn := &WebSocketConnection{
		UseNewProtocol: true,
	}
	payload := map[string]interface{}{
		"id":   tcpID + ":" + requestID,
		"data": base64.StdEncoding.EncodeToString([]byte(payloadData)),
	}

	handleResponse(ctx, wsConn, payload)
	handleResponse(ctx, wsConn, payload)

	if callCount != 1 {
		t.Fatalf("expected callback to be called once, got %d", callCount)
	}

	if got != payloadData {
		t.Fatalf("unexpected callback payload: %q", got)
	}
}

