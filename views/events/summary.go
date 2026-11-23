package events

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

type summaryState struct {
	Summary      model.EventsSummary
	WarningsOnly bool
	Paused       bool
	PendingDelta int
	Truncated    bool
	Shown        int
}

type summaryPanel struct {
	title string
	root  *tview.Flex
	text  *tview.TextView
	state summaryState
}

func newSummaryPanel(title string) *summaryPanel {
	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetWrap(false)
	tv.SetBorder(false)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tv, 0, 1, false)
	root.SetBorder(true)
	root.SetTitle(title)
	root.SetTitleAlign(tview.AlignLeft)
	ui.SetFlexFocused(root, false)

	p := &summaryPanel{
		title: title,
		root:  root,
		text:  tv,
	}
	p.render()
	return p
}

func (p *summaryPanel) Layout(_ any)                 {}
func (p *summaryPanel) DrawHeader(_ any)             { p.render() }
func (p *summaryPanel) DrawFooter(_ any)             {}
func (p *summaryPanel) Clear()                       {}
func (p *summaryPanel) GetTitle() string             { return p.title }
func (p *summaryPanel) GetRootView() tview.Primitive { return p.root }
func (p *summaryPanel) GetChildrenViews() []tview.Primitive {
	return []tview.Primitive{p.root}
}

func (p *summaryPanel) SetFocused(focused bool) {
	ui.SetFlexFocused(p.root, focused)
}

func (p *summaryPanel) DrawBody(data any) {
	st, ok := data.(summaryState)
	if !ok {
		return
	}
	p.state = st
	p.render()
}

func (p *summaryPanel) render() {
	if p.text == nil {
		return
	}
	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()
	if mono {
		p.text.SetBackgroundColor(tcell.ColorDefault)
		p.text.SetTextColor(tcell.ColorDefault)
	} else {
		p.text.SetBackgroundColor(pal.CanvasBg)
		p.text.SetTextColor(pal.DataSecondary)
	}

	lblOpen, valOpen, mutedOpen, okOpen, warnOpen, errOpen, resetTag := "", "", "", "", "", "", ""
	if !mono {
		lblOpen = "[" + ui.TagFg(pal.DataPrimary) + "]"
		valOpen = "[" + ui.TagFg(pal.DataSecondary) + "]"
		mutedOpen = "[" + ui.TagFg(pal.StatusMuted) + "]"
		okOpen = "[" + ui.TagFg(pal.StatusOK) + "]"
		warnOpen = "[" + ui.TagFg(pal.StatusWarning) + "]"
		errOpen = "[" + ui.TagFg(pal.StatusError) + "]"
		resetTag = "[-]"
	}

	s := p.state.Summary
	sep := fmt.Sprintf("  %s|%s  ", mutedOpen, resetTag)

	warnColor := okOpen
	if s.WarningCount > 0 {
		warnColor = warnOpen
		if mono {
			warnColor = "[::b]"
		}
	}
	warnClose := resetTag
	if mono && s.WarningCount > 0 {
		warnClose = "[::-]"
	}

	var parts []string
	totalLabel := fmt.Sprintf("%d", s.Total)
	if p.state.Truncated {
		totalLabel = fmt.Sprintf("%d (showing %d)", s.Total, p.state.Shown)
	}
	parts = append(parts, fmt.Sprintf("%sEvents:%s %s%s%s", lblOpen, resetTag, valOpen, totalLabel, resetTag))
	parts = append(parts, fmt.Sprintf("%sWarnings:%s %s%d%s", lblOpen, resetTag, warnColor, s.WarningCount, warnClose))
	parts = append(parts, fmt.Sprintf("%sNormal:%s %s%d%s", lblOpen, resetTag, valOpen, s.NormalCount, resetTag))

	if s.TopReason != "" {
		reasonColor := valOpen
		if s.WarningCount > 0 {
			reasonColor = warnOpen
		}
		parts = append(parts, fmt.Sprintf("%sTop:%s %s%s (%d)%s", lblOpen, resetTag, reasonColor, s.TopReason, s.TopReasonCount, resetTag))
	}

	filterBadge := "ALL"
	filterColor := valOpen
	if p.state.WarningsOnly {
		filterBadge = "WARNINGS ONLY"
		filterColor = warnOpen
	}

	modeBadge := "LIVE"
	modeColor := okOpen
	if p.state.Paused {
		if p.state.PendingDelta > 0 {
			modeBadge = fmt.Sprintf("PAUSED (+%d new)", p.state.PendingDelta)
		} else {
			modeBadge = "PAUSED"
		}
		modeColor = errOpen
	}

	parts = append(parts, fmt.Sprintf("[%s%s%s · %s%s%s]", filterColor, filterBadge, resetTag, modeColor, modeBadge, resetTag))

	p.text.SetText(" " + strings.Join(parts, sep))
}
