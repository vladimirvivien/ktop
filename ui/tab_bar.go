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

func buildTabsMarkup(tabs []TabItem, activeIdx int) (string, int) {
	mono := IsMonochrome()
	pal := ActivePalette()

	var tabsMarkup strings.Builder
	tabsWidth := 0

	for i, tab := range tabs {
		label := fmt.Sprintf(" %s ", tab.Title)
		if tab.Key != 0 {
			label = fmt.Sprintf(" [%c] %s ", tab.Key, tab.Title)
		}
		escaped := tview.Escape(label)
		tabsWidth += tview.TaggedStringWidth(escaped)

		if i == activeIdx {
			if mono {
				tabsMarkup.WriteString("[::rb]" + escaped + "[::-]")
			} else {
				fmt.Fprintf(&tabsMarkup, "[%s:%s:b]%s[-:-:-]", TagFg(pal.TabActiveFg), TagFg(pal.TabActiveBg), escaped)
			}
		} else {
			if mono {
				tabsMarkup.WriteString(escaped)
			} else {
				fmt.Fprintf(&tabsMarkup, "[%s:%s:-]%s[-:-:-]", TagFg(pal.TabInactiveFg), TagFg(pal.TabInactiveBg), escaped)
			}
		}
	}

	return tabsMarkup.String(), tabsWidth
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

	tabsMarkup, tabsWidth := buildTabsMarkup(tb.tabs, tb.activeIdx)

	tb.tabsView.SetTextColor(tcell.ColorDefault)
	if !mono {
		tb.tabsView.SetTextColor(pal.CanvasFg)
	}
	tb.tabsView.SetText(tabsMarkup)

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

// TabFrame wraps a content primitive in a bordered container whose top border
// displays navigation tabs on the left and an optional drill-down breadcrumb on the right.
type TabFrame struct {
	*tview.Box
	content    tview.Primitive
	tabs       []TabItem
	activeIdx  int
	breadcrumb string
}

// NewTabFrame creates a bordered tab container wrapping content.
func NewTabFrame(content tview.Primitive) *TabFrame {
	box := tview.NewBox().SetBorder(true)
	return &TabFrame{
		Box:     box,
		content: content,
	}
}

// GetView returns the TabFrame as a tview.Primitive for layout composition.
func (tf *TabFrame) GetView() tview.Primitive {
	return tf
}

// SetContent replaces the wrapped child primitive rendered inside the frame.
func (tf *TabFrame) SetContent(content tview.Primitive) {
	tf.content = content
}

// SetTabs replaces the displayed tabs and clamps the active tab index.
func (tf *TabFrame) SetTabs(tabs []TabItem) {
	tf.tabs = slices.Clone(tabs)
	if len(tf.tabs) == 0 {
		tf.activeIdx = 0
	} else if tf.activeIdx < 0 || tf.activeIdx >= len(tf.tabs) {
		tf.activeIdx = 0
	}
}

// Tabs returns a copy of the configured tabs.
func (tf *TabFrame) Tabs() []TabItem {
	return slices.Clone(tf.tabs)
}

// SetActive updates the active tab index.
func (tf *TabFrame) SetActive(index int) {
	if index < 0 || index >= len(tf.tabs) {
		return
	}
	tf.activeIdx = index
}

// ActiveIndex returns the currently selected tab index.
func (tf *TabFrame) ActiveIndex() int {
	return tf.activeIdx
}

// SetBreadcrumb sets the right-aligned drill-down breadcrumb trail.
func (tf *TabFrame) SetBreadcrumb(breadcrumb string) {
	tf.breadcrumb = strings.TrimSpace(breadcrumb)
}

// Breadcrumb returns the current breadcrumb text.
func (tf *TabFrame) Breadcrumb() string {
	return tf.breadcrumb
}

// Refresh is a no-op hook kept for parity with TabBar; Draw reads the active palette live.
func (tf *TabFrame) Refresh() {}

// Draw renders the outer border, overlays tabs and breadcrumb onto the top border line,
// and draws the wrapped content primitive inside the inner rectangle.
func (tf *TabFrame) Draw(screen tcell.Screen) {
	mono := IsMonochrome()
	pal := ActivePalette()
	defaultFg := pal.CanvasFg
	if mono {
		defaultFg = tcell.ColorDefault
		tf.Box.SetBackgroundColor(tcell.ColorDefault)
		tf.Box.SetBorderColor(tcell.ColorDefault)
	} else {
		tf.Box.SetBackgroundColor(pal.CanvasBg)
		tf.Box.SetBorderColor(pal.BorderNormal)
	}

	// Draw via Box directly so the outer frame keeps single-line unfocused borders
	// while inner page panels indicate keyboard focus.
	tf.Box.Draw(screen)

	x, y, w, h := tf.GetRect()
	if w >= 4 && h >= 2 {
		leftX := x + 2
		rightLimit := x + w - 2
		avail := rightLimit - leftX

		tabsEnd := leftX
		if avail > 0 && len(tf.tabs) > 0 {
			tabsMarkup, tabsWidth := buildTabsMarkup(tf.tabs, tf.activeIdx)
			drawW := min(tabsWidth, avail)
			tview.Print(screen, tabsMarkup, leftX, y, drawW, tview.AlignLeft, defaultFg)
			tabsEnd = leftX + drawW
		}

		if tf.breadcrumb != "" {
			crumbStart := tabsEnd
			if len(tf.tabs) > 0 {
				crumbStart++ // Keep at least one border dash between tabs and breadcrumb.
			}
			crumbAvail := rightLimit - crumbStart
			if crumbAvail > 0 {
				escapedCrumb := " " + tview.Escape(tf.breadcrumb) + " "
				crumbWidth := tview.TaggedStringWidth(escapedCrumb)
				var crumbMarkup string
				crumbFg := pal.Accent
				if mono {
					crumbFg = tcell.ColorDefault
					crumbMarkup = "[::b]" + escapedCrumb + "[::-]"
				} else {
					crumbMarkup = fmt.Sprintf("[%s:%s:b]%s[-:-:-]", TagFg(pal.Accent), TagFg(pal.CanvasBg), escapedCrumb)
				}
				drawX := max(crumbStart, rightLimit-crumbWidth)
				drawW := rightLimit - drawX
				tview.Print(screen, crumbMarkup, drawX, y, drawW, tview.AlignLeft, crumbFg)
			}
		}
	}

	if tf.content != nil {
		ix, iy, iw, ih := tf.GetInnerRect()
		if iw > 0 && ih > 0 {
			tf.content.SetRect(ix, iy, iw, ih)
			tf.content.Draw(screen)
		}
	}
}

// Focus delegates focus to the wrapped content primitive.
func (tf *TabFrame) Focus(delegate func(p tview.Primitive)) {
	if tf.content != nil {
		delegate(tf.content)
		return
	}
	tf.Box.Focus(delegate)
}

// Blur delegates blur to the wrapped content primitive.
func (tf *TabFrame) Blur() {
	if tf.content != nil {
		tf.content.Blur()
	}
	tf.Box.Blur()
}

// HasFocus reports whether the wrapped content primitive or frame has focus.
func (tf *TabFrame) HasFocus() bool {
	if tf.content != nil && tf.content.HasFocus() {
		return true
	}
	return tf.Box.HasFocus()
}

// InputHandler delegates keyboard input to the wrapped content primitive.
func (tf *TabFrame) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return tf.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if tf.content != nil {
			if handler := tf.content.InputHandler(); handler != nil {
				handler(event, setFocus)
			}
		}
	})
}

// MouseHandler delegates mouse events inside the frame to the wrapped content primitive.
func (tf *TabFrame) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return tf.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if !tf.InRect(event.Position()) {
			return false, nil
		}
		if tf.content != nil {
			if handler := tf.content.MouseHandler(); handler != nil {
				return handler(action, event, setFocus)
			}
		}
		return false, nil
	})
}
