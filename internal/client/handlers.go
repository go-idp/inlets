package client

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/gorilla/websocket"
)

const upstreamRequestTimeout = 60 * time.Second

// maxUpBufferedResponse caps in-memory buffering for non-chunked upstream HTTP responses in the
// streaming tunnel path. Chunked encoding uses the dechunker stream instead of ReadAll.
const maxUpBufferedResponse = 10 << 20

const messageFlagFIN = 0x01

const (
	binaryMessageTypeTCPClose uint8 = 0x05
	upstreamTCPDialTimeout          = 10 * time.Second
)

type httpStreamSession struct {
	conn          net.Conn
	contentLength int64
}

// tunnelHTTPRequestLine returns the first line of a raw HTTP/1.x message for logging.
func tunnelHTTPRequestLine(raw string) string {
	if raw == "" {
		return "(empty)"
	}
	if i := strings.Index(raw, "\r\n"); i >= 0 {
		return strings.TrimSpace(raw[:i])
	}
	if i := strings.Index(raw, "\n"); i >= 0 {
		return strings.TrimSpace(raw[:i])
	}
	const max = 120
	if len(raw) > max {
		return raw[:max] + "..."
	}
	return raw
}

func (c *Client) handleAuthenticateResponse(payload interface{}) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var resp AuthenticateResponse
	if err := json.Unmarshal(dataBytes, &resp); err != nil {
		return err
	}

	if !resp.OK {
		c.authTimeout.Stop()
		return fmt.Errorf("authentication failed: %s", resp.Message)
	}

	c.authTimeout.Stop()
	c.logger.Printf("[authenticate] connected")

	// Save clientId and containerId from server response
	if resp.ClientId != "" {
		c.clientId = resp.ClientId
		c.logger.Printf("[authenticate] Client ID: %s", c.clientId)
	}
	if resp.ContainerId != "" {
		c.containerId = resp.ContainerId
		c.logger.Printf("[authenticate] Container ID: %s", c.containerId)
	}

	// Handle protocol negotiation
	if resp.Config != nil && resp.Config.NegotiatedCapabilities != nil {
		c.negotiatedCapabilities = resp.Config.NegotiatedCapabilities
		c.logger.Printf("[authenticate] Using new protocol")
		if resp.Config.NegotiatedCapabilities.Features != nil {
			if resp.Config.NegotiatedCapabilities.Features.Compression != nil {
				preferred := resp.Config.NegotiatedCapabilities.Features.Compression.Preferred
				if preferred == "" && len(resp.Config.NegotiatedCapabilities.Features.Compression.Algorithms) > 0 {
					preferred = resp.Config.NegotiatedCapabilities.Features.Compression.Algorithms[0]
				}
				c.logger.Printf("[authenticate] Negotiated compression: %v (preferred: %s)",
					resp.Config.NegotiatedCapabilities.Features.Compression.Algorithms, preferred)
			}
		}
	} else {
		c.negotiatedCapabilities = nil
		c.logger.Printf("[authenticate] Using legacy protocol")
	}

	if c.opts.Type == "http" {
		c.logger.Printf("Forwarding: %s -> %s://%s:%d", resp.URL, c.opts.Type, c.opts.UpstreamHost, c.opts.UpstreamPort)
	}

	c.spawnServerConfiguredTunnels(&resp)

	return nil
}

func (c *Client) spawnServerConfiguredTunnels(resp *AuthenticateResponse) {
	if c.opts.OpaqueChild || resp.Config == nil || len(resp.Config.Tunnels) == 0 {
		return
	}
	snap := AuthSnapshotFromOptions(c.opts)
	myIdx := -1
	if snap != nil {
		myIdx = MatchTunnelSpecIndex(snap, resp.Config.Tunnels)
	}
	for i := range resp.Config.Tunnels {
		if myIdx >= 0 && i == myIdx {
			continue
		}
		spec := resp.Config.Tunnels[i]
		childOpts, err := ChildOptionsFromSpec(c.opts, &spec)
		if err != nil {
			c.logger.Printf("[tunnels] skip %q: %v", spec.Name, err)
			continue
		}
		go func(sp TunnelSpec, o *Options) {
			c.logger.Printf("[tunnels] starting server-listed tunnel %q (%s)", sp.Name, sp.Type)
			cl := New(o)
			if err := cl.Run(); err != nil {
				c.logger.Printf("[tunnels] tunnel %q exited: %v", sp.Name, err)
			}
		}(spec, childOpts)
	}
}

func (c *Client) handleHTTPRequest(payload interface{}) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var reqData RequestData
	if err = json.Unmarshal(dataBytes, &reqData); err != nil {
		return err
	}
	if reqData.ID == "" {
		return nil
	}

	rawOuter, err := base64.StdEncoding.DecodeString(reqData.Data)
	if err != nil {
		c.logger.Printf("[tunnel:http] outer base64: %v", err)
		c.sendTunneledHTTPErrorResponse(reqData.ID, http.StatusBadGateway, "invalid tunnel request encoding")
		return err
	}

	msg, err := parseBinaryMessage(rawOuter)
	if err != nil {
		c.logger.Printf("[tunnel:http] parse binary: %v", err)
		c.sendTunneledHTTPErrorResponse(reqData.ID, http.StatusBadGateway, "invalid tunnel request encoding")
		return err
	}

	useStream := c.negotiatedCapabilities != nil && c.negotiatedCapabilities.Flags&CapabilityFlagHTTPBodyStream != 0
	if !useStream && msg.Type != binaryMessageTypeHTTPRequest {
		c.logger.Printf("[tunnel:http] unexpected message type %d", msg.Type)
		return fmt.Errorf("unexpected message type %d", msg.Type)
	}

	switch msg.Type {
	case binaryMessageTypeHTTPRequest:
		return c.dispatchFullHTTPRequest(reqData)
	case binaryMessageTypeHTTPRequestHead:
		return c.handleHTTPRequestStreamHead(reqData.ID, msg)
	case binaryMessageTypeHTTPRequestBody:
		return c.handleHTTPRequestStreamBody(reqData.ID, msg)
	default:
		return fmt.Errorf("unhandled tunnel message type %d", msg.Type)
	}
}

func (c *Client) dispatchFullHTTPRequest(reqData RequestData) error {
	var raw []byte
	var err error
	if c.negotiatedCapabilities != nil {
		raw, err = decodeNewProtocolHTTPRequestPayload(reqData.Data, c.negotiatedCapabilities)
	} else {
		raw, err = decodeLegacyHTTPRequestPayload(reqData.Data)
	}
	if err != nil {
		c.logger.Printf("[tunnel:http] failed to decode tunneled request: %v", err)
		if reqData.ID != "" {
			c.sendTunneledHTTPErrorResponse(reqData.ID, http.StatusBadGateway, "invalid tunnel request encoding")
		}
		return err
	}
	go c.forwardHTTPRequest(reqData.ID, string(raw))
	return nil
}

func parseContentLengthFromHeaders(head []byte) int64 {
	sc := bufio.NewScanner(bytes.NewReader(head))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			break
		}
		const prefix = "Content-Length:"
		if len(line) > len(prefix) && strings.EqualFold(line[:len(prefix)], prefix) {
			var n int64
			_, _ = fmt.Sscanf(strings.TrimSpace(line[len(prefix):]), "%d", &n)
			return n
		}
	}
	return -1
}

func (c *Client) handleHTTPRequestStreamHead(id string, msg BinaryMessage) error {
	head, err := decompressTunnelSemanticHead(msg.Data, c.negotiatedCapabilities)
	if err != nil {
		c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, "invalid tunnel request encoding")
		return err
	}
	fin := (msg.Flags & messageFlagFIN) != 0
	head = injectUpstreamBasicAuth(head, c.opts.UpstreamUsername, c.opts.UpstreamPassword)
	reqLine := tunnelHTTPRequestLine(string(head))

	c.httpStreamMu.Lock()
	if _, exists := c.httpStreamSess[id]; exists {
		c.httpStreamMu.Unlock()
		return fmt.Errorf("duplicate HTTP stream for id %s", id)
	}
	c.httpStreamMu.Unlock()

	upstreamAddr := joinHostPort(c.opts.UpstreamHost, c.opts.UpstreamPort)
	conn, err := net.DialTimeout("tcp", upstreamAddr, 10*time.Second)
	if err != nil {
		c.logger.Printf("[tunnel:http] id=%s %s upstream connect failed: %v", id, reqLine, err)
		c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, fmt.Sprintf("upstream unreachable: %v", err))
		return err
	}
	if err := conn.SetDeadline(time.Now().Add(upstreamRequestTimeout)); err != nil {
		conn.Close()
		c.sendTunneledHTTPErrorResponse(id, http.StatusInternalServerError, "upstream deadline error")
		return err
	}
	if _, err := conn.Write(head); err != nil {
		conn.Close()
		c.logger.Printf("[tunnel:http] id=%s upstream write failed: %v", id, err)
		c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, fmt.Sprintf("upstream write failed: %v", err))
		return err
	}

	if !fin {
		cl := parseContentLengthFromHeaders(head)
		c.httpStreamMu.Lock()
		c.httpStreamSess[id] = &httpStreamSession{conn: conn, contentLength: cl}
		c.httpStreamMu.Unlock()
		return nil
	}

	go c.readUpstreamAndStreamHTTPResponse(id, conn, reqLine)
	return nil
}

func (c *Client) handleHTTPRequestStreamBody(id string, msg BinaryMessage) error {
	c.httpStreamMu.Lock()
	sess, ok := c.httpStreamSess[id]
	if ok {
		delete(c.httpStreamSess, id)
	}
	c.httpStreamMu.Unlock()
	if !ok || sess == nil {
		c.logger.Printf("[tunnel:http] body chunk for unknown stream %s", id)
		return fmt.Errorf("unknown stream %s", id)
	}
	fin := (msg.Flags & messageFlagFIN) != 0
	if len(msg.Data) > 0 {
		if _, err := sess.conn.Write(msg.Data); err != nil {
			sess.conn.Close()
			c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, fmt.Sprintf("upstream write failed: %v", err))
			return err
		}
	}
	reqLine := tunnelHTTPRequestLine("(stream)")
	if !fin {
		c.httpStreamMu.Lock()
		c.httpStreamSess[id] = sess
		c.httpStreamMu.Unlock()
		return nil
	}
	go c.readUpstreamAndStreamHTTPResponse(id, sess.conn, reqLine)
	return nil
}

func (c *Client) maybeCompressSemanticResponseHead(payload []byte) ([]byte, error) {
	if c.negotiatedCapabilities == nil || c.negotiatedCapabilities.Flags&CapabilityFlagCompression == 0 || len(payload) == 0 {
		return payload, nil
	}
	alg := "brotli"
	if c.negotiatedCapabilities.Features != nil && c.negotiatedCapabilities.Features.Compression != nil && c.negotiatedCapabilities.Features.Compression.Preferred != "" {
		alg = strings.ToLower(c.negotiatedCapabilities.Features.Compression.Preferred)
	}
	switch alg {
	case "gzip":
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		if _, err := w.Write(payload); err != nil {
			_ = w.Close()
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	default:
		var buf bytes.Buffer
		w := brotli.NewWriter(&buf)
		if _, err := w.Write(payload); err != nil {
			_ = w.Close()
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
}

func (c *Client) sendSemanticHTTPResponse(msgType uint8, id string, payload []byte, fin bool) error {
	out := payload
	var err error
	if msgType == binaryMessageTypeHTTPResponseHead {
		out, err = c.maybeCompressSemanticResponseHead(payload)
		if err != nil {
			return err
		}
	}
	c.sequenceCounterMu.Lock()
	seq := c.sequenceCounter[id]
	c.sequenceCounter[id] = seq + 1
	c.sequenceCounterMu.Unlock()
	flags := uint8(0)
	if fin {
		flags |= messageFlagFIN
	}
	bin := buildBinaryMessage(BinaryMessage{Type: msgType, StreamID: id, Sequence: seq, Flags: flags, Data: out})
	b64 := base64.StdEncoding.EncodeToString(bin)
	return c.sendMonitorMessage("response", ResponseData{ID: id, Data: b64})
}

func (c *Client) readUpstreamAndStreamHTTPResponse(id string, upstream net.Conn, reqLine string) {
	defer upstream.Close()
	c.logger.Printf("[tunnel:http] request id=%s %s (streaming response)", id, reqLine)

	resp, err := http.ReadResponse(bufio.NewReader(upstream), nil)
	if err != nil {
		c.logger.Printf("[tunnel:http] id=%s upstream read response failed: %v", id, err)
		c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, fmt.Sprintf("upstream read failed: %v", err))
		return
	}
	defer resp.Body.Close()

	// Read full body (net/http de-chunks). We re-frame headers to drop Transfer-Encoding: chunked
	// and set Content-Length so the browser never sees chunked framing with de-chunked bytes.
	bodySlurp, err := io.ReadAll(io.LimitReader(resp.Body, maxUpBufferedResponse+1))
	if err != nil {
		c.logger.Printf("[tunnel:http] id=%s read upstream body: %v", id, err)
		c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, "upstream read failed: body")
		return
	}
	if len(bodySlurp) > maxUpBufferedResponse {
		c.logger.Printf("[tunnel:http] id=%s upstream body exceeds max buffer", id)
		c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, "upstream response too large for tunnel")
		return
	}
	headBytes, err := responseHeadForBufferedUpstreamBody(resp, bodySlurp)
	if err != nil {
		c.logger.Printf("[tunnel:http] id=%s build response head: %v", id, err)
		c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, "failed to frame upstream response")
		return
	}
	if len(bodySlurp) == 0 {
		if err := c.sendSemanticHTTPResponse(binaryMessageTypeHTTPResponseHead, id, headBytes, true); err != nil {
			c.logger.Printf("[tunnel:http] id=%s send response head failed: %v", id, err)
			return
		}
		c.logger.Printf("[tunnel:http] id=%s -> %s", id, resp.Status)
		return
	}
	if err := c.sendSemanticHTTPResponse(binaryMessageTypeHTTPResponseHead, id, headBytes, false); err != nil {
		c.logger.Printf("[tunnel:http] id=%s send response head failed: %v", id, err)
		return
	}
	const chunk = 32 * 1024
	for off := 0; off < len(bodySlurp); {
		end := off + chunk
		if end > len(bodySlurp) {
			end = len(bodySlurp)
		}
		piece := bodySlurp[off:end]
		off = end
		fin := off >= len(bodySlurp)
		if err := c.sendSemanticHTTPResponse(binaryMessageTypeHTTPResponseBody, id, piece, fin); err != nil {
			c.logger.Printf("[tunnel:http] id=%s send response body failed: %v", id, err)
			return
		}
	}
	c.logger.Printf("[tunnel:http] id=%s -> %s", id, resp.Status)
}

// tunneledRequestIsWebSocket returns true for a raw HTTP/1.1 request that looks like a WebSocket opening handshake.
func tunneledRequestIsWebSocket(raw string) bool {
	d := strings.ToLower(raw)
	if !strings.Contains(d, "upgrade: websocket") {
		return false
	}
	for _, line := range strings.Split(raw, "\n") {
		l := strings.TrimSpace(strings.ToLower(line))
		if strings.HasPrefix(l, "connection:") && strings.Contains(l, "upgrade") {
			return true
		}
	}
	return false
}

func (c *Client) forwardHTTPRequest(id string, data string) {
	reqLine := tunnelHTTPRequestLine(data)
	c.logger.Printf("[tunnel:http] request id=%s %s", id, reqLine)

	upstreamAddr := joinHostPort(c.opts.UpstreamHost, c.opts.UpstreamPort)
	conn, err := net.DialTimeout("tcp", upstreamAddr, 10*time.Second)
	if err != nil {
		c.logger.Printf("[tunnel:http] id=%s %s upstream connect failed: %v", id, reqLine, err)
		c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, fmt.Sprintf("upstream unreachable: %v", err))
		return
	}
	skipClose := false
	defer func() {
		if !skipClose {
			_ = conn.Close()
		}
	}()

	data = string(injectUpstreamBasicAuth([]byte(data), c.opts.UpstreamUsername, c.opts.UpstreamPassword))

	if err := conn.SetDeadline(time.Now().Add(upstreamRequestTimeout)); err != nil {
		c.logger.Printf("[tunnel:http] id=%s %s upstream deadline: %v", id, reqLine, err)
		c.sendTunneledHTTPErrorResponse(id, http.StatusInternalServerError, "upstream deadline error")
		return
	}

	if _, err := conn.Write([]byte(data)); err != nil {
		c.logger.Printf("[tunnel:http] id=%s %s upstream write failed: %v", id, reqLine, err)
		c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, fmt.Sprintf("upstream write failed: %v", err))
		return
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		c.logger.Printf("[tunnel:http] id=%s %s upstream read response failed: %v", id, reqLine, err)
		c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, fmt.Sprintf("upstream read failed: %v", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	var response bytes.Buffer
	if err := resp.Write(&response); err != nil {
		c.logger.Printf("[tunnel:http] id=%s %s serialize upstream response failed: %v", id, reqLine, err)
		c.sendTunneledHTTPErrorResponse(id, http.StatusBadGateway, "failed to read upstream response")
		return
	}

	compressed, err := compress(base64.StdEncoding.EncodeToString(response.Bytes()))
	if err != nil {
		c.logger.Printf("[tunnel:http] id=%s %s compress response failed: %v", id, reqLine, err)
		c.sendTunneledHTTPErrorResponse(id, http.StatusInternalServerError, "response encoding error")
		return
	}

	respData := ResponseData{
		ID:   id,
		Data: compressed,
	}

	if err := c.sendMonitorMessage("response", respData); err != nil {
		c.logger.Printf("[tunnel:http] id=%s %s send response to server failed: %v", id, reqLine, err)
		return
	}
	c.logger.Printf("[tunnel:http] id=%s %s -> %s", id, reqLine, resp.Status)

	useDataPlane := c.negotiatedCapabilities != nil && (c.negotiatedCapabilities.Flags&CapabilityFlagTCPOverWS) != 0
	if useDataPlane && resp.StatusCode == 101 && tunneledRequestIsWebSocket(data) && c.containerId != "" {
		parts := strings.SplitN(id, ":", 2)
		if len(parts) == 2 {
			streamID := fmt.Sprintf("%s:%s", c.containerId, parts[1])
			skipClose = true
			go c.runTCPDataChannelRelay(streamID, conn)
			return
		}
	}
}

// sendTunneledHTTPErrorResponse returns a minimal HTTP/1.1 error to the browser when upstream fails.
// Without this, the server tunnel waits until TCP timeout with no bytes (looks like "pending").
func (c *Client) sendTunneledHTTPErrorResponse(id string, code int, msg string) {
	if id == "" {
		return
	}
	if msg == "" {
		msg = http.StatusText(code)
	}
	if msg == "" {
		msg = "Error"
	}
	reason := http.StatusText(code)
	if reason == "" {
		reason = "Error"
	}
	body := []byte(msg)
	raw := fmt.Sprintf("HTTP/1.1 %d %s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		code, reason, len(body), string(body))

	compressed, err := compress(base64.StdEncoding.EncodeToString([]byte(raw)))
	if err != nil {
		return
	}
	if err := c.sendMonitorMessage("response", ResponseData{ID: id, Data: compressed}); err != nil {
		c.logger.Printf("Failed to send tunnel error response: %v", err)
	}
}

func (c *Client) handleTCPReady(payload interface{}) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var readyData TCPReadyData
	if err := json.Unmarshal(dataBytes, &readyData); err != nil {
		return err
	}

	c.logger.Printf("Forwarding: tcp://%s:%d -> %s:%d", readyData.Host, readyData.Port, c.opts.UpstreamHost, c.opts.UpstreamPort)
	return nil
}

func (c *Client) handleTCPConnect(payload interface{}, remoteHost string) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var connectData TCPConnectData
	if err := json.Unmarshal(dataBytes, &connectData); err != nil {
		return err
	}

	c.logger.Printf("[tunnel:tcp][user] connected (request id: %s, ip: %s)", connectData.RequestID, connectData.IP)

	// Check if using new protocol (TCP over WebSocket)
	useNewProtocol := c.negotiatedCapabilities != nil &&
		(c.negotiatedCapabilities.Flags&CapabilityFlagTCPOverWS) != 0

	if useNewProtocol {
		// New protocol: TCP over WebSocket
		// Register stream immediately to avoid race condition with tcp:data
		streamID := fmt.Sprintf("%s:%s", connectData.ID, connectData.RequestID)
		c.tcpStreamsMu.Lock()
		// Use nil as placeholder - will be replaced when connection is established
		// This ensures handleTCPData can find the stream even if data arrives before connection is ready
		c.tcpStreams[streamID] = nil
		c.tcpStreamsMu.Unlock()

		// Now establish connection asynchronously
		go c.forwardTCPConnectionOverWS(connectData.ID, connectData.RequestID, remoteHost)
	} else {
		// Legacy protocol: independent TCP connections
		go c.forwardTCPConnection(connectData.ID, connectData.RequestID, remoteHost)
	}

	return nil
}

func (c *Client) forwardTCPConnection(id, requestID, remoteHost string) {
	localAddr := joinHostPort(c.opts.UpstreamHost, c.opts.UpstreamPort)
	localConn, err := net.Dial("tcp", localAddr)
	if err != nil {
		c.logger.Printf("[local] failed to connect: %v", err)
		return
	}

	remoteAddr := joinHostPort(remoteHost, c.opts.RemoteTCPPort)
	remoteConn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		c.logger.Printf("[remote] failed to connect: %v", err)
		localConn.Close()
		return
	}

	var signedSecret string
	if c.opts.AuthType == "credentials" {
		signedSecret = c.opts.ClientSecret
	} else {
		signedSecret = c.opts.Token
	}

	signature := hmacSHA256(id, signedSecret)
	authData := fmt.Sprintf("%s%s%s%s", tunnelTCPFlag, id, requestID, signature)

	if _, err := remoteConn.Write([]byte(authData)); err != nil {
		c.logger.Printf("[remote] failed to send auth: %v", err)
		localConn.Close()
		remoteConn.Close()
		return
	}

	buffer := make([]byte, len(tunnelTCPOKFlag)+4096)
	n, err := remoteConn.Read(buffer)
	if err != nil {
		c.logger.Printf("[remote] failed to read auth response: %v", err)
		localConn.Close()
		remoteConn.Close()
		return
	}

	okFlag := string(buffer[:len(tunnelTCPOKFlag)])
	if okFlag != tunnelTCPOKFlag {
		if n > len(tunnelTCPOKFlag) {
			errorMsg := string(buffer[len(tunnelTCPOKFlag):n])
			c.logger.Printf("[remote] authentication error: %s", errorMsg)
		} else {
			c.logger.Printf("[remote] authentication error: expected %s, got %s", tunnelTCPOKFlag, okFlag)
		}
		localConn.Close()
		remoteConn.Close()
		return
	}

	c.logger.Printf("[remote] authenticated")

	if n > len(tunnelTCPOKFlag) {
		rest := buffer[len(tunnelTCPOKFlag):n]
		if _, err := localConn.Write(rest); err != nil {
			c.logger.Printf("[local] failed to write remaining data: %v", err)
		}
	}

	go pipeConn(localConn, remoteConn)
	go pipeConn(remoteConn, localConn)
}

func pipeConn(src, dst net.Conn) {
	defer src.Close()
	defer dst.Close()

	buffer := make([]byte, 4096)
	for {
		n, err := src.Read(buffer)
		if err != nil {
			return
		}
		if _, err := dst.Write(buffer[:n]); err != nil {
			return
		}
	}
}

// sendTCPStreamCloseNotify tells the server to tear down the user-facing TCP connection for this stream
// (e.g. local upstream dial failed). Without this, the server keeps the source socket open and clients hang.
func (c *Client) sendTCPStreamCloseNotify(streamID string) error {
	c.sequenceCounterMu.Lock()
	seq := c.sequenceCounter[streamID]
	c.sequenceCounter[streamID] = seq + 1
	c.sequenceCounterMu.Unlock()

	binaryMsg := buildBinaryMessage(BinaryMessage{
		Type:     binaryMessageTypeTCPClose,
		StreamID: streamID,
		Sequence: seq,
		Flags:    1, // FIN — single-frame control
		Data:     nil,
	})
	dataConn, writeMu := c.getDataChannel(streamID)
	if dataConn == nil {
		return fmt.Errorf("data channel not available for stream %s", streamID)
	}
	if writeMu != nil {
		writeMu.Lock()
	}
	err := dataConn.WriteMessage(websocket.BinaryMessage, binaryMsg)
	if writeMu != nil {
		writeMu.Unlock()
	}
	return err
}

// runTCPDataChannelRelay runs after the inlets control plane (monitor) is done with a single
// HTTP or TCP open: raw bytes to/from the server use the per-stream /_/data WebSocket and MessageTypeTCPData.
func (c *Client) runTCPDataChannelRelay(streamID string, localConn net.Conn) {
	c.tcpStreamsMu.Lock()
	c.tcpStreams[streamID] = localConn
	c.tcpStreamsMu.Unlock()

	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			c.tcpStreamsMu.Lock()
			_, exists := c.tcpStreams[streamID]
			if exists {
				delete(c.tcpStreams, streamID)
			}
			c.tcpStreamsMu.Unlock()
			_ = localConn.Close()
			c.removeDataChannel(streamID)
			c.logger.Printf("[tcp:data][%s] stream closed", streamID)
		})
	}

	go func() {
		defer cleanup()

		for i := 0; i < 50; i++ {
			if conn, _ := c.getDataChannel(streamID); conn != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		buffer := make([]byte, 4096)
		for {
			n, err := localConn.Read(buffer)
			if err != nil {
				errStr := err.Error()
				isNormalClose := false
				if errStr == "EOF" {
					isNormalClose = true
				}
				if opErr, ok := err.(*net.OpError); ok {
					if opErr.Err == syscall.ECONNRESET ||
						strings.Contains(errStr, "connection reset by peer") {
						isNormalClose = true
					}
				} else if strings.Contains(errStr, "connection reset by peer") {
					isNormalClose = true
				}
				if strings.Contains(errStr, "use of closed network connection") {
					isNormalClose = true
				}
				if isNormalClose {
					return
				}
				c.logger.Printf("[local][%s] read error: %v", streamID, err)
				return
			}
			if n == 0 {
				c.logger.Printf("[local][%s] connection closed (EOF)", streamID)
				return
			}
			c.sequenceCounterMu.Lock()
			seq := c.sequenceCounter[streamID]
			c.sequenceCounter[streamID] = seq + 1
			c.sequenceCounterMu.Unlock()
			binaryMsg := buildBinaryMessage(BinaryMessage{
				Type:     0x03,
				StreamID: streamID,
				Sequence: seq,
				Flags:    0x01,
				Data:     buffer[:n],
			})
			dataConn, writeMu := c.getDataChannel(streamID)
			if dataConn == nil {
				c.logger.Printf("[tcp:data][%s] Data connection is not ready", streamID)
				return
			}
			if writeMu != nil {
				writeMu.Lock()
			}
			err = dataConn.WriteMessage(websocket.BinaryMessage, binaryMsg)
			if writeMu != nil {
				writeMu.Unlock()
			}
			if err != nil {
				c.logger.Printf("[tcp:data][%s] Failed to send data to server: %v", streamID, err)
				return
			}
		}
	}()
}

// forwardTCPConnectionOverWS handles TCP connection using new protocol (TCP over WebSocket)
func (c *Client) forwardTCPConnectionOverWS(id, requestID, remoteHost string) {
	streamID := fmt.Sprintf("%s:%s", id, requestID)

	// Connect to local upstream
	localAddr := joinHostPort(c.opts.UpstreamHost, c.opts.UpstreamPort)
	localConn, err := net.DialTimeout("tcp", localAddr, upstreamTCPDialTimeout)
	if err != nil {
		c.logger.Printf("[local][%s] failed to connect: %v", streamID, err)
		if notifyErr := c.sendTCPStreamCloseNotify(streamID); notifyErr != nil {
			c.logger.Printf("[local][%s] failed to notify server of upstream failure: %v", streamID, notifyErr)
		}
		c.tcpStreamsMu.Lock()
		delete(c.tcpStreams, streamID)
		c.tcpStreamsMu.Unlock()
		c.removeDataChannel(streamID)
		return
	}

	c.logger.Printf("[local][%s] connected", streamID)
	c.runTCPDataChannelRelay(streamID, localConn)
}

// handleTCPDataBinary handles binary TCP data message directly
func (c *Client) handleTCPDataBinary(messageBuffer []byte) error {
	// Parse binary message
	binaryMsg, err := parseBinaryMessage(messageBuffer)
	if err != nil {
		return fmt.Errorf("failed to parse binary message: %v", err)
	}

	// Extract actual TCP data from binary message
	data := binaryMsg.Data

	// Find the local connection for this stream
	var localConn net.Conn
	var exists bool
	var isPlaceholder bool

	// Wait for placeholder to become a real conn or disappear (dial failed / stream closed).
	// Window must cover upstreamTCPDialTimeout plus scheduling slack so user bytes are not dropped
	// while the local socket is still connecting.
	deadline := time.Now().Add(upstreamTCPDialTimeout + 3*time.Second)
	for time.Now().Before(deadline) {
		c.tcpStreamsMu.RLock()
		conn, found := c.tcpStreams[binaryMsg.StreamID]
		c.tcpStreamsMu.RUnlock()

		if !found {
			c.logger.Printf("[tcp:data][%s] stream ended before upstream ready, dropping %d bytes",
				binaryMsg.StreamID, len(data))
			return nil
		}
		exists = true
		if conn != nil {
			localConn = conn
			isPlaceholder = false
			break
		}
		isPlaceholder = true
		time.Sleep(10 * time.Millisecond)
	}
	if !exists || isPlaceholder {
		c.logger.Printf("[tcp:data][%s] Stream not ready (exists: %v, placeholder: %v), ignoring %d bytes",
			binaryMsg.StreamID, exists, isPlaceholder, len(data))
		return nil
	}

	if localConn == nil {
		c.logger.Printf("[tcp:data][%s] Local connection is nil, ignoring %d bytes", binaryMsg.StreamID, len(data))
		return nil
	}

	// Write data to local connection
	if _, err := localConn.Write(data); err != nil {
		c.logger.Printf("[tcp:data] Failed to write to local connection for stream %s: %v", binaryMsg.StreamID, err)
		// Clean up the stream
		c.tcpStreamsMu.Lock()
		delete(c.tcpStreams, binaryMsg.StreamID)
		c.tcpStreamsMu.Unlock()
		localConn.Close()
		return err
	}

	return nil
}

// handleTCPData handles TCP data from legacy protocol (JSON format)
// This is kept for backward compatibility with legacy protocol
func (c *Client) handleTCPData(payload interface{}) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var tcpData TCPData
	if err := json.Unmarshal(dataBytes, &tcpData); err != nil {
		return err
	}

	// Decode base64 data - this is a binary message, not raw TCP data
	messageBuffer, err := base64.StdEncoding.DecodeString(tcpData.Data)
	if err != nil {
		return fmt.Errorf("failed to decode base64 message: %v", err)
	}

	// Use the binary handler
	return c.handleTCPDataBinary(messageBuffer)
}
