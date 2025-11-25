package health

import (
	"errors"
	"testing"
	"time"
)

func TestAPIHealthTracker_InitialState(t *testing.T) {
	tracker := NewAPIHealthTracker(nil)

	if tracker.GetState() != APIHealthy {
		t.Errorf("expected initial state to be APIHealthy, got %v", tracker.GetState())
	}

	if !tracker.IsHealthy() {
		t.Error("expected IsHealthy() to return true initially")
	}

	if tracker.IsDisconnected() {
		t.Error("expected IsDisconnected() to return false initially")
	}
}

func TestAPIHealthTracker_ReportSuccess(t *testing.T) {
	var stateChangeCalled bool
	var lastState APIState
	var lastMessage string

	tracker := NewAPIHealthTracker(func(state APIState, msg string) {
		stateChangeCalled = true
		lastState = state
		lastMessage = msg
	})

	// Report success when already healthy should not trigger callback
	tracker.ReportSuccess()
	if stateChangeCalled {
		t.Error("expected no state change callback when already healthy")
	}

	// Simulate an error then multiple successes (need 3 consecutive)
	// But first, disable minUnhealthyTime for testing by setting lastErrorTime in the past
	tracker.ReportError(errors.New("test error"))
	stateChangeCalled = false

	// Manually set lastErrorTime to past to bypass minUnhealthyTime check in tests
	tracker.mu.Lock()
	tracker.lastErrorTime = time.Now().Add(-15 * time.Second) // 15s ago, past the 10s minimum
	tracker.mu.Unlock()

	// First success - should not restore yet
	tracker.ReportSuccess()
	if stateChangeCalled {
		t.Error("expected no state change after first success (need 3 consecutive)")
	}

	// Second success - should not restore yet
	tracker.ReportSuccess()
	if stateChangeCalled {
		t.Error("expected no state change after second success (need 3 consecutive)")
	}

	// Third success - NOW should restore
	tracker.ReportSuccess()
	if !stateChangeCalled {
		t.Error("expected state change callback after 3 consecutive successes")
	}

	if lastState != APIHealthy {
		t.Errorf("expected state to be APIHealthy, got %v", lastState)
	}

	if lastMessage != "API connection restored" {
		t.Errorf("expected message 'API connection restored', got '%s'", lastMessage)
	}
}

func TestAPIHealthTracker_ReportError(t *testing.T) {
	var states []APIState
	var messages []string

	tracker := NewAPIHealthTracker(func(state APIState, msg string) {
		states = append(states, state)
		messages = append(messages, msg)
	})

	// First error
	tracker.ReportError(errors.New("test error"))

	if tracker.GetState() != APIUnhealthy {
		t.Errorf("expected state to be APIUnhealthy after first error, got %v", tracker.GetState())
	}

	if len(states) != 1 || states[0] != APIUnhealthy {
		t.Error("expected first state change to be APIUnhealthy")
	}
}

func TestAPIHealthTracker_MaxRetries(t *testing.T) {
	var disconnectedCalled bool

	tracker := NewAPIHealthTracker(nil)
	tracker.SetOnDisconnected(func() {
		disconnectedCalled = true
	})

	// Report errors until max retries exceeded
	for i := 0; i <= 5; i++ {
		tracker.ReportError(errors.New("test error"))
	}

	if tracker.GetState() != APIDisconnected {
		t.Errorf("expected state to be APIDisconnected after max retries, got %v", tracker.GetState())
	}

	if !disconnectedCalled {
		t.Error("expected OnDisconnected callback to be called")
	}

	if !tracker.IsDisconnected() {
		t.Error("expected IsDisconnected() to return true")
	}
}

func TestAPIHealthTracker_TryReconnect(t *testing.T) {
	var lastState APIState

	tracker := NewAPIHealthTracker(func(state APIState, msg string) {
		lastState = state
	})

	// Get to disconnected state
	for i := 0; i <= 5; i++ {
		tracker.ReportError(errors.New("test error"))
	}

	if tracker.GetState() != APIDisconnected {
		t.Fatalf("expected APIDisconnected state, got %v", tracker.GetState())
	}

	// Try reconnect
	tracker.TryReconnect()

	if tracker.GetState() != APIUnhealthy {
		t.Errorf("expected state to be APIUnhealthy after TryReconnect, got %v", tracker.GetState())
	}

	if lastState != APIUnhealthy {
		t.Errorf("expected last state change to be APIUnhealthy, got %v", lastState)
	}

	// Retry count should be reset
	if tracker.GetRetryCount() != 0 {
		t.Errorf("expected retry count to be 0 after TryReconnect, got %d", tracker.GetRetryCount())
	}
}

func TestAPIHealthTracker_TryReconnectWhenNotDisconnected(t *testing.T) {
	tracker := NewAPIHealthTracker(nil)

	initialState := tracker.GetState()
	tracker.TryReconnect()

	if tracker.GetState() != initialState {
		t.Error("TryReconnect should not change state when not disconnected")
	}
}

func TestAPIHealthTracker_GetStatusMessage(t *testing.T) {
	tracker := NewAPIHealthTracker(nil)

	// Healthy state
	msg := tracker.GetStatusMessage()
	if msg != "Connected" {
		t.Errorf("expected 'Connected', got '%s'", msg)
	}

	// Unhealthy state
	tracker.ReportError(errors.New("test"))
	msg = tracker.GetStatusMessage()
	if msg != "Retrying (1/5)" {
		t.Errorf("expected 'Retrying (1/5)', got '%s'", msg)
	}

	// Disconnected state
	for i := 0; i < 5; i++ {
		tracker.ReportError(errors.New("test"))
	}
	msg = tracker.GetStatusMessage()
	if msg != "Disconnected - Press R to reconnect" {
		t.Errorf("expected 'Disconnected - Press R to reconnect', got '%s'", msg)
	}
}

func TestAPIHealthTracker_OnHealthyCallback(t *testing.T) {
	var healthyCalled bool

	tracker := NewAPIHealthTracker(nil)
	tracker.SetOnHealthy(func() {
		healthyCalled = true
	})

	// Trigger error then recovery (need 3 consecutive successes)
	tracker.ReportError(errors.New("test"))

	// Manually set lastErrorTime to past to bypass minUnhealthyTime check in tests
	tracker.mu.Lock()
	tracker.lastErrorTime = time.Now().Add(-15 * time.Second) // 15s ago, past the 10s minimum
	tracker.mu.Unlock()

	tracker.ReportSuccess()
	tracker.ReportSuccess()
	tracker.ReportSuccess() // Third one triggers the callback

	if !healthyCalled {
		t.Error("expected OnHealthy callback to be called")
	}
}

func TestAPIHealthTracker_MinUnhealthyTime(t *testing.T) {
	var stateChangeCalled bool

	tracker := NewAPIHealthTracker(func(state APIState, msg string) {
		if state == APIHealthy {
			stateChangeCalled = true
		}
	})

	// Report error to transition to unhealthy
	tracker.ReportError(errors.New("test error"))

	// Try to recover immediately - should NOT work due to minUnhealthyTime
	tracker.ReportSuccess()
	tracker.ReportSuccess()
	tracker.ReportSuccess()

	if stateChangeCalled {
		t.Error("expected no recovery within minUnhealthyTime period")
	}

	if tracker.GetState() != APIUnhealthy {
		t.Errorf("expected state to remain APIUnhealthy, got %v", tracker.GetState())
	}

	// Now set lastErrorTime to the past to simulate time passing
	tracker.mu.Lock()
	tracker.lastErrorTime = time.Now().Add(-15 * time.Second)
	tracker.mu.Unlock()

	// Now recovery should work
	tracker.ReportSuccess()
	tracker.ReportSuccess()
	tracker.ReportSuccess()

	if !stateChangeCalled {
		t.Error("expected recovery after minUnhealthyTime period passed")
	}

	if tracker.GetState() != APIHealthy {
		t.Errorf("expected state to be APIHealthy, got %v", tracker.GetState())
	}
}

func TestAPIState_String(t *testing.T) {
	tests := []struct {
		state    APIState
		expected string
	}{
		{APIHealthy, "Healthy"},
		{APIUnhealthy, "Unhealthy"},
		{APIDisconnected, "Disconnected"},
		{APIState(99), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("APIState(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}

func TestAPIHealthTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewAPIHealthTracker(nil)

	// Test concurrent access doesn't cause race conditions
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			tracker.ReportSuccess()
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			tracker.ReportError(errors.New("test"))
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = tracker.GetState()
			_ = tracker.IsHealthy()
			_ = tracker.GetStatusMessage()
		}
		done <- true
	}()

	// Wait for all goroutines with timeout
	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent operations")
		}
	}
}
