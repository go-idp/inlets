package data

import (
	"net/http"
	"sync"

	"github.com/go-idp/inlets/internal/server/types"
	"github.com/go-zoox/logger"
	"github.com/gorilla/websocket"
)

// WebSocketDataChannelHandler handles WebSocket data channel connections for new protocol
type WebSocketDataChannelHandler struct {
	ctx      *types.Context
	upgrader websocket.Upgrader
}

// NewDataChannelHandler creates a new WebSocket data channel handler
func NewDataChannelHandler(ctx *types.Context) *WebSocketDataChannelHandler {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins
		},
	}

	return &WebSocketDataChannelHandler{
		ctx:      ctx,
		upgrader: upgrader,
	}
}

// HandleConnection handles WebSocket upgrade requests for data channel
func (h *WebSocketDataChannelHandler) HandleConnection(w http.ResponseWriter, r *http.Request) {
	// Get identifiers from query parameters
	clientId := r.URL.Query().Get("clientId")
	containerId := r.URL.Query().Get("containerId")
	streamId := r.URL.Query().Get("streamId")

	if clientId == "" || containerId == "" || streamId == "" {
		logger.Infof("[monitor:ws:data] Missing clientId/containerId/streamId in query parameters")
		http.Error(w, "Missing clientId, containerId or streamId", http.StatusBadRequest)
		return
	}

	// Verify container exists
	container := h.ctx.Container.Get(containerId)
	if container == nil {
		logger.Infof("[monitor:ws:data] Container not found: %s", containerId)
		http.Error(w, "Container not found", http.StatusNotFound)
		return
	}

	// Verify clientId matches
	if container.ClientId != clientId {
		logger.Infof("[monitor:ws:data] ClientId mismatch: expected %s, got %s", container.ClientId, clientId)
		http.Error(w, "ClientId mismatch", http.StatusForbidden)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Infof("[monitor:ws:data] Failed to upgrade connection: %v", err)
		return
	}

	logger.Infof("[monitor:ws:data] New data channel connection: clientId=%s, containerId=%s, streamId=%s", clientId, containerId, streamId)

	// Store data channel connection in container (per stream)
	dataWriteMu := &sync.Mutex{}
	if container.DataMu != nil {
		container.DataMu.Lock()
		container.DataSockets[streamId] = conn
		container.DataWriteMu[streamId] = dataWriteMu
		container.DataMu.Unlock()
	}

	// Set data channel in adapter if using new protocol
	if container.UseNewProtocol && container.Adapter != nil {
		if binaryAdapter, ok := container.Adapter.(interface {
			SetDataChannelForStream(streamId string, dataConn *websocket.Conn, dataWriteMu *sync.Mutex)
			RemoveDataChannelForStream(streamId string)
		}); ok {
			binaryAdapter.SetDataChannelForStream(streamId, conn, dataWriteMu)
			logger.Infof("[monitor:ws:data] Data channel set in adapter for stream: %s", streamId)
		} else {
			logger.Infof("[monitor:ws:data] Warning: adapter does not support per-stream data channel for container: %s", containerId)
		}
	} else if container.UseNewProtocol {
		logger.Infof("[monitor:ws:data] Warning: adapter not found in container: %s (data channel will not be used)", containerId)
	}

	// Handle data channel messages
	h.handleConnection(conn, containerId, streamId, container)
}

// handleConnection handles messages from data channel
func (h *WebSocketDataChannelHandler) handleConnection(conn *websocket.Conn, containerId, streamId string, container *types.TunnelMapping) {
	defer func() {
		logger.Infof("[monitor:ws:data] Data channel disconnected for container: %s streamId: %s", containerId, streamId)
		if container.DataMu != nil {
			container.DataMu.Lock()
			delete(container.DataSockets, streamId)
			delete(container.DataWriteMu, streamId)
			container.DataMu.Unlock()
		}
		if container.UseNewProtocol && container.Adapter != nil {
			if binaryAdapter, ok := container.Adapter.(interface {
				RemoveDataChannelForStream(streamId string)
			}); ok {
				binaryAdapter.RemoveDataChannelForStream(streamId)
			}
		}
		conn.Close()
	}()

	logger.Infof("[monitor:ws:data] Starting data channel message loop for container: %s", containerId)

	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			// Check if this is a close error (any close error, including abnormal closure)
			// When connection is closed, ReadMessage returns an error, but this is expected behavior
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
				// Any close error (normal or abnormal) - connection is already closed, just log as info
				logger.Infof("[monitor:ws:data] Data channel closed for container %s: %v", containerId, err)
			} else if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
				// Unexpected close code, but still a close error - log as info
				logger.Infof("[monitor:ws:data] Data channel closed for container %s: %v", containerId, err)
			} else {
				// Check if it's a CloseError by type assertion
				if _, ok := err.(*websocket.CloseError); ok {
					// It's a CloseError but not in the expected codes - still a close, log as info
					logger.Infof("[monitor:ws:data] Data channel closed for container %s: %v", containerId, err)
				} else {
					// Non-close error (e.g., network error, read error) - this is a real error
					logger.Infof("[monitor:ws:data] ReadMessage error for container %s: %v", containerId, err)
				}
			}
			return
		}

		if messageType == websocket.BinaryMessage {
			// Handle binary messages for new protocol
			if container.Adapter != nil {
				if binaryAdapter, ok := container.Adapter.(interface {
					HandleBinaryMessage([]byte) error
				}); ok {
					binaryAdapter.HandleBinaryMessage(message)
				}
			}
		} else {
			// Data channel only accepts binary messages for new protocol
			logger.Infof("[monitor:ws:data] Unexpected message type on data channel: %d (expected BinaryMessage)", messageType)
		}
	}
}
