package client

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const upstreamRequestTimeout = 60 * time.Second

func (c *Client) handleAuthenticateResponse(payload interface{}) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var resp AuthenticateResponse
	if err := json.Unmarshal(dataBytes, &resp); err != nil {
		return err
	}

	if !resp.OK {
		c.authTimeout.Stop()
		return fmt.Errorf("authentication failed: %s", resp.Message)
	}

	c.authTimeout.Stop()
	c.logger.Printf("[authenticate] connected")

	// Save clientId and containerId from server response
	if resp.ClientId != "" {
		c.clientId = resp.ClientId
		c.logger.Printf("[authenticate] Client ID: %s", c.clientId)
	}
	if resp.ContainerId != "" {
		c.containerId = resp.ContainerId
		c.logger.Printf("[authenticate] Container ID: %s", c.containerId)
	}

	// Handle protocol negotiation
	if resp.Config != nil && resp.Config.NegotiatedCapabilities != nil {
		c.negotiatedCapabilities = resp.Config.NegotiatedCapabilities
		c.logger.Printf("[authenticate] Using new protocol")
		if resp.Config.NegotiatedCapabilities.Features != nil {
			if resp.Config.NegotiatedCapabilities.Features.Compression != nil {
				preferred := resp.Config.NegotiatedCapabilities.Features.Compression.Preferred
				if preferred == "" && len(resp.Config.NegotiatedCapabilities.Features.Compression.Algorithms) > 0 {
					preferred = resp.Config.NegotiatedCapabilities.Features.Compression.Algorithms[0]
				}
				c.logger.Printf("[authenticate] Negotiated compression: %v (preferred: %s)",
					resp.Config.NegotiatedCapabilities.Features.Compression.Algorithms, preferred)
			}
		}
	} else {
		c.negotiatedCapabilities = nil
		c.logger.Printf("[authenticate] Using legacy protocol")
	}

	if c.opts.Type == "http" {
		c.logger.Printf("Forwarding: %s -> %s://%s:%d", resp.URL, c.opts.Type, c.opts.UpstreamHost, c.opts.UpstreamPort)
	}

	return nil
}

func (c *Client) handleHTTPRequest(payload interface{}) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var reqData RequestData
	if err := json.Unmarshal(dataBytes, &reqData); err != nil {
		return err
	}

	// Decompress data (currently no-op, matching TypeScript implementation)
	data, err := decompress(reqData.Data)
	if err != nil {
		return err
	}

	// Forward HTTP request to upstream
	go c.forwardHTTPRequest(reqData.ID, data)

	return nil
}

func (c *Client) forwardHTTPRequest(id string, data string) {
	upstreamAddr := joinHostPort(c.opts.UpstreamHost, c.opts.UpstreamPort)
	conn, err := net.DialTimeout("tcp", upstreamAddr, 10*time.Second)
	if err != nil {
		c.logger.Printf("Failed to connect to upstream: %v", err)
		return
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(upstreamRequestTimeout)); err != nil {
		c.logger.Printf("Failed to set upstream deadline: %v", err)
		return
	}

	if _, err := conn.Write([]byte(data)); err != nil {
		c.logger.Printf("Failed to write to upstream: %v", err)
		return
	}

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		c.logger.Printf("Failed to read upstream HTTP response: %v", err)
		return
	}
	defer resp.Body.Close()

	var response bytes.Buffer
	if err := resp.Write(&response); err != nil {
		c.logger.Printf("Failed to serialize upstream HTTP response: %v", err)
		return
	}

	compressed, err := compress(base64.StdEncoding.EncodeToString(response.Bytes()))
	if err != nil {
		c.logger.Printf("Failed to compress response: %v", err)
		return
	}

	respData := ResponseData{
		ID:   id,
		Data: compressed,
	}

	if err := c.sendMonitorMessage("response", respData); err != nil {
		c.logger.Printf("Failed to send response: %v", err)
	}
}

func (c *Client) handleTCPReady(payload interface{}) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var readyData TCPReadyData
	if err := json.Unmarshal(dataBytes, &readyData); err != nil {
		return err
	}

	c.logger.Printf("Forwarding: tcp://%s:%d -> %s:%d", readyData.Host, readyData.Port, c.opts.UpstreamHost, c.opts.UpstreamPort)
	return nil
}

func (c *Client) handleTCPConnect(payload interface{}, remoteHost string) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var connectData TCPConnectData
	if err := json.Unmarshal(dataBytes, &connectData); err != nil {
		return err
	}

	c.logger.Printf("[tunnel:tcp][user] connected (request id: %s, ip: %s)", connectData.RequestID, connectData.IP)

	// Check if using new protocol (TCP over WebSocket)
	useNewProtocol := c.negotiatedCapabilities != nil &&
		(c.negotiatedCapabilities.Flags&CapabilityFlagTCPOverWS) != 0

	if useNewProtocol {
		// New protocol: TCP over WebSocket
		// Register stream immediately to avoid race condition with tcp:data
		streamID := fmt.Sprintf("%s:%s", connectData.ID, connectData.RequestID)
		c.tcpStreamsMu.Lock()
		// Use nil as placeholder - will be replaced when connection is established
		// This ensures handleTCPData can find the stream even if data arrives before connection is ready
		c.tcpStreams[streamID] = nil
		c.tcpStreamsMu.Unlock()

		// Now establish connection asynchronously
		go c.forwardTCPConnectionOverWS(connectData.ID, connectData.RequestID, remoteHost)
	} else {
		// Legacy protocol: independent TCP connections
		go c.forwardTCPConnection(connectData.ID, connectData.RequestID, remoteHost)
	}

	return nil
}

func (c *Client) forwardTCPConnection(id, requestID, remoteHost string) {
	localAddr := joinHostPort(c.opts.UpstreamHost, c.opts.UpstreamPort)
	localConn, err := net.Dial("tcp", localAddr)
	if err != nil {
		c.logger.Printf("[local] failed to connect: %v", err)
		return
	}

	remoteAddr := joinHostPort(remoteHost, c.opts.RemoteTCPPort)
	remoteConn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		c.logger.Printf("[remote] failed to connect: %v", err)
		localConn.Close()
		return
	}

	var signedSecret string
	if c.opts.AuthType == "credentials" {
		signedSecret = c.opts.ClientSecret
	} else {
		signedSecret = c.opts.Token
	}

	signature := hmacSHA256(id, signedSecret)
	authData := fmt.Sprintf("%s%s%s%s", tunnelTCPFlag, id, requestID, signature)

	if _, err := remoteConn.Write([]byte(authData)); err != nil {
		c.logger.Printf("[remote] failed to send auth: %v", err)
		localConn.Close()
		remoteConn.Close()
		return
	}

	buffer := make([]byte, len(tunnelTCPOKFlag)+4096)
	n, err := remoteConn.Read(buffer)
	if err != nil {
		c.logger.Printf("[remote] failed to read auth response: %v", err)
		localConn.Close()
		remoteConn.Close()
		return
	}

	okFlag := string(buffer[:len(tunnelTCPOKFlag)])
	if okFlag != tunnelTCPOKFlag {
		if n > len(tunnelTCPOKFlag) {
			errorMsg := string(buffer[len(tunnelTCPOKFlag):n])
			c.logger.Printf("[remote] authentication error: %s", errorMsg)
		} else {
			c.logger.Printf("[remote] authentication error: expected %s, got %s", tunnelTCPOKFlag, okFlag)
		}
		localConn.Close()
		remoteConn.Close()
		return
	}

	c.logger.Printf("[remote] authenticated")

	if n > len(tunnelTCPOKFlag) {
		rest := buffer[len(tunnelTCPOKFlag):n]
		if _, err := localConn.Write(rest); err != nil {
			c.logger.Printf("[local] failed to write remaining data: %v", err)
		}
	}

	go pipeConn(localConn, remoteConn)
	go pipeConn(remoteConn, localConn)
}

func pipeConn(src, dst net.Conn) {
	defer src.Close()
	defer dst.Close()

	buffer := make([]byte, 4096)
	for {
		n, err := src.Read(buffer)
		if err != nil {
			return
		}
		if _, err := dst.Write(buffer[:n]); err != nil {
			return
		}
	}
}

// forwardTCPConnectionOverWS handles TCP connection using new protocol (TCP over WebSocket)
func (c *Client) forwardTCPConnectionOverWS(id, requestID, remoteHost string) {
	streamID := fmt.Sprintf("%s:%s", id, requestID)

	// Connect to local upstream
	localAddr := joinHostPort(c.opts.UpstreamHost, c.opts.UpstreamPort)
	localConn, err := net.Dial("tcp", localAddr)
	if err != nil {
		c.logger.Printf("[local][%s] failed to connect: %v", streamID, err)
		// Clean up placeholder if connection failed
		c.tcpStreamsMu.Lock()
		delete(c.tcpStreams, streamID)
		c.tcpStreamsMu.Unlock()
		return
	}

	c.logger.Printf("[local][%s] connected", streamID)

	// Update the stream with actual connection
	// The stream was already registered in handleTCPConnect (with nil placeholder) to avoid race condition
	c.tcpStreamsMu.Lock()
	// Always update - replace placeholder (nil) with actual connection
	c.tcpStreams[streamID] = localConn
	c.tcpStreamsMu.Unlock()

	// Track if cleanup has been called to avoid double cleanup
	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			c.tcpStreamsMu.Lock()
			_, exists := c.tcpStreams[streamID]
			if exists {
				delete(c.tcpStreams, streamID)
			}
			c.tcpStreamsMu.Unlock()

			if localConn != nil {
				localConn.Close()
			}
			// Close and remove data channel for this stream
			c.removeDataChannel(streamID)
			c.logger.Printf("[tcp:data][%s] stream closed", streamID)
		})
	}

	// Setup local socket event handlers (similar to setupLocalSocket in TypeScript)
	// Handle local connection errors
	go func() {
		// Monitor connection state
		// In Go, we can't directly listen to socket events like in Node.js
		// Instead, we rely on Read() returning errors when connection closes
		// But we can also check connection state periodically or use SetDeadline
	}()

	// Read from local connection and send to server via WebSocket
	go func() {
		defer cleanup()

		// Wait for data channel to be ready for this stream (up to 5 seconds)
		for i := 0; i < 50; i++ {
			if conn, _ := c.getDataChannel(streamID); conn != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		buffer := make([]byte, 4096)
		for {
			n, err := localConn.Read(buffer)
			if err != nil {
				// Check if it's a normal connection close
				// "connection reset by peer" is normal when the remote side closes the connection
				errStr := err.Error()
				isNormalClose := false

				// Check for EOF
				if errStr == "EOF" {
					isNormalClose = true
				}

				// Check for connection reset by peer (ECONNRESET)
				if opErr, ok := err.(*net.OpError); ok {
					if opErr.Err == syscall.ECONNRESET ||
						strings.Contains(errStr, "connection reset by peer") {
						isNormalClose = true
					}
				} else if strings.Contains(errStr, "connection reset by peer") {
					isNormalClose = true
				}

				// Check for use of closed network connection
				if strings.Contains(errStr, "use of closed network connection") {
					isNormalClose = true
				}

				if isNormalClose {
					// Normal connection close - don't log as error
					// Connection was closed by the remote side, which is expected behavior
					return
				}

				// Unexpected error
				c.logger.Printf("[local][%s] read error: %v", streamID, err)
				return
			}

			if n == 0 {
				// EOF - normal connection close
				c.logger.Printf("[local][%s] connection closed (EOF)", streamID)
				return
			}

			// Build binary message according to protocol
			// Get next sequence number for this stream
			c.sequenceCounterMu.Lock()
			seq := c.sequenceCounter[streamID]
			c.sequenceCounter[streamID] = seq + 1
			c.sequenceCounterMu.Unlock()

			// Build binary message: TCP_DATA (0x03) with FIN flag (0x01)
			binaryMsg := buildBinaryMessage(BinaryMessage{
				Type:     0x03, // TCP_DATA
				StreamID: streamID,
				Sequence: seq,
				Flags:    0x01, // FIN flag for single chunk
				Data:     buffer[:n],
			})

			// Send binary message directly via WebSocket BinaryMessage
			dataConn, writeMu := c.getDataChannel(streamID)
			if dataConn == nil {
				c.logger.Printf("[tcp:data][%s] Data connection is not ready", streamID)
				return
			}

			if writeMu != nil {
				writeMu.Lock()
			}
			err = dataConn.WriteMessage(websocket.BinaryMessage, binaryMsg)
			if writeMu != nil {
				writeMu.Unlock()
			}
			if err != nil {
				c.logger.Printf("[tcp:data][%s] Failed to send data to server: %v", streamID, err)
				return
			}
		}
	}()

	// Note: Data from server to client is handled by handleTCPData
	// The cleanup will be called when:
	// 1. Read loop exits (connection closed/error)
	// 2. handleTCPData detects write error and calls cleanup
}

// handleTCPDataBinary handles binary TCP data message directly
func (c *Client) handleTCPDataBinary(messageBuffer []byte) error {
	// Parse binary message
	binaryMsg, err := parseBinaryMessage(messageBuffer)
	if err != nil {
		return fmt.Errorf("failed to parse binary message: %v", err)
	}

	// Extract actual TCP data from binary message
	data := binaryMsg.Data

	// Find the local connection for this stream
	var localConn net.Conn
	var exists bool
	var isPlaceholder bool

	// Retry logic to handle race condition
	// The stream should be registered in handleTCPConnect (with nil placeholder)
	// and updated in forwardTCPConnectionOverWS when connection is established
	for retry := 0; retry < 20; retry++ {
		c.tcpStreamsMu.RLock()
		conn, found := c.tcpStreams[binaryMsg.StreamID]
		c.tcpStreamsMu.RUnlock()

		if found {
			exists = true
			if conn == nil {
				// Placeholder - connection not established yet, wait a bit
				isPlaceholder = true
				if retry < 19 {
					time.Sleep(10 * time.Millisecond)
					continue
				}
			} else {
				// Real connection found
				localConn = conn
				isPlaceholder = false
				break
			}
		} else {
			// Stream not registered yet - this shouldn't happen if handleTCPConnect was called first
			// But wait a bit in case of race condition
			if retry < 19 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}
	}
	if !exists || isPlaceholder {
		c.logger.Printf("[tcp:data][%s] Stream not ready (exists: %v, placeholder: %v), ignoring %d bytes",
			binaryMsg.StreamID, exists, isPlaceholder, len(data))
		return nil
	}

	if localConn == nil {
		c.logger.Printf("[tcp:data][%s] Local connection is nil, ignoring %d bytes", binaryMsg.StreamID, len(data))
		return nil
	}

	// Write data to local connection
	if _, err := localConn.Write(data); err != nil {
		c.logger.Printf("[tcp:data] Failed to write to local connection for stream %s: %v", binaryMsg.StreamID, err)
		// Clean up the stream
		c.tcpStreamsMu.Lock()
		delete(c.tcpStreams, binaryMsg.StreamID)
		c.tcpStreamsMu.Unlock()
		localConn.Close()
		return err
	}

	return nil
}

// handleTCPData handles TCP data from legacy protocol (JSON format)
// This is kept for backward compatibility with legacy protocol
func (c *Client) handleTCPData(payload interface{}) error {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var tcpData TCPData
	if err := json.Unmarshal(dataBytes, &tcpData); err != nil {
		return err
	}

	// Decode base64 data - this is a binary message, not raw TCP data
	messageBuffer, err := base64.StdEncoding.DecodeString(tcpData.Data)
	if err != nil {
		return fmt.Errorf("failed to decode base64 message: %v", err)
	}

	// Use the binary handler
	return c.handleTCPDataBinary(messageBuffer)
}
