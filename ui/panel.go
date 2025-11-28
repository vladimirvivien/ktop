package ui

import (
	"context"

	"github.com/rivo/tview"
)

type Panel interface {
	Layout(data interface{})
	DrawHeader(data interface{})
	// TODO add context to DrawXXX methods
	DrawBody(data interface{})
	DrawFooter(param interface{})
	Clear()
	GetTitle() string
	GetRootView() tview.Primitive
	GetChildrenViews() []tview.Primitive
}

type PanelController interface {
	Panel
	Run(context.Context) error
}

// EscapablePanel is an optional interface that panels can implement
// to indicate they have state that ESC should clear before quitting the app
type EscapablePanel interface {
	// HasEscapableState returns true if the panel has state that ESC should clear
	// (e.g., active filter, edit mode, selection)
	HasEscapableState() bool
	// HandleEscape handles ESC key press - clears state and returns true if handled
	HandleEscape() bool
}
