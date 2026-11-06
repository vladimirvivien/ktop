package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// HelpModalConfig configures the HelpModal display and callbacks
type HelpModalConfig struct {
	CurrentPage  string // "Overview", "Node Detail", "Pod Detail", "Container Logs"
	FocusedPanel string // "header", "summary", "nodes", "pods", "containers", "events", "logs"
	OnClose      func()
}

// HelpModal provides a centered, interactive popup displaying keyboard shortcuts
type HelpModal struct {
	grid     *tview.Grid
	textView *tview.TextView
	onClose  func()
}

// NewHelpModal creates a keyboard shortcut help overlay modal
func NewHelpModal(cfg HelpModalConfig) *HelpModal {
	m := &HelpModal{
		onClose: cfg.OnClose,
	}

	m.textView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)

	m.textView.SetText(m.renderHelpContent(cfg))
	m.textView.SetInputCapture(m.HandleInput)

	title := " Keyboard Shortcuts "
	if cfg.CurrentPage != "" {
		title = fmt.Sprintf(" Keyboard Shortcuts — %s ", cfg.CurrentPage)
	}

	instructions := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[yellow]Esc[-] / [yellow]Enter[-] / [yellow]?[-] / [yellow]q[-] Close   [yellow]↑/↓[-] Scroll")

	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(m.textView, 0, 1, true).
		AddItem(instructions, 1, 0, false)
	box.SetBorder(true).
		SetTitle(title).
		SetTitleAlign(tview.AlignCenter)

	m.grid = tview.NewGrid().
		SetColumns(0, 74, 0).
		SetRows(0, 24, 0).
		AddItem(box, 1, 1, 1, 1, 0, 0, true)

	return m
}

func (m *HelpModal) renderHelpContent(cfg HelpModalConfig) string {
	var b strings.Builder

	// Helper for context badges
	badge := func(match bool, label string) string {
		if match {
			return fmt.Sprintf(" [lime](Active: %s)[-]", label)
		}
		return ""
	}

	isTableActive := cfg.FocusedPanel == "nodes" || cfg.FocusedPanel == "pods" || cfg.FocusedPanel == "containers"
	isHeaderActive := cfg.FocusedPanel == "header"
	isLogsActive := cfg.FocusedPanel == "logs" || cfg.CurrentPage == "Container Logs"
	isDetailActive := cfg.CurrentPage == "Node Detail" || cfg.CurrentPage == "Pod Detail"

	// Section 1: Navigation & General
	b.WriteString("[yellow]GENERAL & NAVIGATION[-]\n")
	b.WriteString("  [aqua]Tab[-] / [aqua]Shift+Tab[-]   Cycle focus (Header → Summary → Nodes → Pods)\n")
	b.WriteString("  [aqua]Enter[-]           Open detail view for selected Node or Pod\n")
	b.WriteString("  [aqua]y[-] / [aqua]Y[-]           Copy selected resource name to clipboard\n")
	b.WriteString("  [aqua]?[-] / [aqua]F1[-]          Toggle this keyboard shortcuts help modal\n")
	b.WriteString("  [aqua]Esc[-]             Clear filter / Back to previous view / Double-ESC quit\n\n")

	// Section 2: Table Sorting
	activeTableLabel := ""
	if isTableActive {
		activeTableLabel = strings.Title(cfg.FocusedPanel)
	}
	tableHeader := fmt.Sprintf("[yellow]TABLE SORTING[-] (Nodes / Pods / Containers)%s\n", badge(isTableActive, activeTableLabel))
	if isTableActive {
		tableHeader = "[green]●[-] " + tableHeader
	}
	b.WriteString(tableHeader)
	b.WriteString("  [aqua]n[-] Name     [aqua]c[-] CPU      [aqua]m[-] Memory    [aqua]r[-] Restarts\n")
	b.WriteString("  [aqua]a[-] Age      [aqua]p[-] Pods/IP  [aqua]s[-] Status    [aqua]o[-] Containers\n")
	b.WriteString("  [aqua]l[-] Ready/Role  [darkgray](Repeat sort key to toggle ascending/descending)[-]\n\n")

	// Section 3: Filtering & Columns
	filterHeader := fmt.Sprintf("[yellow]FILTERING & COLUMNS[-]%s\n", badge(isHeaderActive, "Namespace Header"))
	if isHeaderActive {
		filterHeader = "[green]●[-] " + filterHeader
	}
	b.WriteString(filterHeader)
	b.WriteString("  [aqua]/[-]               Start text filter (or filter namespace in Header)\n")
	b.WriteString("  [aqua]=[-]               Open column customization dialog\n")
	b.WriteString("  [aqua]Enter[-]           Confirm filter text or column selection\n")
	b.WriteString("  [aqua]Esc[-]             Cancel filter edit or clear active filter\n\n")

	// Section 4: Detail Views & Logs
	detailHeader := fmt.Sprintf("[yellow]DETAIL VIEWS & LOGS[-]%s\n", badge(isLogsActive || isDetailActive, cfg.CurrentPage))
	if isLogsActive || isDetailActive {
		detailHeader = "[green]●[-] " + detailHeader
	}
	b.WriteString(detailHeader)
	b.WriteString("  [aqua]Tab[-]             Switch sections (Events, Containers, Volumes, Logs)\n")
	b.WriteString("  [aqua]s[-]               Container spec view / Stream logs toggle\n")
	b.WriteString("  [aqua]t[-] Toggle time   [aqua]w[-] Wrap logs   [aqua]m[-] More logs   [aqua]x[-] Expand\n")
	b.WriteString("  [aqua]g[-] / [aqua]G[-]       Jump to oldest / newest log line\n")

	return b.String()
}

// HandleInput processes keyboard events for closing or scrolling the help modal
func (m *HelpModal) HandleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape, tcell.KeyEnter, tcell.KeyF1:
		if m.onClose != nil {
			m.onClose()
		}
		return nil
	case tcell.KeyUp:
		row, col := m.textView.GetScrollOffset()
		if row > 0 {
			m.textView.ScrollTo(row-1, col)
		}
		return nil
	case tcell.KeyDown:
		row, col := m.textView.GetScrollOffset()
		m.textView.ScrollTo(row+1, col)
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case '?', 'q', 'Q':
			if m.onClose != nil {
				m.onClose()
			}
			return nil
		case 'j':
			row, col := m.textView.GetScrollOffset()
			m.textView.ScrollTo(row+1, col)
			return nil
		case 'k':
			row, col := m.textView.GetScrollOffset()
			if row > 0 {
				m.textView.ScrollTo(row-1, col)
			}
			return nil
		}
	}
	return event
}

// GetRootView returns the root grid primitive to be displayed in tview.Pages
func (m *HelpModal) GetRootView() tview.Primitive {
	return m.grid
}

// GetFocusPrimitive returns the inner text view that receives focus and keyboard navigation
func (m *HelpModal) GetFocusPrimitive() tview.Primitive {
	return m.textView
}
