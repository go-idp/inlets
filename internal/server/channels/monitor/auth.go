package monitor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-idp/inlets/internal/client"
	"github.com/go-idp/inlets/internal/server/protocol"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/go-idp/inlets/internal/server/utils"
	"github.com/go-zoox/logger"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// handleAuthenticate handles authentication
func handleAuthenticate(
	ctx *types.Context,
	options *CreateWebSocketOptions,
	emitter *EventEmitter,
	wsConn *WebSocketConnection,
	payload interface{},
	isAuthenticated *bool,
	subDomain *string,
) error {
	// Parse authentication payload
	authBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var auth client.Authentication
	if err := json.Unmarshal(authBytes, &auth); err != nil {
		return err
	}

	// Define clientId
	clientId := auth.ClientId
	if clientId == "" {
		clientId = fmt.Sprintf("anonymous-%s", uuid.New().String()[:8])
	}

	logger.Infof("[monitor:ws][%s] version: %s", clientId, auth.Version)

	// Version check (warning only)
	clientVersion := auth.Version
	serverVersion := options.Version
	if !utils.IsVersionGreaterOrEqual(clientVersion, serverVersion) {
		logger.Infof("[monitor:ws] Warning: client version(%s) should be >= server(%s)", clientVersion, serverVersion)
		// Send warning
		warnMsg := []interface{}{
			"warn",
			fmt.Sprintf("Warning: client version(%s) should be >= server(%s)", clientVersion, serverVersion),
		}
		warnBytes, _ := json.Marshal(warnMsg)
		wsConn.writeMu.Lock()
		wsConn.WriteMessage(websocket.TextMessage, warnBytes)
		wsConn.writeMu.Unlock()
	}

	// Get token
	authType := types.AuthType(auth.AuthType)
	if authType == "" {
		authType = types.AuthTypeToken
	}

	tokenOptions := &types.GetTokenOptions{
		Type: types.TunnelType(auth.Type),
	}

	tokenRes, err := options.Token(authType, auth.ClientId, tokenOptions)
	if err != nil {
		sendAuthResponse(wsConn, options, false, fmt.Sprintf("invalid client: %v", err), "", nil)
		return err
	}

	// Check public auth type
	if tokenRes.AuthType == types.AuthTypePublic && auth.SubDomain != "" {
		sendAuthResponse(wsConn, options, false, "subDomain is not allowed for public authType", "", nil)
		return fmt.Errorf("subDomain not allowed for public auth")
	}

	signedSecret := tokenRes.Token
	if signedSecret == "" {
		sendAuthResponse(wsConn, options, false, fmt.Sprintf("invalid client(%s)", authType), "", nil)
		return fmt.Errorf("invalid token")
	}

	// Verify signature
	expectedSignature := utils.HMACSHA512(strconv.FormatInt(auth.Timestamp, 10), signedSecret)
	if expectedSignature != auth.Signature {
		sendAuthResponse(wsConn, options, false, "invalid signature", "", nil)
		return fmt.Errorf("invalid signature")
	}

	logger.Infof("[monitor:ws][%s] type: %s", clientId, auth.Type)

	// Capability negotiation
	negotiatedCapabilities := negotiateCapabilities(auth.Capabilities)
	useNewProtocol := negotiatedCapabilities != nil

	protocolSummary := formatProtocolConfiguration(negotiatedCapabilities)
	if useNewProtocol {
		logger.Infof("[monitor:ws][%s] Using new protocol", clientId)
	} else {
		logger.Infof("[monitor:ws][%s] Using legacy protocol", clientId)
	}
	logger.Infof("[monitor:ws][%s] protocol configuration => %s", clientId, protocolSummary)

	// Save capabilities to connection
	wsConn.mu.Lock()
	wsConn.Capabilities = negotiatedCapabilities
	wsConn.UseNewProtocol = useNewProtocol
	wsConn.IsLegacyClient = isLegacyClient(&auth)
	wsConn.mu.Unlock()

	// Create protocol adapter
	adapter := protocol.Create(wsConn.Conn, negotiatedCapabilities, false)
	adapter.SetConnWriteMu(&wsConn.writeMu)
	wsConn.mu.Lock()
	wsConn.Adapter = adapter
	wsConn.mu.Unlock()

	// Create container
	containerId := uuid.New().String()

	// Handle tunnel type
	if auth.Type == "tcp" {
		if auth.TunnelPort != 0 {
			logger.Infof("[monitor:ws][%s] tunnel port: %d", clientId, auth.TunnelPort)
		}

		ctx.Container.Create(containerId, options.Token, wsConn.Conn, &auth, &wsConn.writeMu)

		// Store adapter and protocol info in container
		wsConn.mu.RLock()
		adapter := wsConn.Adapter
		useNewProtocol := wsConn.UseNewProtocol
		wsConn.mu.RUnlock()

		if err := ctx.Container.Set(containerId, "adapter", adapter); err != nil {
			logger.Infof("[monitor:ws] Failed to set adapter in container: %v", err)
		}
		if err := ctx.Container.Set(containerId, "useNewProtocol", useNewProtocol); err != nil {
			logger.Infof("[monitor:ws] Failed to set useNewProtocol in container: %v", err)
		}

	} else if auth.Type == "http" {
		// HTTP tunnel
		wsConn.mu.RLock()
		adapter := wsConn.Adapter
		useNewProtocol := wsConn.UseNewProtocol
		wsConn.mu.RUnlock()

		if domainContainer, ok := ctx.DomainMappings.(interface {
			BindWSWithMetadata(wsSocket *websocket.Conn, subDomain string, clientID string, adapter interface{}, useNewProtocol bool) string
		}); ok {
			if auth.SubDomain == "" {
				*subDomain = domainContainer.BindWSWithMetadata(wsConn.Conn, "", clientId, adapter, useNewProtocol)
			} else {
				logger.Infof("[monitor:ws][%s][domain] request: %s.%s", clientId, auth.SubDomain, options.Domain)

				if ctx.DomainMappings.Has(auth.SubDomain) {
					sendAuthResponse(wsConn, options, false, "domain id has been used, please use another", "", nil)
					return fmt.Errorf("subdomain already used")
				}

				*subDomain = domainContainer.BindWSWithMetadata(wsConn.Conn, auth.SubDomain, clientId, adapter, useNewProtocol)
			}
		} else {
			if auth.SubDomain == "" {
				*subDomain = ctx.DomainMappings.BindWS(wsConn.Conn, "")
			} else {
				logger.Infof("[monitor:ws][%s][domain] request: %s.%s", clientId, auth.SubDomain, options.Domain)

				if ctx.DomainMappings.Has(auth.SubDomain) {
					sendAuthResponse(wsConn, options, false, "domain id has been used, please use another", "", nil)
					return fmt.Errorf("subdomain already used")
				}

				*subDomain = ctx.DomainMappings.BindWS(wsConn.Conn, auth.SubDomain)
			}
		}

		logger.Infof("[monitor:ws][%s][domain] %s.%s", clientId, *subDomain, options.Domain)
	} else {
		return fmt.Errorf("unknown authentication type: %s", auth.Type)
	}

	// Set connection metadata
	wsConn.mu.Lock()
	wsConn.ContainerID = containerId
	wsConn.ClientID = clientId
	wsConn.mu.Unlock()

	// Build server config
	config := &client.Config{
		Version:                options.Version,
		NegotiatedCapabilities: negotiatedCapabilities,
	}

	// Merge client config if exists
	if tokenRes.Config != nil {
		config.Notification = tokenRes.Config.Notification
	}

	// Send authentication success response
	url := getServerUrlBySubDomain(*subDomain, options, wsConn.RequestHost)
	sendAuthResponse(wsConn, options, true, "", url, config)

	logger.Infof("[monitor:ws][%s] authenticated successfully (container: %s)", clientId, containerId)

	// Set start time for traffic stats
	ctx.TrafficStats.SetStartTime(clientId)

	// Setup new protocol response handler
	if useNewProtocol {
		adapter.OnHTTPResponse(func(id string, data []byte) error {
			parts := strings.Split(id, ":")
			if len(parts) >= 2 {
				tcpId := parts[0]
				requestId := strings.Join(parts[1:], ":")
				callback := ctx.CallbackContainer.Take(tcpId, requestId)
				if callback != nil {
					callback(base64Encode(data))
				}
			}
			return nil
		})
	}

	// Send notification (async to avoid blocking message loop)
	if options.Notification != nil {
		go func() {
			options.Notification.Notify(fmt.Sprintf("[上线] 客户端 - %s", clientId), []string{
				fmt.Sprintf("客户端版本：%s", auth.Version),
				fmt.Sprintf("客户端类型：%s", auth.Type),
				fmt.Sprintf("客户端授权方式：%s", auth.AuthType),
				fmt.Sprintf("客户端端口：%d", auth.TunnelPort),
				fmt.Sprintf("当前时间：%s", time.Now().Format("2006-01-02 15:04:05")),
			})
		}()
	}

	// Emit tunnel event (async to avoid blocking)
	go func() {
		emitter.Emit("tunnel", map[string]interface{}{
			"type":        auth.Type,
			"containerId": containerId,
		})
	}()

	return nil
}

// handleResponse handles HTTP response from client
func handleResponse(ctx *types.Context, wsConn *WebSocketConnection, payload interface{}) {
	// Both legacy and new protocol clients send ["response", {id, data}] as TextMessage on the monitor channel.
	// BinaryProtocolAdapter.setupEventListeners is not started on the server; skipping here left responses unhandled
	// and HTTP tunnels stuck until timeout.
	id, data, ok := parseHTTPResponsePayload(payload)
	if !ok {
		logger.Infof("[monitor:ws] handleResponse: missing id/data (payload type %T)", payload)
		return
	}

	parts := strings.Split(id, ":")
	if len(parts) < 2 {
		logger.Infof("[monitor:ws] handleResponse: malformed id %q", id)
		return
	}
	tcpId := parts[0]
	requestId := strings.Join(parts[1:], ":")

	rawOuter, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		logger.Infof("[monitor:ws] handleResponse: base64 decode: %v", err)
		return
	}
	if bm, perr := protocol.ParseBinaryMessage(rawOuter); perr == nil {
		if bm.Type == protocol.MessageTypeHTTPResponseHead || bm.Type == protocol.MessageTypeHTTPResponseBody {
			payload := bm.Data
			if bm.Type == protocol.MessageTypeHTTPResponseHead && wsConn != nil {
				var derr error
				payload, derr = protocol.DecompressBinaryPayloadForCapabilities(wsConn.Capabilities, payload)
				if derr != nil {
					logger.Infof("[monitor:ws] handleResponse: decompress response head: %v", derr)
					return
				}
			}
			if ctx.HTTPStreamDispatch == nil {
				logger.Infof("[monitor:ws] handleResponse: semantic HTTP response but HTTPStreamDispatch unset")
				return
			}
			fin := (bm.Flags & protocol.MessageFlagFIN) != 0
			if !ctx.HTTPStreamDispatch(tcpId, requestId, uint8(bm.Type), payload, fin) {
				logger.Infof("[monitor:ws] handleResponse: no stream session for semantic frame id %q", id)
			}
			return
		}
	}

	callback := ctx.CallbackContainer.Take(tcpId, requestId)
	if callback == nil {
		logger.Infof("[monitor:ws] handleResponse: no pending tunnel request for id %q", id)
		return
	}
	decoded, err := base64Decode(data)
	if err != nil {
		logger.Infof("[monitor:ws] handleResponse: base64 decode inner: %v", err)
		return
	}
	callback(decoded)
}

// handleDisconnect handles client disconnect
func handleDisconnect(ctx *types.Context, options *CreateWebSocketOptions, wsConn *WebSocketConnection, subDomain string, clientId string) {
	wsConn.mu.RLock()
	containerId := wsConn.ContainerID
	wsConn.mu.RUnlock()

	// Set end time for traffic stats
	ctx.TrafficStats.SetEndTime(clientId)

	// Get stats
	statsInfo := ctx.TrafficStats.FormatStats(clientId)
	logger.Infof("[monitor:ws][%s] disconnected - Traffic Stats: %s", clientId, statsInfo)

	// Unbind domain mapping
	if subDomain != "" {
		ctx.DomainMappings.UnbindWS(subDomain)
	}

	// Get container for notification
	container := ctx.Container.Get(containerId)
	if container == nil {
		logger.Infof("[monitor:ws] Cannot get container id: %s", containerId)
		return
	}

	// Send notification
	if options.Notification != nil {
		options.Notification.Notify(fmt.Sprintf("[掉线] 客户端 - %s", clientId), []string{
			fmt.Sprintf("客户端版本：%s", container.Version),
			fmt.Sprintf("客户端类型：%s", container.Type),
			fmt.Sprintf("客户端授权方式：%s", container.AuthType),
			fmt.Sprintf("客户端端口：%d", func() int {
				if container.TunnelPort != nil {
					return *container.TunnelPort
				}
				return 0
			}()),
			fmt.Sprintf("流量统计：%s", statsInfo),
			fmt.Sprintf("当前时间：%s", time.Now().Format("2006-01-02 15:04:05")),
		})
	}

	// Cleanup adapter
	wsConn.mu.RLock()
	if wsConn.Adapter != nil {
		wsConn.Adapter.Destroy()
	}
	wsConn.mu.RUnlock()

	// Cleanup container (this will close TCP server and release port)
	if container.Destroy != nil {
		container.Destroy()
		logger.Infof("[monitor:ws][%s] Container destroyed, TCP server closed and port released", clientId)
	}
}
