package client

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
)

// Data channel (/_/data) uses the same JSON ping/pong and timing as the monitor
// channel so long-lived streams (e.g. tunneled WebSocket) see periodic traffic
// and idle middleboxes are less likely to drop the connection.

func (c *Client) startDataChannelHeartbeat(streamID string) {
	c.dataHeartbeatMu.Lock()
	if t := c.dataPingTimer[streamID]; t != nil {
		t.Stop()
	}
	if t := c.dataPingTimeoutTimer[streamID]; t != nil {
		t.Stop()
	}
	c.dataHeartbeatMu.Unlock()
	c.scheduleDataChannelPing(streamID, 0)
}

func (c *Client) stopDataChannelHeartbeat(streamID string) {
	c.dataHeartbeatMu.Lock()
	defer c.dataHeartbeatMu.Unlock()
	if t := c.dataPingTimer[streamID]; t != nil {
		t.Stop()
	}
	delete(c.dataPingTimer, streamID)
	if t := c.dataPingTimeoutTimer[streamID]; t != nil {
		t.Stop()
	}
	delete(c.dataPingTimeoutTimer, streamID)
}

func (c *Client) scheduleDataChannelPing(streamID string, delay time.Duration) {
	if delay < 0 {
		delay = 0
	}
	c.dataHeartbeatMu.Lock()
	if c.dataPingTimer[streamID] != nil {
		c.dataPingTimer[streamID].Stop()
	}
	c.dataPingTimer[streamID] = time.AfterFunc(delay, func() {
		c.sendDataChannelPing(streamID)
	})
	c.dataHeartbeatMu.Unlock()
}

func (c *Client) sendDataChannelPing(streamID string) {
	conn, writeMu := c.getDataChannel(streamID)
	if conn == nil {
		c.stopDataChannelHeartbeat(streamID)
		return
	}
	pi := c.pingInterval
	if pi <= 0 {
		pi = defaultPingInterval
	}
	pt := c.pingTimeout
	if pt <= 0 {
		pt = defaultPingTimeout
	}
	msg := []interface{}{"ping", nil}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	writeMu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, data)
	writeMu.Unlock()
	if err != nil {
		c.logger.Printf("Data channel failed to send ping for stream %s: %v", streamID, err)
		c.removeDataChannel(streamID)
		return
	}
	c.dataHeartbeatMu.Lock()
	if c.dataPingTimeoutTimer[streamID] != nil {
		c.dataPingTimeoutTimer[streamID].Stop()
	}
	c.dataPingTimeoutTimer[streamID] = time.AfterFunc(pt, func() {
		c.logger.Printf("Data channel ping timeout for stream %s, closing", streamID)
		c.removeDataChannel(streamID)
	})
	c.dataHeartbeatMu.Unlock()
}

func (c *Client) handleDataChannelPong(streamID string) {
	c.dataHeartbeatMu.Lock()
	if t := c.dataPingTimeoutTimer[streamID]; t != nil {
		t.Stop()
		delete(c.dataPingTimeoutTimer, streamID)
	}
	c.dataHeartbeatMu.Unlock()
	interval := c.pingInterval
	if interval <= 0 {
		interval = defaultPingInterval
	}
	c.scheduleDataChannelPing(streamID, interval)
}

func (c *Client) sendDataChannelPong(streamID string) {
	msg := []interface{}{"pong", nil}
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	conn, writeMu := c.getDataChannel(streamID)
	if conn == nil {
		return
	}
	writeMu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, data)
	writeMu.Unlock()
	if err != nil {
		c.logger.Printf("Data channel failed to send pong for stream %s: %v", streamID, err)
	}
}

func (c *Client) restartAllDataChannelHeartbeats() {
	c.dataConnMu.RLock()
	ids := make([]string, 0, len(c.dataConns))
	for id := range c.dataConns {
		ids = append(ids, id)
	}
	c.dataConnMu.RUnlock()
	for _, id := range ids {
		c.startDataChannelHeartbeat(id)
	}
}
