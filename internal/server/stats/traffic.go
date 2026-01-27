package stats

import (
	"fmt"
	"sync"
	"time"
)

// TrafficStats represents traffic statistics
type TrafficStats struct {
	UploadBytes    int64
	DownloadBytes  int64
	Connections    int64
	Requests       int64
	StartTime      int64
	EndTime        *int64
	LastUpdateTime int64
}

// TrafficStatsData contains traffic statistics data
type TrafficStatsData struct {
	Global     *TrafficStats
	ByClientId map[string]*TrafficStats
}

// TrafficStatsContainer interface for traffic statistics operations
type TrafficStatsContainer interface {
	AddUploadBytes(clientId string, bytes int64)
	AddDownloadBytes(clientId string, bytes int64)
	AddConnection(clientId string)
	AddRequest(clientId string)
	GetStats(clientId string) interface{}
	Reset(clientId string)
	SetStartTime(clientId string)
	SetEndTime(clientId string)
	FormatStats(clientId string) string
}

// trafficStatsContainer implements TrafficStatsContainer
type trafficStatsContainer struct {
	mu    sync.RWMutex
	stats *TrafficStatsData
}

// NewTrafficStatsContainer creates a new traffic statistics container
func NewTrafficStatsContainer() TrafficStatsContainer {
	return &trafficStatsContainer{
		stats: &TrafficStatsData{
			Global: &TrafficStats{
				UploadBytes:    0,
				DownloadBytes:  0,
				Connections:    0,
				Requests:       0,
				StartTime:      time.Now().UnixMilli(),
				EndTime:        nil,
				LastUpdateTime: time.Now().UnixMilli(),
			},
			ByClientId: make(map[string]*TrafficStats),
		},
	}
}

// getOrCreateClientStats gets or creates client statistics (assumes lock is already held)
func (c *trafficStatsContainer) getOrCreateClientStats(clientId string) *TrafficStats {
	if clientId == "" {
		return c.stats.Global
	}

	if c.stats.ByClientId[clientId] == nil {
		c.stats.ByClientId[clientId] = &TrafficStats{
			UploadBytes:    0,
			DownloadBytes:  0,
			Connections:    0,
			Requests:       0,
			StartTime:      time.Now().UnixMilli(),
			EndTime:        nil,
			LastUpdateTime: time.Now().UnixMilli(),
		}
	}

	return c.stats.ByClientId[clientId]
}

// AddUploadBytes adds upload bytes to statistics
func (c *trafficStatsContainer) AddUploadBytes(clientId string, bytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	clientStats := c.getOrCreateClientStats(clientId)
	clientStats.UploadBytes += bytes
	clientStats.LastUpdateTime = time.Now().UnixMilli()

	// Also update global statistics if clientId is provided
	if clientId != "" {
		c.stats.Global.UploadBytes += bytes
		c.stats.Global.LastUpdateTime = time.Now().UnixMilli()
	}
}

// AddDownloadBytes adds download bytes to statistics
func (c *trafficStatsContainer) AddDownloadBytes(clientId string, bytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	clientStats := c.getOrCreateClientStats(clientId)
	clientStats.DownloadBytes += bytes
	clientStats.LastUpdateTime = time.Now().UnixMilli()

	// Also update global statistics if clientId is provided
	if clientId != "" {
		c.stats.Global.DownloadBytes += bytes
		c.stats.Global.LastUpdateTime = time.Now().UnixMilli()
	}
}

// AddConnection adds a connection to statistics
func (c *trafficStatsContainer) AddConnection(clientId string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	clientStats := c.getOrCreateClientStats(clientId)
	clientStats.Connections += 1
	clientStats.LastUpdateTime = time.Now().UnixMilli()

	// Also update global statistics if clientId is provided
	if clientId != "" {
		c.stats.Global.Connections += 1
		c.stats.Global.LastUpdateTime = time.Now().UnixMilli()
	}
}

// AddRequest adds a request to statistics
func (c *trafficStatsContainer) AddRequest(clientId string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	clientStats := c.getOrCreateClientStats(clientId)
	clientStats.Requests += 1
	clientStats.LastUpdateTime = time.Now().UnixMilli()

	// Also update global statistics if clientId is provided
	if clientId != "" {
		c.stats.Global.Requests += 1
		c.stats.Global.LastUpdateTime = time.Now().UnixMilli()
	}
}

// GetStats gets statistics for a client or all statistics
func (c *trafficStatsContainer) GetStats(clientId string) interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if clientId == "" {
		return c.stats
	}

	if stats, exists := c.stats.ByClientId[clientId]; exists {
		return stats
	}

	return nil
}

// Reset resets statistics for a client or all statistics
func (c *trafficStatsContainer) Reset(clientId string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if clientId == "" {
		// Reset global statistics
		c.stats.Global = &TrafficStats{
			UploadBytes:    0,
			DownloadBytes:  0,
			Connections:    0,
			Requests:       0,
			StartTime:      time.Now().UnixMilli(),
			EndTime:        nil,
			LastUpdateTime: time.Now().UnixMilli(),
		}
		// Reset all client statistics
		c.stats.ByClientId = make(map[string]*TrafficStats)
	} else {
		// Reset specific client statistics
		delete(c.stats.ByClientId, clientId)
	}
}

// SetStartTime sets the start time for a client
func (c *trafficStatsContainer) SetStartTime(clientId string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	clientStats := c.getOrCreateClientStats(clientId)
	// Reset start time and end time for new connection session
	clientStats.StartTime = time.Now().UnixMilli()
	clientStats.EndTime = nil
	clientStats.LastUpdateTime = time.Now().UnixMilli()
}

// SetEndTime sets the end time for a client
func (c *trafficStatsContainer) SetEndTime(clientId string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	clientStats := c.getOrCreateClientStats(clientId)
	// Set end time if not already set
	if clientStats.EndTime == nil {
		now := time.Now().UnixMilli()
		clientStats.EndTime = &now
		clientStats.LastUpdateTime = now
	}
}

// FormatStats formats statistics as a readable string
func (c *trafficStatsContainer) FormatStats(clientId string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var stats *TrafficStats
	if clientId == "" {
		stats = c.stats.Global
	} else {
		var exists bool
		stats, exists = c.stats.ByClientId[clientId]
		if !exists {
			return "No stats available"
		}
	}

	if stats == nil {
		return "No stats available"
	}

	formatBytes := func(bytes int64) string {
		if bytes < 1024 {
			return fmt.Sprintf("%d B", bytes)
		}
		if bytes < 1024*1024 {
			return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
		}
		if bytes < 1024*1024*1024 {
			return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
		}
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	}

	totalBytes := stats.UploadBytes + stats.DownloadBytes

	// Calculate duration: from client connection to disconnection
	// If endTime is not set, use current time (still connected)
	endTime := time.Now().UnixMilli()
	if stats.EndTime != nil {
		endTime = *stats.EndTime
	}
	duration := (endTime - stats.StartTime) / 1000 // seconds
	hours := duration / 3600
	minutes := (duration % 3600) / 60
	seconds := duration % 60

	var durationStr string
	if hours > 0 {
		durationStr = fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		durationStr = fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		durationStr = fmt.Sprintf("%ds", seconds)
	}

	return fmt.Sprintf("上传: %s, 下载: %s, 总计: %s, 请求数: %d, 连接数: %d, 运行时长: %s",
		formatBytes(stats.UploadBytes),
		formatBytes(stats.DownloadBytes),
		formatBytes(totalBytes),
		stats.Requests,
		stats.Connections,
		durationStr,
	)
}
