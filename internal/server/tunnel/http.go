package tunnel

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-idp/inlets/internal/server/protocol"
	"github.com/go-idp/inlets/internal/server/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// HTTPTunnel handles HTTP tunnel requests
type HTTPTunnel struct {
	ctx          *types.Context
	domain       string
	subDomainRe  *regexp.Regexp
	requestCount int64
	mu           sync.Mutex
}

// CreateHTTPTunnel creates a new HTTP tunnel
func CreateHTTPTunnel(ctx *types.Context, domain string) *HTTPTunnel {
	// Escape domain for regex
	escapedDomain := regexp.QuoteMeta(domain)
	subDomainRe := regexp.MustCompile(fmt.Sprintf(`([^.]+)\.%s`, escapedDomain))

	return &HTTPTunnel{
		ctx:         ctx,
		domain:      domain,
		subDomainRe: subDomainRe,
	}
}

// Attach attaches the HTTP tunnel to an HTTP server
func (t *HTTPTunnel) Attach(server *http.Server) {
	// DON'T use ConnState - it interferes with WebSocket upgrades
	// Instead, use HTTP handlers to process requests
	// The WebSocket handler (registered first) will handle WebSocket upgrades
	// This catch-all handler will handle all other requests

	// Add a catch-all handler that processes non-WebSocket requests
	// IMPORTANT: This must be registered AFTER WebSocket handler
	// WebSocket handler is registered in wsMonitor.Attach() which is called first
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Skip WebSocket path - it's handled by WebSocket monitor
		if r.URL.Path == t.ctx.Config.WSPath {
			// This shouldn't happen if WebSocket handler is registered correctly
			// But just in case, return 404
			http.NotFound(w, r)
			return
		}

		// For other requests, hijack the connection to handle at TCP level
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "HTTP/1.1 required", http.StatusHTTPVersionNotSupported)
			return
		}

		conn, _, err := hijacker.Hijack()
		if err != nil {
			log.Printf("[tunnel:http] Failed to hijack connection: %v", err)
			return
		}

		// Handle the connection at TCP level
		go t.handleConnection(conn)
	})
}

// handleConnection handles a new TCP connection
func (t *HTTPTunnel) handleConnection(tcpConn net.Conn) {
	defer tcpConn.Close()

	socketConfig := &socketConfig{
		tcpID:     uuid.New().String(),
		domain:    "",
		subDomain: "",
		isWS:      false,
		clientID:  "",
	}

	// Read data from connection
	reader := bufio.NewReader(tcpConn)

	// Handle multiple requests on the same connection (HTTP/1.1 keep-alive)
	for {
		// Read HTTP request
		req, err := http.ReadRequest(reader)
		if err != nil {
			// Connection closed or error reading request
			return
		}

		// Read request body if present
		var bodyData []byte
		if req.ContentLength > 0 {
			bodyData = make([]byte, req.ContentLength)
			if _, err := reader.Read(bodyData); err != nil {
				log.Printf("[tunnel:http] Error reading request body: %v", err)
				return
			}
		} else if req.Header.Get("Transfer-Encoding") == "chunked" {
			// Handle chunked encoding
			bodyData, err = readChunkedBody(reader)
			if err != nil {
				log.Printf("[tunnel:http] Error reading chunked body: %v", err)
				return
			}
		}

		// Reconstruct the full HTTP request
		var requestData strings.Builder
		requestData.WriteString(fmt.Sprintf("%s %s %s\r\n", req.Method, req.URL.RequestURI(), req.Proto))
		req.Header.Write(&requestData)
		requestData.WriteString("\r\n")
		if len(bodyData) > 0 {
			requestData.Write(bodyData)
		}

		// Process the request
		data := requestData.String()
		if err := t.processRequest(tcpConn, socketConfig, data, req); err != nil {
			log.Printf("[tunnel:http] Error processing request: %v", err)
			// Don't return, continue to next request if keep-alive
			if req.Close {
				return
			}
		}

		// Check if connection should be closed
		if req.Close {
			return
		}
	}
}

// readChunkedBody reads a chunked-encoded HTTP body
func readChunkedBody(reader *bufio.Reader) ([]byte, error) {
	var body []byte
	for {
		// Read chunk size line
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)

		// Parse chunk size (hex)
		var chunkSize int64
		if _, err := fmt.Sscanf(line, "%x", &chunkSize); err != nil {
			return nil, fmt.Errorf("invalid chunk size: %s", line)
		}

		if chunkSize == 0 {
			// Last chunk, read trailing CRLF
			reader.ReadString('\n')
			break
		}

		// Read chunk data
		chunk := make([]byte, chunkSize)
		if _, err := reader.Read(chunk); err != nil {
			return nil, err
		}

		// Read CRLF after chunk
		reader.ReadString('\n')

		body = append(body, chunk...)
	}
	return body, nil
}

// socketConfig holds configuration for a socket connection
type socketConfig struct {
	tcpID     string
	domain    string
	subDomain string
	isWS      bool
	clientID  string
}

// processRequest processes an HTTP request
func (t *HTTPTunnel) processRequest(tcpConn net.Conn, socketConfig *socketConfig, data string, req *http.Request) error {
	t.mu.Lock()
	t.requestCount++
	requestCount := t.requestCount
	t.mu.Unlock()

	// Skip if this is a WebSocket connection
	if socketConfig.isWS {
		return nil
	}

	// Parse HTTP request to extract Host header
	if socketConfig.domain == "" {
		socketConfig.domain = req.Host
		// Check if this is a WebSocket upgrade request
		if req.URL.Path == t.ctx.Config.WSPath {
			socketConfig.isWS = true
			return nil
		}

		// Extract subdomain
		if socketConfig.domain != "" {
			matches := t.subDomainRe.FindStringSubmatch(socketConfig.domain)
			if len(matches) > 1 {
				socketConfig.subDomain = matches[1]
			}
		}
	}

	requestLog := fmt.Sprintf("[%s.%s][request: %d]", socketConfig.subDomain, t.domain, requestCount)

	if socketConfig.subDomain == "" {
		log.Printf("[404]%s", requestLog)
		destroyConnection(tcpConn)
		return fmt.Errorf("no subdomain found")
	}

	// Get domain mapping
	domainMapping := t.ctx.DomainMappings.Get(socketConfig.subDomain)
	if domainMapping == nil {
		log.Printf("[404]%s", requestLog)
		destroyConnection(tcpConn)
		return fmt.Errorf("domain mapping not found")
	}

	// Bind TCP socket to domain mapping
	tcpConnPtr := &tcpConn
	t.ctx.DomainMappings.BindTCP(socketConfig.subDomain, tcpConnPtr)

	wsConn := domainMapping.WSSocket
	if wsConn == nil {
		log.Printf("[404]%s", requestLog)
		destroyConnection(tcpConn)
		return fmt.Errorf("websocket connection not found")
	}

	log.Printf("%s", requestLog)

	// Generate request ID
	requestID := fmt.Sprintf("%s@%d", uuid.New().String(), time.Now().UnixMilli())
	id := fmt.Sprintf("%s:%s", socketConfig.tcpID, requestID)

	// Get clientId and adapter from domain mapping
	clientID := domainMapping.ClientID
	useNewProtocol := domainMapping.UseNewProtocol

	// Get adapter (stored as interface{}, need to cast)
	var adapter protocol.ProtocolAdapter
	if domainMapping.Adapter != nil {
		if a, ok := domainMapping.Adapter.(protocol.ProtocolAdapter); ok {
			adapter = a
		}
	}

	socketConfig.clientID = clientID

	// Calculate request bytes
	requestBytes := int64(len(data))

	// Check upload bandwidth limit
	if !t.ctx.BandwidthLimiter.CheckUpload(clientID, requestBytes) {
		log.Printf("[tunnel:http][%s] Upload bandwidth limit exceeded for client: %s", socketConfig.subDomain, clientID)
		destroyConnection(tcpConn)
		return fmt.Errorf("upload bandwidth limit exceeded")
	}

	// Add request and upload bytes to stats
	t.ctx.TrafficStats.AddRequest(clientID)
	t.ctx.TrafficStats.AddUploadBytes(clientID, requestBytes)

	// Send request
	if useNewProtocol && adapter != nil {
		// New protocol: use adapter
		dataBytes := []byte(data)
		if err := adapter.SendHTTPRequest(id, dataBytes); err != nil {
			log.Printf("[tunnel:http] Failed to send HTTP request: %v", err)
			destroyConnection(tcpConn)
			return err
		}
	} else {
		// Legacy protocol: use adapter if available, otherwise send directly
		if adapter != nil {
			// Use adapter even for legacy protocol
			dataBytes := []byte(data)
			if err := adapter.SendHTTPRequest(id, dataBytes); err != nil {
				log.Printf("[tunnel:http] Failed to send HTTP request via adapter: %v", err)
				destroyConnection(tcpConn)
				return err
			}
		} else {
			// Fallback: send directly via WebSocket (legacy format)
			// This should not happen if adapter is properly set up
			message := []interface{}{
				"request",
				map[string]interface{}{
					"id":   id,
					"data": data, // Legacy protocol expects processed data
				},
			}

			messageBytes, err := json.Marshal(message)
			if err != nil {
				log.Printf("[tunnel:http] Failed to marshal message: %v", err)
				destroyConnection(tcpConn)
				return err
			}

			if err := wsConn.WriteMessage(websocket.TextMessage, messageBytes); err != nil {
				log.Printf("[tunnel:http] Failed to write WebSocket message: %v", err)
				destroyConnection(tcpConn)
				return err
			}
		}
	}

	// Create callback for response
	wrappedCallback := func(responseData string) {
		// Calculate response bytes
		responseBytes, err := base64.StdEncoding.DecodeString(responseData)
		if err != nil {
			// If decode fails, use string length as approximation
			responseBytes = []byte(responseData)
		}

		responseBytesLen := int64(len(responseBytes))

		// Check download bandwidth limit
		if !t.ctx.BandwidthLimiter.CheckDownload(clientID, responseBytesLen) {
			log.Printf("[tunnel:http][%s] Download bandwidth limit exceeded for client: %s", socketConfig.subDomain, clientID)
			destroyConnection(tcpConn)
			return
		}

		// Add download bytes to stats
		t.ctx.TrafficStats.AddDownloadBytes(clientID, responseBytesLen)

		// Write response to TCP connection
		if _, err := tcpConn.Write(responseBytes); err != nil {
			log.Printf("[tunnel:http] Failed to write response: %v", err)
			destroyConnection(tcpConn)
			return
		}
	}

	// Store callback
	t.ctx.CallbackContainer.Set(socketConfig.tcpID, requestID, wrappedCallback)

	// Handle connection close
	go func() {
		// Wait for connection to close
		buf := make([]byte, 1)
		tcpConn.Read(buf)

		// Cleanup
		t.ctx.CallbackContainer.Remove(socketConfig.tcpID)

		// Log stats if clientID is available
		if socketConfig.clientID != "" {
			statsInfo := t.ctx.TrafficStats.FormatStats(socketConfig.clientID)
			subDomainInfo := ""
			if socketConfig.subDomain != "" {
				subDomainInfo = fmt.Sprintf("[%s]", socketConfig.subDomain)
			}
			log.Printf("[tunnel:http]%s connection closed - Traffic Stats: %s", subDomainInfo, statsInfo)
		}
	}()

	return nil
}

// extractHostFromRawRequest extracts Host header from raw HTTP request
func extractHostFromRawRequest(data string) string {
	lines := strings.Split(data, "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), "host:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// destroyConnection sends 404 response and closes connection
func destroyConnection(tcpConn net.Conn) {
	response := []string{
		"HTTP/1.1 404 Not Found",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Length: 9",
		fmt.Sprintf("Date: %s", time.Now().UTC().Format(http.TimeFormat)),
		"Connection: close",
		"",
		"Not Found",
	}

	responseStr := strings.Join(response, "\r\n")
	tcpConn.Write([]byte(responseStr))
	tcpConn.Close()
}
