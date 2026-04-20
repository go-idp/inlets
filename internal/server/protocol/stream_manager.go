package protocol

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-zoox/logger"
)

// StreamState represents the state of a stream
type StreamState string

const (
	StreamStateInitializing StreamState = "initializing"
	StreamStateActive       StreamState = "active"
	StreamStatePaused       StreamState = "paused"
	StreamStateCompleted    StreamState = "completed"
	StreamStateError        StreamState = "error"
)

// StreamChunk represents a chunk of stream data
type StreamChunk struct {
	Sequence uint32
	Data     []byte
	IsLast   bool
}

// Stream represents a data stream
type Stream struct {
	ID               string
	State            StreamState
	Chunks           map[uint32]*StreamChunk
	ExpectedSequence uint32
	TotalChunks      int
	OnComplete       func(data []byte)
	OnError          func(error error)
	CompletedData    []byte
	createdAt        time.Time
	lastActivity     time.Time
	mu               sync.Mutex
}

// NewStream creates a new stream
func NewStream(id string) *Stream {
	now := time.Now()
	return &Stream{
		ID:               id,
		State:            StreamStateInitializing,
		Chunks:           make(map[uint32]*StreamChunk),
		ExpectedSequence: 0,
		TotalChunks:      -1, // -1 means unknown
		createdAt:        now,
		lastActivity:     now,
	}
}

// AddChunk adds a chunk to the stream
func (s *Stream) AddChunk(sequence uint32, data []byte, isLast bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StreamStateCompleted || s.State == StreamStateError {
		return // Stream is completed or errored, ignore new data
	}

	s.lastActivity = time.Now()

	s.Chunks[sequence] = &StreamChunk{
		Sequence: sequence,
		Data:     data,
		IsLast:   isLast,
	}

	if isLast {
		s.TotalChunks = int(sequence) + 1
	}

	// Try to reassemble
	s.tryReassemble()
}

// tryReassemble tries to reassemble data in sequence order
func (s *Stream) tryReassemble() {
	// Check for consecutive chunks in sequence order
	for {
		chunk, exists := s.Chunks[s.ExpectedSequence]
		if !exists {
			break
		}

		delete(s.Chunks, s.ExpectedSequence)

		// Merge data
		if s.CompletedData == nil {
			s.CompletedData = make([]byte, len(chunk.Data))
			copy(s.CompletedData, chunk.Data)
		} else {
			merged := make([]byte, len(s.CompletedData)+len(chunk.Data))
			copy(merged, s.CompletedData)
			copy(merged[len(s.CompletedData):], chunk.Data)
			s.CompletedData = merged
		}

		s.ExpectedSequence++

		// Check if completed
		if chunk.IsLast {
			s.State = StreamStateCompleted
			if s.OnComplete != nil && s.CompletedData != nil {
				s.OnComplete(s.CompletedData)
			}
			return
		}

		// Check if all chunks received (if total is known)
		if s.TotalChunks > 0 && s.ExpectedSequence >= uint32(s.TotalChunks) {
			s.State = StreamStateCompleted
			if s.OnComplete != nil && s.CompletedData != nil {
				s.OnComplete(s.CompletedData)
			}
			return
		}
	}
}

// Pause pauses the stream
func (s *Stream) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StreamStateActive {
		s.State = StreamStatePaused
	}
}

// Resume resumes the stream
func (s *Stream) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StreamStatePaused {
		s.State = StreamStateActive
		// Try to reassemble after resuming
		s.tryReassemble()
	}
}

// Destroy destroys the stream
func (s *Stream) Destroy() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Chunks = nil
	s.CompletedData = nil
	s.OnComplete = nil
	s.OnError = nil
}

// StreamManager manages multiple data streams
type StreamManager struct {
	streams          map[string]*Stream
	defaultChunkSize int
	maxStreamAge     time.Duration
	// stallTimeout: if no chunk arrives for this long while reassembly is incomplete, evict the stream.
	stallTimeout time.Duration
	mu           sync.RWMutex
	stopCleanup  chan struct{}
}

// NewStreamManager creates a new stream manager
func NewStreamManager(defaultChunkSize int) *StreamManager {
	sm := &StreamManager{
		streams:          make(map[string]*Stream),
		defaultChunkSize: defaultChunkSize,
		maxStreamAge:     5 * time.Minute,
		stallTimeout:     2 * time.Minute,
		stopCleanup:      make(chan struct{}),
	}

	// Start cleanup timer
	go sm.cleanupLoop()

	return sm
}

// SplitIntoChunks splits data into chunks
func (sm *StreamManager) SplitIntoChunks(data []byte, chunkSize int) [][]byte {
	size := chunkSize
	if size <= 0 {
		size = sm.defaultChunkSize
	}

	chunks := make([][]byte, 0)
	for i := 0; i < len(data); i += size {
		end := i + size
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[i:end])
	}

	return chunks
}

// CreateStream creates a new stream
func (sm *StreamManager) CreateStream(streamId string, onComplete func(data []byte), onError func(error error)) *Stream {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	stream := NewStream(streamId)
	stream.State = StreamStateActive
	stream.OnComplete = onComplete
	stream.OnError = onError

	sm.streams[streamId] = stream
	return stream
}

// EnsureStream returns the existing stream or creates one with the given callbacks.
// If the stream already exists, callbacks are ignored (first registration wins).
func (sm *StreamManager) EnsureStream(streamId string, onComplete func(data []byte), onError func(error error)) *Stream {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if s := sm.streams[streamId]; s != nil {
		return s
	}

	stream := NewStream(streamId)
	stream.State = StreamStateActive
	stream.OnComplete = onComplete
	stream.OnError = onError
	sm.streams[streamId] = stream
	return stream
}

// GetStream gets a stream by ID
func (sm *StreamManager) GetStream(streamId string) *Stream {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.streams[streamId]
}

// AddChunk adds a chunk to an existing stream. The stream must be created first
// (e.g. via EnsureStream from handleBinaryMessage); auto-create was removed because
// it called CreateStream while holding the manager lock (deadlock) and used nil onComplete.
func (sm *StreamManager) AddChunk(streamId string, sequence int, data []byte, isLast bool) {
	sm.mu.RLock()
	stream := sm.streams[streamId]
	sm.mu.RUnlock()

	if stream == nil {
		logger.Infof("[protocol:stream] AddChunk: no stream for %q seq=%d last=%v (dropped)", streamId, sequence, isLast)
		return
	}

	stream.AddChunk(uint32(sequence), data, isLast)
}

// RemoveStream removes a stream
func (sm *StreamManager) RemoveStream(streamId string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	stream := sm.streams[streamId]
	if stream != nil {
		stream.Destroy()
		delete(sm.streams, streamId)
	}
}

// cleanupLoop periodically cleans up expired streams
func (sm *StreamManager) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.cleanup()
		case <-sm.stopCleanup:
			return
		}
	}
}

// cleanup removes completed streams and evicts stalled partial reassemblies.
func (sm *StreamManager) cleanup() {
	now := time.Now()
	stall := sm.stallTimeout
	if stall <= 0 {
		stall = 2 * time.Minute
	}

	sm.mu.Lock()
	ids := make([]string, 0, len(sm.streams))
	for id := range sm.streams {
		ids = append(ids, id)
	}
	sm.mu.Unlock()

	for _, streamId := range ids {
		var errCb func(error)
		shouldRemove := false

		sm.mu.RLock()
		stream := sm.streams[streamId]
		sm.mu.RUnlock()
		if stream == nil {
			continue
		}

		stream.mu.Lock()
		state := stream.State
		completedOrErr := state == StreamStateCompleted || state == StreamStateError
		stale := !completedOrErr &&
			(now.Sub(stream.lastActivity) > stall || now.Sub(stream.createdAt) > sm.maxStreamAge)
		if stale {
			errCb = stream.OnError
			stream.State = StreamStateError
		}
		shouldRemove = completedOrErr || stale
		stream.mu.Unlock()

		if errCb != nil {
			errCb(fmt.Errorf("stream %s: reassembly stalled or exceeded max age", streamId))
		}
		if shouldRemove {
			sm.RemoveStream(streamId)
		}
	}
}

// Destroy destroys the stream manager
func (sm *StreamManager) Destroy() {
	close(sm.stopCleanup)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, stream := range sm.streams {
		stream.Destroy()
	}
	sm.streams = nil
}

