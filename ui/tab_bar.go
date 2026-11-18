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
	root       *tview.Flex
	tabsView   *tview.TextView
	crumbView  *tview.TextView
	tabs       []TabItem
	activeIdx  int
	breadcrumb string
}

// NewTabBar creates a single-row navigation tab bar.
func NewTabBar() *TabBar {
	tabsView := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false)
	tabsView.SetBorder(false)

	crumbView := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetTextAlign(tview.AlignRight)
	crumbView.SetBorder(false)

	root := tview.NewFlex().SetDirection(tview.FlexColumn)
	root.SetBorder(false)

	tb := &TabBar{
		root:      root,
		tabsView:  tabsView,
		crumbView: crumbView,
	}
	tb.render()
	return tb
}

// GetView returns the underlying tview primitive for layout composition.
func (tb *TabBar) GetView() tview.Primitive {
	return tb.root
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
	mono := IsMonochrome()
	pal := ActivePalette()
	bg := pal.CanvasBg
	if mono {
		bg = tcell.ColorDefault
	}

	tb.root.SetBackgroundColor(bg)
	tb.tabsView.SetBackgroundColor(bg)
	tb.crumbView.SetBackgroundColor(bg)

	var tabsMarkup strings.Builder
	tabsWidth := 0

	for i, tab := range tb.tabs {
		label := fmt.Sprintf(" %s ", tab.Title)
		if tab.Key != 0 {
			label = fmt.Sprintf(" [%c] %s ", tab.Key, tab.Title)
		}
		tabsWidth += tview.TaggedStringWidth(tview.Escape(label))
		escaped := tview.Escape(label)

		if i == tb.activeIdx {
			if mono {
				tabsMarkup.WriteString("[::rb]" + escaped + "[::-]")
			} else {
				fmt.Fprintf(&tabsMarkup, "[%s:%s:b]%s[-:-:-]", TagFg(pal.HeaderFg), TagFg(pal.HeaderBg), escaped)
			}
		} else {
			if mono {
				tabsMarkup.WriteString(escaped)
			} else {
				fmt.Fprintf(&tabsMarkup, "[%s:%s:-]%s[-:-:-]", TagFg(pal.TabInactiveFg), TagFg(pal.TabInactiveBg), escaped)
			}
		}
	}

	tb.tabsView.SetTextColor(tcell.ColorDefault)
	if !mono {
		tb.tabsView.SetTextColor(pal.CanvasFg)
	}
	tb.tabsView.SetText(tabsMarkup.String())

	crumbMarkup := ""
	if tb.breadcrumb != "" {
		escapedCrumb := tview.Escape(tb.breadcrumb) + " "
		if mono {
			tb.crumbView.SetTextColor(tcell.ColorDefault)
			crumbMarkup = "[::b]" + escapedCrumb + "[::-]"
		} else {
			tb.crumbView.SetTextColor(pal.Accent)
			crumbMarkup = fmt.Sprintf("[%s:%s:b]%s[-:-:-]", TagFg(pal.Accent), TagFg(pal.CanvasBg), escapedCrumb)
		}
	} else if !mono {
		tb.crumbView.SetTextColor(pal.StatusMuted)
	} else {
		tb.crumbView.SetTextColor(tcell.ColorDefault)
	}
	tb.crumbView.SetText(crumbMarkup)

	tb.root.Clear()
	if tabsWidth > 0 {
		tb.root.AddItem(tb.tabsView, tabsWidth, 0, false)
	}
	tb.root.AddItem(tb.crumbView, 0, 1, false)
}
