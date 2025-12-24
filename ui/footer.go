package ui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

// FooterItem represents a single keyboard shortcut in the footer
type FooterItem struct {
	Key    string // e.g., "Tab", "ESC", "↑↓"
	Action string // e.g., "Next", "Back", "Navigate"
}

// Footer creates and manages the navigation footer
type Footer struct {
	root    *tview.Flex     // Bordered container
	view    *tview.TextView // Text content
	context FooterContext
}

// NewFooter creates a new footer component with border
func NewFooter() *Footer {
	f := &Footer{
		root: tview.NewFlex(),
		view: tview.NewTextView(),
	}
	f.view.SetDynamicColors(true)
	f.view.SetTextAlign(tview.AlignCenter)

	// Add border to root container (matches header style)
	f.root.SetBorder(true)
	f.root.AddItem(f.view, 0, 1, false)

	return f
}

// GetView returns the underlying tview primitive (bordered container)
func (f *Footer) GetView() tview.Primitive {
	return f.root
}

// SetContext updates the footer with new context and re-renders
func (f *Footer) SetContext(ctx FooterContext) {
	f.context = ctx
	f.render()
}

// render updates the footer text based on current context
func (f *Footer) render() {
	if f.context == nil {
		f.view.SetText("")
		return
	}

	items := f.context.GetItems()
	var parts []string
	for _, item := range items {
		// Escape the key to prevent tview from interpreting brackets as color tags
		escapedKey := tview.Escape(item.Key)
		parts = append(parts, fmt.Sprintf("[yellow]%s[-] [white]%s[-]", escapedKey, item.Action))
	}
	f.view.SetText(strings.Join(parts, " • "))
}

// FormatShortcut formats a single shortcut for display (utility function)
func FormatShortcut(key, action string) string {
	return fmt.Sprintf("[yellow]%s[-] [white]%s[-]", key, action)
}
