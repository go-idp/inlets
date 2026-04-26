package client

import (
	"encoding/json"
	"os"
	"time"
)

func (c *Client) startAuthTimeout() {
	if c.authTimeout != nil {
		c.authTimeout.Stop()
	}

	duration := c.authTimeoutDuration()
	c.authTimeout = time.AfterFunc(duration, func() {
		c.logger.Printf("Authentication timeout, exiting")
		os.Exit(1)
	})
}

func (c *Client) authTimeoutDuration() time.Duration {
	if c.opts.HealthcheckInt > 0 {
		return time.Duration(c.opts.HealthcheckInt) * time.Millisecond
	}

	return defaultAuthTimeout
}

func (c *Client) startHeartbeat() {
	c.stopHeartbeat()

	// Mark heartbeat as active
	c.heartbeatMu.Lock()
	c.heartbeatActive = true
	c.heartbeatMu.Unlock()

	if c.pingInterval <= 0 {
		c.pingInterval = defaultPingInterval
	}
	if c.pingTimeout <= 0 {
		c.pingTimeout = defaultPingTimeout
	}

	c.schedulePing(0)
}

func (c *Client) stopHeartbeat() {
	// Mark heartbeat as inactive first to prevent new timers from starting
	c.heartbeatMu.Lock()
	c.heartbeatActive = false
	c.heartbeatMu.Unlock()

	if c.pingTimer != nil {
		c.pingTimer.Stop()
		c.pingTimer = nil
	}
	if c.pingTimeoutTimer != nil {
		c.pingTimeoutTimer.Stop()
		c.pingTimeoutTimer = nil
	}
}

func (c *Client) schedulePing(delay time.Duration) {
	// Check if heartbeat is still active
	c.heartbeatMu.Lock()
	active := c.heartbeatActive
	c.heartbeatMu.Unlock()

	if !active || c.monitorConn == nil {
		return
	}

	if c.pingTimer != nil {
		c.pingTimer.Stop()
	}

	if delay < 0 {
		delay = 0
	}

	c.pingTimer = time.AfterFunc(delay, func() {
		// Check again before sending ping
		c.heartbeatMu.Lock()
		active := c.heartbeatActive
		c.heartbeatMu.Unlock()

		if active {
			c.sendPing()
		}
	})
}

func (c *Client) sendPing() {
	// Check if heartbeat is still active
	c.heartbeatMu.Lock()
	active := c.heartbeatActive
	c.heartbeatMu.Unlock()

	if !active || c.monitorConn == nil {
		return
	}

	if err := c.sendMonitorMessage("ping", nil); err != nil {
		c.logger.Printf("Failed to send ping message: %v", err)
		return
	}

	// fmt.Println("send ping")

	if c.pingTimeoutTimer != nil {
		c.pingTimeoutTimer.Stop()
	}

	c.pingTimeoutTimer = time.AfterFunc(c.pingTimeout, func() {
		// Check if heartbeat is still active before handling timeout
		c.heartbeatMu.Lock()
		active := c.heartbeatActive
		c.heartbeatMu.Unlock()

		if !active {
			return
		}

		c.logger.Printf("Ping timeout, closing connection")
		c.closingMu.Lock()
		if c.closing {
			c.closingMu.Unlock()
			return
		}
		c.closing = true
		c.closingMu.Unlock()

		// Stop heartbeat
		c.stopHeartbeat()

		// Close connection
		if c.monitorConn != nil {
			_ = c.monitorConn.Close()
		}

		// Trigger disconnect handling (which will trigger reconnect)
		c.handleDisconnect()
	})
}

func (c *Client) handlePong() {
	// fmt.Println("received pong")

	// Check if heartbeat is still active
	c.heartbeatMu.Lock()
	active := c.heartbeatActive
	c.heartbeatMu.Unlock()

	if !active {
		return
	}

	if c.pingTimeoutTimer != nil {
		c.pingTimeoutTimer.Stop()
		c.pingTimeoutTimer = nil
	}

	c.schedulePing(c.pingInterval)
}

func (c *Client) handleSocketConfig(payload interface{}) {
	dataBytes, err := json.Marshal(payload)
	if err != nil {
		c.logger.Printf("Failed to marshal socket config: %v", err)
		return
	}

	var cfg struct {
		PingInterval int `json:"pingInterval"`
		PingTimeout  int `json:"pingTimeout"`
	}

	if err := json.Unmarshal(dataBytes, &cfg); err != nil {
		c.logger.Printf("Failed to unmarshal socket config: %v", err)
		return
	}

	if cfg.PingInterval > 0 {
		c.pingInterval = time.Duration(cfg.PingInterval) * time.Millisecond
	}
	if cfg.PingTimeout > 0 {
		c.pingTimeout = time.Duration(cfg.PingTimeout) * time.Millisecond
	}

	c.logger.Printf("Heartbeat config updated: interval=%v timeout=%v", c.pingInterval, c.pingTimeout)
	c.startHeartbeat()
	c.restartAllDataChannelHeartbeats()
}
