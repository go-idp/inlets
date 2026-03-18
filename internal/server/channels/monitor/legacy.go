package monitor

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-zoox/logger"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// HandleConnectionLegacy handles a legacy protocol WebSocket connection (/_client)
// Legacy protocol: single connection handles all messages including tcp:data
func (h *MonitorChannelHandler) HandleConnectionLegacy(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Infof("[monitor:ws:legacy] Failed to upgrade connection: %v", err)
		return
	}

	logger.Infof("[monitor:ws:legacy] New WebSocket connection from %s", conn.RemoteAddr())

	wsConn := &WebSocketConnection{
		Conn: conn,
	}

	// Send 'id' message to client (required by @znode/websocket library)
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
			logger.Infof("[monitor:ws:legacy] Connection removed without authorization")
			conn.Close()
		}
	})

	// Handle messages
	go func() {
		defer conn.Close()
		defer authTimeout.Stop()

		logger.Infof("[monitor:ws:legacy] Starting message reading loop for %s", conn.RemoteAddr())
		for {
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				// Check if this is a close error (any close error, including abnormal closure)
				// When connection is closed, ReadMessage returns an error, but this is expected behavior
				if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
					// Any close error (normal or abnormal) - connection is already closed, just log as info
					logger.Infof("[monitor:ws:legacy] Connection closed: %v", err)
				} else if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
					// Unexpected close code, but still a close error - log as info
					logger.Infof("[monitor:ws:legacy] Connection closed: %v", err)
				} else {
					// Check if it's a CloseError by type assertion
					if _, ok := err.(*websocket.CloseError); ok {
						// It's a CloseError but not in the expected codes - still a close, log as info
						logger.Infof("[monitor:ws:legacy] Connection closed: %v", err)
					} else {
						// Non-close error (e.g., network error, read error) - this is a real error
						logger.Infof("[monitor:ws:legacy] ReadMessage error: %v", err)
					}
				}
				if isAuthenticated {
					handleDisconnect(h.ctx, h.options, wsConn, subDomain, wsConn.ClientID)
				}
				return
			}

			// logger.Infof("[monitor:ws:legacy] Received message: type=%d, len=%d", messageType, len(message))
			if messageType == websocket.TextMessage {
				// Parse JSON message: ["event", payload]
				var msgArray []interface{}
				if err := json.Unmarshal(message, &msgArray); err != nil {
					logger.Infof("[monitor:ws:legacy] Failed to parse JSON message: %v, raw: %s", err, string(message))
					continue
				}

				if len(msgArray) < 1 {
					logger.Infof("[monitor:ws:legacy] Empty message array")
					continue
				}

				event, ok := msgArray[0].(string)
				if !ok {
					logger.Infof("[monitor:ws:legacy] Event is not a string: %v (type: %T)", msgArray[0], msgArray[0])
					continue
				}

				var payload interface{}
				if len(msgArray) > 1 {
					payload = msgArray[1]
				}

				// Handle ping/echo
				if event == "ping" {
					sendPong(wsConn)
					continue
				}
				if event == "echo" {
					// Legacy protocol uses echo for ping/pong
					continue
				}

				switch event {
				case "authenticate":
					authTimeout.Stop()
					if err := handleAuthenticate(h.ctx, h.options, h.emitter, wsConn, payload, &isAuthenticated, &subDomain); err != nil {
						logger.Infof("[monitor:ws:legacy] Authentication failed: %v", err)
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
					if isAuthenticated {
						wsConn.mu.RLock()
						adapter := wsConn.Adapter
						wsConn.mu.RUnlock()
						if adapter != nil {
							if legacyAdapter, ok := adapter.(interface {
								HandleEvent(string, interface{}) error
							}); ok {
								legacyAdapter.HandleEvent("request", payload)
							}
						}
					}
				case "tcp:data":
					// Legacy protocol: tcp:data comes through monitor channel
					if isAuthenticated {
						wsConn.mu.RLock()
						adapter := wsConn.Adapter
						wsConn.mu.RUnlock()
						if adapter != nil {
							if legacyAdapter, ok := adapter.(interface {
								HandleEvent(string, interface{}) error
							}); ok {
								legacyAdapter.HandleEvent("tcp:data", payload)
							}
						}
					}
				default:
					logger.Infof("[monitor:ws:legacy] Unhandled event: %s (payload type: %T)", event, payload)
				}
			}
		}
	}()
}
