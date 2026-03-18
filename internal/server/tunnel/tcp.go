package tunnel

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/go-idp/inlets/internal/server/protocol"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/go-zoox/logger"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// dataChannelReadyChan is used to signal when data channel is ready per stream
var dataChannelReadyChan = make(map[string]chan bool)
var dataChannelReadyMu sync.Mutex

// TCPTunnel handles TCP tunnel connections
type TCPTunnel struct {
	ctx *types.Context
}

// CreateTCPTunnel creates a new TCP tunnel
func CreateTCPTunnel(ctx *types.Context) *TCPTunnel {
	return &TCPTunnel{
		ctx: ctx,
	}
}

// Options contains options for creating a TCP tunnel server
type Options struct {
	ContainerID string
	Domain      string
}

// CreateServer creates a TCP tunnel server for a container
func (t *TCPTunnel) CreateServer(options Options) error {
	containerID := options.ContainerID
	domain := options.Domain

	// Get container
	container := t.ctx.Container.Get(containerID)
	if container == nil {
		return fmt.Errorf("unknown container with id: %s", containerID)
	}

	if container.WSSocket == nil {
		return fmt.Errorf("container wsSocket is required with id: %s", containerID)
	}

	// Check if TCP server already exists for this container
	if container.SourceServer != nil {
		logger.Infof("[tunnel:tcp   ] TCP server already exists for container: %s, skipping creation", containerID)
		return nil
	}

	// Get or allocate source port
	// Use TunnelPort if specified by client, otherwise allocate an available port
	var sourcePort int
	if container.TunnelPort != nil && *container.TunnelPort != 0 {
		// Client specified a tunnel port - use it
		sourcePort = *container.TunnelPort
	} else {
		// Allocate available port
		port, err := getAvailablePort()
		if err != nil {
			return fmt.Errorf("failed to allocate port: %v", err)
		}
		sourcePort = port
	}

	// Update container with source port
	if err := t.ctx.Container.Set(containerID, "sourcePort", sourcePort); err != nil {
		return fmt.Errorf("failed to set source port: %v", err)
	}

	// Notify client that TCP server is ready
	tcpReadyData := map[string]interface{}{
		"host": domain,
		"port": sourcePort,
	}
	message := []interface{}{
		"tcp:ready",
		tcpReadyData,
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal tcp:ready message: %v", err)
	}

	// Use writeMu to protect WriteMessage (gorilla/websocket requires serialized writes)
	if container.WriteMu != nil {
		container.WriteMu.Lock()
		defer container.WriteMu.Unlock()
	}
	if err := container.WSSocket.WriteMessage(websocket.TextMessage, messageBytes); err != nil {
		return fmt.Errorf("failed to send tcp:ready message: %v", err)
	}

	// Create source TCP server (for user connections)
	return t.createSourceTCPServer(domain, sourcePort, containerID, container)
}

// createSourceTCPServer creates a TCP server that listens for user connections
func (t *TCPTunnel) createSourceTCPServer(
	domain string,
	port int,
	containerID string,
	container *types.TunnelMapping,
) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %v", port, err)
	}

	// Update container with source server
	if err := t.ctx.Container.Set(containerID, "sourceServer", listener); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set source server: %v", err)
	}

	logger.Infof("[tunnel:tcp   ] listen at 0.0.0.0:%d", port)

	// Accept connections in a goroutine
	go func() {
		defer listener.Close()

		for {
			conn, err := listener.Accept()
			if err != nil {
				// Listener closed
				return
			}

			// Handle connection
			go t.handleSourceConnection(conn, containerID, container)
		}
	}()

	return nil
}

// handleSourceConnection handles a new source connection (user connection)
func (t *TCPTunnel) handleSourceConnection(
	conn net.Conn,
	containerID string,
	container *types.TunnelMapping,
) {
	// Generate request ID
	requestID := uuid.New().String()
	logger.Infof("[tunnel:tcp   ][user][request][start] request id: %s, ip: %s", requestID, conn.RemoteAddr())

	// Register request
	t.ctx.Container.RegisterRequest(containerID, requestID, &conn)

	// Get protocol adapter from container
	var adapter protocol.ProtocolAdapter
	useNewProtocol := container.UseNewProtocol

	if container.Adapter != nil {
		if a, ok := container.Adapter.(protocol.ProtocolAdapter); ok {
			adapter = a
		}
	}

	streamID := fmt.Sprintf("%s:%s", containerID, requestID)

	// For new protocol, ensure per-stream data channel is open before setting up stream
	if useNewProtocol && adapter != nil {
		if err := t.ensureDataChannelForStream(containerID, streamID, requestID, container); err != nil {
			logger.Infof("[tunnel:tcp   ] Failed to ensure data channel for stream %s: %v", streamID, err)
			conn.Close()
			return
		}
		// New protocol: TCP over WebSocket
		t.setupTCPStreamOverWebSocket(containerID, streamID, conn, adapter, container.WSSocket)
	}

	// Notify client to connect
	tcpConnectData := map[string]interface{}{
		"id":        containerID,
		"requestId": requestID,
		"ip":        conn.RemoteAddr().String(),
	}
	message := []interface{}{
		"tcp:connect",
		tcpConnectData,
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		logger.Infof("[tunnel:tcp   ] Failed to marshal tcp:connect message: %v", err)
		conn.Close()
		return
	}

	// Use writeMu to protect WriteMessage (gorilla/websocket requires serialized writes)
	if container.WriteMu != nil {
		container.WriteMu.Lock()
		defer container.WriteMu.Unlock()
	}
	if err := container.WSSocket.WriteMessage(websocket.TextMessage, messageBytes); err != nil {
		logger.Infof("[tunnel:tcp   ] Failed to send tcp:connect message: %v", err)
		conn.Close()
		return
	}

	// Connection will be closed by setupTCPStreamOverWebSocket or when legacy protocol is used
	// For legacy protocol, the connection is managed by the TCP monitor
	// For new protocol, the connection is managed by setupTCPStreamOverWebSocket
}

// ensureDataChannelForStream ensures data channel is open for a specific stream, requesting client to open it
func (t *TCPTunnel) ensureDataChannelForStream(containerID, streamID, requestID string, container *types.TunnelMapping) error {
	// Check if data channel already exists for this stream
	if container.DataMu != nil {
		container.DataMu.RLock()
		_, exists := container.DataSockets[streamID]
		container.DataMu.RUnlock()
		if exists {
			return nil
		}
	}

	// Create a channel to wait for data channel ready signal
	dataChannelReadyMu.Lock()
	readyChan := make(chan bool, 1)
	dataChannelReadyChan[streamID] = readyChan
	dataChannelReadyMu.Unlock()

	// Send request to open data channel for this stream
	payload := map[string]interface{}{
		"streamId":  streamID,
		"requestId": requestID,
	}
	message := []interface{}{
		"data:channel:open",
		payload,
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		dataChannelReadyMu.Lock()
		delete(dataChannelReadyChan, streamID)
		dataChannelReadyMu.Unlock()
		return fmt.Errorf("failed to marshal data:channel:open message: %v", err)
	}

	if container.WriteMu != nil {
		container.WriteMu.Lock()
		defer container.WriteMu.Unlock()
	}
	if err := container.WSSocket.WriteMessage(websocket.TextMessage, messageBytes); err != nil {
		dataChannelReadyMu.Lock()
		delete(dataChannelReadyChan, streamID)
		dataChannelReadyMu.Unlock()
		return fmt.Errorf("failed to send data:channel:open message: %v", err)
	}

	logger.Infof("[tunnel:tcp   ] Requested client to open data channel for stream: %s", streamID)

	// Wait for data channel to be ready (with timeout)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-readyChan:
			// Clean up
			dataChannelReadyMu.Lock()
			delete(dataChannelReadyChan, streamID)
			dataChannelReadyMu.Unlock()
			logger.Infof("[tunnel:tcp   ] Data channel is ready for stream: %s", streamID)
			return nil
		case <-ticker.C:
			// Check if data channel is already set (in case ready message was missed)
			if container.DataMu != nil {
				container.DataMu.RLock()
				_, exists := container.DataSockets[streamID]
				container.DataMu.RUnlock()
				if exists {
					dataChannelReadyMu.Lock()
					delete(dataChannelReadyChan, streamID)
					dataChannelReadyMu.Unlock()
					logger.Infof("[tunnel:tcp   ] Data channel is ready (detected) for stream: %s", streamID)
					return nil
				}
			}
		case <-timeout:
			// Clean up
			dataChannelReadyMu.Lock()
			delete(dataChannelReadyChan, streamID)
			dataChannelReadyMu.Unlock()
			return fmt.Errorf("timeout waiting for data channel to be ready for stream %s", streamID)
		}
	}
}

// NotifyDataChannelReady notifies that data channel is ready for a stream
func NotifyDataChannelReady(streamID string) {
	dataChannelReadyMu.Lock()
	defer dataChannelReadyMu.Unlock()
	if readyChan, exists := dataChannelReadyChan[streamID]; exists {
		select {
		case readyChan <- true:
		default:
		}
	}
}

// setupTCPStreamOverWebSocket sets up TCP stream over WebSocket (new protocol)
func (t *TCPTunnel) setupTCPStreamOverWebSocket(
	containerID string,
	streamID string,
	sourceConn net.Conn,
	adapter protocol.ProtocolAdapter,
	wsConn *websocket.Conn,
) {
	// Get clientId for statistics
	container := t.ctx.Container.Get(containerID)
	clientID := ""
	if container != nil {
		clientID = container.ClientId
	}

	// Add connection to stats
	t.ctx.TrafficStats.AddConnection(clientID)

	// Handle data from source socket -> WebSocket (upload)
	var uploadMu sync.Mutex
	sourceConnClosed := false
	var unsubscribe func()

	// Handle data from source socket -> WebSocket (upload)
	go func() {
		defer func() {
			// Cleanup on connection close
			uploadMu.Lock()
			sourceConnClosed = true
			uploadMu.Unlock()

			// Unsubscribe from TCP data handler
			if unsubscribe != nil {
				unsubscribe()
			}

			// Close source connection
			sourceConn.Close()

			// Log stats
			statsInfo := t.ctx.TrafficStats.FormatStats(clientID)
			logger.Infof("[tunnel:tcp   ][%s] connection closed - Traffic Stats: %s", streamID, statsInfo)
		}()

		buf := make([]byte, 32*1024) // 32KB buffer
		for {
			n, err := sourceConn.Read(buf)
			if err != nil {
				// Connection closed or error
				return
			}

			if n == 0 {
				continue
			}

			data := buf[:n]

			// Check upload bandwidth limit
			if !t.ctx.BandwidthLimiter.CheckUpload(clientID, int64(n)) {
				logger.Infof("[tunnel:tcp   ][%s] Upload bandwidth limit exceeded for client: %s", streamID, clientID)
				// Wait a bit and retry
				time.Sleep(100 * time.Millisecond)
				if !t.ctx.BandwidthLimiter.CheckUpload(clientID, int64(n)) {
					logger.Infof("[tunnel:tcp   ][%s] Upload bandwidth limit still exceeded, dropping data", streamID)
					continue
				}
			}

			// Add upload bytes to stats
			t.ctx.TrafficStats.AddUploadBytes(clientID, int64(n))

			// Send TCP data via adapter
			// The adapter will automatically use data channel if available (for new protocol)
			if err := adapter.SendTCPData(streamID, data); err != nil {
				logger.Infof("[tunnel:tcp   ][%s] Failed to send TCP data over WebSocket: %v", streamID, err)
				return
			}
		}
	}()

	// Handle data from WebSocket -> source socket (download)
	// Register TCP data handler
	unsubscribe = adapter.OnTCPData(func(streamId string, data []byte) error {
		if streamId != streamID {
			return nil // Not for this stream
		}

		uploadMu.Lock()
		closed := sourceConnClosed
		uploadMu.Unlock()

		if closed {
			return nil // Connection already closed
		}

		// Check download bandwidth limit
		if !t.ctx.BandwidthLimiter.CheckDownload(clientID, int64(len(data))) {
			logger.Infof("[tunnel:tcp   ][%s] Download bandwidth limit exceeded for client: %s", streamID, clientID)
			// Wait a bit and retry
			time.Sleep(100 * time.Millisecond)
			if !t.ctx.BandwidthLimiter.CheckDownload(clientID, int64(len(data))) {
				logger.Infof("[tunnel:tcp   ][%s] Download bandwidth limit still exceeded, dropping data", streamID)
				return nil
			}
		}

		// Add download bytes to stats
		t.ctx.TrafficStats.AddDownloadBytes(clientID, int64(len(data)))

		// Write to source connection
		if _, err := sourceConn.Write(data); err != nil {
			logger.Infof("[tunnel:tcp   ][%s] Failed to write to source socket: %v", streamID, err)
			// Mark connection as closed
			uploadMu.Lock()
			sourceConnClosed = true
			uploadMu.Unlock()
			// Close connection
			sourceConn.Close()
			// Unsubscribe from TCP data handler
			if unsubscribe != nil {
				unsubscribe()
			}
			return err
		}

		return nil
	})

	// Cleanup on connection close
	// The cleanup will be handled when:
	// 1. Upload goroutine exits (connection closed/error)
	// 2. Download handler detects write error
	// We don't need a separate cleanup goroutine that blocks on Read
}

// getAvailablePort finds an available port
func getAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}
