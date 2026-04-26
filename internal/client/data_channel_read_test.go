package client

import (
	"errors"
	"io"
	"net"
	"syscall"
	"testing"

	"github.com/gorilla/websocket"
)

func TestIsDataChannelReadClosedNormally(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		err   error
		wantC bool
	}{
		{"nil", nil, false},
		{"EOF", io.EOF, true},
		{"net ErrClosed", net.ErrClosed, true},
		{"ws close", &websocket.CloseError{Code: websocket.CloseNormalClosure, Text: "bye"}, true},
		{"use of closed", errors.New("read tcp: use of closed network connection"), true},
		{"reset string", errors.New("read tcp: connection reset by peer"), true},
		{"OpError ECONNRESET", &net.OpError{Err: syscall.ECONNRESET}, true},
		{"timeout", &net.OpError{Err: syscall.ETIMEDOUT}, false},
		{"other", errors.New("tls: bad record MAC"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isDataChannelReadClosedNormally(tc.err)
			if got != tc.wantC {
				t.Errorf("isDataChannelReadClosedNormally(%v) = %v, want %v", tc.err, got, tc.wantC)
			}
		})
	}
}
