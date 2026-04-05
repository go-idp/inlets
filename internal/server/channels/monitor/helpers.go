package monitor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-zoox/logger"
	"github.com/gorilla/websocket"
)

// Server capabilities configuration
var serverCapabilities = &client.Capabilities{
	Flags: client.CapabilityFlagBinaryProtocol |
		client.CapabilityFlagCompression |
		client.CapabilityFlagStreaming |
		client.CapabilityFlagFlowControl |
		client.CapabilityFlagHTTPBinary |
		client.CapabilityFlagHTTPStreaming |
		client.CapabilityFlagTCPOverWS |
		client.CapabilityFlagTCPMultiplex |
		client.CapabilityFlagHTTPBodyStream,
	Version: "2.0.0",
	Features: &client.CapabilityFeatures{
		Compression: &client.CompressionFeatures{
			Algorithms: []string{"brotli", "gzip"},
		},
		ChunkSize: &client.ChunkSizeFeatures{
			Min:     1024,
			Max:     1024 * 1024,
			Default: 64 * 1024,
		},
		FlowControl: &client.FlowControlFeatures{
			WindowSize: 1024 * 1024, // 1MB
		},
	},
}

// sendPong sends pong response to client ping
func sendPong(wsConn *WebSocketConnection) {
	msg := []interface{}{
		"pong",
		nil,
	}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		logger.Infof("[monitor:ws] Failed to marshal pong message: %v", err)
		return
	}

	// gorilla/websocket requires serialized writes - use writeMu
	wsConn.writeMu.Lock()
	err = wsConn.WriteMessage(websocket.TextMessage, msgBytes)
	wsConn.writeMu.Unlock()

	if err != nil {
		logger.Infof("[monitor:ws] Failed to send pong message: %v", err)
	}
}

// sendAuthResponse sends authentication response
func sendAuthResponse(wsConn *WebSocketConnection, options *CreateWebSocketOptions, ok bool, message string, url string, config *client.Config) {
	response := map[string]interface{}{
		"ok": ok,
	}

	if !ok {
		response["message"] = message
	} else {
		response["version"] = options.Version
		if url != "" {
			response["url"] = url
		}
		if config != nil {
			response["config"] = config
		}
		// Include clientId and containerId for client to connect data channel
		wsConn.mu.RLock()
		response["clientId"] = wsConn.ClientID
		response["containerId"] = wsConn.ContainerID
		wsConn.mu.RUnlock()
	}

	msg := []interface{}{"authenticate", response}
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		logger.Infof("[monitor:ws] Failed to marshal auth response: %v", err)
		return
	}
	wsConn.writeMu.Lock()
	err = wsConn.WriteMessage(websocket.TextMessage, msgBytes)
	wsConn.writeMu.Unlock()
	if err != nil {
		logger.Infof("[monitor:ws] Failed to send auth response: %v", err)
	}
}

// hostOnly returns the host part of a Host header value (strips bracketed IPv6 and port).
func hostOnly(hostport string) string {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

// getServerUrlBySubDomain gets the public tunnel URL for a subdomain.
// If options.Domain is empty, requestHost (from the WebSocket :authority / Host) is used so clients
// still get a usable URL when the server forgot to set -domain (otherwise https://sub. with no suffix).
func getServerUrlBySubDomain(subDomain string, options *CreateWebSocketOptions, requestHost string) string {
	if subDomain == "" {
		return ""
	}

	domain := strings.TrimSpace(options.Domain)
	if domain == "" {
		domain = strings.TrimSpace(requestHost)
		if domain != "" {
			logger.Infof("[monitor:ws] server domain not configured; using Host %q for public tunnel URL (set server domain if it differs from the WebSocket host)", domain)
		}
	}
	if domain == "" {
		return ""
	}

	if options.Secure {
		return fmt.Sprintf("https://%s.%s", subDomain, domain)
	}

	return fmt.Sprintf("http://%s.%s:%d", subDomain, domain, options.Port)
}

// negotiateCapabilities negotiates capabilities between client and server
func negotiateCapabilities(clientCapabilities *client.Capabilities) *client.Capabilities {
	// Legacy client: no capabilities or flags are 0
	if clientCapabilities == nil || clientCapabilities.Flags == 0 {
		return nil
	}

	// Negotiate flags (intersection)
	negotiatedFlags := clientCapabilities.Flags & serverCapabilities.Flags

	// If no common capabilities, fallback to legacy
	if negotiatedFlags == 0 {
		logger.Infof("[monitor:ws] No common capabilities found, falling back to legacy protocol")
		return nil
	}

	// Negotiate features
	negotiatedFeatures := &client.CapabilityFeatures{}

	// Compression algorithm negotiation
	if negotiatedFlags&client.CapabilityFlagCompression != 0 {
		clientAlgorithms := []string{}
		if clientCapabilities.Features != nil && clientCapabilities.Features.Compression != nil {
			clientAlgorithms = clientCapabilities.Features.Compression.Algorithms
		}
		serverAlgorithms := serverCapabilities.Features.Compression.Algorithms

		commonAlgorithms := []string{}
		for _, alg := range clientAlgorithms {
			for _, serverAlg := range serverAlgorithms {
				if alg == serverAlg {
					commonAlgorithms = append(commonAlgorithms, alg)
					break
				}
			}
		}

		if len(commonAlgorithms) > 0 {
			preferred := commonAlgorithms[0]
			for _, alg := range commonAlgorithms {
				if alg == "brotli" {
					preferred = "brotli"
					break
				}
			}
			negotiatedFeatures.Compression = &client.CompressionFeatures{
				Algorithms: commonAlgorithms,
				Preferred:  preferred,
			}
		}
	}

	// Chunk size negotiation
	if negotiatedFlags&client.CapabilityFlagStreaming != 0 {
		clientChunk := clientCapabilities.Features.ChunkSize
		serverChunk := serverCapabilities.Features.ChunkSize

		if clientChunk != nil && serverChunk != nil {
			min := clientChunk.Min
			if serverChunk.Min > min {
				min = serverChunk.Min
			}
			max := clientChunk.Max
			if serverChunk.Max < max {
				max = serverChunk.Max
			}
			defaultSize := clientChunk.Default
			if serverChunk.Default < defaultSize {
				defaultSize = serverChunk.Default
			}
			negotiatedFeatures.ChunkSize = &client.ChunkSizeFeatures{
				Min:     min,
				Max:     max,
				Default: defaultSize,
			}
		} else if serverChunk != nil {
			negotiatedFeatures.ChunkSize = serverChunk
		}
	}

	// Flow control window negotiation
	if negotiatedFlags&client.CapabilityFlagFlowControl != 0 {
		clientWindow := 0
		if clientCapabilities.Features != nil && clientCapabilities.Features.FlowControl != nil {
			clientWindow = clientCapabilities.Features.FlowControl.WindowSize
		}
		serverWindow := serverCapabilities.Features.FlowControl.WindowSize

		windowSize := clientWindow
		if serverWindow < windowSize || windowSize == 0 {
			windowSize = serverWindow
		}
		if windowSize == 0 {
			windowSize = 1024 * 1024 // 1MB default
		}

		negotiatedFeatures.FlowControl = &client.FlowControlFeatures{
			WindowSize: windowSize,
		}
	}

	return &client.Capabilities{
		Flags:    negotiatedFlags,
		Version:  serverCapabilities.Version,
		Features: negotiatedFeatures,
	}
}

// isLegacyClient checks if client is legacy
func isLegacyClient(auth *client.Authentication) bool {
	return auth.Capabilities == nil ||
		auth.Capabilities.Flags == 0
}

// formatProtocolConfiguration formats protocol configuration for logging
func formatProtocolConfiguration(capabilities *client.Capabilities) string {
	if capabilities == nil {
		return "legacy (flags=0x0)"
	}

	capabilityNames := []struct {
		flag  int
		label string
	}{
		{client.CapabilityFlagBinaryProtocol, "BINARY_PROTOCOL"},
		{client.CapabilityFlagCompression, "COMPRESSION"},
		{client.CapabilityFlagStreaming, "STREAMING"},
		{client.CapabilityFlagFlowControl, "FLOW_CONTROL"},
		{client.CapabilityFlagHTTPBinary, "HTTP_BINARY"},
		{client.CapabilityFlagHTTPStreaming, "HTTP_STREAMING"},
		{client.CapabilityFlagTCPOverWS, "TCP_OVER_WS"},
		{client.CapabilityFlagTCPMultiplex, "TCP_MULTIPLEX"},
		{client.CapabilityFlagHTTPBodyStream, "HTTP_BODY_STREAM"},
	}

	enabled := []string{}
	for _, item := range capabilityNames {
		if capabilities.Flags&item.flag != 0 {
			enabled = append(enabled, item.label)
		}
	}

	parts := []string{
		fmt.Sprintf("features=[%s]", strings.Join(enabled, ", ")),
		fmt.Sprintf("version=%s", capabilities.Version),
	}

	featureParts := []string{}
	if capabilities.Features != nil {
		if capabilities.Features.Compression != nil {
			algorithms := strings.Join(capabilities.Features.Compression.Algorithms, "/")
			preferred := ""
			if capabilities.Features.Compression.Preferred != "" {
				preferred = fmt.Sprintf(", preferred=%s", capabilities.Features.Compression.Preferred)
			}
			featureParts = append(featureParts, fmt.Sprintf("compression(%s%s)", algorithms, preferred))
		}

		if capabilities.Features.ChunkSize != nil {
			featureParts = append(featureParts, fmt.Sprintf("chunkSize(min=%d, max=%d, default=%d)",
				capabilities.Features.ChunkSize.Min,
				capabilities.Features.ChunkSize.Max,
				capabilities.Features.ChunkSize.Default))
		}

		if capabilities.Features.FlowControl != nil {
			featureParts = append(featureParts, fmt.Sprintf("flowControl(window=%d)", capabilities.Features.FlowControl.WindowSize))
		}
	}

	if len(featureParts) > 0 {
		parts = append(parts, strings.Join(featureParts, "; "))
	}

	return strings.Join(parts, " | ")
}

// base64Encode encodes data to base64 string
func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// base64Decode decodes base64 string to data
func base64Decode(data string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// parseHTTPResponsePayload extracts id and data from a client ["response", payload] message.
func parseHTTPResponsePayload(payload interface{}) (id string, data string, ok bool) {
	if m, ok := payload.(map[string]interface{}); ok {
		idStr, _ := m["id"].(string)
		dataStr, _ := m["data"].(string)
		if idStr != "" && dataStr != "" {
			return idStr, dataStr, true
		}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", "", false
	}
	var rd struct {
		ID   string `json:"id"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(b, &rd); err != nil {
		return "", "", false
	}
	if rd.ID != "" && rd.Data != "" {
		return rd.ID, rd.Data, true
	}
	return "", "", false
}
