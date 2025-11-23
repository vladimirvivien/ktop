package ui

import (
	"fmt"

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

// NewToast creates a styled tview.Modal for toast notifications
func NewToast(message string, level ToastLevel) *tview.Modal {
	icon, color := getIconAndColor(level)

	modal := tview.NewModal().
		SetText(fmt.Sprintf("%s %s", icon, message)).
		SetBackgroundColor(color)

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
