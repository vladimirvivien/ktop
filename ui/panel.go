package ui

import (
	"context"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Panel interface {
	Layout(data any)
	DrawHeader(data any)
	// TODO add context to DrawXXX methods
	DrawBody(data any)
	DrawFooter(param any)
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

// FocusablePanel is an optional interface that panels can implement
// to support visual focus indication with double-border and color change
type FocusablePanel interface {
	// SetFocused updates the panel's visual state for focus
	SetFocused(focused bool)
}

// SetBoxFocused applies focus styling to a tview.Box (or any type that embeds it).
func SetBoxFocused(box *tview.Box, focused bool) {
	if box == nil {
		return
	}
	if IsMonochrome() {
		box.SetBorderColor(tcell.ColorDefault)
		box.SetTitleColor(tcell.ColorDefault)
		if focused {
			box.SetBorderAttributes(tcell.AttrBold)
		} else {
			box.SetBorderAttributes(tcell.AttrNone)
		}
		return
	}
	p := ActivePalette()
	box.SetBorderAttributes(tcell.AttrNone)
	if focused {
		box.SetBorderColor(p.BorderFocused)
		box.SetTitleColor(p.TitleFocused)
	} else {
		box.SetBorderColor(p.BorderNormal)
		box.SetTitleColor(p.TitleNormal)
	}
}

// SetFlexFocused applies focus styling to a tview.Flex container.
func SetFlexFocused(flex *tview.Flex, focused bool) {
	if flex == nil {
		return
	}
	if IsMonochrome() {
		flex.SetBorderColor(tcell.ColorDefault)
		flex.SetTitleColor(tcell.ColorDefault)
		if focused {
			flex.SetBorderAttributes(tcell.AttrBold)
		} else {
			flex.SetBorderAttributes(tcell.AttrNone)
		}
		return
	}
	p := ActivePalette()
	flex.SetBorderAttributes(tcell.AttrNone)
	if focused {
		flex.SetBorderColor(p.BorderFocused)
		flex.SetTitleColor(p.TitleFocused)
	} else {
		flex.SetBorderColor(p.BorderNormal)
		flex.SetTitleColor(p.TitleNormal)
	}
}

// SetTableFocused applies focus styling to a tview.Table.
func SetTableFocused(table *tview.Table, focused bool) {
	if table == nil {
		return
	}
	if IsMonochrome() {
		table.SetBorderColor(tcell.ColorDefault)
		table.SetTitleColor(tcell.ColorDefault)
		if focused {
			table.SetBorderAttributes(tcell.AttrBold)
		} else {
			table.SetBorderAttributes(tcell.AttrNone)
		}
		return
	}
	p := ActivePalette()
	table.SetBorderAttributes(tcell.AttrNone)
	if focused {
		table.SetBorderColor(p.BorderFocused)
		table.SetTitleColor(p.TitleFocused)
	} else {
		table.SetBorderColor(p.BorderNormal)
		table.SetTitleColor(p.TitleNormal)
	}
}

// FocusBorderColor returns the tcell color for focused panels.
func FocusBorderColor() tcell.Color {
	if IsMonochrome() {
		return tcell.ColorDefault
	}
	return ActivePalette().BorderFocused
}

// UnfocusBorderColor returns the tcell color for unfocused panels.
func UnfocusBorderColor() tcell.Color {
	if IsMonochrome() {
		return tcell.ColorDefault
	}
	return ActivePalette().BorderNormal
}

// Double-border Unicode box-drawing characters
const (
	DoubleBorderHorizontal  = '═'
	DoubleBorderVertical    = '║'
	DoubleBorderTopLeft     = '╔'
	DoubleBorderTopRight    = '╗'
	DoubleBorderBottomLeft  = '╚'
	DoubleBorderBottomRight = '╝'
)

// DrawDoubleBorder draws a double-line border around the given rectangle
func DrawDoubleBorder(screen tcell.Screen, x, y, width, height int, color tcell.Color) {
	style := tcell.StyleDefault.Foreground(color)

	// Draw corners
	screen.SetContent(x, y, DoubleBorderTopLeft, nil, style)
	screen.SetContent(x+width-1, y, DoubleBorderTopRight, nil, style)
	screen.SetContent(x, y+height-1, DoubleBorderBottomLeft, nil, style)
	screen.SetContent(x+width-1, y+height-1, DoubleBorderBottomRight, nil, style)

	// Draw horizontal lines
	for i := x + 1; i < x+width-1; i++ {
		screen.SetContent(i, y, DoubleBorderHorizontal, nil, style)
		screen.SetContent(i, y+height-1, DoubleBorderHorizontal, nil, style)
	}

	// Draw vertical lines
	for i := y + 1; i < y+height-1; i++ {
		screen.SetContent(x, i, DoubleBorderVertical, nil, style)
		screen.SetContent(x+width-1, i, DoubleBorderVertical, nil, style)
	}
}
