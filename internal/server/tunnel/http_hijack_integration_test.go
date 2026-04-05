package tunnel

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	servercontainer "github.com/go-idp/inlets/internal/server/container"
	"github.com/go-idp/inlets/internal/server/limiter"
	"github.com/go-idp/inlets/internal/server/protocol"
	"github.com/go-idp/inlets/internal/server/stats"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/gorilla/websocket"
)

// fakeHTTPTunnelAdapter implements protocol.ProtocolAdapter for tunnel tests: captures tunneled
// raw HTTP bytes and completes each request by invoking the server's callback (same contract as
// a real client responding on the monitor channel).
type fakeHTTPTunnelAdapter struct {
	callbacks types.CallbackContainer
	mu        sync.Mutex
	requests  [][]byte
	responses []string
}

var _ protocol.ProtocolAdapter = (*fakeHTTPTunnelAdapter)(nil)

func (f *fakeHTTPTunnelAdapter) responseForIndex(idx int) string {
	if len(f.responses) > idx {
		return f.responses[idx]
	}
	return "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok"
}

func splitTunnelWireID(id string) (tcpID, requestID string, ok bool) {
	i := strings.Index(id, ":")
	if i <= 0 || i >= len(id)-1 {
		return "", "", false
	}
	return id[:i], id[i+1:], true
}

func (f *fakeHTTPTunnelAdapter) SendHTTPRequest(id string, data []byte) error {
	f.mu.Lock()
	f.requests = append(f.requests, append([]byte(nil), data...))
	idx := len(f.requests) - 1
	f.mu.Unlock()

	tcpID, reqID, ok := splitTunnelWireID(id)
	if !ok {
		return nil
	}
	resp := f.responseForIndex(idx)
	if cb := f.callbacks.Take(tcpID, reqID); cb != nil {
		cb(resp)
	}
	return nil
}

func (f *fakeHTTPTunnelAdapter) SendHTTPResponse(id string, data []byte) error  { return nil }
func (f *fakeHTTPTunnelAdapter) SendTCPData(streamId string, data []byte) error { return nil }
func (f *fakeHTTPTunnelAdapter) OnHTTPRequest(handler func(id string, data []byte) error) {
}
func (f *fakeHTTPTunnelAdapter) OnHTTPResponse(handler func(id string, data []byte) error) {
}
func (f *fakeHTTPTunnelAdapter) OnTCPData(handler func(streamId string, data []byte) error) func() {
	return func() {}
}
func (f *fakeHTTPTunnelAdapter) Destroy()                      {}
func (f *fakeHTTPTunnelAdapter) SetConnWriteMu(mu *sync.Mutex) {}

func (f *fakeHTTPTunnelAdapter) captured() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.requests))
	for i := range f.requests {
		out[i] = append([]byte(nil), f.requests[i]...)
	}
	return out
}

func testTunnelContext() *types.Context {
	return &types.Context{
		Config:            types.DefaultServerConfig(),
		DomainMappings:    servercontainer.NewDomainContainer(),
		CallbackContainer: servercontainer.NewCallbackContainer(),
		Container:         servercontainer.NewTunnelContainer(),
		TrafficStats:      stats.NewTrafficStatsContainer(),
		BandwidthLimiter:  limiter.NewBandwidthLimiter(nil),
	}
}

// dummyWSSocket satisfies processRequest's non-nil check when adapter handles HTTP; it must not
// be used for I/O in the new-protocol + adapter path.
func dummyWSSocket() *websocket.Conn {
	return new(websocket.Conn)
}

func startTunnelHTTPServer(t *testing.T, ctx *types.Context, domain string) (baseURL string, cleanup func()) {
	t.Helper()
	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ht := CreateHTTPTunnel(ctx, domain)
	ht.Attach(srv)
	go func() {
		_ = srv.Serve(ln)
	}()
	return "http://" + ln.Addr().String(), func() {
		_ = srv.Close()
		_ = ln.Close()
	}
}

func registerAppExampleMapping(t *testing.T, ctx *types.Context, adapter protocol.ProtocolAdapter) {
	t.Helper()
	// subdomain "app" for Host app.example.com with tunnel domain example.com
	type binder interface {
		BindWSWithMetadata(wsSocket *websocket.Conn, subDomain string, clientID string, adapter interface{}, useNewProtocol bool) string
	}
	b, ok := ctx.DomainMappings.(binder)
	if !ok {
		t.Fatal("DomainMappings implementation must expose BindWSWithMetadata")
	}
	b.BindWSWithMetadata(dummyWSSocket(), "app", "test-client", adapter, true)
}

// TestHTTPTunnelHijackFirstRequestDoesNotBlock verifies the fix for reading the first tunneled
// request: net/http has already parsed it before Hijack; the tunnel must use handler Request + raw
// bytes, not ReadRequest on a fresh bufio.Reader on the conn (which would block for keep-alive).
func TestHTTPTunnelHijackFirstRequestDoesNotBlock(t *testing.T) {
	adapter := &fakeHTTPTunnelAdapter{}
	ctx := testTunnelContext()
	adapter.callbacks = ctx.CallbackContainer
	registerAppExampleMapping(t, ctx, adapter)

	baseURL, cleanup := startTunnelHTTPServer(t, ctx, "example.com")
	defer cleanup()

	host := strings.TrimPrefix(baseURL, "http://")
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	fmt.Fprintf(conn, "GET /hello HTTP/1.1\r\nHost: app.example.com\r\nConnection: close\r\n\r\n")
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d, body %q", resp.StatusCode, body)
	}
	if string(body) != "ok" {
		t.Fatalf("body %q, want ok", body)
	}

	reqs := adapter.captured()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 tunneled request, got %d", len(reqs))
	}
	if !strings.Contains(string(reqs[0]), "GET /hello HTTP/1.1") {
		t.Fatalf("tunneled raw missing request line: %q", reqs[0])
	}
	if !strings.Contains(string(reqs[0]), "Host: app.example.com") {
		t.Fatalf("tunneled raw missing Host: %q", reqs[0])
	}
}

// TestHTTPTunnelHijackKeepAliveSecondRequest ensures the Hijack bufio.Reader is used for the
// second HTTP request on the same TCP connection.
func TestHTTPTunnelHijackKeepAliveSecondRequest(t *testing.T) {
	adapter := &fakeHTTPTunnelAdapter{
		responses: []string{
			"HTTP/1.1 200 OK\r\nContent-Length: 1\r\nConnection: keep-alive\r\n\r\n1",
			"HTTP/1.1 200 OK\r\nContent-Length: 1\r\nConnection: close\r\n\r\n2",
		},
	}
	ctx := testTunnelContext()
	adapter.callbacks = ctx.CallbackContainer
	registerAppExampleMapping(t, ctx, adapter)

	baseURL, cleanup := startTunnelHTTPServer(t, ctx, "example.com")
	defer cleanup()
	host := strings.TrimPrefix(baseURL, "http://")

	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(conn)

	fmt.Fprintf(conn, "GET /one HTTP/1.1\r\nHost: app.example.com\r\nConnection: keep-alive\r\n\r\n")
	resp1, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("first response: %v", err)
	}
	b1, _ := io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()
	if string(b1) != "1" {
		t.Fatalf("first body %q", b1)
	}

	fmt.Fprintf(conn, "GET /two HTTP/1.1\r\nHost: app.example.com\r\nConnection: close\r\n\r\n")
	resp2, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("second response: %v", err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()
	if string(b2) != "2" {
		t.Fatalf("second body %q", b2)
	}

	reqs := adapter.captured()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 tunneled requests, got %d", len(reqs))
	}
	if !strings.Contains(string(reqs[0]), "GET /one ") {
		t.Fatalf("first tunneled: %q", reqs[0])
	}
	if !strings.Contains(string(reqs[1]), "GET /two ") {
		t.Fatalf("second tunneled: %q", reqs[1])
	}
}

// TestHTTPTunnelHijackPOSTBodyBeforeHijack ensures the handler reads r.Body before Hijack and the
// rebuilt raw request includes the body bytes.
func TestHTTPTunnelHijackPOSTBodyBeforeHijack(t *testing.T) {
	adapter := &fakeHTTPTunnelAdapter{}
	ctx := testTunnelContext()
	adapter.callbacks = ctx.CallbackContainer
	registerAppExampleMapping(t, ctx, adapter)

	baseURL, cleanup := startTunnelHTTPServer(t, ctx, "example.com")
	defer cleanup()
	host := strings.TrimPrefix(baseURL, "http://")

	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	payload := `{"x":1}`
	fmt.Fprintf(conn, "POST /api HTTP/1.1\r\nHost: app.example.com\r\nContent-Type: application/json\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		len(payload), payload)

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	reqs := adapter.captured()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	raw := string(reqs[0])
	if !strings.Contains(raw, "POST /api ") {
		t.Fatalf("missing POST line: %q", raw)
	}
	if !strings.Contains(raw, payload) {
		t.Fatalf("POST body not in tunneled raw: %q", raw)
	}
}
