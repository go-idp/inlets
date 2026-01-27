package limiter

import (
	"sync"
	"time"
)

// TokenBucket implements a token bucket for rate limiting
type TokenBucket struct {
	tokens     float64
	lastRefill time.Time
	capacity   float64
	refillRate float64 // tokens per second
	mu         sync.Mutex
}

// NewTokenBucket creates a new token bucket
func NewTokenBucket(capacity float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     capacity,
		lastRefill: time.Now(),
		capacity:   capacity,
		refillRate: refillRate,
	}
}

// CanConsume checks if there are enough tokens (without consuming)
func (tb *TokenBucket) CanConsume(tokens float64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()
	return tb.tokens >= tokens
}

// Consume tries to consume the specified number of tokens
func (tb *TokenBucket) Consume(tokens float64) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()

	if tb.tokens >= tokens {
		tb.tokens -= tokens
		return true
	}

	return false
}

// GetAvailableTokens returns the current number of available tokens
func (tb *TokenBucket) GetAvailableTokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.refill()
	return tb.tokens
}

// refill adds tokens based on elapsed time
func (tb *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tokensToAdd := elapsed * tb.refillRate

	tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
	tb.lastRefill = now
}

// min returns the minimum of two float64 values
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// BandwidthLimit represents bandwidth limit configuration
type BandwidthLimit struct {
	UploadBytesPerSecond   *int64 `yaml:"upload,omitempty"`
	DownloadBytesPerSecond *int64 `yaml:"download,omitempty"`
}

// ClientBandwidthLimits contains bandwidth limits configuration
type ClientBandwidthLimits struct {
	ByClientId map[string]*BandwidthLimit
	Global     *BandwidthLimit
}

// BandwidthLimiter interface for bandwidth limiting operations
type BandwidthLimiter interface {
	CheckUpload(clientId string, bytes int64) bool
	CheckDownload(clientId string, bytes int64) bool
	SetClientLimit(clientId string, limit *BandwidthLimit)
	RemoveClientLimit(clientId string)
}

// bandwidthLimiter implements BandwidthLimiter
type bandwidthLimiter struct {
	mu                    sync.RWMutex
	limits                *ClientBandwidthLimits
	globalUploadBucket    *TokenBucket
	globalDownloadBucket  *TokenBucket
	clientUploadBuckets   map[string]*TokenBucket
	clientDownloadBuckets map[string]*TokenBucket
}

// NewBandwidthLimiter creates a new bandwidth limiter
func NewBandwidthLimiter(limits *ClientBandwidthLimits) BandwidthLimiter {
	if limits == nil {
		limits = &ClientBandwidthLimits{
			ByClientId: make(map[string]*BandwidthLimit),
		}
	}

	limiter := &bandwidthLimiter{
		limits:                limits,
		clientUploadBuckets:   make(map[string]*TokenBucket),
		clientDownloadBuckets: make(map[string]*TokenBucket),
	}

	// Initialize global buckets if limits are set
	if limits.Global != nil {
		if limits.Global.UploadBytesPerSecond != nil {
			uploadRate := float64(*limits.Global.UploadBytesPerSecond)
			limiter.globalUploadBucket = NewTokenBucket(uploadRate, uploadRate)
		}
		if limits.Global.DownloadBytesPerSecond != nil {
			downloadRate := float64(*limits.Global.DownloadBytesPerSecond)
			limiter.globalDownloadBucket = NewTokenBucket(downloadRate, downloadRate)
		}
	}

	return limiter
}

// getClientUploadBucket gets or creates a client upload bucket
func (bl *bandwidthLimiter) getClientUploadBucket(clientId string) *TokenBucket {
	if clientId == "" {
		return nil
	}

	bl.mu.RLock()
	limit := bl.limits.ByClientId[clientId]
	bl.mu.RUnlock()

	if limit == nil || limit.UploadBytesPerSecond == nil {
		return nil
	}

	bl.mu.Lock()
	defer bl.mu.Unlock()

	if bucket, exists := bl.clientUploadBuckets[clientId]; exists {
		return bucket
	}

	uploadRate := float64(*limit.UploadBytesPerSecond)
	bucket := NewTokenBucket(uploadRate, uploadRate)
	bl.clientUploadBuckets[clientId] = bucket
	return bucket
}

// getClientDownloadBucket gets or creates a client download bucket
func (bl *bandwidthLimiter) getClientDownloadBucket(clientId string) *TokenBucket {
	if clientId == "" {
		return nil
	}

	bl.mu.RLock()
	limit := bl.limits.ByClientId[clientId]
	bl.mu.RUnlock()

	if limit == nil || limit.DownloadBytesPerSecond == nil {
		return nil
	}

	bl.mu.Lock()
	defer bl.mu.Unlock()

	if bucket, exists := bl.clientDownloadBuckets[clientId]; exists {
		return bucket
	}

	downloadRate := float64(*limit.DownloadBytesPerSecond)
	bucket := NewTokenBucket(downloadRate, downloadRate)
	bl.clientDownloadBuckets[clientId] = bucket
	return bucket
}

// CheckUpload checks if upload is allowed for the specified bytes
// Rules:
// 1. If client limit is configured, client cannot exceed its own limit
// 2. If global limit is configured, all clients' total bandwidth cannot exceed global limit
// 3. Both limits must be satisfied
func (bl *bandwidthLimiter) CheckUpload(clientId string, bytes int64) bool {
	bytesFloat := float64(bytes)

	// If no clientId, only check global limit
	if clientId == "" {
		if bl.globalUploadBucket != nil {
			return bl.globalUploadBucket.Consume(bytesFloat)
		}
		return true // No limit
	}

	clientBucket := bl.getClientUploadBucket(clientId)

	// First check if there are enough tokens (without consuming)
	// 1. Check client-level limit
	if clientBucket != nil && !clientBucket.CanConsume(bytesFloat) {
		return false // Client limit not satisfied
	}

	// 2. Check global limit (shared total limit for all clients)
	if bl.globalUploadBucket != nil && !bl.globalUploadBucket.CanConsume(bytesFloat) {
		return false // Global limit not satisfied
	}

	// Both limits are satisfied, now actually consume tokens
	// First consume global limit (because it's shared by all clients)
	if bl.globalUploadBucket != nil {
		if !bl.globalUploadBucket.Consume(bytesFloat) {
			// Should not reach here theoretically, as we already checked
			return false
		}
	}

	// Then consume client limit
	if clientBucket != nil {
		if !clientBucket.Consume(bytesFloat) {
			// Should not reach here theoretically, as we already checked
			return false
		}
	}

	return true
}

// CheckDownload checks if download is allowed for the specified bytes
// Rules:
// 1. If client limit is configured, client cannot exceed its own limit
// 2. If global limit is configured, all clients' total bandwidth cannot exceed global limit
// 3. Both limits must be satisfied
func (bl *bandwidthLimiter) CheckDownload(clientId string, bytes int64) bool {
	bytesFloat := float64(bytes)

	// If no clientId, only check global limit
	if clientId == "" {
		if bl.globalDownloadBucket != nil {
			return bl.globalDownloadBucket.Consume(bytesFloat)
		}
		return true // No limit
	}

	clientBucket := bl.getClientDownloadBucket(clientId)

	// First check if there are enough tokens (without consuming)
	// 1. Check client-level limit
	if clientBucket != nil && !clientBucket.CanConsume(bytesFloat) {
		return false // Client limit not satisfied
	}

	// 2. Check global limit (shared total limit for all clients)
	if bl.globalDownloadBucket != nil && !bl.globalDownloadBucket.CanConsume(bytesFloat) {
		return false // Global limit not satisfied
	}

	// Both limits are satisfied, now actually consume tokens
	// First consume global limit (because it's shared by all clients)
	if bl.globalDownloadBucket != nil {
		if !bl.globalDownloadBucket.Consume(bytesFloat) {
			// Should not reach here theoretically, as we already checked
			return false
		}
	}

	// Then consume client limit
	if clientBucket != nil {
		if !clientBucket.Consume(bytesFloat) {
			// Should not reach here theoretically, as we already checked
			return false
		}
	}

	return true
}

// SetClientLimit sets bandwidth limit for a client
func (bl *bandwidthLimiter) SetClientLimit(clientId string, limit *BandwidthLimit) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	if bl.limits.ByClientId == nil {
		bl.limits.ByClientId = make(map[string]*BandwidthLimit)
	}

	if bl.limits.ByClientId[clientId] == nil {
		bl.limits.ByClientId[clientId] = &BandwidthLimit{}
	}

	// Merge with existing limit
	existing := bl.limits.ByClientId[clientId]
	if limit.UploadBytesPerSecond != nil {
		existing.UploadBytesPerSecond = limit.UploadBytesPerSecond
	}
	if limit.DownloadBytesPerSecond != nil {
		existing.DownloadBytesPerSecond = limit.DownloadBytesPerSecond
	}

	// Recreate client buckets
	if limit.UploadBytesPerSecond != nil {
		uploadRate := float64(*limit.UploadBytesPerSecond)
		bl.clientUploadBuckets[clientId] = NewTokenBucket(uploadRate, uploadRate)
	}
	if limit.DownloadBytesPerSecond != nil {
		downloadRate := float64(*limit.DownloadBytesPerSecond)
		bl.clientDownloadBuckets[clientId] = NewTokenBucket(downloadRate, downloadRate)
	}
}

// RemoveClientLimit removes bandwidth limit for a client
func (bl *bandwidthLimiter) RemoveClientLimit(clientId string) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	delete(bl.limits.ByClientId, clientId)
	delete(bl.clientUploadBuckets, clientId)
	delete(bl.clientDownloadBuckets, clientId)
}

// UpdateLimits updates the bandwidth limits configuration
func (bl *bandwidthLimiter) UpdateLimits(limits *ClientBandwidthLimits) {
	if limits == nil {
		limits = &ClientBandwidthLimits{
			ByClientId: make(map[string]*BandwidthLimit),
		}
	}

	bl.mu.Lock()
	defer bl.mu.Unlock()

	// Update limits reference
	bl.limits = limits

	// Update global buckets
	if limits.Global != nil {
		if limits.Global.UploadBytesPerSecond != nil {
			uploadRate := float64(*limits.Global.UploadBytesPerSecond)
			bl.globalUploadBucket = NewTokenBucket(uploadRate, uploadRate)
		} else {
			bl.globalUploadBucket = nil
		}
		if limits.Global.DownloadBytesPerSecond != nil {
			downloadRate := float64(*limits.Global.DownloadBytesPerSecond)
			bl.globalDownloadBucket = NewTokenBucket(downloadRate, downloadRate)
		} else {
			bl.globalDownloadBucket = nil
		}
	} else {
		bl.globalUploadBucket = nil
		bl.globalDownloadBucket = nil
	}

	// Clear and rebuild client buckets
	bl.clientUploadBuckets = make(map[string]*TokenBucket)
	bl.clientDownloadBuckets = make(map[string]*TokenBucket)

	// Rebuild client buckets from new limits
	if limits.ByClientId != nil {
		for clientId, limit := range limits.ByClientId {
			if limit.UploadBytesPerSecond != nil {
				uploadRate := float64(*limit.UploadBytesPerSecond)
				bl.clientUploadBuckets[clientId] = NewTokenBucket(uploadRate, uploadRate)
			}
			if limit.DownloadBytesPerSecond != nil {
				downloadRate := float64(*limit.DownloadBytesPerSecond)
				bl.clientDownloadBuckets[clientId] = NewTokenBucket(downloadRate, downloadRate)
			}
		}
	}
}
