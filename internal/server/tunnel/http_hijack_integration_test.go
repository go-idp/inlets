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

	"github.com/go-idp/inlets/internal/client"
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

func (f *fakeHTTPTunnelAdapter) SendHTTPResponse(id string, data []byte) error { return nil }
func (f *fakeHTTPTunnelAdapter) SendHTTPRequestHead(id string, head []byte, fin bool) error {
	return protocol.ErrSemanticHTTPNotSupported
}
func (f *fakeHTTPTunnelAdapter) SendHTTPRequestBody(id string, chunk []byte, fin bool) error {
	return protocol.ErrSemanticHTTPNotSupported
}
func (f *fakeHTTPTunnelAdapter) SendHTTPResponseHead(id string, head []byte, fin bool) error {
	return nil
}
func (f *fakeHTTPTunnelAdapter) SendHTTPResponseBody(id string, chunk []byte, fin bool) error {
	return nil
}
func (f *fakeHTTPTunnelAdapter) SendTCPData(streamId string, data []byte) error { return nil }
func (f *fakeHTTPTunnelAdapter) OnHTTPRequest(handler func(id string, data []byte) error) {
}
func (f *fakeHTTPTunnelAdapter) OnHTTPResponse(handler func(id string, data []byte) error) {
}
func (f *fakeHTTPTunnelAdapter) OnHTTPRequestHead(handler func(id string, head []byte, fin bool) error) {
}
func (f *fakeHTTPTunnelAdapter) OnHTTPRequestBody(handler func(id string, chunk []byte, fin bool) error) {
}
func (f *fakeHTTPTunnelAdapter) OnHTTPResponseHead(handler func(id string, head []byte, fin bool) error) {
}
func (f *fakeHTTPTunnelAdapter) OnHTTPResponseBody(handler func(id string, chunk []byte, fin bool) error) {
}
func (f *fakeHTTPTunnelAdapter) OnTCPData(handler func(streamId string, data []byte) error) func() {
	return func() {}
}
func (f *fakeHTTPTunnelAdapter) Destroy()                      {}
func (f *fakeHTTPTunnelAdapter) SetConnWriteMu(mu *sync.Mutex) {}

func (f *fakeHTTPTunnelAdapter) NegotiatedFlags() int { return 0 }

// streamRecordAdapter captures semantic HTTP request head/body frames for tests.
type streamRecordAdapter struct {
	fakeHTTPTunnelAdapter
	mu         sync.Mutex
	heads      [][]byte
	bodies     [][]byte
	bodyFins   []bool
	onComplete func(tcpID, reqID string)
}

func (s *streamRecordAdapter) NegotiatedFlags() int {
	return client.CapabilityFlagHTTPBodyStream | client.CapabilityFlagBinaryProtocol
}

func (s *streamRecordAdapter) SendHTTPRequest(id string, data []byte) error {
	return protocol.ErrSemanticHTTPNotSupported
}

func (s *streamRecordAdapter) SendHTTPRequestHead(id string, head []byte, fin bool) error {
	s.mu.Lock()
	s.heads = append(s.heads, append([]byte(nil), head...))
	s.mu.Unlock()
	_ = fin
	return nil
}

func (s *streamRecordAdapter) SendHTTPRequestBody(id string, chunk []byte, fin bool) error {
	s.mu.Lock()
	s.bodies = append(s.bodies, append([]byte(nil), chunk...))
	s.bodyFins = append(s.bodyFins, fin)
	s.mu.Unlock()
	tcpID, reqID, ok := splitTunnelWireID(id)
	if ok && fin && s.onComplete != nil {
		s.onComplete(tcpID, reqID)
	}
	return nil
}

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

func startTunnelHTTPServer(t *testing.T, ctx *types.Context, domain string) (baseURL string, ht *HTTPTunnel, cleanup func()) {
	t.Helper()
	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ht = CreateHTTPTunnel(ctx, domain)
	ht.Attach(srv)
	go func() {
		_ = srv.Serve(ln)
	}()
	return "http://" + ln.Addr().String(), ht, func() {
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

	baseURL, _, cleanup := startTunnelHTTPServer(t, ctx, "example.com")
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

func TestHTTPTunnelRequestBodyTooLargeBeforeHijack(t *testing.T) {
	old := maxHTTPTunnelRequestBodyBytes
	maxHTTPTunnelRequestBodyBytes = 64
	defer func() { maxHTTPTunnelRequestBodyBytes = old }()

	ctx := testTunnelContext()
	baseURL, _, cleanup := startTunnelHTTPServer(t, ctx, "example.com")
	defer cleanup()
	host := strings.TrimPrefix(baseURL, "http://")

	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	payload := strings.Repeat("x", 100)
	fmt.Fprintf(conn, "POST /big HTTP/1.1\r\nHost: app.example.com\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		len(payload), payload)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want %d", resp.StatusCode, http.StatusRequestEntityTooLarge)
	}
}

func TestHTTPTunnelKeepAliveRequestBodyTooLarge(t *testing.T) {
	old := maxHTTPTunnelRequestBodyBytes
	maxHTTPTunnelRequestBodyBytes = 32
	defer func() { maxHTTPTunnelRequestBodyBytes = old }()

	adapter := &fakeHTTPTunnelAdapter{
		responses: []string{
			"HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: keep-alive\r\n\r\n",
		},
	}
	ctx := testTunnelContext()
	adapter.callbacks = ctx.CallbackContainer
	registerAppExampleMapping(t, ctx, adapter)

	baseURL, _, cleanup := startTunnelHTTPServer(t, ctx, "example.com")
	defer cleanup()
	host := strings.TrimPrefix(baseURL, "http://")

	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	fmt.Fprintf(conn, "GET /one HTTP/1.1\r\nHost: app.example.com\r\nConnection: keep-alive\r\n\r\n")
	br := bufio.NewReader(conn)
	resp1, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("first response: %v", err)
	}
	_, _ = io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()
	if resp1.StatusCode != 200 {
		t.Fatalf("first status %d", resp1.StatusCode)
	}

	payload := strings.Repeat("y", 64)
	fmt.Fprintf(conn, "POST /two HTTP/1.1\r\nHost: app.example.com\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		len(payload), payload)
	resp2, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("second response: %v", err)
	}
	_, _ = io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("second status %d, want %d", resp2.StatusCode, http.StatusRequestEntityTooLarge)
	}
	reqs := adapter.captured()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 tunneled request, got %d", len(reqs))
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

	baseURL, _, cleanup := startTunnelHTTPServer(t, ctx, "example.com")
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
	if !strings.Contains(string(reqs[0]), "Host: app.example.com") {
		t.Fatalf("first tunneled raw missing Host: %q", reqs[0])
	}
	if !strings.Contains(string(reqs[1]), "GET /two ") {
		t.Fatalf("second tunneled: %q", reqs[1])
	}
	if !strings.Contains(string(reqs[1]), "Host: app.example.com") {
		t.Fatalf("second tunneled raw missing Host: %q", reqs[1])
	}
}

// TestHTTPTunnelHijackPOSTBodyBeforeHijack ensures the handler reads r.Body before Hijack and the
// rebuilt raw request includes the body bytes.
func TestHTTPTunnelHijackPOSTBodyBeforeHijack(t *testing.T) {
	adapter := &fakeHTTPTunnelAdapter{}
	ctx := testTunnelContext()
	adapter.callbacks = ctx.CallbackContainer
	registerAppExampleMapping(t, ctx, adapter)

	baseURL, _, cleanup := startTunnelHTTPServer(t, ctx, "example.com")
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

func TestHTTPTunnelDispatchSemanticClientResponse(t *testing.T) {
	ctx := testTunnelContext()
	ht := CreateHTTPTunnel(ctx, "example.com")

	r, w := net.Pipe()
	defer r.Close()
	defer w.Close()

	const tcpID, reqID = "tsem", "rsem"
	key := tcpID + ":" + reqID
	ht.streamRespMu.Lock()
	ht.streamResp[key] = &streamResponseSession{tcpConn: w, clientID: "c", subDomain: "s"}
	ht.streamRespMu.Unlock()

	head := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
	errCh := make(chan error, 1)
	go func() {
		buf := make([]byte, len(head))
		_, err := io.ReadFull(r, buf)
		if err != nil {
			errCh <- err
			return
		}
		if string(buf) != string(head) {
			errCh <- fmt.Errorf("read %q want %q", buf, head)
			return
		}
		errCh <- nil
	}()

	if !ht.dispatchSemanticClientResponse(tcpID, reqID, uint8(protocol.MessageTypeHTTPResponseHead), head, true) {
		_ = w.Close()
		<-errCh
		t.Fatal("dispatch returned false")
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	ht.streamRespMu.Lock()
	_, exists := ht.streamResp[key]
	ht.streamRespMu.Unlock()
	if exists {
		t.Fatal("stream session should be removed after FIN head")
	}
}

func TestHTTPTunnelSemanticStreamPOSTReassemblesHeadAndBody(t *testing.T) {
	ctx := testTunnelContext()
	ad := &streamRecordAdapter{}
	ad.callbacks = ctx.CallbackContainer
	baseURL, ht, cleanup := startTunnelHTTPServer(t, ctx, "example.com")
	defer cleanup()
	ad.onComplete = func(tcpID, reqID string) {
		raw := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok")
		if !ht.dispatchSemanticClientResponse(tcpID, reqID, uint8(protocol.MessageTypeHTTPResponseHead), raw, true) {
			t.Error("dispatchSemanticClientResponse expected true")
		}
	}
	registerAppExampleMapping(t, ctx, ad)
	host := strings.TrimPrefix(baseURL, "http://")
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	payload := "hello"
	fmt.Fprintf(conn, "POST /p HTTP/1.1\r\nHost: app.example.com\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		len(payload), payload)
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}

	ad.mu.Lock()
	defer ad.mu.Unlock()
	if len(ad.heads) != 1 {
		t.Fatalf("expected 1 head frame, got %d", len(ad.heads))
	}
	if !strings.Contains(string(ad.heads[0]), "Host: app.example.com") {
		t.Fatalf("head missing Host: %q", ad.heads[0])
	}
	var sb strings.Builder
	sb.Write(ad.heads[0])
	for _, b := range ad.bodies {
		sb.Write(b)
	}
	full := sb.String()
	if !strings.Contains(full, "POST /p ") || !strings.Contains(full, payload) {
		t.Fatalf("reassembled request missing POST or body: %q", full)
	}
	if len(ad.bodyFins) == 0 || !ad.bodyFins[len(ad.bodyFins)-1] {
		t.Fatalf("expected last body chunk FIN, got %v", ad.bodyFins)
	}
}
