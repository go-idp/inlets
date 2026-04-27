package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Close reasons the server may send when a public (unauthenticated) monitor session hits its time limit.
// The server decides policy; the client only reacts. Legacy text kept for older servers.
const (
	publicMonitorSessionTimeoutCloseReason      = "public monitor session timeout"
	legacyPublicHTTPNoAuthTimeoutCloseReason    = "public http no-auth timeout"
)

// Client wraps a websocket tunnel session and manages forwarding/heartbeat.
type Client struct {
	opts                   *Options
	monitorConn            *websocket.Conn // Monitor channel: ping/pong, auth, control messages
	authTimeout            *time.Timer
	reconnectTimeout       *time.Timer
	logger                 *log.Logger
	monitorWriteMu         sync.Mutex             // Mutex for monitor connection writes
	dataWriteMu            map[string]*sync.Mutex // Per-stream data channel write mutex
	dataConns              map[string]*websocket.Conn
	dataConnMu             *sync.RWMutex
	// data channel application ping/pong (per stream), same wire format and timing as monitor
	dataHeartbeatMu         sync.Mutex
	dataPingTimer           map[string]*time.Timer
	dataPingTimeoutTimer    map[string]*time.Timer
	negotiatedCapabilities *Capabilities
	clientId               string // Client ID from server after authentication
	containerId            string // Container ID from server after authentication

	pingInterval     time.Duration
	pingTimeout      time.Duration
	pingTimer        *time.Timer
	pingTimeoutTimer *time.Timer

	// TCP stream management for new protocol (TCP over WebSocket)
	tcpStreamsMu sync.RWMutex
	tcpStreams   map[string]net.Conn // streamId -> local connection

	// Sequence counter for binary message protocol
	sequenceCounterMu sync.Mutex
	sequenceCounter   map[string]uint32 // streamId -> sequence number

	// Connection state
	closingMu sync.Mutex
	closing   bool // true if connection is being closed

	// Heartbeat state
	heartbeatMu     sync.Mutex
	heartbeatActive bool // true if heartbeat is active

	// separatedChannels is true when using /_/monitor (and data channel); false for legacy /_client
	// or after HTTP 404 on /_/monitor forces legacy. Reconnect reuses this choice.
	separatedChannels bool

	httpStreamMu   sync.Mutex
	httpStreamSess map[string]*httpStreamSession
	targets        connectionTargets
}

type connectionTargets struct {
	monitorURL          string
	legacyURL           string
	dataBaseURL         string
	remoteHost          string
	allowLegacyFallback bool
}

// New constructs a Client with sane defaults.
func New(opts *Options) *Client {
	// Set default reconnect retries and interval if not specified
	if opts.ReconnectMaxRetries <= 0 {
		opts.ReconnectMaxRetries = defaultReconnectMaxRetries
	}
	if opts.ReconnectInterval <= 0 {
		opts.ReconnectInterval = defaultReconnectInterval
	}

	return &Client{
		opts:            opts,
		logger:          log.New(os.Stdout, "[inlets] ", log.LstdFlags),
		pingInterval:    defaultPingInterval,
		pingTimeout:     defaultPingTimeout,
		tcpStreams:      make(map[string]net.Conn),
		sequenceCounter: make(map[string]uint32),
		dataConns:              make(map[string]*websocket.Conn),
		dataWriteMu:            make(map[string]*sync.Mutex),
		dataConnMu:             &sync.RWMutex{},
		dataPingTimer:          make(map[string]*time.Timer),
		dataPingTimeoutTimer:   make(map[string]*time.Timer),
		httpStreamSess:         make(map[string]*httpStreamSession),
	}
}

// Run boots the websocket tunnel and blocks until an unrecoverable error happens.
func (c *Client) Run() error {
	c.logger.Printf("Version: %s", c.opts.Version)

	targets, err := buildConnectionTargets(c.opts)
	if err != nil {
		return fmt.Errorf("invalid server configuration: %v", err)
	}
	c.targets = targets

	clientWantsV2 := compareVersion(c.opts.Version, "2.0.0") >= 0
	useNewProtocol, err := c.establishMonitor(targets, clientWantsV2)
	if err != nil {
		return fmt.Errorf("failed to connect monitor channel: %v", err)
	}
	c.separatedChannels = useNewProtocol

	if useNewProtocol {
		c.logger.Printf("Using new protocol with separated channels")
	} else if clientWantsV2 {
		c.logger.Printf("Using v1 legacy protocol: server is not v2 or does not expose %s (single WebSocket %s)", wsMonitorPath, wsPath)
	} else {
		c.logger.Printf("Using legacy protocol with single connection")
	}

	// Stop any existing heartbeat before starting new one
	c.stopHeartbeat()

	// Start heartbeat and auth timeout on monitor channel
	c.startHeartbeat()
	c.startAuthTimeout()

	// Authenticate on monitor channel
	if err := c.authenticate(); err != nil {
		return err
	}

	// Wait for authentication to complete and get clientId/containerId
	// This will be set in handleAuthenticateResponse

	// Cancel any existing reconnect timeout (shouldn't exist on initial connect, but just in case)
	if c.reconnectTimeout != nil {
		c.reconnectTimeout.Stop()
		c.reconnectTimeout = nil
	}

	// Handle monitor messages (ping/pong, auth, control, request, tcp:ready, tcp:connect)
	// Data channel connection will be initiated in handleMonitorMessages after authentication
	go c.handleMonitorMessages(targets.remoteHost, useNewProtocol)

	// Block forever (monitor and data handlers run in goroutines)
	select {}
}

// readDialErrorBodySnippet reads a short prefix of the response body for logs, then closes the body.
func readDialErrorBodySnippet(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 512))
	_ = resp.Body.Close()
	if err != nil || len(b) == 0 {
		return ""
	}
	s := strings.TrimSpace(string(b))
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 180 {
		return s[:180] + "..."
	}
	return s
}

// monitorDialFailureSummary builds a clear, user-facing explanation for a failed monitor WebSocket dial.
func monitorDialFailureSummary(wsURL string, err error, resp *http.Response, bodySnippet string) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("target=%s", wsURL))
	if err != nil {
		parts = append(parts, fmt.Sprintf("error=%v", err))
	}
	if resp != nil {
		parts = append(parts, fmt.Sprintf("http=%s", resp.Status))
		switch resp.StatusCode {
		case http.StatusNotFound:
			parts = append(parts, "hint=HTTP 404: wrong path or server is pre-v2 (no "+wsMonitorPath+"). Check -remote points at the inlets server.")
		case http.StatusUnauthorized, http.StatusForbidden:
			parts = append(parts, "hint=HTTP 401/403: request rejected before WebSocket upgrade (check proxy auth, IP allowlists, or server access rules).")
		case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			parts = append(parts, "hint=5xx from server or proxy while upgrading; check reverse proxy WebSocket settings and inlets server health.")
		case http.StatusMethodNotAllowed, http.StatusNotImplemented:
			parts = append(parts, "hint=Method/status suggests the URL is not a WebSocket endpoint (wrong path or plain HTTP handler).")
		default:
			if resp.StatusCode >= 400 {
				parts = append(parts, "hint=Non-success HTTP status during WebSocket handshake; see server or proxy logs for this path.")
			}
		}
	} else {
		parts = append(parts, "hint=no HTTP response (wrong host/port, TLS mismatch ws/wss, firewall, or dial timeout).")
	}
	if bodySnippet != "" {
		parts = append(parts, fmt.Sprintf("body=%q", bodySnippet))
	}
	return strings.Join(parts, " | ")
}

// establishMonitor connects the monitor WebSocket. For client v2+ it tries /_/monitor first; on HTTP 404
// it immediately falls back to legacy /_client (older servers). Other errors retry on the v2 path only.
func (c *Client) establishMonitor(targets connectionTargets, clientWantsV2 bool) (useSeparated bool, err error) {
	if !clientWantsV2 {
		return false, c.connectMonitorChannel(targets.legacyURL)
	}

	newURL := targets.monitorURL
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}

	maxRetries := 1024
	var lastSummary string
	for i := 0; i < maxRetries; i++ {
		c.logger.Printf("Connecting to monitor channel ... (attempt %d/%d)", i+1, maxRetries)
		conn, resp, err := dialer.Dial(newURL, nil)
		if err == nil {
			if resp != nil {
				_ = resp.Body.Close()
			}
			c.monitorConn = conn
			c.logger.Printf("Monitor channel connected successfully")
			return true, nil
		}
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
			_ = resp.Body.Close()
			if !targets.allowLegacyFallback {
				return false, fmt.Errorf("the specified server does not support v2; use --remote and --remote-tcp-port with --legacy")
			}
			c.logger.Printf("Server does not appear to be v2 (HTTP 404 on %s); switching to v1 legacy protocol (%s)", wsMonitorPath, wsPath)
			return false, c.connectMonitorChannel(targets.legacyURL)
		}

		bodySnippet := ""
		if resp != nil {
			bodySnippet = readDialErrorBodySnippet(resp)
		}
		lastSummary = monitorDialFailureSummary(newURL, err, resp, bodySnippet)

		if i < maxRetries-1 {
			waitTime := time.Duration(i+1) * 2 * time.Second
			c.logger.Printf("Monitor connection failed, retrying in %v: %s", waitTime, lastSummary)
			time.Sleep(waitTime)
		} else {
			c.logger.Printf("Monitor connection failed (no more retries): %s", lastSummary)
		}
	}

	return false, fmt.Errorf("failed to connect monitor channel after %d attempts: %s", maxRetries, lastSummary)
}

// connectMonitorChannel connects to the monitor WebSocket channel
func (c *Client) connectMonitorChannel(wsURL string) error {
	u, err := url.Parse(wsURL)
	if err != nil {
		return fmt.Errorf("invalid monitor URL: %v", err)
	}

	maxRetries := 1024
	var lastSummary string
	for i := 0; i < maxRetries; i++ {
		c.logger.Printf("Connecting to monitor channel ... (attempt %d/%d)", i+1, maxRetries)
		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		}

		conn, resp, err := dialer.Dial(u.String(), nil)
		if err == nil {
			if resp != nil {
				_ = resp.Body.Close()
			}
			c.monitorConn = conn
			c.logger.Printf("Monitor channel connected successfully")
			return nil
		}

		bodySnippet := ""
		if resp != nil {
			bodySnippet = readDialErrorBodySnippet(resp)
		}
		lastSummary = monitorDialFailureSummary(u.String(), err, resp, bodySnippet)

		if i < maxRetries-1 {
			waitTime := time.Duration(i+1) * 2 * time.Second
			c.logger.Printf("Monitor connection failed, retrying in %v: %s", waitTime, lastSummary)
			time.Sleep(waitTime)
		} else {
			c.logger.Printf("Monitor connection failed (no more retries): %s", lastSummary)
		}
	}

	return fmt.Errorf("failed to connect monitor channel after %d attempts: %s", maxRetries, lastSummary)
}

// connectDataChannel connects to the data WebSocket channel for a specific stream
func (c *Client) connectDataChannel(streamID, wsURL string) (*websocket.Conn, error) {
	// Check if data channel is already connected for this stream
	c.dataConnMu.RLock()
	if existing := c.dataConns[streamID]; existing != nil {
		c.dataConnMu.RUnlock()
		c.logger.Printf("Data channel already connected for stream %s, skipping", streamID)
		return existing, nil
	}
	c.dataConnMu.RUnlock()

	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("invalid data URL: %v", err)
	}

	maxRetries := 3
	var connErr error
	for i := 0; i < maxRetries; i++ {
		c.logger.Printf("Connecting to data channel for stream %s... (attempt %d/%d)", streamID, i+1, maxRetries)
		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		}

		conn, resp, err := dialer.Dial(u.String(), nil)
		if err != nil && resp != nil {
			_ = resp.Body.Close()
		}
		connErr = err
		if err == nil {
			c.dataConnMu.Lock()
			c.dataConns[streamID] = conn
			c.dataWriteMu[streamID] = &sync.Mutex{}
			c.dataConnMu.Unlock()
			c.logger.Printf("Data channel connected successfully for stream %s", streamID)
			return conn, nil
		}

		if i < maxRetries-1 {
			waitTime := time.Duration(i+1) * 2 * time.Second
			if resp != nil {
				c.logger.Printf("Data connection failed for stream %s: %v (HTTP %s), retrying in %v...", streamID, err, resp.Status, waitTime)
			} else {
				c.logger.Printf("Data connection failed for stream %s: %v, retrying in %v...", streamID, err, waitTime)
			}
			time.Sleep(waitTime)
		}
	}

	return nil, fmt.Errorf("failed to connect data channel for stream %s after %d attempts: %v", streamID, maxRetries, connErr)
}

func (c *Client) authenticate() error {
	timestamp := time.Now().UnixMilli()

	// Determine signed secret
	var signedSecret string
	if c.opts.AuthType == "credentials" {
		signedSecret = c.opts.ClientSecret
	} else if c.opts.AuthType == "token" {
		signedSecret = c.opts.Token
	} else {
		signedSecret = "public"
	}

	// Generate signature
	signature := hmacSHA512(strconv.FormatInt(timestamp, 10), signedSecret)

	// Get client capabilities based on version; use legacy auth when connected via /_client even if binary is v2+
	capabilities := GetClientCapabilities(c.opts.Version)
	if compareVersion(c.opts.Version, "2.0.0") >= 0 && !c.separatedChannels {
		capabilities = nil
	}

	auth := Authentication{
		Version:      c.opts.Version,
		Type:         c.opts.Type,
		Port:         c.opts.UpstreamPort,
		SubDomain:    c.opts.SubDomain,
		TunnelPort:   tunnelPortFromOptions(c.opts),
		Timestamp:    timestamp,
		AuthType:     c.opts.AuthType,
		ClientId:     c.opts.ClientId,
		Signature:    signature,
		Capabilities: capabilities,
		OpaqueChild:  c.opts.OpaqueChild,
	}
	if strings.EqualFold(c.opts.Type, "http") && strings.TrimSpace(c.opts.UpstreamUsername) != "" {
		auth.HTTPIngressBasic = &HTTPTunnelAuth{
			Type:     "basic",
			Username: c.opts.UpstreamUsername,
			Password: c.opts.UpstreamPassword,
		}
	}

	// Send authentication on monitor channel - message format is [type, payload]
	if err := c.sendMonitorMessage("authenticate", auth); err != nil {
		return fmt.Errorf("failed to send authentication: %v", err)
	}

	if capabilities != nil {
		c.logger.Printf("Authenticating with capabilities (version: %s)...", c.opts.Version)
	} else if compareVersion(c.opts.Version, "2.0.0") >= 0 {
		c.logger.Printf("Authenticating with v1 legacy protocol (client binary %s, non-v2 server)...", c.opts.Version)
	} else {
		c.logger.Printf("Authenticating with legacy protocol (version: %s)...", c.opts.Version)
	}
	return nil
}

// handleMonitorMessages handles messages from monitor channel
// For legacy protocol: handles all messages (ping/pong, auth, control, request, tcp:ready, tcp:connect, tcp:data)
// For new protocol: handles ping/pong, auth, control, request, tcp:ready, tcp:connect (tcp:data goes to data channel)
func (c *Client) handleMonitorMessages(remoteHost string, useNewProtocol bool) {
	for {
		if c.monitorConn == nil {
			return
		}

		var msgArray []interface{}
		if err := c.monitorConn.ReadJSON(&msgArray); err != nil {
			if shouldExitOnPublicHTTPNoAuthTimeoutClose(err) {
				c.logger.Printf("Monitor channel closed by server (unauthenticated public session time limit); exiting without reconnect")
				os.Exit(0)
			}

			// Check if connection is already being closed (e.g., by ping timeout)
			c.closingMu.Lock()
			isClosing := c.closing
			c.closingMu.Unlock()

			// If connection was closed due to ping timeout, handleDisconnect was already called
			if isClosing {
				// Try to reconnect (reconnect will reset closing flag)
				if err := c.reconnect(); err != nil {
					c.logger.Printf("Reconnection failed: %v", err)
					os.Exit(1)
				}
				// New reconnect spawns its own reader; stop this goroutine
				return
			}

			// Check if it's a close error
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) ||
				websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Printf("Monitor channel closed: %v", err)
				c.handleDisconnect()
				// Try to reconnect
				if err := c.reconnect(); err != nil {
					c.logger.Printf("Reconnection failed: %v", err)
					os.Exit(1)
				}
				return
			}

			// For other errors, check if connection is closed
			errStr := err.Error()
			if strings.Contains(errStr, "use of closed network connection") ||
				strings.Contains(errStr, "connection reset by peer") ||
				strings.Contains(errStr, "EOF") {
				c.logger.Printf("Monitor channel error: %v", err)
				c.handleDisconnect()
				// Try to reconnect
				if err := c.reconnect(); err != nil {
					c.logger.Printf("Reconnection failed: %v", err)
					return
				}
				return
			}

			c.logger.Printf("Monitor channel read error: %v", err)
			return
		}

		if len(msgArray) == 0 {
			continue
		}

		event, ok := msgArray[0].(string)
		if !ok {
			continue
		}

		var payload interface{}
		if len(msgArray) > 1 {
			payload = msgArray[1]
		}

		// Monitor channel handles different events based on protocol version
		// Legacy protocol: all messages (including tcp:data)
		// New protocol: all messages except tcp:data (which goes to data channel)
		switch event {
		case "authenticate":
			// Cancel reconnect timeout on successful authentication
			if c.reconnectTimeout != nil {
				c.reconnectTimeout.Stop()
				c.reconnectTimeout = nil
			}
			if err := c.handleAuthenticateResponse(payload); err != nil {
				c.logger.Printf("Error handling authenticate response: %v", err)
				// if failed to handle authenticate response, exit the client
				os.Exit(1)
				return
			}
			// Data channel will be created on-demand when server requests it
		case "request":
			if err := c.handleHTTPRequest(payload); err != nil {
				c.logger.Printf("[tunnel:http] handle request message failed: %v", err)
			}
		case "tcp:ready":
			if err := c.handleTCPReady(payload); err != nil {
				c.logger.Printf("Error handling TCP ready: %v", err)
			}
		case "tcp:connect":
			if err := c.handleTCPConnect(payload, remoteHost); err != nil {
				c.logger.Printf("Error handling TCP connect: %v", err)
			}
		case "data:channel:open":
			// Server requests to open data channel for specific stream
			if useNewProtocol && c.clientId != "" && c.containerId != "" {
				var streamID string
				if payloadMap, ok := payload.(map[string]interface{}); ok {
					if sID, ok := payloadMap["streamId"].(string); ok {
						streamID = sID
					}
				}
				if streamID == "" {
					c.logger.Printf("Warning: data:channel:open missing streamId, ignoring")
					continue
				}
				if c.targets.dataBaseURL == "" {
					c.logger.Printf("Warning: data channel URL base is empty, ignoring stream %s", streamID)
					continue
				}

				query := url.Values{}
				query.Set("clientId", c.clientId)
				query.Set("containerId", c.containerId)
				query.Set("streamId", streamID)
				dataURL := fmt.Sprintf("%s?%s", c.targets.dataBaseURL, query.Encode())
				conn, err := c.connectDataChannel(streamID, dataURL)
				if err != nil {
					c.logger.Printf("Warning: failed to connect data channel for %s: %v", streamID, err)
				} else {
					// Register before handleDataChannel: upload can arrive on the data WebSocket before
					// the monitor delivers tcp:connect (separate connections / scheduling).
					c.registerTCPStreamPlaceholder(streamID)
					go c.handleDataChannel(streamID, conn)
					if err := c.sendMonitorMessage("data:channel:ready", map[string]interface{}{"streamId": streamID}); err != nil {
						c.logger.Printf("Failed to send data:channel:ready for %s: %v", streamID, err)
					}
				}
			}
		case "tcp:data":
			// For legacy protocol, handle tcp:data in monitor channel
			// For new protocol, tcp:data should come from data channel
			if !useNewProtocol {
				if err := c.handleTCPData(payload); err != nil {
					c.logger.Printf("Error handling TCP data: %v", err)
				}
			} else {
				c.logger.Printf("Warning: received tcp:data on monitor channel (should come from data channel)")
			}
		case "disconnect":
			c.handleDisconnect()
		case "error":
			if errMsg, ok := payload.(map[string]interface{}); ok {
				if msg, ok := errMsg["message"].(string); ok {
					c.logger.Printf("Server error: %s", msg)
				}
				if fatal, _ := errMsg["fatal"].(bool); fatal {
					os.Exit(1)
				}
			}
		case "@@CONFIG":
			c.handleSocketConfig(payload)
		case "pong":
			c.handlePong()
		case "id":
			if id, ok := payload.(string); ok {
				c.logger.Printf("Connected to server (id: %s)", id)
			}
		case "warn":
			if msg, ok := payload.(string); ok {
				c.logger.Printf("Server warn: %s", msg)
			}
		default:
			c.logger.Printf("Unhandled monitor event: %s", event)
		}
	}
}

// shouldExitOnPublicHTTPNoAuthTimeoutClose detects a server-initiated close for the public monitor session TTL
// so we exit without useless reconnect. Policy lives on the server only.
func shouldExitOnPublicHTTPNoAuthTimeoutClose(err error) bool {
	if err == nil {
		return false
	}
	low := func(s string) string { return strings.ToLower(s) }
	check := func(s string) bool {
		s = low(s)
		return strings.Contains(s, low(publicMonitorSessionTimeoutCloseReason)) ||
			strings.Contains(s, low(legacyPublicHTTPNoAuthTimeoutCloseReason))
	}
	if closeErr, ok := err.(*websocket.CloseError); ok {
		return check(closeErr.Text)
	}
	return check(err.Error())
}

// isDataChannelReadClosedNormally classifies dataConn.ReadMessage errors after the peer
// or local code closed the WebSocket. These are not actionable failures; logging them
// as "read error" is noisy when removeDataChannel/Close races with the read loop.
func isDataChannelReadClosedNormally(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		return true
	}
	s := err.Error()
	if strings.Contains(s, "use of closed network connection") {
		return true
	}
	if strings.Contains(s, "connection reset by peer") {
		return true
	}
	if opErr, ok := err.(*net.OpError); ok {
		if opErr.Err == syscall.ECONNRESET {
			return true
		}
	}
	return false
}

// handleDataChannel handles messages from a per-stream data channel (binary TCP data; text ping/pong)
func (c *Client) handleDataChannel(streamID string, dataConn *websocket.Conn) {
	c.startDataChannelHeartbeat(streamID)
	defer c.stopDataChannelHeartbeat(streamID)

	for {
		messageType, message, err := dataConn.ReadMessage()
		if err != nil {
			if !isDataChannelReadClosedNormally(err) {
				c.logger.Printf("Data channel read error for %s: %v", streamID, err)
			}
			c.removeDataChannel(streamID)
			return
		}

		if messageType == websocket.BinaryMessage {
			if err := c.handleTCPDataBinary(message); err != nil {
				c.logger.Printf("Error handling TCP data for %s: %v", streamID, err)
			}
		} else if messageType == websocket.TextMessage {
			if len(message) == 0 {
				continue
			}
			var msgArray []interface{}
			if err := json.Unmarshal(message, &msgArray); err != nil {
				c.logger.Printf("Data channel JSON parse error for %s: %v", streamID, err)
				continue
			}
			if len(msgArray) < 1 {
				continue
			}
			ev, _ := msgArray[0].(string)
			switch ev {
			case "pong":
				c.handleDataChannelPong(streamID)
			case "ping":
				c.sendDataChannelPong(streamID)
			}
		} else {
			c.logger.Printf("Unexpected message type on data channel %s: %d (expected Binary or Text)", streamID, messageType)
		}
	}
}

func (c *Client) handleDisconnect() {
	c.logger.Printf("Server Disconnected")
	c.stopHeartbeat()
	c.closeAllDataChannels()

	// Set reconnect timeout
	// Calculate timeout based on max retries and interval, with some buffer
	// Default: 1000 retries * 3s = 3000s, add 60s buffer = 3060s
	maxRetries := c.opts.ReconnectMaxRetries
	if maxRetries <= 0 {
		maxRetries = defaultReconnectMaxRetries
	}
	retryInterval := c.opts.ReconnectInterval
	if retryInterval <= 0 {
		retryInterval = defaultReconnectInterval
	}
	reconnectTimeout := time.Duration(maxRetries) * retryInterval
	// Add 60 seconds buffer
	reconnectTimeout += 60 * time.Second

	if c.reconnectTimeout != nil {
		c.reconnectTimeout.Stop()
	}
	c.reconnectTimeout = time.AfterFunc(reconnectTimeout, func() {
		c.logger.Printf("Reconnect timeout after %v, exiting", reconnectTimeout)
		os.Exit(1)
	})
}

func (c *Client) reconnect() error {
	// Reset closing flag
	c.closingMu.Lock()
	c.closing = false
	c.closingMu.Unlock()

	// Stop heartbeat before reconnecting to prevent multiple heartbeat goroutines
	c.stopHeartbeat()

	// Cancel reconnect timeout
	if c.reconnectTimeout != nil {
		c.reconnectTimeout.Stop()
		c.reconnectTimeout = nil
	}

	// Close existing connections
	if c.monitorConn != nil {
		c.monitorConn.Close()
		c.monitorConn = nil
	}
	c.closeAllDataChannels()

	useNewProtocol := c.separatedChannels
	monitorURL := c.targets.legacyURL
	if useNewProtocol {
		monitorURL = c.targets.monitorURL
	}
	if err := c.connectMonitorChannel(monitorURL); err != nil {
		return fmt.Errorf("failed to reconnect monitor channel: %v", err)
	}

	// Start heartbeat and auth timeout on monitor channel
	c.startHeartbeat()
	c.startAuthTimeout()

	// Re-authenticate
	if err := c.authenticate(); err != nil {
		return fmt.Errorf("re-authentication failed: %v", err)
	}

	// Handle monitor messages
	go c.handleMonitorMessages(c.targets.remoteHost, useNewProtocol)

	// Cancel reconnect timeout
	if c.reconnectTimeout != nil {
		c.reconnectTimeout.Stop()
		c.reconnectTimeout = nil
	}

	return nil
}

// Utility functions

// getDataChannel returns data connection and write mutex for a stream
func (c *Client) getDataChannel(streamID string) (*websocket.Conn, *sync.Mutex) {
	c.dataConnMu.RLock()
	defer c.dataConnMu.RUnlock()
	return c.dataConns[streamID], c.dataWriteMu[streamID]
}

// removeDataChannel removes and closes data channel for a stream
func (c *Client) removeDataChannel(streamID string) {
	c.stopDataChannelHeartbeat(streamID)
	c.dataConnMu.Lock()
	conn := c.dataConns[streamID]
	delete(c.dataConns, streamID)
	delete(c.dataWriteMu, streamID)
	c.dataConnMu.Unlock()
	if conn != nil {
		conn.Close()
	}
}

// closeAllDataChannels closes and clears all data channels
func (c *Client) closeAllDataChannels() {
	c.dataConnMu.RLock()
	ids := make([]string, 0, len(c.dataConns))
	for id := range c.dataConns {
		ids = append(ids, id)
	}
	c.dataConnMu.RUnlock()
	for _, id := range ids {
		c.stopDataChannelHeartbeat(id)
	}
	c.dataConnMu.Lock()
	for streamID, conn := range c.dataConns {
		if conn != nil {
			conn.Close()
		}
		delete(c.dataConns, streamID)
		delete(c.dataWriteMu, streamID)
	}
	c.dataConnMu.Unlock()
}

// sendMonitorMessage sends a message on the monitor channel
func (c *Client) sendMonitorMessage(event string, payload interface{}) error {
	c.monitorWriteMu.Lock()
	defer c.monitorWriteMu.Unlock()

	// Check if connection is closing
	c.closingMu.Lock()
	isClosing := c.closing
	c.closingMu.Unlock()

	if c.monitorConn == nil || isClosing {
		return fmt.Errorf("monitor websocket connection is not ready")
	}

	message := []interface{}{event, payload}
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	return c.monitorConn.WriteMessage(websocket.TextMessage, data)
}

// sendDataMessage is no longer supported with per-stream data channels
func (c *Client) sendDataMessage(event string, payload interface{}) error {
	return fmt.Errorf("sendDataMessage is not supported for per-stream data channels")
}

func buildConnectionTargets(opts *Options) (connectionTargets, error) {
	if strings.TrimSpace(opts.Server) != "" {
		return buildConnectionTargetsFromServer(opts.Server)
	}
	return buildConnectionTargetsFromRemote(opts.Remote)
}

func buildConnectionTargetsFromRemote(remote string) (connectionTargets, error) {
	trimmedRemote := strings.TrimSpace(remote)
	if trimmedRemote == "" {
		return connectionTargets{}, fmt.Errorf("remote address is required")
	}

	host, port, err := splitRemoteHostPort(trimmedRemote)
	if err != nil {
		return connectionTargets{}, fmt.Errorf("invalid remote address format: %w", err)
	}

	protocol := "ws"
	if port == "443" {
		protocol = "wss"
	}

	return connectionTargets{
		monitorURL:          fmt.Sprintf("%s://%s%s", protocol, trimmedRemote, wsMonitorPath),
		legacyURL:           fmt.Sprintf("%s://%s%s", protocol, trimmedRemote, wsPath),
		dataBaseURL:         fmt.Sprintf("%s://%s%s", protocol, trimmedRemote, wsDataPath),
		remoteHost:          host,
		allowLegacyFallback: true,
	}, nil
}

func buildConnectionTargetsFromServer(serverRaw string) (connectionTargets, error) {
	server := strings.TrimSpace(serverRaw)
	if server == "" {
		return connectionTargets{}, fmt.Errorf("server URL is required")
	}

	u, err := url.Parse(server)
	if err != nil {
		return connectionTargets{}, fmt.Errorf("invalid server URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return connectionTargets{}, fmt.Errorf("server URL must include scheme and host")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return connectionTargets{}, fmt.Errorf("server URL must not include query string or fragment")
	}

	scheme := strings.ToLower(u.Scheme)
	wsScheme := ""
	switch scheme {
	case "http":
		wsScheme = "ws"
	case "https":
		wsScheme = "wss"
	case "ws", "wss":
		wsScheme = scheme
	default:
		return connectionTargets{}, fmt.Errorf("unsupported server URL scheme %q", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return connectionTargets{}, fmt.Errorf("server URL host is required")
	}

	port := u.Port()
	if port == "" {
		switch scheme {
		case "http", "ws":
			port = "80"
		default:
			port = "443"
		}
	}

	prefix := strings.TrimSuffix(u.EscapedPath(), "/")
	if prefix == "/" {
		prefix = ""
	}

	hostPort := net.JoinHostPort(host, port)
	base := fmt.Sprintf("%s://%s", wsScheme, hostPort)

	return connectionTargets{
		monitorURL:          base + joinURLPath(prefix, wsMonitorPath),
		legacyURL:           base + joinURLPath(prefix, wsPath),
		dataBaseURL:         base + joinURLPath(prefix, wsDataPath),
		remoteHost:          host,
		allowLegacyFallback: false,
	}, nil
}

func joinURLPath(prefix, suffix string) string {
	trimmedPrefix := strings.TrimSuffix(prefix, "/")
	if trimmedPrefix == "" {
		return suffix
	}
	if !strings.HasPrefix(trimmedPrefix, "/") {
		trimmedPrefix = "/" + trimmedPrefix
	}
	return trimmedPrefix + suffix
}

func splitRemoteHostPort(remote string) (string, string, error) {
	host, port, err := net.SplitHostPort(remote)
	if err == nil {
		return host, port, nil
	}
	// Keep backward compatibility with host:port values that were historically parsed by split.
	parts := strings.Split(remote, ":")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], nil
	}
	return "", "", err
}
