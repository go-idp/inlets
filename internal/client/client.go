package client

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
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
		dataConns:       make(map[string]*websocket.Conn),
		dataWriteMu:     make(map[string]*sync.Mutex),
		dataConnMu:      &sync.RWMutex{},
	}
}

// Run boots the websocket tunnel and blocks until an unrecoverable error happens.
func (c *Client) Run() error {
	c.logger.Printf("Version: %s", c.opts.Version)

	// Parse remote address
	remoteParts := strings.Split(c.opts.Remote, ":")
	if len(remoteParts) != 2 {
		return fmt.Errorf("invalid remote address format")
	}
	remoteHost := remoteParts[0]
	remotePort := remoteParts[1]

	// Determine protocol
	protocol := "ws"
	if remotePort == "443" {
		protocol = "wss"
	}

	// Check if we should use new protocol (version 2.0.0+)
	useNewProtocol := compareVersion(c.opts.Version, "2.0.0") >= 0

	var monitorPath string
	if useNewProtocol {
		// New protocol: use separated channels
		monitorPath = wsMonitorPath
		c.logger.Printf("Using new protocol with separated channels")
	} else {
		// Legacy protocol: use single connection
		monitorPath = wsPath
		c.logger.Printf("Using legacy protocol with single connection")
	}

	// Connect to monitor channel (for ping/pong, auth, control)
	monitorURL := fmt.Sprintf("%s://%s%s", protocol, c.opts.Remote, monitorPath)
	if err := c.connectMonitorChannel(monitorURL); err != nil {
		return fmt.Errorf("failed to connect monitor channel: %v", err)
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
	go c.handleMonitorMessages(remoteHost, useNewProtocol)

	// Block forever (monitor and data handlers run in goroutines)
	select {}
}

// connectMonitorChannel connects to the monitor WebSocket channel
func (c *Client) connectMonitorChannel(wsURL string) error {
	u, err := url.Parse(wsURL)
	if err != nil {
		return fmt.Errorf("invalid monitor URL: %v", err)
	}

	maxRetries := 1024
	var connErr error
	for i := 0; i < maxRetries; i++ {
		c.logger.Printf("Connecting to monitor channel ... (attempt %d/%d)", i+1, maxRetries)
		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		}

		conn, _, connErr := dialer.Dial(u.String(), nil)
		if connErr == nil {
			c.monitorConn = conn
			c.logger.Printf("Monitor channel connected successfully")
			return nil
		}

		if i < maxRetries-1 {
			waitTime := time.Duration(i+1) * 2 * time.Second
			c.logger.Printf("Monitor connection failed, retrying in %v...", waitTime)
			time.Sleep(waitTime)
		}
	}

	return fmt.Errorf("failed to connect monitor channel after %d attempts: %v", maxRetries, connErr)
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

		conn, _, connErr := dialer.Dial(u.String(), nil)
		if connErr == nil {
			c.dataConnMu.Lock()
			c.dataConns[streamID] = conn
			c.dataWriteMu[streamID] = &sync.Mutex{}
			c.dataConnMu.Unlock()
			c.logger.Printf("Data channel connected successfully for stream %s", streamID)
			return conn, nil
		}

		if i < maxRetries-1 {
			waitTime := time.Duration(i+1) * 2 * time.Second
			c.logger.Printf("Data connection failed for stream %s, retrying in %v...", streamID, waitTime)
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

	// Get client capabilities based on version
	capabilities := GetClientCapabilities(c.opts.Version)

	auth := Authentication{
		Version:      c.opts.Version,
		Type:         c.opts.Type,
		Port:         c.opts.UpstreamPort,
		SubDomain:    c.opts.SubDomain,
		TunnelPort:   c.opts.Port,
		Timestamp:    timestamp,
		AuthType:     c.opts.AuthType,
		ClientId:     c.opts.ClientId,
		Signature:    signature,
		Capabilities: capabilities,
	}

	// Send authentication on monitor channel - message format is [type, payload]
	if err := c.sendMonitorMessage("authenticate", auth); err != nil {
		return fmt.Errorf("failed to send authentication: %v", err)
	}

	if capabilities != nil {
		c.logger.Printf("Authenticating with capabilities (version: %s)...", c.opts.Version)
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
				return
			}
			// Data channel will be created on-demand when server requests it
		case "request":
			if err := c.handleHTTPRequest(payload); err != nil {
				c.logger.Printf("Error handling HTTP request: %v", err)
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

				remoteParts := strings.Split(c.opts.Remote, ":")
				if len(remoteParts) == 2 {
					remotePort := remoteParts[1]
					protocol := "ws"
					if remotePort == "443" {
						protocol = "wss"
					}
					dataURL := fmt.Sprintf("%s://%s%s?clientId=%s&containerId=%s&streamId=%s", protocol, c.opts.Remote, wsDataPath, c.clientId, c.containerId, streamID)
					conn, err := c.connectDataChannel(streamID, dataURL)
					if err != nil {
						c.logger.Printf("Warning: failed to connect data channel for %s: %v", streamID, err)
					} else {
						// Start handling data messages for this stream
						go c.handleDataChannel(streamID, conn)
						// Notify server that data channel is ready
						if err := c.sendMonitorMessage("data:channel:ready", map[string]interface{}{"streamId": streamID}); err != nil {
							c.logger.Printf("Failed to send data:channel:ready for %s: %v", streamID, err)
						}
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

// handleDataChannel handles messages from a per-stream data channel (only binary TCP data messages)
func (c *Client) handleDataChannel(streamID string, dataConn *websocket.Conn) {
	for {
		messageType, message, err := dataConn.ReadMessage()
		if err != nil {
			c.logger.Printf("Data channel read error for %s: %v", streamID, err)
			c.removeDataChannel(streamID)
			return
		}

		// Only handle binary messages (TCP data)
		if messageType == websocket.BinaryMessage {
			if err := c.handleTCPDataBinary(message); err != nil {
				c.logger.Printf("Error handling TCP data for %s: %v", streamID, err)
			}
		} else {
			c.logger.Printf("Unexpected message type on data channel %s: %d (expected BinaryMessage)", streamID, messageType)
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

	// Parse remote address
	remoteParts := strings.Split(c.opts.Remote, ":")
	if len(remoteParts) != 2 {
		return fmt.Errorf("invalid remote address format")
	}
	remoteHost := remoteParts[0]
	remotePort := remoteParts[1]

	// Determine protocol
	protocol := "ws"
	if remotePort == "443" {
		protocol = "wss"
	}

	// Check if we should use new protocol (version 2.0.0+)
	useNewProtocol := compareVersion(c.opts.Version, "2.0.0") >= 0

	var monitorPath string
	if useNewProtocol {
		// New protocol: use separated channels
		monitorPath = wsMonitorPath
	} else {
		// Legacy protocol: use single connection
		monitorPath = wsPath
	}

	// Reconnect monitor channel
	monitorURL := fmt.Sprintf("%s://%s%s", protocol, c.opts.Remote, monitorPath)
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
	go c.handleMonitorMessages(remoteHost, useNewProtocol)

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
