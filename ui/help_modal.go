package ui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// HelpModalConfig configures the HelpModal display and callbacks
type HelpModalConfig struct {
	CurrentPage  string   // "Overview", "Node Detail", "Pod Detail", "Container Logs"
	FocusedPanel string   // "header", "summary", "nodes", "pods", "containers", "events", "logs"
	PageTitles   []string // Registered top-level page titles
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

	p := ActivePalette()
	m.textView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	m.textView.SetTextColor(p.CanvasFg)
	m.textView.SetBackgroundColor(p.CanvasBg)

	m.textView.SetText(m.renderHelpContent(cfg))
	m.textView.SetInputCapture(m.HandleInput)

	title := " Keyboard Shortcuts "
	if cfg.CurrentPage != "" {
		title = fmt.Sprintf(" Keyboard Shortcuts — %s ", cfg.CurrentPage)
	}

	instrText := "[::b]Esc[::-] / [::b]Enter[::-] / [::b]?[::-] / [::b]q[::-] Close   [::b]↑/↓[::-] Scroll"
	if !IsMonochrome() {
		keyTag := TagFg(p.DataPrimary)
		instrText = fmt.Sprintf("[%s]Esc[-] / [%s]Enter[-] / [%s]?[-] / [%s]q[-] Close   [%s]↑/↓[-] Scroll",
			keyTag, keyTag, keyTag, keyTag, keyTag)
	}
	instructions := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(instrText)
	instructions.SetTextColor(p.CanvasFg)
	instructions.SetBackgroundColor(p.CanvasBg)

	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(m.textView, 0, 1, true).
		AddItem(instructions, 1, 0, false)
	box.SetBackgroundColor(p.CanvasBg)
	box.SetBorder(true).
		SetTitle(title).
		SetTitleAlign(tview.AlignCenter)
	SetFlexFocused(box, true)

	m.grid = tview.NewGrid().
		SetColumns(0, 74, 0).
		SetRows(0, 24, 0).
		AddItem(box, 1, 1, 1, 1, 0, 0, true)

	return m
}

func (m *HelpModal) renderHelpContent(cfg HelpModalConfig) string {
	var b strings.Builder
	p := ActivePalette()
	mono := IsMonochrome()

	key := func(k string) string {
		if mono {
			return "[::b]" + k + "[::-]"
		}
		return fmt.Sprintf("[%s]%s[-]", TagFg(p.Accent), k)
	}
	section := func(title string) string {
		if mono {
			return "[::bu]" + title + "[::-]"
		}
		return fmt.Sprintf("[%s]%s[-]", TagFg(p.DataPrimary), title)
	}
	muted := func(text string) string {
		if mono {
			return "[::d]" + text + "[::-]"
		}
		return fmt.Sprintf("[%s]%s[-]", TagFg(p.StatusMuted), text)
	}
	dot := func() string {
		if mono {
			return "● "
		}
		return fmt.Sprintf("[%s]●[-] ", TagFg(p.GaugeLow))
	}
	badge := func(match bool, label string) string {
		if !match {
			return ""
		}
		if mono {
			return fmt.Sprintf(" [::b](Active: %s)[::-]", label)
		}
		return fmt.Sprintf(" [%s](Active: %s)[-]", TagFg(p.StatusOK), label)
	}

	isTableActive := cfg.FocusedPanel == "nodes" || cfg.FocusedPanel == "pods" || cfg.FocusedPanel == "containers"
	isHeaderActive := cfg.FocusedPanel == "header"
	isLogsActive := cfg.FocusedPanel == "logs" || cfg.CurrentPage == "Container Logs"
	isDetailActive := cfg.CurrentPage == "Node Detail" || cfg.CurrentPage == "Pod Detail"

	// Section 1: Navigation & General
	b.WriteString(section("GENERAL & NAVIGATION") + "\n")
	fmt.Fprintf(&b, "  %s / %s   Cycle focus across Header and page panels\n", key("Tab"), key("Shift+Tab"))
	if len(cfg.PageTitles) > 1 {
		fmt.Fprintf(&b, "  %s / %s     Switch page (%s)\n", key(fmt.Sprintf("1-%d", len(cfg.PageTitles))), key("[ / ]"), strings.Join(cfg.PageTitles, ", "))
	} else {
		fmt.Fprintf(&b, "  %s / %s     Switch top-level page\n", key("1-9"), key("[ / ]"))
	}
	fmt.Fprintf(&b, "  %s           Open detail view for selected Node or Pod\n", key("Enter"))
	fmt.Fprintf(&b, "  %s / %s           Copy selected resource name to clipboard\n", key("y"), key("Y"))
	fmt.Fprintf(&b, "  %s / %s          Toggle this keyboard shortcuts help modal\n", key("?"), key("F1"))
	fmt.Fprintf(&b, "  %s             Clear filter / Back to previous view / Double-ESC quit\n\n", key("Esc"))

	// Section 2: Table Sorting
	activeTableLabel := ""
	if isTableActive {
		activeTableLabel = strings.Title(cfg.FocusedPanel)
	}
	tableHeader := fmt.Sprintf("%s (Nodes / Pods / Containers)%s\n", section("TABLE SORTING"), badge(isTableActive, activeTableLabel))
	if isTableActive {
		tableHeader = dot() + tableHeader
	}
	b.WriteString(tableHeader)
	fmt.Fprintf(&b, "  %s Name     %s CPU      %s Memory    %s Restarts\n", key("n"), key("c"), key("m"), key("r"))
	fmt.Fprintf(&b, "  %s Age      %s Pods/IP  %s Status    %s Containers\n", key("a"), key("p"), key("s"), key("o"))
	fmt.Fprintf(&b, "  %s Ready/Role  %s\n\n", key("l"), muted("(Repeat sort key to toggle ascending/descending)"))

	// Section 3: Filtering & Columns
	filterHeader := fmt.Sprintf("%s%s\n", section("FILTERING & COLUMNS"), badge(isHeaderActive, "Namespace Header"))
	if isHeaderActive {
		filterHeader = dot() + filterHeader
	}
	b.WriteString(filterHeader)
	fmt.Fprintf(&b, "  %s               Start text filter (or filter namespace in Header)\n", key("/"))
	fmt.Fprintf(&b, "  %s               Open column customization dialog\n", key("="))
	fmt.Fprintf(&b, "  %s           Confirm filter text or column selection\n", key("Enter"))
	fmt.Fprintf(&b, "  %s             Cancel filter edit or clear active filter\n\n", key("Esc"))

	// Section 4: Detail Views & Logs
	detailHeader := fmt.Sprintf("%s%s\n", section("DETAIL VIEWS & LOGS"), badge(isLogsActive || isDetailActive, cfg.CurrentPage))
	if isLogsActive || isDetailActive {
		detailHeader = dot() + detailHeader
	}
	b.WriteString(detailHeader)
	fmt.Fprintf(&b, "  %s             Switch sections (Events, Containers, Volumes, Logs)\n", key("Tab"))
	fmt.Fprintf(&b, "  %s               Container spec view / Stream logs toggle\n", key("s"))
	fmt.Fprintf(&b, "  %s Toggle time   %s Wrap logs   %s More logs   %s Expand\n", key("t"), key("w"), key("m"), key("x"))
	fmt.Fprintf(&b, "  %s / %s       Jump to oldest / newest log line\n", key("g"), key("G"))

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
