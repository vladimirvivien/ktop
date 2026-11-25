package storage

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
	summary model.StorageSummary
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
	s, ok := data.(model.StorageSummary)
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

	pvcText := fmt.Sprintf("%sPVCs:%s %s%d/%d bound%s", lblOpen, resetTag, okOpen, s.PVCsBound, s.PVCsTotal, resetTag)
	if s.PVCsPending > 0 {
		pvcText += fmt.Sprintf(" %s(%d pending)%s", warnOpen, s.PVCsPending, resetTag)
	}
	if s.HighFillCount > 0 {
		pvcText += fmt.Sprintf(" %s(%d >=85%%)%s", errOpen, s.HighFillCount, resetTag)
	}

	parts := []string{
		pvcText,
		fmt.Sprintf("%sPVs:%s %s%d/%d bound%s", lblOpen, resetTag, valOpen, s.PVsBound, s.PVsTotal, resetTag),
		fmt.Sprintf("%sClasses:%s %s%d%s", lblOpen, resetTag, valOpen, s.StorageClasses, resetTag),
	}
	if s.TotalCapacity > 0 {
		parts = append(parts, fmt.Sprintf("%sUsed:%s %s%s/%s%s", lblOpen, resetTag, valOpen, ui.FormatBytes(s.TotalUsed), ui.FormatBytes(s.TotalCapacity), resetTag))
	}
	parts = append(parts, fmt.Sprintf("%sDisk I/O:%s %sR %s  W %s%s",
		lblOpen, resetTag, valOpen, ui.FormatBytesRate(s.DiskReadRateBps), ui.FormatBytesRate(s.DiskWriteRateBps), resetTag))

	p.text.SetText(" " + strings.Join(parts, sep))
}
