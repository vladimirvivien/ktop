package health

import (
	"fmt"
	"sync"
	"time"
)

// APIState represents the current health state of the Kubernetes API connection
type APIState int

const (
	APIHealthy      APIState = iota // Connected and working
	APIUnhealthy                    // Experiencing errors, retrying
	APIDisconnected                 // Given up, waiting for manual reconnect
)

func (s APIState) String() string {
	switch s {
	case APIHealthy:
		return "Healthy"
	case APIUnhealthy:
		return "Unhealthy"
	case APIDisconnected:
		return "Disconnected"
	default:
		return "Unknown"
	}
}

// APIHealthTracker monitors Kubernetes API server health with retry logic
type APIHealthTracker struct {
	state             APIState
	retryCount        int
	maxRetries        int
	baseBackoff       time.Duration
	lastError         error
	lastSuccessTime   time.Time
	lastErrorTime     time.Time // Track when we last saw an error
	retryTimer        *time.Timer
	consecutiveOK     int           // Consecutive successful checks needed to recover
	requiredConsecOK  int           // Number of consecutive OKs required before declaring healthy
	minUnhealthyTime  time.Duration // Minimum time to stay unhealthy before recovery
	mu                sync.RWMutex

	// Callbacks
	onStateChange    func(APIState, string) // For toast notifications
	onHealthy        func()                 // Resume normal operation
	onDisconnected   func()                 // Zero out UI values
	onTryReconnect   func()                 // Trigger immediate health check
}

// NewAPIHealthTracker creates a new health tracker with the given state change callback
func NewAPIHealthTracker(onStateChange func(APIState, string)) *APIHealthTracker {
	return &APIHealthTracker{
		state:            APIHealthy,
		maxRetries:       5,
		baseBackoff:      2 * time.Second,
		lastSuccessTime:  time.Now(),
		onStateChange:    onStateChange,
		requiredConsecOK: 3,               // Require 3 consecutive successful checks before declaring healthy
		minUnhealthyTime: 10 * time.Second, // Must stay unhealthy for at least 10 seconds before recovery
	}
}

// SetOnHealthy sets the callback for when connection becomes healthy
func (h *APIHealthTracker) SetOnHealthy(callback func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onHealthy = callback
}

// SetOnDisconnected sets the callback for when connection is lost after all retries
func (h *APIHealthTracker) SetOnDisconnected(callback func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onDisconnected = callback
}

// SetOnTryReconnect sets the callback for when user requests reconnection
// This allows the controller to trigger an immediate health check
func (h *APIHealthTracker) SetOnTryReconnect(callback func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onTryReconnect = callback
}

// ReportSuccess should be called when an API operation succeeds
func (h *APIHealthTracker) ReportSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.lastSuccessTime = time.Now()
	h.lastError = nil

	// Cancel any pending retry timer
	if h.retryTimer != nil {
		h.retryTimer.Stop()
		h.retryTimer = nil
	}

	// If already healthy, just reset consecutive counter and return
	if h.state == APIHealthy {
		h.consecutiveOK = h.requiredConsecOK // Keep it at max
		return
	}

	// Check if we've been unhealthy long enough to consider recovery
	// This prevents false recovery from stale/cached responses right after an error
	if !h.lastErrorTime.IsZero() && time.Since(h.lastErrorTime) < h.minUnhealthyTime {
		// Too soon after last error - don't count this success
		// This prevents flapping from cached responses
		return
	}

	// Increment consecutive success counter
	h.consecutiveOK++

	// Require multiple consecutive successes before declaring healthy
	// This prevents flapping when connection is unstable
	if h.consecutiveOK >= h.requiredConsecOK {
		h.state = APIHealthy
		h.retryCount = 0
		h.consecutiveOK = h.requiredConsecOK

		if h.onStateChange != nil {
			h.onStateChange(APIHealthy, "API connection restored")
		}
		if h.onHealthy != nil {
			h.onHealthy()
		}
	}
}

// ReportError should be called when an API operation fails
func (h *APIHealthTracker) ReportError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Already disconnected, don't process more errors or update timestamps
	// This allows recovery when server comes back online
	if h.state == APIDisconnected {
		return
	}

	h.lastError = err
	h.lastErrorTime = time.Now()
	h.consecutiveOK = 0 // Reset consecutive success counter on any error

	// First error - transition to unhealthy
	if h.state == APIHealthy {
		h.state = APIUnhealthy
		h.retryCount = 1
		if h.onStateChange != nil {
			h.onStateChange(APIUnhealthy, "API connection lost. Retrying...")
		}
		h.scheduleRetry()
	} else if h.state == APIUnhealthy {
		h.retryCount++
		if h.retryCount > h.maxRetries {
			h.state = APIDisconnected
			if h.onStateChange != nil {
				h.onStateChange(APIDisconnected, "API disconnected. Press R to reconnect")
			}
			if h.onDisconnected != nil {
				h.onDisconnected()
			}
		} else {
			if h.onStateChange != nil {
				h.onStateChange(APIUnhealthy, fmt.Sprintf("Reconnecting... (%d/%d)", h.retryCount, h.maxRetries))
			}
			h.scheduleRetry()
		}
	}
}

// scheduleRetry sets up the next retry with exponential backoff
func (h *APIHealthTracker) scheduleRetry() {
	// Cancel existing timer
	if h.retryTimer != nil {
		h.retryTimer.Stop()
	}

	// Calculate backoff: 2s, 4s, 8s, 16s, 32s
	backoff := h.baseBackoff * time.Duration(1<<(h.retryCount-1))

	h.retryTimer = time.AfterFunc(backoff, func() {
		h.mu.Lock()
		if h.state == APIUnhealthy && h.onStateChange != nil {
			remaining := h.maxRetries - h.retryCount
			if remaining > 0 {
				h.onStateChange(APIUnhealthy, fmt.Sprintf("Retry in %v... (%d left)", backoff, remaining))
			}
		}
		h.mu.Unlock()
	})
}

// TryReconnect attempts to reconnect when in disconnected state (called when user presses 'R')
func (h *APIHealthTracker) TryReconnect() {
	h.mu.Lock()

	if h.state != APIDisconnected {
		h.mu.Unlock()
		return
	}

	h.state = APIUnhealthy
	h.retryCount = 0
	h.lastError = nil
	h.lastErrorTime = time.Time{} // Reset error time to allow immediate recovery

	if h.onStateChange != nil {
		h.onStateChange(APIUnhealthy, "Reconnecting...")
	}

	// Capture callback before releasing lock
	reconnectCallback := h.onTryReconnect
	h.mu.Unlock()

	// Trigger immediate health check (outside lock to avoid deadlock)
	if reconnectCallback != nil {
		go reconnectCallback()
	}
}

// GetState returns the current API health state
func (h *APIHealthTracker) GetState() APIState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.state
}

// GetLastError returns the last error encountered
func (h *APIHealthTracker) GetLastError() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastError
}

// GetRetryCount returns the current retry count
func (h *APIHealthTracker) GetRetryCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.retryCount
}

// GetStatusMessage returns a human-readable status message
func (h *APIHealthTracker) GetStatusMessage() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	switch h.state {
	case APIHealthy:
		return "Connected"
	case APIUnhealthy:
		return fmt.Sprintf("Retrying (%d/%d)", h.retryCount, h.maxRetries)
	case APIDisconnected:
		return "Disconnected - Press R to reconnect"
	default:
		return "Unknown"
	}
}

// IsHealthy returns true if the API connection is healthy
func (h *APIHealthTracker) IsHealthy() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.state == APIHealthy
}

// IsDisconnected returns true if the API connection has been lost after all retries
func (h *APIHealthTracker) IsDisconnected() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.state == APIDisconnected
}
