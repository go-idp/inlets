package protocol

import (
	"sync"
)

// WindowState represents the state of a flow control window
type WindowState struct {
	SendWindow    int // Send window size (bytes sent but not acknowledged)
	ReceiveWindow int // Receive window size (bytes that can be received)
	MaxWindowSize int // Maximum window size
}

// FlowController implements backpressure control mechanism
type FlowController struct {
	windows            map[string]*WindowState
	defaultMaxWindowSize int
	onBackpressure     func(streamId string, pause bool)
	mu                 sync.RWMutex
}

// NewFlowController creates a new flow controller
func NewFlowController(defaultMaxWindowSize int, onBackpressure func(streamId string, pause bool)) *FlowController {
	return &FlowController{
		windows:             make(map[string]*WindowState),
		defaultMaxWindowSize: defaultMaxWindowSize,
		onBackpressure:      onBackpressure,
	}
}

// InitializeStream initializes the window for a stream
func (fc *FlowController) InitializeStream(streamId string, maxWindowSize ...int) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	windowSize := fc.defaultMaxWindowSize
	if len(maxWindowSize) > 0 && maxWindowSize[0] > 0 {
		windowSize = maxWindowSize[0]
	}

	fc.windows[streamId] = &WindowState{
		SendWindow:    0,
		ReceiveWindow: windowSize,
		MaxWindowSize: windowSize,
	}
}

// CanSend checks if data can be sent (read-only check, no state modification)
func (fc *FlowController) CanSend(streamId string, size int) bool {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	window := fc.windows[streamId]
	if window == nil {
		// Stream not initialized, assume it can send (will be initialized on first Send)
		return true
	}

	return window.SendWindow+size <= window.MaxWindowSize
}

// TrySend attempts to send data atomically (checks and updates in one operation)
// Returns true if the send was successful, false if the window is full
func (fc *FlowController) TrySend(streamId string, size int) bool {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	window := fc.windows[streamId]
	if window == nil {
		// Auto-initialize if stream not initialized
		windowSize := fc.defaultMaxWindowSize
		fc.windows[streamId] = &WindowState{
			SendWindow:    0,
			ReceiveWindow: windowSize,
			MaxWindowSize: windowSize,
		}
		window = fc.windows[streamId]
	}

	// Check if we can send
	if window.SendWindow+size > window.MaxWindowSize {
		// Trigger backpressure signal
		if fc.onBackpressure != nil {
			fc.onBackpressure(streamId, true)
		}
		return false
	}

	// Update send window atomically
	window.SendWindow += size
	return true
}

// Send sends data (updates send window)
// Deprecated: Use TrySend for atomic check-and-update operation
func (fc *FlowController) Send(streamId string, size int) bool {
	return fc.TrySend(streamId, size)
}

// OnAck receives acknowledgment (updates send window)
func (fc *FlowController) OnAck(streamId string, ackedSize int) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	window := fc.windows[streamId]
	if window != nil {
		if window.SendWindow > ackedSize {
			window.SendWindow -= ackedSize
		} else {
			window.SendWindow = 0
		}

		// If window has space, cancel backpressure
		if window.SendWindow < window.MaxWindowSize/2 && fc.onBackpressure != nil {
			fc.onBackpressure(streamId, false)
		}
	}
}

// Receive receives data (updates receive window)
func (fc *FlowController) Receive(streamId string, size int) bool {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	window := fc.windows[streamId]
	if window == nil {
		// Auto-initialize if stream not initialized
		windowSize := fc.defaultMaxWindowSize
		fc.windows[streamId] = &WindowState{
			SendWindow:    0,
			ReceiveWindow: windowSize,
			MaxWindowSize: windowSize,
		}
		window = fc.windows[streamId]
	}

	if window.ReceiveWindow < size {
		// Receive window insufficient, trigger backpressure
		if fc.onBackpressure != nil {
			fc.onBackpressure(streamId, true)
		}
		return false
	}

	window.ReceiveWindow -= size
	return true
}

// ReleaseReceiveWindow releases receive window (data has been processed)
func (fc *FlowController) ReleaseReceiveWindow(streamId string, size int) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	window := fc.windows[streamId]
	if window != nil {
		newWindow := window.ReceiveWindow + size
		if newWindow > window.MaxWindowSize {
			window.ReceiveWindow = window.MaxWindowSize
		} else {
			window.ReceiveWindow = newWindow
		}

		// If window has space, cancel backpressure
		if window.ReceiveWindow > window.MaxWindowSize/2 && fc.onBackpressure != nil {
			fc.onBackpressure(streamId, false)
		}
	}
}

// GetWindowState gets the window state for a stream
func (fc *FlowController) GetWindowState(streamId string) *WindowState {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	return fc.windows[streamId]
}

// RemoveStream removes the window for a stream
func (fc *FlowController) RemoveStream(streamId string) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	delete(fc.windows, streamId)
}

// Clear clears all windows
func (fc *FlowController) Clear() {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.windows = make(map[string]*WindowState)
}

