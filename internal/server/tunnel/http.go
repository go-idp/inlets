package tunnel

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/protocol"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/go-zoox/logger"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// HTTPTunnel handles HTTP tunnel requests
type HTTPTunnel struct {
	ctx          *types.Context
	domain       string
	subDomainRe  *regexp.Regexp
	requestCount int64
	mu           sync.Mutex
	streamRespMu sync.Mutex
	streamResp   map[string]*streamResponseSession
}

type streamResponseSession struct {
	tcpConn   net.Conn
	clientID  string
	subDomain string
	timer     *time.Timer
}

const requestResponseTimeout = 60 * time.Second

// maxHTTPTunnelRequestBodyBytes caps how much of each HTTP request body is buffered for tunneling
// (full buffering is required by the current wire format). Tests may lower this value temporarily.
var maxHTTPTunnelRequestBodyBytes = int64(32 << 20)

var errTunnelRequestBodyTooLarge = errors.New("tunnel: request body exceeds limit")

func readTunnelRequestBody(body io.ReadCloser, max int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	defer body.Close()
	limited := io.LimitReader(body, max+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, errTunnelRequestBodyTooLarge
	}
	return data, nil
}

// appendForwardedHTTPRequest writes the HTTP/1.x request line, Host (if any), other headers, CRLF, and body.
// Incoming server-side requests store Host in req.Host; it is not included in req.Header.Write.
func appendForwardedHTTPRequest(b *strings.Builder, req *http.Request, body []byte) {
	b.WriteString(fmt.Sprintf("%s %s %s\r\n", req.Method, req.URL.RequestURI(), req.Proto))
	if host := req.Host; host != "" {
		b.WriteString("Host: ")
		b.WriteString(host)
		b.WriteString("\r\n")
	}
	req.Header.Write(b)
	b.WriteString("\r\n")
	if len(body) > 0 {
		b.Write(body)
	}
}

// CreateHTTPTunnel creates a new HTTP tunnel
func CreateHTTPTunnel(ctx *types.Context, domain string) *HTTPTunnel {
	domain = strings.TrimSpace(domain)
	// Escape domain for regex
	escapedDomain := regexp.QuoteMeta(domain)
	subDomainRe := regexp.MustCompile(fmt.Sprintf(`([^.]+)\.%s`, escapedDomain))

	return &HTTPTunnel{
		ctx:         ctx,
		domain:      domain,
		subDomainRe: subDomainRe,
		streamResp:  make(map[string]*streamResponseSession),
	}
}

func appendForwardedHTTPHeadersOnly(b *strings.Builder, req *http.Request) {
	b.WriteString(fmt.Sprintf("%s %s %s\r\n", req.Method, req.URL.RequestURI(), req.Proto))
	if host := req.Host; host != "" {
		b.WriteString("Host: ")
		b.WriteString(host)
		b.WriteString("\r\n")
	}
	req.Header.Write(b)
	b.WriteString("\r\n")
}

func canSemanticStreamRequestBody(req *http.Request) bool {
	te := req.Header.Get("Transfer-Encoding")
	if strings.Contains(strings.ToLower(te), "chunked") {
		return false
	}
	return req.ContentLength >= 0
}

func hasConfiguredHTTPAuths(dm *types.DomainMapping) bool {
	return dm != nil && len(dm.HTTPAuths) > 0
}

func matchesAuthorizationHeader(authHeader string, auth client.HTTPTunnelAuth) bool {
	header := strings.TrimSpace(authHeader)
	if header == "" {
		return false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(parts[0]))
	value := strings.TrimSpace(parts[1])

	switch strings.ToLower(strings.TrimSpace(auth.Type)) {
	case "basic":
		if auth.Username == "" {
			return false
		}
		expect := base64.StdEncoding.EncodeToString([]byte(auth.Username + ":" + auth.Password))
		return scheme == "basic" && value == expect
	case "bearer":
		token := strings.TrimSpace(auth.Token)
		return scheme == "bearer" && token != "" && value == token
	default:
		return false
	}
}

func isHTTPRequestAuthorized(req *http.Request, auths []client.HTTPTunnelAuth) bool {
	if len(auths) == 0 {
		return true
	}
	got := req.Header.Get("Authorization")
	if strings.TrimSpace(got) == "" {
		return false
	}
	for i := range auths {
		if matchesAuthorizationHeader(got, auths[i]) {
			return true
		}
	}
	return false
}

func writeUnauthorized(tcpConn net.Conn, auths []client.HTTPTunnelAuth) {
	msg := "Unauthorized"
	hasBasic := false
	hasBearer := false
	for _, a := range auths {
		switch strings.ToLower(strings.TrimSpace(a.Type)) {
		case "basic":
			hasBasic = true
		case "bearer":
			hasBearer = true
		}
	}

	headers := []string{
		"HTTP/1.1 401 Unauthorized",
		"Content-Type: text/plain; charset=utf-8",
		fmt.Sprintf("Content-Length: %d", len(msg)),
		fmt.Sprintf("Date: %s", time.Now().UTC().Format(http.TimeFormat)),
		"Connection: keep-alive",
	}
	if hasBasic {
		headers = append(headers, `WWW-Authenticate: Basic realm="inlets"`)
	}
	if hasBearer {
		headers = append(headers, "WWW-Authenticate: Bearer")
	}
	headers = append(headers, "", msg)
	_, _ = tcpConn.Write([]byte(strings.Join(headers, "\r\n")))
}

func bodyStreamNegotiated(dm *types.DomainMapping) bool {
	if dm == nil || !dm.UseNewProtocol || dm.Adapter == nil {
		return false
	}
	pa, ok := dm.Adapter.(protocol.ProtocolAdapter)
	if !ok {
		return false
	}
	return pa.NegotiatedFlags()&client.CapabilityFlagHTTPBodyStream != 0
}

func (t *HTTPTunnel) subDomainFromRequest(r *http.Request) string {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if m := t.subDomainRe.FindStringSubmatch(host); len(m) > 1 {
		return m[1]
	}
	return ""
}

func (t *HTTPTunnel) initSocketConfig(sc *socketConfig, req *http.Request) {
	if sc.domain != "" {
		return
	}
	host := req.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	sc.domain = host
	if req.URL.Path == t.ctx.Config.WSPath {
		sc.isWS = true
		return
	}
	if sc.domain != "" {
		if m := t.subDomainRe.FindStringSubmatch(sc.domain); len(m) > 1 {
			sc.subDomain = m[1]
		}
	}
}

func (t *HTTPTunnel) shouldStreamHTTPRequest(sc *socketConfig, req *http.Request) bool {
	if sc.subDomain == "" {
		return false
	}
	dm := t.ctx.DomainMappings.Get(sc.subDomain)
	if hasConfiguredHTTPAuths(dm) {
		return false
	}
	return bodyStreamNegotiated(dm) && canSemanticStreamRequestBody(req)
}

func (t *HTTPTunnel) finishStreamResponseSession(tcpID, requestID string) {
	key := tcpID + ":" + requestID
	t.streamRespMu.Lock()
	delete(t.streamResp, key)
	t.streamRespMu.Unlock()
	t.ctx.CallbackContainer.Take(tcpID, requestID)
}

// dispatchSemanticClientResponse writes semantic-streaming HTTP response frames to the browser connection.
func (t *HTTPTunnel) dispatchSemanticClientResponse(tcpID, requestID string, msgType uint8, payload []byte, fin bool) bool {
	key := tcpID + ":" + requestID
	t.streamRespMu.Lock()
	sess, ok := t.streamResp[key]
	t.streamRespMu.Unlock()
	if !ok || sess == nil {
		return false
	}
	if sess.timer != nil {
		sess.timer.Stop()
		sess.timer = nil
	}
	clientID := sess.clientID
	sub := sess.subDomain
	conn := sess.tcpConn
	n := int64(len(payload))
	if !t.ctx.BandwidthLimiter.CheckDownload(clientID, n) {
		logger.Infof("[tunnel:http][%s] Download bandwidth limit exceeded for client: %s", sub, clientID)
		destroyConnection(conn)
		t.finishStreamResponseSession(tcpID, requestID)
		return true
	}
	t.ctx.TrafficStats.AddDownloadBytes(clientID, n)
	if _, err := conn.Write(payload); err != nil {
		logger.Infof("[tunnel:http] Failed to write streaming response: %v", err)
		destroyConnection(conn)
		t.finishStreamResponseSession(tcpID, requestID)
		return true
	}
	if msgType == uint8(protocol.MessageTypeHTTPResponseHead) && fin {
		t.finishStreamResponseSession(tcpID, requestID)
		return true
	}
	if msgType == uint8(protocol.MessageTypeHTTPResponseBody) && fin {
		t.finishStreamResponseSession(tcpID, requestID)
	}
	return true
}

// ServeMuxFor returns the *http.ServeMux that srv dispatches to, or nil if srv uses a non-ServeMux Handler.
// When Handler is nil, net/http uses DefaultServeMux at serve time; we register there to match that behavior.
func ServeMuxFor(srv *http.Server) *http.ServeMux {
	if srv == nil {
		return http.DefaultServeMux
	}
	if mux, ok := srv.Handler.(*http.ServeMux); ok && mux != nil {
		return mux
	}
	if srv.Handler == nil {
		return http.DefaultServeMux
	}
	return nil
}

// Attach registers the HTTP tunnel on the same *http.ServeMux as server (server.Handler must be that mux or nil).
func (t *HTTPTunnel) Attach(server *http.Server) {
	mux := ServeMuxFor(server)
	if mux == nil {
		logger.Infof("[tunnel:http] Attach: server.Handler is %T, not *http.ServeMux; HTTP tunnel not registered", server.Handler)
		return
	}
	t.ctx.HTTPStreamDispatch = t.dispatchSemanticClientResponse

	// DON'T use ConnState - it interferes with WebSocket upgrades
	// Instead, use HTTP handlers to process requests
	// The WebSocket handler (registered first) will handle WebSocket upgrades
	// This catch-all handler will handle all other requests

	// Add a catch-all handler that processes non-WebSocket requests
	// IMPORTANT: This must be registered AFTER WebSocket handler
	// WebSocket handler is registered in wsMonitor.Attach() which is called first
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Skip WebSocket path - it's handled by WebSocket monitor
		if r.URL.Path == t.ctx.Config.WSPath {
			// This shouldn't happen if WebSocket handler is registered correctly
			// But just in case, return 404
			http.NotFound(w, r)
			return
		}

		// For other requests, hijack the connection to handle at TCP level
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "HTTP/1.1 required", http.StatusHTTPVersionNotSupported)
			return
		}

		sub := t.subDomainFromRequest(r)
		dm := t.ctx.DomainMappings.Get(sub)
		useStream := dm != nil && !hasConfiguredHTTPAuths(dm) && bodyStreamNegotiated(dm) && canSemanticStreamRequestBody(r)

		var firstData string
		if useStream {
			var hb strings.Builder
			appendForwardedHTTPHeadersOnly(&hb, r)
			firstData = hb.String()
		} else {
			bodyData, err := readTunnelRequestBody(r.Body, maxHTTPTunnelRequestBodyBytes)
			if err != nil {
				if errors.Is(err, errTunnelRequestBodyTooLarge) {
					http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
					return
				}
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			var firstReq strings.Builder
			appendForwardedHTTPRequest(&firstReq, r, bodyData)
			firstData = firstReq.String()
		}

		conn, rw, err := hijacker.Hijack()
		if err != nil {
			logger.Infof("[tunnel:http] Failed to hijack connection: %v", err)
			return
		}

		var br *bufio.Reader
		if rw != nil && rw.Reader != nil {
			br = rw.Reader
		} else {
			br = bufio.NewReader(conn)
		}

		if useStream {
			go t.handleConnection(conn, br, r, firstData, true, r.ContentLength)
		} else {
			go t.handleConnection(conn, br, r, firstData, false, 0)
		}
	})
}

// handleConnection handles a hijacked TCP connection. firstReq/firstData are the request already
// parsed by net/http; br must be the Hijack buffer reader (or a new reader) for subsequent keep-alive reads.
// When streamFirst is true, firstData is headers-only and streamCL is Content-Length for the first request body (may be 0).
func (t *HTTPTunnel) handleConnection(tcpConn net.Conn, br *bufio.Reader, firstReq *http.Request, firstData string, streamFirst bool, streamCL int64) {
	socketConfig := &socketConfig{
		tcpID:     uuid.New().String(),
		domain:    "",
		subDomain: "",
		isWS:      false,
		clientID:  "",
	}
	defer func() {
		tcpConn.Close()
		t.ctx.CallbackContainer.Remove(socketConfig.tcpID)
		t.streamRespMu.Lock()
		for k, sess := range t.streamResp {
			if strings.HasPrefix(k, socketConfig.tcpID+":") {
				if sess != nil && sess.timer != nil {
					sess.timer.Stop()
				}
				delete(t.streamResp, k)
			}
		}
		t.streamRespMu.Unlock()

		if socketConfig.clientID != "" {
			statsInfo := t.ctx.TrafficStats.FormatStats(socketConfig.clientID)
			subDomainInfo := ""
			if socketConfig.subDomain != "" {
				subDomainInfo = fmt.Sprintf("[%s]", socketConfig.subDomain)
			}
			logger.Infof("[tunnel:http]%s connection closed - Traffic Stats: %s", subDomainInfo, statsInfo)
		}
	}()

	t.initSocketConfig(socketConfig, firstReq)
	if socketConfig.isWS {
		return
	}

	processOneBuffered := func(req *http.Request, data string) bool {
		if err := t.processRequest(tcpConn, socketConfig, data, req); err != nil {
			logger.Infof("[tunnel:http] Error processing request: %v", err)
		}
		if req.Close {
			return false
		}
		return true
	}

	processOne := func(req *http.Request, br *bufio.Reader) bool {
		if t.shouldStreamHTTPRequest(socketConfig, req) {
			var hb strings.Builder
			appendForwardedHTTPHeadersOnly(&hb, req)
			head := hb.String()
			if err := t.processRequestStream(tcpConn, br, socketConfig, head, req, req.ContentLength, false); err != nil {
				logger.Infof("[tunnel:http] Error processing request: %v", err)
			}
		} else {
			bodyData, err := readTunnelRequestBody(req.Body, maxHTTPTunnelRequestBodyBytes)
			if err != nil {
				if errors.Is(err, errTunnelRequestBodyTooLarge) {
					writeRequestEntityTooLarge(tcpConn)
				} else {
					logger.Infof("[tunnel:http] Error reading request body: %v", err)
				}
				return false
			}
			var full strings.Builder
			appendForwardedHTTPRequest(&full, req, bodyData)
			if err := t.processRequest(tcpConn, socketConfig, full.String(), req); err != nil {
				logger.Infof("[tunnel:http] Error processing request: %v", err)
			}
		}
		if req.Close {
			return false
		}
		return true
	}

	if firstReq != nil {
		if streamFirst {
			if err := t.processRequestStream(tcpConn, br, socketConfig, firstData, firstReq, streamCL, true); err != nil {
				logger.Infof("[tunnel:http] Error processing request: %v", err)
			}
			if firstReq.Close {
				return
			}
		} else {
			if !processOneBuffered(firstReq, firstData) {
				return
			}
		}
	}

	for {
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		t.initSocketConfig(socketConfig, req)
		if socketConfig.isWS {
			return
		}
		if !processOne(req, br) {
			return
		}
	}
}

// socketConfig holds configuration for a socket connection
type socketConfig struct {
	tcpID     string
	domain    string
	subDomain string
	isWS      bool
	clientID  string
}

// processRequestStream sends HTTP via semantic streaming (head + body chunks). readBodyFromBR is true for the first request after Hijack.
func (t *HTTPTunnel) processRequestStream(tcpConn net.Conn, br *bufio.Reader, socketConfig *socketConfig, head string, req *http.Request, contentLength int64, readBodyFromBR bool) error {
	t.mu.Lock()
	t.requestCount++
	requestCount := t.requestCount
	t.mu.Unlock()

	if socketConfig.isWS {
		return nil
	}

	requestLog := fmt.Sprintf("[%s.%s][request: %d]", socketConfig.subDomain, t.domain, requestCount)

	if socketConfig.subDomain == "" {
		logger.Infof("[404]%s", requestLog)
		destroyConnection(tcpConn)
		return fmt.Errorf("no subdomain found")
	}

	domainMapping := t.ctx.DomainMappings.Get(socketConfig.subDomain)
	if domainMapping == nil {
		logger.Infof("[404]%s", requestLog)
		destroyConnection(tcpConn)
		return fmt.Errorf("domain mapping not found")
	}
	if !isHTTPRequestAuthorized(req, domainMapping.HTTPAuths) {
		logger.Infof("[401]%s", requestLog)
		if contentLength > 0 {
			if readBodyFromBR {
				_, _ = io.CopyN(io.Discard, br, contentLength)
			} else if req.Body != nil {
				_, _ = io.Copy(io.Discard, io.LimitReader(req.Body, contentLength))
				_ = req.Body.Close()
			}
		}
		writeUnauthorized(tcpConn, domainMapping.HTTPAuths)
		return nil
	}

	tcpConnPtr := &tcpConn
	t.ctx.DomainMappings.BindTCP(socketConfig.subDomain, tcpConnPtr)

	wsConn := domainMapping.WSSocket
	if wsConn == nil {
		logger.Infof("[404]%s", requestLog)
		destroyConnection(tcpConn)
		return fmt.Errorf("websocket connection not found")
	}

	logger.Infof("%s", requestLog)

	requestID := fmt.Sprintf("%s@%d", uuid.New().String(), time.Now().UnixMilli())
	id := fmt.Sprintf("%s:%s", socketConfig.tcpID, requestID)

	clientID := domainMapping.ClientID
	useNewProtocol := domainMapping.UseNewProtocol

	var adapter protocol.ProtocolAdapter
	if domainMapping.Adapter != nil {
		if a, ok := domainMapping.Adapter.(protocol.ProtocolAdapter); ok {
			adapter = a
		}
	}

	socketConfig.clientID = clientID

	requestBytes := int64(len(head)) + contentLength
	if contentLength < 0 {
		requestBytes = int64(len(head))
	}

	if !t.ctx.BandwidthLimiter.CheckUpload(clientID, requestBytes) {
		logger.Infof("[tunnel:http][%s] Upload bandwidth limit exceeded for client: %s", socketConfig.subDomain, clientID)
		destroyConnection(tcpConn)
		return fmt.Errorf("upload bandwidth limit exceeded")
	}

	t.ctx.TrafficStats.AddRequest(clientID)
	t.ctx.TrafficStats.AddUploadBytes(clientID, requestBytes)

	streamKey := socketConfig.tcpID + ":" + requestID
	var timeoutTimer *time.Timer

	wrappedCallback := func(responseData string) {
		if timeoutTimer != nil {
			timeoutTimer.Stop()
		}
		responseBytes, err := base64.StdEncoding.DecodeString(responseData)
		if err != nil {
			responseBytes = []byte(responseData)
		}
		responseBytesLen := int64(len(responseBytes))
		if !t.ctx.BandwidthLimiter.CheckDownload(clientID, responseBytesLen) {
			logger.Infof("[tunnel:http][%s] Download bandwidth limit exceeded for client: %s", socketConfig.subDomain, clientID)
			destroyConnection(tcpConn)
			return
		}
		t.ctx.TrafficStats.AddDownloadBytes(clientID, responseBytesLen)
		if _, err := tcpConn.Write(responseBytes); err != nil {
			logger.Infof("[tunnel:http] Failed to write response: %v", err)
			destroyConnection(tcpConn)
		}
	}

	t.ctx.CallbackContainer.Set(socketConfig.tcpID, requestID, wrappedCallback)

	sess := &streamResponseSession{tcpConn: tcpConn, clientID: clientID, subDomain: socketConfig.subDomain}
	t.streamRespMu.Lock()
	t.streamResp[streamKey] = sess
	t.streamRespMu.Unlock()

	timeoutTimer = time.AfterFunc(requestResponseTimeout, func() {
		t.streamRespMu.Lock()
		delete(t.streamResp, streamKey)
		t.streamRespMu.Unlock()
		timeoutCallback := t.ctx.CallbackContainer.Take(socketConfig.tcpID, requestID)
		if timeoutCallback != nil {
			logger.Infof("[tunnel:http][%s] request timeout (id: %s)", socketConfig.subDomain, id)
			timeoutCallback(buildGatewayTimeoutResponse())
		}
	})
	sess.timer = timeoutTimer

	sendErr := func(err error) error {
		if timeoutTimer != nil {
			timeoutTimer.Stop()
		}
		t.streamRespMu.Lock()
		delete(t.streamResp, streamKey)
		t.streamRespMu.Unlock()
		t.ctx.CallbackContainer.Take(socketConfig.tcpID, requestID)
		logger.Infof("[tunnel:http] Failed to send HTTP request: %v", err)
		destroyConnection(tcpConn)
		return err
	}

	if !useNewProtocol || adapter == nil {
		return sendErr(fmt.Errorf("semantic streaming requires new protocol adapter"))
	}

	headFin := contentLength == 0
	if err := adapter.SendHTTPRequestHead(id, []byte(head), headFin); err != nil {
		return sendErr(err)
	}

	if contentLength > 0 {
		var bodyReader io.Reader
		if readBodyFromBR {
			bodyReader = io.LimitReader(br, contentLength)
		} else {
			bodyReader = io.LimitReader(req.Body, contentLength)
		}
		buf := make([]byte, 32*1024)
		var written int64
		for written < contentLength {
			toRead := int64(len(buf))
			rem := contentLength - written
			if rem < toRead {
				toRead = rem
			}
			n, err := bodyReader.Read(buf[:toRead])
			if n > 0 {
				written += int64(n)
				fin := written >= contentLength
				if err := adapter.SendHTTPRequestBody(id, buf[:n], fin); err != nil {
					if !readBodyFromBR && req.Body != nil {
						_ = req.Body.Close()
					}
					return sendErr(err)
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					if written < contentLength {
						if !readBodyFromBR && req.Body != nil {
							_ = req.Body.Close()
						}
						return sendErr(fmt.Errorf("unexpected EOF reading request body"))
					}
					break
				}
				if !readBodyFromBR && req.Body != nil {
					_ = req.Body.Close()
				}
				return sendErr(err)
			}
		}
		if !readBodyFromBR && req.Body != nil {
			_ = req.Body.Close()
		}
	}

	return nil
}

// processRequest processes an HTTP request
func (t *HTTPTunnel) processRequest(tcpConn net.Conn, socketConfig *socketConfig, data string, req *http.Request) error {
	t.mu.Lock()
	t.requestCount++
	requestCount := t.requestCount
	t.mu.Unlock()

	// Skip if this is a WebSocket connection
	if socketConfig.isWS {
		return nil
	}

	// Parse HTTP request to extract Host header
	if socketConfig.domain == "" {
		host := req.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		socketConfig.domain = host
		// Check if this is a WebSocket upgrade request
		if req.URL.Path == t.ctx.Config.WSPath {
			socketConfig.isWS = true
			return nil
		}

		// Extract subdomain
		if socketConfig.domain != "" {
			matches := t.subDomainRe.FindStringSubmatch(socketConfig.domain)
			if len(matches) > 1 {
				socketConfig.subDomain = matches[1]
			}
		}
	}

	requestLog := fmt.Sprintf("[%s.%s][request: %d]", socketConfig.subDomain, t.domain, requestCount)

	if socketConfig.subDomain == "" {
		logger.Infof("[404]%s", requestLog)
		destroyConnection(tcpConn)
		return fmt.Errorf("no subdomain found")
	}

	// Get domain mapping
	domainMapping := t.ctx.DomainMappings.Get(socketConfig.subDomain)
	if domainMapping == nil {
		logger.Infof("[404]%s", requestLog)
		destroyConnection(tcpConn)
		return fmt.Errorf("domain mapping not found")
	}
	if !isHTTPRequestAuthorized(req, domainMapping.HTTPAuths) {
		logger.Infof("[401]%s", requestLog)
		writeUnauthorized(tcpConn, domainMapping.HTTPAuths)
		return nil
	}

	// Bind TCP socket to domain mapping
	tcpConnPtr := &tcpConn
	t.ctx.DomainMappings.BindTCP(socketConfig.subDomain, tcpConnPtr)

	wsConn := domainMapping.WSSocket
	if wsConn == nil {
		logger.Infof("[404]%s", requestLog)
		destroyConnection(tcpConn)
		return fmt.Errorf("websocket connection not found")
	}

	logger.Infof("%s", requestLog)

	// Generate request ID
	requestID := fmt.Sprintf("%s@%d", uuid.New().String(), time.Now().UnixMilli())
	id := fmt.Sprintf("%s:%s", socketConfig.tcpID, requestID)

	// Get clientId and adapter from domain mapping
	clientID := domainMapping.ClientID
	useNewProtocol := domainMapping.UseNewProtocol

	// Get adapter (stored as interface{}, need to cast)
	var adapter protocol.ProtocolAdapter
	if domainMapping.Adapter != nil {
		if a, ok := domainMapping.Adapter.(protocol.ProtocolAdapter); ok {
			adapter = a
		}
	}

	socketConfig.clientID = clientID

	// Calculate request bytes
	requestBytes := int64(len(data))

	// Check upload bandwidth limit
	if !t.ctx.BandwidthLimiter.CheckUpload(clientID, requestBytes) {
		logger.Infof("[tunnel:http][%s] Upload bandwidth limit exceeded for client: %s", socketConfig.subDomain, clientID)
		destroyConnection(tcpConn)
		return fmt.Errorf("upload bandwidth limit exceeded")
	}

	// Add request and upload bytes to stats
	t.ctx.TrafficStats.AddRequest(clientID)
	t.ctx.TrafficStats.AddUploadBytes(clientID, requestBytes)

	// Create callback for response before sending request to avoid race where
	// a fast response arrives before callback registration.
	var timeoutTimer *time.Timer
	wrappedCallback := func(responseData string) {
		if timeoutTimer != nil {
			timeoutTimer.Stop()
		}

		// Calculate response bytes
		responseBytes, err := base64.StdEncoding.DecodeString(responseData)
		if err != nil {
			// If decode fails, use string length as approximation
			responseBytes = []byte(responseData)
		}

		responseBytesLen := int64(len(responseBytes))

		// Check download bandwidth limit
		if !t.ctx.BandwidthLimiter.CheckDownload(clientID, responseBytesLen) {
			logger.Infof("[tunnel:http][%s] Download bandwidth limit exceeded for client: %s", socketConfig.subDomain, clientID)
			destroyConnection(tcpConn)
			return
		}

		// Add download bytes to stats
		t.ctx.TrafficStats.AddDownloadBytes(clientID, responseBytesLen)

		// Write response to TCP connection
		if _, err := tcpConn.Write(responseBytes); err != nil {
			logger.Infof("[tunnel:http] Failed to write response: %v", err)
			destroyConnection(tcpConn)
			return
		}
	}

	// Store callback before sending request.
	t.ctx.CallbackContainer.Set(socketConfig.tcpID, requestID, wrappedCallback)
	timeoutTimer = time.AfterFunc(requestResponseTimeout, func() {
		// Ensure timeout response is sent at most once.
		timeoutCallback := t.ctx.CallbackContainer.Take(socketConfig.tcpID, requestID)
		if timeoutCallback != nil {
			logger.Infof("[tunnel:http][%s] request timeout (id: %s)", socketConfig.subDomain, id)
			timeoutCallback(buildGatewayTimeoutResponse())
		}
	})

	// Send request
	sendErr := func(err error) error {
		if timeoutTimer != nil {
			timeoutTimer.Stop()
		}
		// Remove callback if request dispatch fails.
		t.ctx.CallbackContainer.Take(socketConfig.tcpID, requestID)
		logger.Infof("[tunnel:http] Failed to send HTTP request: %v", err)
		destroyConnection(tcpConn)
		return err
	}

	if useNewProtocol && adapter != nil {
		// New protocol: use adapter
		dataBytes := []byte(data)
		if err := adapter.SendHTTPRequest(id, dataBytes); err != nil {
			return sendErr(err)
		}
	} else {
		// Legacy protocol: use adapter if available, otherwise send directly
		if adapter != nil {
			// Use adapter even for legacy protocol
			dataBytes := []byte(data)
			if err := adapter.SendHTTPRequest(id, dataBytes); err != nil {
				return sendErr(err)
			}
		} else {
			// Fallback: send directly via WebSocket (legacy format)
			// This should not happen if adapter is properly set up
			message := []interface{}{
				"request",
				map[string]interface{}{
					"id":   id,
					"data": data, // Legacy protocol expects processed data
				},
			}

			messageBytes, err := json.Marshal(message)
			if err != nil {
				return sendErr(err)
			}

			if err := wsConn.WriteMessage(websocket.TextMessage, messageBytes); err != nil {
				return sendErr(err)
			}
		}
	}

	return nil
}

// destroyConnection sends 404 response and closes connection
func destroyConnection(tcpConn net.Conn) {
	response := []string{
		"HTTP/1.1 404 Not Found",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Length: 9",
		fmt.Sprintf("Date: %s", time.Now().UTC().Format(http.TimeFormat)),
		"Connection: close",
		"",
		"Not Found",
	}

	responseStr := strings.Join(response, "\r\n")
	tcpConn.Write([]byte(responseStr))
	tcpConn.Close()
}

func writeRequestEntityTooLarge(tcpConn net.Conn) {
	const msg = "Request Entity Too Large"
	response := []string{
		"HTTP/1.1 413 Request Entity Too Large",
		"Content-Type: text/plain; charset=utf-8",
		fmt.Sprintf("Content-Length: %d", len(msg)),
		fmt.Sprintf("Date: %s", time.Now().UTC().Format(http.TimeFormat)),
		"Connection: close",
		"",
		msg,
	}
	_, _ = tcpConn.Write([]byte(strings.Join(response, "\r\n")))
	_ = tcpConn.Close()
}

func buildGatewayTimeoutResponse() string {
	response := []string{
		"HTTP/1.1 504 Gateway Timeout",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Length: 15",
		fmt.Sprintf("Date: %s", time.Now().UTC().Format(http.TimeFormat)),
		"Connection: close",
		"",
		"Gateway Timeout",
	}

	return strings.Join(response, "\r\n")
}
