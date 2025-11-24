package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestNewToast(t *testing.T) {
	tests := []struct {
		name          string
		message       string
		level         ToastLevel
		expectedIcon  string
		expectedColor tcell.Color
	}{
		{
			name:          "Info toast",
			message:       "Loading metrics",
			level:         ToastInfo,
			expectedIcon:  "ℹ",
			expectedColor: tcell.ColorDarkBlue,
		},
		{
			name:          "Success toast",
			message:       "Connected",
			level:         ToastSuccess,
			expectedIcon:  "✓",
			expectedColor: tcell.ColorDarkGreen,
		},
		{
			name:          "Warning toast",
			message:       "Slow response",
			level:         ToastWarning,
			expectedIcon:  "⚠",
			expectedColor: tcell.ColorDarkOrange,
		},
		{
			name:          "Error toast",
			message:       "Connection failed",
			level:         ToastError,
			expectedIcon:  "✗",
			expectedColor: tcell.ColorDarkRed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toast := NewToast(tt.message, tt.level)
			if toast == nil {
				t.Fatal("NewToast returned nil")
			}

			// Verify the toast was created (Modal doesn't expose GetText, so just check creation)
			// The actual rendering is handled by tview
		})
	}
}

func TestGetIconAndColor(t *testing.T) {
	tests := []struct {
		level         ToastLevel
		expectedIcon  string
		expectedColor tcell.Color
	}{
		{ToastInfo, "ℹ", tcell.ColorDarkBlue},
		{ToastSuccess, "✓", tcell.ColorDarkGreen},
		{ToastWarning, "⚠", tcell.ColorDarkOrange},
		{ToastError, "✗", tcell.ColorDarkRed},
	}

	for _, tt := range tests {
		t.Run(tt.expectedIcon, func(t *testing.T) {
			icon, color := getIconAndColor(tt.level)
			if icon != tt.expectedIcon {
				t.Errorf("Expected icon %q, got %q", tt.expectedIcon, icon)
			}
			if color != tt.expectedColor {
				t.Errorf("Expected color %v, got %v", tt.expectedColor, color)
			}
		})
	}
}
