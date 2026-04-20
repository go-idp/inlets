package tunnel

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-idp/inlets/internal/client"
	"github.com/gorilla/websocket"
)

// tcpTestPipe returns a server-side WebSocket (for TunnelMapping.WSSocket) and a cleanup func.
// Outbound monitor messages are drained on the server conn so writes do not block.
func tcpTestPipe(t *testing.T) (serverWS *websocket.Conn, cleanup func()) {
	t.Helper()

	upgraded := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("websocket upgrade: %v", err)
			return
		}
		upgraded <- c
		go func() {
			for {
				if _, _, err := c.ReadMessage(); err != nil {
					return
				}
			}
		}()
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial websocket: %v", err)
	}

	select {
	case serverWS = <-upgraded:
	case <-time.After(2 * time.Second):
		clientWS.Close()
		srv.Close()
		t.Fatal("timeout waiting for server websocket")
	}

	return serverWS, func() {
		clientWS.Close()
		serverWS.Close()
		srv.Close()
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// TestTCPCreateServerReusesSourcePort verifies that when the client did not request a fixed
// TunnelPort (0), an existing SourcePort is reused instead of allocating a new random port.
func TestTCPCreateServerReusesSourcePort(t *testing.T) {
	serverWS, cleanup := tcpTestPipe(t)
	defer cleanup()

	ctx := testTunnelContext()
	const cid = "container-reuse-port"
	auth := &client.Authentication{
		Version:    "1.0",
		Type:       "tcp",
		Port:       0,
		TunnelPort: 0,
		Timestamp:  1,
		Signature:  "sig",
	}
	ctx.Container.Create(cid, nil, serverWS, auth, &sync.Mutex{})

	wantPort := freeTCPPort(t)
	if err := ctx.Container.Set(cid, "sourcePort", wantPort); err != nil {
		t.Fatalf("set sourcePort: %v", err)
	}

	tt := CreateTCPTunnel(ctx)
	if err := tt.CreateServer(Options{ContainerID: cid, Domain: "t.example"}); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	m := ctx.Container.Get(cid)
	if m == nil {
		t.Fatal("container missing")
	}
	if m.SourcePort == nil || *m.SourcePort != wantPort {
		t.Fatalf("SourcePort = %v, want %d", m.SourcePort, wantPort)
	}
	if m.SourceServer == nil {
		t.Fatal("SourceServer is nil")
	}
	if got := m.SourceServer.Addr().(*net.TCPAddr).Port; got != wantPort {
		t.Fatalf("listener port = %d, want %d", got, wantPort)
	}
}

// TestTCPTunnelListenerRecreatesOnClose closes the public TCP listener and expects the accept
// loop to clear SourceServer and call CreateServer again on the same port so new dials succeed.
func TestTCPTunnelListenerRecreatesOnClose(t *testing.T) {
	serverWS, cleanup := tcpTestPipe(t)
	defer cleanup()

	ctx := testTunnelContext()
	const cid = "container-recover"
	auth := &client.Authentication{
		Version:    "1.0",
		Type:       "tcp",
		Port:       0,
		TunnelPort: 0,
		Timestamp:  1,
		Signature:  "sig",
	}
	ctx.Container.Create(cid, nil, serverWS, auth, &sync.Mutex{})

	tt := CreateTCPTunnel(ctx)
	if err := tt.CreateServer(Options{ContainerID: cid, Domain: "t.example"}); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	m := ctx.Container.Get(cid)
	if m == nil || m.SourceServer == nil || m.SourcePort == nil {
		t.Fatalf("initial state: m=%v sourceServer=%v sourcePort=%v", m != nil, m != nil && m.SourceServer != nil, m != nil && m.SourcePort != nil)
	}
	port := *m.SourcePort
	oldListener := m.SourceServer

	if err := oldListener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var newL net.Listener
	for time.Now().Before(deadline) {
		m = ctx.Container.Get(cid)
		if m != nil && m.SourceServer != nil && m.SourceServer != oldListener {
			newL = m.SourceServer
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if newL == nil {
		t.Fatal("listener was not recreated after Close")
	}
	if m.SourcePort == nil || *m.SourcePort != port {
		t.Fatalf("SourcePort after recovery = %v, want %d", m.SourcePort, port)
	}

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial recovered listener: %v", err)
	}
	c.Close()
}
