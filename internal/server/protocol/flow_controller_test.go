package protocol

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestFlowControllerRaceCondition tests that TrySend is atomic and prevents race conditions
func TestFlowControllerRaceCondition(t *testing.T) {
	fc := NewFlowController(1000, nil) // 1KB window
	streamId := "test-stream"

	// Initialize stream
	fc.InitializeStream(streamId, 1000)

	// Number of concurrent goroutines
	numGoroutines := 10
	chunkSize := 100 // Each goroutine will try to send 100 bytes
	totalAttempts := 100 // Each goroutine will try 100 times

	var successCount int64
	var failureCount int64
	var wg sync.WaitGroup

	// Launch multiple goroutines that try to send concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < totalAttempts; j++ {
				if fc.TrySend(streamId, chunkSize) {
					atomic.AddInt64(&successCount, 1)
				} else {
					atomic.AddInt64(&failureCount, 1)
				}
				// Small delay to increase chance of race condition
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()

	// Get final window state
	window := fc.GetWindowState(streamId)
	if window == nil {
		t.Fatal("Window state is nil")
	}

	// Calculate expected send window
	// With 1000 byte window and 100 byte chunks, we can send at most 10 chunks
	// But due to concurrency, we might have sent more if there was a race condition
	expectedMaxWindow := int64(window.MaxWindowSize)
	actualWindow := int64(window.SendWindow)

	// Verify that send window never exceeds max window size
	if actualWindow > expectedMaxWindow {
		t.Errorf("Send window exceeded max window size: %d > %d", actualWindow, expectedMaxWindow)
	}

	// Verify that the number of successful sends matches the send window
	// (each successful send adds chunkSize to SendWindow)
	if actualWindow != int64(successCount)*int64(chunkSize) {
		t.Errorf("Send window (%d) does not match successful sends (%d * %d = %d)",
			actualWindow, successCount, chunkSize, int64(successCount)*int64(chunkSize))
	}

	t.Logf("Concurrent sends: success=%d, failure=%d, final window=%d/%d",
		successCount, failureCount, actualWindow, expectedMaxWindow)
}

// TestFlowControllerConcurrentStreams tests that multiple streams have independent windows
func TestFlowControllerConcurrentStreams(t *testing.T) {
	fc := NewFlowController(1000, nil)
	streamIds := []string{"stream-1", "stream-2", "stream-3"}

	// Initialize all streams
	for _, streamId := range streamIds {
		fc.InitializeStream(streamId, 1000)
	}

	var wg sync.WaitGroup
	chunkSize := 100
	attemptsPerStream := 20

	// Send data concurrently for each stream
	for _, streamId := range streamIds {
		wg.Add(1)
		go func(sid string) {
			defer wg.Done()
			for i := 0; i < attemptsPerStream; i++ {
				fc.TrySend(sid, chunkSize)
				time.Sleep(time.Microsecond)
			}
		}(streamId)
	}

	wg.Wait()

	// Verify each stream's window is independent
	for _, streamId := range streamIds {
		window := fc.GetWindowState(streamId)
		if window == nil {
			t.Errorf("Window state is nil for stream %s", streamId)
			continue
		}

		if window.SendWindow > window.MaxWindowSize {
			t.Errorf("Stream %s: Send window (%d) exceeded max window size (%d)",
				streamId, window.SendWindow, window.MaxWindowSize)
		}

		t.Logf("Stream %s: window=%d/%d", streamId, window.SendWindow, window.MaxWindowSize)
	}
}

// TestFlowControllerTrySendAtomicity tests that TrySend is truly atomic
func TestFlowControllerTrySendAtomicity(t *testing.T) {
	fc := NewFlowController(100, nil) // Small window: 100 bytes
	streamId := "test-stream"
	fc.InitializeStream(streamId, 100)

	// Try to send chunks that together exceed the window
	// With atomic TrySend, only one should succeed
	chunkSize := 60 // Each chunk is 60 bytes, so only 1 can fit in 100 byte window

	var successCount int64
	var wg sync.WaitGroup

	// Launch 5 goroutines trying to send simultaneously
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if fc.TrySend(streamId, chunkSize) {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	// With atomic TrySend, only one should succeed (window is 100, chunk is 60)
	// After first success, window becomes 60/100, so second 60-byte chunk won't fit
	if successCount > 1 {
		t.Errorf("Expected at most 1 successful send, got %d", successCount)
	}

	window := fc.GetWindowState(streamId)
	if window == nil {
		t.Fatal("Window state is nil")
	}

	// Verify window state is consistent
	if successCount == 1 && window.SendWindow != chunkSize {
		t.Errorf("Expected SendWindow to be %d after 1 successful send, got %d",
			chunkSize, window.SendWindow)
	}

	t.Logf("Atomicity test: %d successful sends, window=%d/%d",
		successCount, window.SendWindow, window.MaxWindowSize)
}

// TestFlowControllerReceiveRaceCondition tests that Receive is atomic
func TestFlowControllerReceiveRaceCondition(t *testing.T) {
	fc := NewFlowController(1000, nil)
	streamId := "test-stream"
	fc.InitializeStream(streamId, 1000)

	// Number of concurrent goroutines trying to receive
	numGoroutines := 10
	chunkSize := 100
	totalAttempts := 100

	var successCount int64
	var failureCount int64
	var wg sync.WaitGroup

	// Launch multiple goroutines that try to receive concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < totalAttempts; j++ {
				if fc.Receive(streamId, chunkSize) {
					atomic.AddInt64(&successCount, 1)
				} else {
					atomic.AddInt64(&failureCount, 1)
				}
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait()

	// Get final window state
	window := fc.GetWindowState(streamId)
	if window == nil {
		t.Fatal("Window state is nil")
	}

	// Verify that receive window never goes negative
	if window.ReceiveWindow < 0 {
		t.Errorf("Receive window went negative: %d", window.ReceiveWindow)
	}

	// Verify that receive window is consistent with successful receives
	expectedReceiveWindow := window.MaxWindowSize - int(successCount)*chunkSize
	if window.ReceiveWindow != expectedReceiveWindow {
		t.Errorf("Receive window mismatch: expected %d, got %d",
			expectedReceiveWindow, window.ReceiveWindow)
	}

	t.Logf("Concurrent receives: success=%d, failure=%d, receive window=%d/%d",
		successCount, failureCount, window.ReceiveWindow, window.MaxWindowSize)
}

// TestFlowControllerWindowOverflowPrevention tests that window never exceeds max size
func TestFlowControllerWindowOverflowPrevention(t *testing.T) {
	fc := NewFlowController(1000, nil)
	streamId := "test-stream"
	fc.InitializeStream(streamId, 1000)

	// Try to send more than the window allows
	// With proper atomic operations, window should never exceed max
	chunkSize := 100
	maxChunks := 10 // 10 * 100 = 1000 (exactly the window size)

	var wg sync.WaitGroup
	var successCount int64

	// Try to send more chunks than the window allows
	for i := 0; i < maxChunks+5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if fc.TrySend(streamId, chunkSize) {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	window := fc.GetWindowState(streamId)
	if window == nil {
		t.Fatal("Window state is nil")
	}

	// Verify window never exceeds max
	if window.SendWindow > window.MaxWindowSize {
		t.Errorf("Send window exceeded max: %d > %d", window.SendWindow, window.MaxWindowSize)
	}

	// Verify we can't send more than maxChunks
	if successCount > int64(maxChunks) {
		t.Errorf("Sent more chunks than window allows: %d > %d", successCount, maxChunks)
	}

	t.Logf("Overflow prevention: sent %d chunks, window=%d/%d",
		successCount, window.SendWindow, window.MaxWindowSize)
}

