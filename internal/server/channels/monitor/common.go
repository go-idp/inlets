package monitor

import (
	"net/http"
	"sync"
	"time"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/notification"
	"github.com/go-idp/inlets/internal/server/protocol"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/gorilla/websocket"
)

// CreateWebSocketOptions contains options for creating WebSocket monitor
type CreateWebSocketOptions struct {
	Version      string
	Domain       string
	Port         int
	Secure       bool
	Token        types.GetToken
	Notification *Notification
	// PublicHTTPNoAuthSessionTTL controls automatic close for public HTTP tunnels without edge auth.
	// Zero means use default (10m).
	PublicHTTPNoAuthSessionTTL time.Duration
	// PublicHTTPNoAuthWarnLeadTime controls when to warn clients before the timeout.
	// Zero means use default (2m).
	PublicHTTPNoAuthWarnLeadTime time.Duration
}

// WebSocketConnection represents a WebSocket connection with metadata
type WebSocketConnection struct {
	*websocket.Conn
	// RequestHost is the HTTP Host from the WebSocket upgrade (host only, no port). Used to build
	// authenticate response URL when server Domain option is empty.
	RequestHost    string
	ContainerID    string
	ClientID       string
	Capabilities   *client.Capabilities
	UseNewProtocol bool
	IsLegacyClient bool
	Adapter        protocol.ProtocolAdapter
	mu             sync.RWMutex
	writeMu        sync.Mutex // Mutex for write operations (gorilla/websocket requires serialized writes)
}

// Notification represents notification service
type Notification struct {
	notifier *notification.Notifier
}

// NewNotification creates a new notification instance
func NewNotification(config *client.NotificationConfig) *Notification {
	return &Notification{
		notifier: notification.NewNotifier(config),
	}
}

// Notify sends a notification
func (n *Notification) Notify(title string, message []string) error {
	if n.notifier == nil {
		return nil
	}
	return n.notifier.Notify(title, message)
}

// EventEmitter emits events
type EventEmitter struct {
	handlers map[string][]func(interface{})
	mu       sync.RWMutex
}

// NewEventEmitter creates a new event emitter
func NewEventEmitter() *EventEmitter {
	return &EventEmitter{
		handlers: make(map[string][]func(interface{})),
	}
}

// On registers an event handler
func (e *EventEmitter) On(event string, handler func(interface{})) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.handlers[event] == nil {
		e.handlers[event] = make([]func(interface{}), 0)
	}
	e.handlers[event] = append(e.handlers[event], handler)
}

// Emit emits an event
func (e *EventEmitter) Emit(event string, data interface{}) {
	e.mu.RLock()
	handlers := e.handlers[event]
	e.mu.RUnlock()

	for _, handler := range handlers {
		go handler(data)
	}
}

// MonitorChannelHandler handles monitor channel connections
type MonitorChannelHandler struct {
	ctx      *types.Context
	options  *CreateWebSocketOptions
	upgrader websocket.Upgrader
	emitter  *EventEmitter
}

// NewMonitorChannelHandler creates a new monitor channel handler
func NewMonitorChannelHandler(ctx *types.Context, options *CreateWebSocketOptions, emitter *EventEmitter) *MonitorChannelHandler {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins
		},
	}

	return &MonitorChannelHandler{
		ctx:      ctx,
		options:  options,
		upgrader: upgrader,
		emitter:  emitter,
	}
}
