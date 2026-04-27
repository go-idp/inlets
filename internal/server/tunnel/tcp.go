package tunnel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/protocol"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/go-zoox/logger"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// dataChannelReadyChan is used to signal when data channel is ready per stream
var dataChannelReadyChan = make(map[string]chan bool)
var dataChannelReadyMu sync.Mutex

// tcpRelaySetupDelay is used before starting the TCP upload loop when the peer is an older v2
// client that does not negotiate CapabilityFlagTCPEarlyStreamRegister.
func tcpRelaySetupDelay(negotiatedFlags int) time.Duration {
	if negotiatedFlags&client.CapabilityFlagTCPOverWS != 0 &&
		negotiatedFlags&client.CapabilityFlagTCPEarlyStreamRegister == 0 {
		return 75 * time.Millisecond
	}
	return 0
}

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
	// Use TunnelPort if specified by client; else reuse SourcePort after listener recovery;
	// otherwise allocate an available port.
	var sourcePort int
	if container.TunnelPort != nil && *container.TunnelPort != 0 {
		sourcePort = *container.TunnelPort
	} else if container.SourcePort != nil && *container.SourcePort != 0 {
		sourcePort = *container.SourcePort
	} else {
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

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", sourcePort))
	if err != nil {
		listenErr := fmt.Errorf("failed to listen on port %d: %v", sourcePort, err)
		sendTCPListenFatalError(container, listenErr)
		return listenErr
	}

	// Notify client only after the public listener is bound (avoids tcp:ready when bind fails).
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
		listener.Close()
		return fmt.Errorf("failed to marshal tcp:ready message: %v", err)
	}

	if container.WriteMu != nil {
		container.WriteMu.Lock()
		err = container.WSSocket.WriteMessage(websocket.TextMessage, messageBytes)
		container.WriteMu.Unlock()
	} else {
		err = container.WSSocket.WriteMessage(websocket.TextMessage, messageBytes)
	}
	if err != nil {
		listener.Close()
		return fmt.Errorf("failed to send tcp:ready message: %v", err)
	}

	if err := t.ctx.Container.Set(containerID, "sourceServer", listener); err != nil {
		listener.Close()
		return fmt.Errorf("failed to set source server: %v", err)
	}

	logger.Infof("[tunnel:tcp   ] listen at 0.0.0.0:%d", sourcePort)

	go t.runTCPAcceptLoop(listener, domain, containerID, container)
	return nil
}

// sendTCPListenFatalError tells the client the TCP tunnel could not be opened; client should exit.
func sendTCPListenFatalError(container *types.TunnelMapping, listenErr error) {
	if container == nil || container.WSSocket == nil || listenErr == nil {
		return
	}
	payload := map[string]interface{}{
		"message": listenErr.Error(),
		"fatal":   true,
		"code":    "tcp_listen_failed",
	}
	msg := []interface{}{"error", payload}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	if container.WriteMu != nil {
		container.WriteMu.Lock()
		defer container.WriteMu.Unlock()
	}
	_ = container.WSSocket.WriteMessage(websocket.TextMessage, b)
}

func (t *TCPTunnel) runTCPAcceptLoop(l net.Listener, domain string, containerID string, container *types.TunnelMapping) {
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			if errors.Is(err, net.ErrClosed) {
				logger.Infof("[tunnel:tcp   ] listener closed for container %s", containerID)
			} else {
				logger.Infof("[tunnel:tcp   ] accept error for container %s: %v — recreating listener", containerID, err)
			}
			if err := t.ctx.Container.Set(containerID, "sourceServer", nil); err != nil {
				logger.Infof("[tunnel:tcp   ] clear sourceServer for %s: %v", containerID, err)
			}
			if recErr := t.CreateServer(Options{ContainerID: containerID, Domain: domain}); recErr != nil {
				logger.Infof("[tunnel:tcp   ] failed to recreate TCP listener for %s: %v", containerID, recErr)
			}
			return
		}

		go t.handleSourceConnection(conn, containerID, container)
	}
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

	// For new protocol, ensure per-stream data channel is open before notifying the client
	if useNewProtocol && adapter != nil {
		if err := t.ensureDataChannelForStream(containerID, streamID, requestID, container); err != nil {
			logger.Infof("[tunnel:tcp   ] Failed to ensure data channel for stream %s: %v", streamID, err)
			conn.Close()
			return
		}
	}

	// Notify client to connect (before upload loop for new protocol so client registers/dials first)
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

	if container.WriteMu != nil {
		container.WriteMu.Lock()
	}
	if err := container.WSSocket.WriteMessage(websocket.TextMessage, messageBytes); err != nil {
		if container.WriteMu != nil {
			container.WriteMu.Unlock()
		}
		logger.Infof("[tunnel:tcp   ] Failed to send tcp:connect message: %v", err)
		conn.Close()
		return
	}
	if container.WriteMu != nil {
		container.WriteMu.Unlock()
	}

	if useNewProtocol && adapter != nil {
		if d := tcpRelaySetupDelay(adapter.NegotiatedFlags()); d > 0 {
			time.Sleep(d)
		}
		t.setupTCPStreamOverWebSocket(containerID, streamID, conn, adapter, container.WSSocket, nil)
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
	onRelayComplete func(),
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
	var unsubscribeClose func()
	var closeOnce sync.Once

	closeUserConn := func() {
		closeOnce.Do(func() {
			uploadMu.Lock()
			sourceConnClosed = true
			uploadMu.Unlock()

			if unsubscribe != nil {
				unsubscribe()
			}
			if unsubscribeClose != nil {
				unsubscribeClose()
			}
			sourceConn.Close()

			if onRelayComplete != nil {
				onRelayComplete()
			}

			statsInfo := t.ctx.TrafficStats.FormatStats(clientID)
			logger.Infof("[tunnel:tcp   ][%s] connection closed - Traffic Stats: %s", streamID, statsInfo)
		})
	}

	// Handle data from source socket -> WebSocket (upload)
	go func() {
		defer closeUserConn()

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
			closeUserConn()
			return err
		}

		return nil
	})

	unsubscribeClose = adapter.OnTCPClose(func(streamId string) error {
		if streamId != streamID {
			return nil
		}
		closeUserConn()
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
