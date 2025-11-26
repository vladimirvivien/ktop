package ui

import (
	"fmt"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ToastLevel defines the severity level of a toast notification
type ToastLevel int

const (
	ToastInfo ToastLevel = iota
	ToastSuccess
	ToastWarning
	ToastError
)

// ToastCallback is called when a button is pressed on the toast
// buttonLabel is the text of the button that was pressed
type ToastCallback func(buttonLabel string)

// NewToast creates a styled tview.Modal for toast notifications (no buttons)
// If a callback is provided, ESC key will trigger it with "Quit"
func NewToast(message string, level ToastLevel) *tview.Modal {
	icon, color := getIconAndColor(level)

	modal := tview.NewModal().
		SetText(fmt.Sprintf("%s %s", icon, message)).
		SetBackgroundColor(color)

	return modal
}

// NewToastWithEscHandler creates a toast that handles ESC key to quit
func NewToastWithEscHandler(message string, level ToastLevel, callback ToastCallback) *tview.Modal {
	icon, color := getIconAndColor(level)

	modal := tview.NewModal().
		SetText(fmt.Sprintf("%s %s", icon, message)).
		SetBackgroundColor(color)

	// Handle ESC key to quit
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			if callback != nil {
				callback("Quit")
			}
			return nil
		}
		return event
	})

	return modal
}

// NewToastWithButtons creates a modal toast with buttons
// The callback is called with the button label when any button is pressed
// ESC key triggers "Quit" callback if Quit button exists, otherwise dismisses
func NewToastWithButtons(message string, level ToastLevel, buttons []string, callback ToastCallback) *tview.Modal {
	icon, color := getIconAndColor(level)

	modal := tview.NewModal().
		SetText(fmt.Sprintf("%s %s", icon, message)).
		SetBackgroundColor(color).
		AddButtons(buttons).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if callback != nil {
				callback(buttonLabel)
			}
		})

	// Handle ESC key to trigger Quit (or dismiss if no Quit button)
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			if callback != nil {
				// Check if Quit button exists
				hasQuit := false
				for _, b := range buttons {
					if b == "Quit" {
						hasQuit = true
						break
					}
				}
				if hasQuit {
					callback("Quit")
				} else {
					// No Quit button, just dismiss (call with empty to signal dismiss)
					callback("")
				}
			}
			return nil // Consume the event
		}
		return event
	})

	return modal
}

func getIconAndColor(level ToastLevel) (string, tcell.Color) {
	switch level {
	case ToastSuccess:
		return "✓", tcell.ColorDarkGreen
	case ToastError:
		return "✗", tcell.ColorDarkRed
	case ToastWarning:
		return "⚠", tcell.ColorDarkOrange
	default: // ToastInfo
		return "ℹ", tcell.ColorDarkBlue
	}
}

// Toast represents a temporary notification message with expiration
type Toast struct {
	Message  string
	Level    ToastLevel
	Duration time.Duration
	Expires  time.Time
}

// ToastManager handles toast notification display and lifecycle
type ToastManager struct {
	current *Toast
	mu      sync.RWMutex
}

// NewToastManager creates a new toast manager
func NewToastManager() *ToastManager {
	return &ToastManager{}
}

// Show displays a toast message for the specified duration
func (t *ToastManager) Show(message string, level ToastLevel, duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.current = &Toast{
		Message:  message,
		Level:    level,
		Duration: duration,
		Expires:  time.Now().Add(duration),
	}
}

// ShowInfo displays an info toast
func (t *ToastManager) ShowInfo(message string, duration time.Duration) {
	t.Show(message, ToastInfo, duration)
}

// ShowWarning displays a warning toast
func (t *ToastManager) ShowWarning(message string, duration time.Duration) {
	t.Show(message, ToastWarning, duration)
}

// ShowError displays an error toast
func (t *ToastManager) ShowError(message string, duration time.Duration) {
	t.Show(message, ToastError, duration)
}

// ShowSuccess displays a success toast
func (t *ToastManager) ShowSuccess(message string, duration time.Duration) {
	t.Show(message, ToastSuccess, duration)
}

// GetActive returns the currently active toast, or nil if expired/none
func (t *ToastManager) GetActive() *Toast {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.current == nil {
		return nil
	}

	if time.Now().After(t.current.Expires) {
		return nil
	}

	return t.current
}

// Clear removes the current toast immediately
func (t *ToastManager) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current = nil
}

// HasActive returns true if there's an active (non-expired) toast
func (t *ToastManager) HasActive() bool {
	return t.GetActive() != nil
}
