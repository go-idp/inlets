package types

import (
	"net"
	"sync"
	"time"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/limiter"
	"github.com/go-idp/inlets/internal/server/stats"
	"github.com/gorilla/websocket"
)

// TunnelType represents the type of tunnel
type TunnelType string

const (
	TunnelTypeHTTP TunnelType = "http"
	TunnelTypeTCP  TunnelType = "tcp"
)

// AuthType represents the authentication type
type AuthType string

const (
	AuthTypeToken       AuthType = "token"
	AuthTypeCredentials AuthType = "credentials"
	AuthTypePublic      AuthType = "public"
)

// GetTokenOptions contains options for token retrieval
type GetTokenOptions struct {
	Type        TunnelType `json:"type,omitempty"`
	OpaqueChild bool       `json:"opaqueChild,omitempty"` // true: spawned session; omit tunnel list in token
}

// TokenResponse represents the response from GetToken function
type TokenResponse struct {
	AuthType AuthType       `json:"authType"`
	Token    string         `json:"token"`
	Config   *client.Config `json:"config,omitempty"`
}

// GetToken is a function type for retrieving tokens
type GetToken func(authType AuthType, clientId string, options *GetTokenOptions) (*TokenResponse, error)

// ServerConfig contains server configuration
type ServerConfig struct {
	Version                    string
	BinName                    string
	PackageName                string
	WSPath                     string // Legacy protocol path (/_client)
	WSMonitorPath              string // New protocol monitor channel path (/_/monitor)
	WSDataPath                 string // New protocol data channel path (/_/data)
	DefaultRemote              string
	DefaultTCPPort             int
	DefaultSecure              bool
	DefaultHealthCheckInterval time.Duration
}

// DefaultServerConfig returns the default server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Version:                    "2.0.0",
		BinName:                    "inlets",
		PackageName:                "inlets",
		WSPath:                     "/_client",   // Legacy protocol (single connection)
		WSMonitorPath:              "/_/monitor", // New protocol monitor channel
		WSDataPath:                 "/_/data",    // New protocol data channel
		DefaultRemote:              "inlets.zcorky.com:443",
		DefaultTCPPort:             8443,
		DefaultSecure:              true,
		DefaultHealthCheckInterval: 30 * time.Second,
	}
}

// DomainMapping represents a domain mapping entry
type DomainMapping struct {
	WSSocket       *websocket.Conn
	TCPSocket      *net.Conn
	ClientID       string
	Adapter        interface{} // protocol.ProtocolAdapter (using interface{} to avoid circular import)
	UseNewProtocol bool
}

// CallbackFunc is a function type for handling HTTP responses
type CallbackFunc func(data string)

// TunnelMapping represents a tunnel container mapping
type TunnelMapping struct {
	WSSocket        *websocket.Conn
	DataSockets     map[string]*websocket.Conn // Per-stream data channel connections
	DataWriteMu     map[string]*sync.Mutex     // Per-stream data channel write mutex
	DataMu          *sync.RWMutex              // Guard for data channel maps
	WriteMu         *sync.Mutex                // Mutex for write operations (gorilla/websocket requires serialized writes)
	Token           GetToken
	Type            TunnelType
	Port            int
	TargetPort      *int
	SourcePort      *int
	SourceServer    net.Listener
	Version         string
	AuthType        AuthType
	ClientId        string
	Signature       string
	ClientTimestamp int64
	TunnelPort      *int
	Requests        map[string]*net.Conn
	Adapter         interface{} // protocol.ProtocolAdapter (using interface{} to avoid circular import)
	UseNewProtocol  bool
	Destroy         func()
}

// TrafficStats and TrafficStatsData are now defined in stats package
// These are kept here for backward compatibility, but should use stats package types
type TrafficStats = stats.TrafficStats
type TrafficStatsData = stats.TrafficStatsData

// BandwidthLimit and ClientBandwidthLimits are now defined in limiter package
// These are kept here for backward compatibility, but should use limiter package types
type BandwidthLimit = limiter.BandwidthLimit
type ClientBandwidthLimits = limiter.ClientBandwidthLimits

// Context contains all server context components
type Context struct {
	Config            *ServerConfig
	DomainMappings    DomainMappingContainer
	CallbackContainer CallbackContainer
	Container         TunnelContainer
	TrafficStats      TrafficStatsContainer
	BandwidthLimiter  BandwidthLimiter
	// HTTPStreamDispatch handles semantic HTTP response frames (MessageType 0x09/0x0a) from the client.
	// msgType is the binary protocol message type byte. Returns true if the frame was consumed.
	HTTPStreamDispatch func(tcpID, requestID string, msgType uint8, payload []byte, fin bool) bool
}

// DomainMappingContainer interface for domain mapping operations
type DomainMappingContainer interface {
	Get(id string) *DomainMapping
	GetAll() map[string]*DomainMapping
	Has(id string) bool
	BindWS(wsSocket *websocket.Conn, subDomain string) string
	UnbindWS(id string)
	BindTCP(id string, tcpSocket *net.Conn)
}

// CallbackContainer interface for callback operations
type CallbackContainer interface {
	Get(tcpId string, requestId string) CallbackFunc
	Take(tcpId string, requestId string) CallbackFunc
	Set(tcpId string, requestId string, callback CallbackFunc)
	Remove(tcpId string)
}

// TunnelContainer interface for tunnel container operations
type TunnelContainer interface {
	Create(id string, token GetToken, wsSocket *websocket.Conn, auth *client.Authentication, writeMu *sync.Mutex)
	Get(id string) *TunnelMapping
	Set(id string, key string, value interface{}) error
	Remove(id string)
	RegisterRequest(containerId string, requestId string, sourceSocket *net.Conn)
	ConnectRequest(containerId string, requestId string, targetSocket *net.Conn) error
}

// TrafficStatsContainer interface is now defined in stats package
type TrafficStatsContainer = stats.TrafficStatsContainer

// BandwidthLimiter interface is now defined in limiter package
type BandwidthLimiter = limiter.BandwidthLimiter
