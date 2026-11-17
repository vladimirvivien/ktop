package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TabItem describes a single top-level page tab in the tab bar.
type TabItem struct {
	ID    string
	Title string
	Key   rune
}

// TabBar renders top-level navigation tabs and an optional right-aligned breadcrumb trail.
type TabBar struct {
	table      *tview.Table
	tabs       []TabItem
	activeIdx  int
	breadcrumb string
}

// NewTabBar creates a single-row navigation tab bar.
func NewTabBar() *TabBar {
	table := tview.NewTable()
	table.SetBorder(false)
	table.SetBorders(false).
		SetSelectable(false, false).
		SetFixed(1, 0)

	tb := &TabBar{
		table: table,
	}
	tb.render()
	return tb
}

// GetView returns the underlying tview primitive for layout composition.
func (tb *TabBar) GetView() tview.Primitive {
	return tb.table
}

// SetTabs replaces the displayed tabs and clamps the active tab index.
func (tb *TabBar) SetTabs(tabs []TabItem) {
	tb.tabs = slices.Clone(tabs)
	if len(tb.tabs) == 0 {
		tb.activeIdx = 0
	} else if tb.activeIdx < 0 || tb.activeIdx >= len(tb.tabs) {
		tb.activeIdx = 0
	}
	tb.render()
}

// Tabs returns a copy of the configured tabs.
func (tb *TabBar) Tabs() []TabItem {
	return slices.Clone(tb.tabs)
}

// SetActive updates the active tab index and redraws the bar.
func (tb *TabBar) SetActive(index int) {
	if index < 0 || index >= len(tb.tabs) {
		return
	}
	tb.activeIdx = index
	tb.render()
}

// ActiveIndex returns the currently selected tab index.
func (tb *TabBar) ActiveIndex() int {
	return tb.activeIdx
}

// SetBreadcrumb sets the right-aligned drill-down breadcrumb trail.
func (tb *TabBar) SetBreadcrumb(breadcrumb string) {
	tb.breadcrumb = strings.TrimSpace(breadcrumb)
	tb.render()
}

// Breadcrumb returns the current breadcrumb text.
func (tb *TabBar) Breadcrumb() string {
	return tb.breadcrumb
}

// Refresh redraws the tab bar using the active theme palette.
func (tb *TabBar) Refresh() {
	tb.render()
}

func (tb *TabBar) render() {
	tb.table.Clear()

	mono := IsMonochrome()
	pal := ActivePalette()
	if mono {
		tb.table.SetBackgroundColor(tcell.ColorDefault)
	} else {
		tb.table.SetBackgroundColor(pal.CanvasBg)
	}

	for i, tab := range tb.tabs {
		label := fmt.Sprintf(" %s ", tab.Title)
		if tab.Key != 0 {
			label = fmt.Sprintf(" [%c] %s ", tab.Key, tab.Title)
		}
		// Escape literal brackets so tview does not strip or parse "[1]" as a style tag.
		cell := tview.NewTableCell(tview.Escape(label)).
			SetAlign(tview.AlignLeft).
			SetExpansion(0)

		if i == tb.activeIdx {
			if mono {
				cell.SetTextColor(tcell.ColorDefault).
					SetBackgroundColor(tcell.ColorDefault).
					SetAttributes(tcell.AttrReverse | tcell.AttrBold)
			} else {
				cell.SetTextColor(pal.HeaderFg).
					SetBackgroundColor(pal.HeaderBg).
					SetAttributes(tcell.AttrBold)
			}
		} else {
			if mono {
				cell.SetTextColor(tcell.ColorDefault).
					SetBackgroundColor(tcell.ColorDefault).
					SetAttributes(tcell.AttrNone)
			} else {
				cell.SetTextColor(pal.CanvasFg).
					SetBackgroundColor(pal.CanvasBg).
					SetAttributes(tcell.AttrNone)
			}
		}
		tb.table.SetCell(0, i, cell)
	}

	crumbText := ""
	if tb.breadcrumb != "" {
		crumbText = tview.Escape(tb.breadcrumb) + " "
	}
	crumbCell := tview.NewTableCell(crumbText).
		SetAlign(tview.AlignRight).
		SetExpansion(1)

	if mono {
		attrs := tcell.AttrNone
		if tb.breadcrumb != "" {
			attrs = tcell.AttrBold
		}
		crumbCell.SetTextColor(tcell.ColorDefault).
			SetBackgroundColor(tcell.ColorDefault).
			SetAttributes(attrs)
	} else {
		crumbCell.SetTextColor(pal.CanvasFg).
			SetBackgroundColor(pal.CanvasBg).
			SetAttributes(tcell.AttrNone)
	}
	tb.table.SetCell(0, len(tb.tabs), crumbCell)
}
