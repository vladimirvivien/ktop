package network

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

type summaryPanel struct {
	title   string
	root    *tview.Flex
	text    *tview.TextView
	summary model.NetworkSummary
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
	s, ok := data.(model.NetworkSummary)
	if !ok {
		return
	}
	p.summary = s
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

	s := p.summary
	sep := fmt.Sprintf("  %s|%s  ", mutedOpen, resetTag)

	svcText := fmt.Sprintf("%sServices:%s %s%d%s", lblOpen, resetTag, valOpen, s.ServicesTotal, resetTag)
	if s.ZeroEndpointServices > 0 {
		svcText += fmt.Sprintf(" %s(%d zero-ep)%s", errOpen, s.ZeroEndpointServices, resetTag)
	}

	netIOText := fmt.Sprintf("%sNet I/O:%s %sn/a%s", lblOpen, resetTag, mutedOpen, resetTag)
	if s.HasMetrics {
		netIOText = fmt.Sprintf("%sNet I/O:%s %sRX %s  TX %s%s",
			lblOpen, resetTag, valOpen, ui.FormatBytesRate(s.ClusterRxRateBps), ui.FormatBytesRate(s.ClusterTxRateBps), resetTag)
	}

	dropsColor := okOpen
	if s.DropsPerSec > 0 {
		dropsColor = warnOpen
	}
	errsColor := okOpen
	if s.ErrorsPerSec > 0 {
		errsColor = errOpen
	}

	parts := []string{
		svcText,
		fmt.Sprintf("%sIngresses:%s %s%d%s", lblOpen, resetTag, valOpen, s.IngressesTotal, resetTag),
		netIOText,
		fmt.Sprintf("%sDrops:%s %s%.1f/s%s  %sErrs:%s %s%.1f/s%s",
			lblOpen, resetTag, dropsColor, s.DropsPerSec, resetTag,
			lblOpen, resetTag, errsColor, s.ErrorsPerSec, resetTag),
	}

	p.text.SetText(" " + strings.Join(parts, sep))
}
