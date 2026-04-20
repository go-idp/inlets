package client

import (
	"strconv"
	"strings"
	"time"
)

const (
	wsPath                     = "/_client"   // Legacy protocol path (single connection)
	wsMonitorPath              = "/_/monitor" // New protocol monitor channel path
	wsDataPath                 = "/_/data"    // New protocol data channel path
	tunnelTCPFlag              = "3e55f8e5-021b-441c-8e3b-64e87ea5f263"
	tunnelTCPOKFlag            = "3e55f8e5-021b-441c-8e3b-64e87ea5f263200\n"
	defaultAuthTimeout         = 30 * time.Second
	defaultReconnectTimeout    = 30 * time.Second
	defaultPingInterval        = 15 * time.Second
	defaultPingTimeout         = 20 * time.Second // Increased from 5s to 20s
	defaultVersion             = "2.0.0"
	defaultReconnectMaxRetries = 1000
	defaultReconnectInterval   = 3 * time.Second
)

// CapabilityFlags represents protocol capability flags
const (
	CapabilityFlagBinaryProtocol = 1 << iota
	CapabilityFlagCompression
	CapabilityFlagStreaming
	CapabilityFlagFlowControl
	CapabilityFlagHTTPBinary
	CapabilityFlagHTTPStreaming
	CapabilityFlagTCPOverWS
	CapabilityFlagTCPMultiplex
	CapabilityFlagHTTPBodyStream // semantic HTTP head+body chunking (not WS-level message chunking)
)

type Options struct {
	Type                string
	UpstreamHost        string
	UpstreamPort        int
	UpstreamUsername    string // HTTP tunnel: Basic auth when dialing upstream (optional)
	UpstreamPassword    string
	AuthType            string
	Token               string
	ClientId            string
	ClientSecret        string
	SubDomain           string
	Port                int
	Remote              string
	RemoteTCPPort       int
	HealthcheckInt      int
	ReportURL           string
	Version             string
	ReconnectMaxRetries int           // Maximum number of reconnection retries, default 1000
	ReconnectInterval   time.Duration // Interval between reconnection attempts, default 3s
	// OpaqueChild: true for sessions auto-spawned from server tunnel list (do not re-spawn; auth omits tunnel list).
	OpaqueChild bool
}

// TunnelSpec is a declared tunnel for a client (server YAML and authenticate config payload).
type TunnelSpec struct {
	Name       string                `yaml:"name" json:"name"`
	Type       string                `yaml:"type" json:"type"`
	Upstream   string                `yaml:"upstream" json:"upstream"`
	SubDomain  string                `yaml:"subDomain" json:"subDomain,omitempty"`   // HTTP: empty = use client `http -s` (or server-assigned when both empty)
	RemotePort int                   `yaml:"remotePort" json:"remotePort,omitempty"` // TCP: 0 or omit = use client -p; else pin public listen port on server
	Auth       *HTTPIncomingAuthRule `yaml:"auth" json:"auth,omitempty"`             // HTTP: optional auth policy validated at server before forwarding.
	// Deprecated: use auth.enable + auth.users.
	Auths []HTTPTunnelAuth `yaml:"auths" json:"auths,omitempty"`
}

// HTTPTunnelAuth configures allowed Authorization values for incoming HTTP requests at the server.
type HTTPTunnelAuth struct {
	Type     string `yaml:"type" json:"type"` // basic | bearer
	Username string `yaml:"username" json:"username,omitempty"`
	Password string `yaml:"password" json:"password,omitempty"`
	Token    string `yaml:"token" json:"token,omitempty"`
}

// HTTPIncomingAuthRule controls incoming Authorization checks for tunneled HTTP requests.
type HTTPIncomingAuthRule struct {
	Enable bool             `yaml:"enable" json:"enable"`
	Users  []HTTPTunnelAuth `yaml:"users" json:"users,omitempty"`
}

type CompressionFeatures struct {
	Algorithms []string `json:"algorithms"`
	Preferred  string   `json:"preferred,omitempty"`
}

type ChunkSizeFeatures struct {
	Min     int `json:"min"`
	Max     int `json:"max"`
	Default int `json:"default"`
}

type FlowControlFeatures struct {
	WindowSize int `json:"windowSize"`
}

type CapabilityFeatures struct {
	Compression *CompressionFeatures `json:"compression,omitempty"`
	ChunkSize   *ChunkSizeFeatures   `json:"chunkSize,omitempty"`
	FlowControl *FlowControlFeatures `json:"flowControl,omitempty"`
}

type Capabilities struct {
	Flags    int                 `json:"flags"`
	Version  string              `json:"version"`
	Features *CapabilityFeatures `json:"features,omitempty"`
}

type Authentication struct {
	Version      string        `json:"version"`
	Type         string        `json:"type"`
	Port         int           `json:"port"`
	SubDomain    string        `json:"subDomain,omitempty"`
	TunnelPort   int           `json:"tunnelPort,omitempty"`
	Timestamp    int64         `json:"timestamp"`
	AuthType     string        `json:"authType,omitempty"`
	ClientId     string        `json:"clientId,omitempty"`
	Signature    string        `json:"signature"`
	Capabilities *Capabilities `json:"capabilities,omitempty"`
	OpaqueChild  bool          `json:"opaqueChild,omitempty"`
	// HTTPIngressBasic: when the server tunnel spec does not define edge auth, enforce this Basic policy on the public URL (same credentials the client uses toward upstream).
	HTTPIngressBasic *HTTPTunnelAuth `json:"httpIngressBasic,omitempty"`
}

type AuthenticateResponse struct {
	OK          bool    `json:"ok"`
	Message     string  `json:"message,omitempty"`
	Version     string  `json:"version,omitempty"`
	URL         string  `json:"url,omitempty"`
	Config      *Config `json:"config,omitempty"`
	ClientId    string  `json:"clientId,omitempty"`    // Client ID from server
	ContainerId string  `json:"containerId,omitempty"` // Container ID from server
}

type Config struct {
	Version                string              `json:"version,omitempty"`
	Notification           *NotificationConfig `json:"notification,omitempty"`
	NegotiatedCapabilities *Capabilities       `json:"negotiatedCapabilities,omitempty"`
	Tunnels                []TunnelSpec        `json:"tunnels,omitempty"`
}

// compareVersion compares two version strings (e.g., "2.0.0" vs "1.9.0")
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersion(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var num1, num2 int
		if i < len(parts1) {
			num1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			num2, _ = strconv.Atoi(parts2[i])
		}

		if num1 < num2 {
			return -1
		} else if num1 > num2 {
			return 1
		}
	}

	return 0
}

// GetClientCapabilities returns the client capabilities based on version
// For version 2.0.0+, returns full capabilities
// For older versions, returns nil (legacy protocol)
func GetClientCapabilities(version string) *Capabilities {
	// For version 2.0.0+, send full capabilities
	// For older versions, return nil (legacy protocol)
	if compareVersion(version, "2.0.0") < 0 {
		return nil
	}

	return &Capabilities{
		Flags: CapabilityFlagBinaryProtocol |
			CapabilityFlagCompression |
			CapabilityFlagStreaming |
			CapabilityFlagFlowControl |
			CapabilityFlagHTTPBinary |
			CapabilityFlagTCPOverWS |
			CapabilityFlagHTTPBodyStream,
		Version: "2.0.0",
		Features: &CapabilityFeatures{
			Compression: &CompressionFeatures{
				Algorithms: []string{"brotli", "gzip"},
			},
			ChunkSize: &ChunkSizeFeatures{
				Min:     1024,
				Max:     512 * 1024,
				Default: 64 * 1024,
			},
			FlowControl: &FlowControlFeatures{
				WindowSize: 512 * 1024,
			},
		},
	}
}

type NotificationConfig struct {
	Provider string       `json:"provider"`
	URL      string       `json:"url"`
	Interval int          `json:"interval,omitempty"`
	Alert    *AlertConfig `json:"alert,omitempty"`
}

type AlertConfig struct {
	Provider string `json:"provider"`
	URL      string `json:"url"`
	Interval int    `json:"interval,omitempty"`
}

type RequestData struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

type ResponseData struct {
	ID   string `json:"id"`
	Data string `json:"data"`
}

type TCPReadyData struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type TCPConnectData struct {
	ID        string `json:"id"`
	RequestID string `json:"requestId"`
	IP        string `json:"ip"`
}

type TCPData struct {
	StreamID string `json:"streamId"`
	Data     string `json:"data"`
}
