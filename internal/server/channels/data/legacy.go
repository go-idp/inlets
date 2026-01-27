package data

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/go-zoox/inlets/internal/server/types"
)

const (
	// TUNNEL_TCP_FLAG is the TCP tunnel authentication flag
	TUNNEL_TCP_FLAG = "3e55f8e5-021b-441c-8e3b-64e87ea5f263"
	// TUNNEL_TCP_OK_FLAG is the TCP tunnel authentication success flag
	TUNNEL_TCP_OK_FLAG = TUNNEL_TCP_FLAG + "200\n"
	// UID_LENGTH is the length of UUID (36 bytes)
	UID_LENGTH = 36
	// REQUEST_ID_LENGTH is the length of request ID (36 bytes)
	REQUEST_ID_LENGTH = 36
	// SIGNATURE_LENGTH is the length of HMAC-SHA256 signature (64 bytes)
	SIGNATURE_LENGTH = 64
)

// TCPMonitor manages TCP connections for legacy protocol
type TCPMonitor struct {
	ctx     *types.Context
	options *CreateTCPMonitorOptions
	server  net.Listener
	mu      sync.RWMutex
}

// CreateTCPMonitorOptions contains options for creating TCP monitor
type CreateTCPMonitorOptions struct {
	Version string
	Domain  string
	Port    int
	Token   types.GetToken
}

// NewDataChannelHandlerLegacy creates a new TCP monitor
func NewDataChannelHandlerLegacy(ctx *types.Context, options *CreateTCPMonitorOptions) (*TCPMonitor, error) {
	monitor := &TCPMonitor{
		ctx:     ctx,
		options: options,
	}

	// Create TCP server
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", options.Port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %d: %v", options.Port, err)
	}

	monitor.server = listener

	// Start accepting connections
	go monitor.acceptConnections()

	return monitor, nil
}

// acceptConnections accepts incoming TCP connections
func (m *TCPMonitor) acceptConnections() {
	for {
		conn, err := m.server.Accept()
		if err != nil {
			// Listener closed
			return
		}

		// Handle connection in a separate goroutine
		go m.handleConnection(conn)
	}
}

// handleConnection handles a new TCP connection
func (m *TCPMonitor) handleConnection(conn net.Conn) {
	log.Printf("[tunnel:client][connect][1] ip %s", conn.RemoteAddr())

	var requestID string
	var isAuthenticated bool
	var authMu sync.Mutex

	// Connection will be managed by tunnel container after authentication

	// Handle data
	buffer := make([]byte, 4096)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			return
		}

		authMu.Lock()
		authenticated := isAuthenticated
		authMu.Unlock()

		if authenticated {
			// Already authenticated, data should be piped to source socket
			// This is handled by the tunnel container's pipeConnections
			// We don't need to process it here
			continue
		}

		// Process authentication
		data := buffer[:n]
		if err := m.processAuthentication(conn, data, &requestID, &isAuthenticated, &authMu); err != nil {
			log.Printf("[monitor:tcp] Authentication error: %v", err)
			return
		}

		authMu.Lock()
		if isAuthenticated {
			authMu.Unlock()
			// Authentication successful, connection will be managed by tunnel container
			// Don't close the connection here, let the container handle it
			return
		}
		authMu.Unlock()
	}
}

// processAuthentication processes TCP authentication
func (m *TCPMonitor) processAuthentication(
	conn net.Conn,
	data []byte,
	requestID *string,
	isAuthenticated *bool,
	authMu *sync.Mutex,
) error {
	// Check minimum data length
	minLength := len(TUNNEL_TCP_FLAG) + UID_LENGTH + REQUEST_ID_LENGTH + SIGNATURE_LENGTH
	if len(data) < minLength {
		return fmt.Errorf("data too short: %d < %d", len(data), minLength)
	}

	// Check FLAG
	flag := string(data[:len(TUNNEL_TCP_FLAG)])
	if flag != TUNNEL_TCP_FLAG {
		log.Printf("[monitor:tcp] unknown flag: %s, should be %s", flag, TUNNEL_TCP_FLAG)
		conn.Write([]byte(fmt.Sprintf("%s401 unauthorized", TUNNEL_TCP_FLAG)))
		conn.Close()
		return fmt.Errorf("invalid flag")
	}

	// Extract ContainerID, RequestID, and Signature
	offset := len(TUNNEL_TCP_FLAG)
	containerID := string(data[offset : offset+UID_LENGTH])
	offset += UID_LENGTH
	requestIDValue := string(data[offset : offset+REQUEST_ID_LENGTH])
	offset += REQUEST_ID_LENGTH
	signature := string(data[offset : offset+SIGNATURE_LENGTH])

	log.Printf("[monitor:tcp] id: %s, request_id: %s, sign: %s", containerID, requestIDValue, signature)

	// Get token for container
	container := m.ctx.Container.Get(containerID)
	if container == nil {
		log.Printf("[monitor:tcp] invalid client(id:%s): %s", containerID, signature)
		conn.Write([]byte(fmt.Sprintf("%s404 invalid client", TUNNEL_TCP_FLAG)))
		conn.Close()
		return fmt.Errorf("container not found")
	}

	// Get token
	tokenRes, err := container.Token(container.AuthType, container.ClientId, &types.GetTokenOptions{
		Type: types.TunnelTypeTCP,
	})
	if err != nil {
		log.Printf("[monitor:tcp] invalid client(id:%s): %s: %v", containerID, signature, err)
		conn.Write([]byte(fmt.Sprintf("%s404 invalid client", TUNNEL_TCP_FLAG)))
		conn.Close()
		return fmt.Errorf("failed to get token: %v", err)
	}

	if tokenRes == nil || tokenRes.Token == "" {
		log.Printf("[monitor:tcp] invalid client(id:%s): %s", containerID, signature)
		conn.Write([]byte(fmt.Sprintf("%s403 invalid client", TUNNEL_TCP_FLAG)))
		conn.Close()
		return fmt.Errorf("invalid token")
	}

	// Verify signature: HMAC-SHA256(ContainerID, secret)
	expectedSignature := hmacSHA256(containerID, tokenRes.Token)
	if signature != expectedSignature {
		log.Printf("[monitor:tcp] invalid signature(id:%s): %s, expected: %s", containerID, signature, expectedSignature)
		conn.Write([]byte(fmt.Sprintf("%s402 invalid signature", TUNNEL_TCP_FLAG)))
		conn.Close()
		return fmt.Errorf("invalid signature")
	}

	// Authentication successful
	authMu.Lock()
	if *requestID == "" {
		*requestID = requestIDValue
		log.Printf("[tunnel:client][connect][2] request id: %s, ip %s", *requestID, conn.RemoteAddr())
	}
	*isAuthenticated = true
	authMu.Unlock()

	// Send OK flag
	if _, err := conn.Write([]byte(TUNNEL_TCP_OK_FLAG)); err != nil {
		return fmt.Errorf("failed to write OK flag: %v", err)
	}

	// Call onAuthenticate callback
	m.onAuthenticate(containerID, requestIDValue, conn)

	return nil
}

// onAuthenticate handles successful authentication
func (m *TCPMonitor) onAuthenticate(containerID string, requestID string, targetSocket net.Conn) {
	container := m.ctx.Container.Get(containerID)
	if container == nil {
		log.Printf("[monitor:tcp] container not found: %s", containerID)
		targetSocket.Close()
		return
	}

	clientID := container.ClientId
	if clientID == "" {
		clientID = "unknown"
	}

	log.Printf("[tunnel:client][%s] connected (container id: %s, request id: %s)", clientID, containerID, requestID)

	// Connect request (this will pipe source socket to target socket)
	if err := m.ctx.Container.ConnectRequest(containerID, requestID, &targetSocket); err != nil {
		log.Printf("[monitor:tcp] failed to connect request: %v", err)
		targetSocket.Close()
		return
	}
}

// GetPort returns the port the TCP monitor is listening on
func (m *TCPMonitor) GetPort() int {
	return m.options.Port
}

// Close closes the TCP monitor
func (m *TCPMonitor) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.server != nil {
		return m.server.Close()
	}

	return nil
}

// hmacSHA256 computes HMAC-SHA256 signature
func hmacSHA256(message, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}
