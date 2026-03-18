package monitor

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-idp/inlets/internal/server/tunnel"
	"github.com/go-zoox/logger"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// HandleConnection handles a new protocol monitor channel WebSocket connection (/_/monitor)
// New protocol: monitor channel handles ping/pong, auth, control messages (tcp:data goes to data channel)
func (h *MonitorChannelHandler) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Infof("[monitor:ws] Failed to upgrade connection: %v", err)
		return
	}

	logger.Infof("[monitor:ws] New WebSocket connection from %s", conn.RemoteAddr())

	wsConn := &WebSocketConnection{
		Conn: conn,
	}

	// Send 'id' message to client
	idMessage := []interface{}{"id", uuid.New().String()}
	idBytes, err := json.Marshal(idMessage)
	if err == nil {
		wsConn.writeMu.Lock()
		conn.WriteMessage(websocket.TextMessage, idBytes)
		wsConn.writeMu.Unlock()
	}

	isAuthenticated := false
	var subDomain string

	// Authentication timeout: 10 seconds
	authTimeout := time.AfterFunc(10*time.Second, func() {
		if !isAuthenticated {
			logger.Infof("[monitor:ws] Connection removed without authorization")
			conn.Close()
		}
	})

	// Handle messages
	go func() {
		defer conn.Close()
		defer authTimeout.Stop()

		logger.Infof("[monitor:ws] Starting message reading loop for %s", conn.RemoteAddr())
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				// Check if this is a close error (any close error, including abnormal closure)
				// When connection is closed, ReadMessage returns an error, but this is expected behavior
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
					// Any close error (normal or abnormal) - connection is already closed, just log as info
					logger.Infof("[monitor:ws] Connection closed: %v", err)
				} else if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
					// Unexpected close code, but still a close error - log as info
					logger.Infof("[monitor:ws] Connection closed: %v", err)
				} else {
					// Check if it's a CloseError by type assertion
					if _, ok := err.(*websocket.CloseError); ok {
						// It's a CloseError but not in the expected codes - still a close, log as info
						logger.Infof("[monitor:ws] Connection closed: %v", err)
					} else {
						// Non-close error (e.g., network error, read error) - this is a real error
						logger.Infof("[monitor:ws] ReadMessage error: %v", err)
					}
				}
				if isAuthenticated {
					handleDisconnect(h.ctx, h.options, wsConn, subDomain, wsConn.ClientID)
				}
				return
			}

			// logger.Infof("[monitor:ws] Received message: type=%d, len=%d", messageType, len(message))
			if messageType == websocket.BinaryMessage {
				// Handle binary messages for new protocol
				if isAuthenticated {
					wsConn.mu.RLock()
					adapter := wsConn.Adapter
					wsConn.mu.RUnlock()

					if adapter != nil {
						if binaryAdapter, ok := adapter.(interface {
							HandleBinaryMessage([]byte) error
						}); ok {
							binaryAdapter.HandleBinaryMessage(message)
						}
					}
				}
			} else if messageType == websocket.TextMessage {
				// Parse JSON message: ["event", payload]
				var msgArray []interface{}
				if err := json.Unmarshal(message, &msgArray); err != nil {
					logger.Infof("[monitor:ws] Failed to parse JSON message: %v, raw: %s", err, string(message))
					continue
				}

				if len(msgArray) < 1 {
					logger.Infof("[monitor:ws] Empty message array")
					continue
				}

				event, ok := msgArray[0].(string)
				if !ok {
					logger.Infof("[monitor:ws] Event is not a string: %v (type: %T)", msgArray[0], msgArray[0])
					continue
				}

				var payload interface{}
				if len(msgArray) > 1 {
					payload = msgArray[1]
				}

				// Handle ping
				if event == "ping" {
					sendPong(wsConn)
					continue
				}

				switch event {
				case "authenticate":
					authTimeout.Stop()
					if err := handleAuthenticate(h.ctx, h.options, h.emitter, wsConn, payload, &isAuthenticated, &subDomain); err != nil {
						logger.Infof("[monitor:ws] Authentication failed: %v", err)
						conn.Close()
						return
					}
					isAuthenticated = true
					continue
				case "response":
					if isAuthenticated {
						handleResponse(h.ctx, wsConn, payload)
					}
				case "request":
					// Handle HTTP request (base64-encoded binary for new protocol)
					if isAuthenticated {
						wsConn.mu.RLock()
						adapter := wsConn.Adapter
						wsConn.mu.RUnlock()

						if adapter != nil {
							if data, ok := payload.(map[string]interface{}); ok {
								if dataStr, ok := data["data"].(string); ok && dataStr != "" {
									// Decode base64
									messageBuffer, err := base64.StdEncoding.DecodeString(dataStr)
									if err == nil {
										if binaryAdapter, ok := adapter.(interface {
											HandleBinaryMessage([]byte) error
										}); ok {
											binaryAdapter.HandleBinaryMessage(messageBuffer)
										}
									}
								}
							}
						}
					}
				case "tcp:data":
					// New protocol: tcp:data should NOT come through monitor channel
					// It should come through data channel instead
					logger.Infof("[monitor:ws] Warning: received tcp:data on monitor channel (should come from data channel)")
				case "data:channel:ready":
					// Client notifies that data channel is ready for a specific stream
					var streamID string
					if payloadMap, ok := payload.(map[string]interface{}); ok {
						if sID, ok := payloadMap["streamId"].(string); ok {
							streamID = sID
						}
					}
					if streamID != "" {
						tunnel.NotifyDataChannelReady(streamID)
						logger.Infof("[monitor:ws] Data channel ready confirmed for stream: %s", streamID)
					} else {
						logger.Infof("[monitor:ws] data:channel:ready missing streamId")
					}
				default:
					logger.Infof("[monitor:ws] Unhandled monitor event: %s (payload type: %T)", event, payload)
				}
			}
		}
	}()
}
