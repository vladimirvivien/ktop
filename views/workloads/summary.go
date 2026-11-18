package workloads

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
	summary model.WorkloadSummaryModel
	ns      string
}

func newSummaryPanel(title string) *summaryPanel {
	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
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

func (p *summaryPanel) Layout(_ any) {}
func (p *summaryPanel) DrawHeader(_ any) {
	p.render()
}
func (p *summaryPanel) DrawFooter(_ any) {}
func (p *summaryPanel) Clear()           {}
func (p *summaryPanel) GetTitle() string { return p.title }
func (p *summaryPanel) GetRootView() tview.Primitive {
	return p.root
}
func (p *summaryPanel) GetChildrenViews() []tview.Primitive {
	return []tview.Primitive{p.root}
}

func (p *summaryPanel) SetFocused(focused bool) {
	ui.SetFlexFocused(p.root, focused)
}

func (p *summaryPanel) DrawBody(data any) {
	switch v := data.(type) {
	case model.WorkloadSummaryModel:
		p.summary = v
	default:
		return
	}
	p.render()
}

func (p *summaryPanel) SetNamespaceFilter(ns string) {
	p.ns = strings.TrimSpace(ns)
	p.render()
}

func (p *summaryPanel) render() {
	if p.text == nil {
		return
	}
	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()
	p.text.SetBackgroundColor(tcell.ColorDefault)
	if !mono {
		p.text.SetBackgroundColor(pal.CanvasBg)
		p.text.SetTextColor(pal.DataSecondary)
	} else {
		p.text.SetTextColor(tcell.ColorDefault)
	}

	lblOpen, valOpen, okOpen, warnOpen, errOpen, resetTag := "", "", "", "", "", ""
	if !mono {
		lblOpen = "[" + ui.TagFg(pal.StatusMuted) + "]"
		valOpen = "[" + ui.TagFg(pal.DataSecondary) + "]"
		okOpen = "[" + ui.TagFg(pal.StatusOK) + "]"
		warnOpen = "[" + ui.TagFg(pal.StatusWarning) + "]"
		errOpen = "[" + ui.TagFg(pal.StatusError) + "]"
		resetTag = "[-]"
	}

	countTag := func(ready, total int) string {
		text := fmt.Sprintf("%d/%d ready", ready, total)
		if mono {
			if total > 0 && ready < total {
				return "[::b]" + text + "[::-]"
			}
			return text
		}
		if total == 0 {
			return lblOpen + "0/0" + resetTag
		}
		if ready < total {
			return warnOpen + text + resetTag
		}
		return okOpen + text + resetTag
	}

	nsLabel := "(all)"
	if p.ns != "" {
		nsLabel = p.ns
	}

	failedJobText := fmt.Sprintf("%d failed", p.summary.JobsFailed)
	if p.summary.JobsFailed > 0 {
		if mono {
			failedJobText = ui.ErrorMarkup(failedJobText)
		} else {
			failedJobText = errOpen + failedJobText + resetTag
		}
	} else if !mono {
		failedJobText = lblOpen + failedJobText + resetTag
	}

	line1 := fmt.Sprintf(
		" %sDeployments:%s %s  │  %sStatefulSets:%s %s  │  %sDaemonSets:%s %s",
		lblOpen, resetTag, countTag(p.summary.DeploymentsReady, p.summary.DeploymentsTotal),
		lblOpen, resetTag, countTag(p.summary.StatefulSetsReady, p.summary.StatefulSetsTotal),
		lblOpen, resetTag, countTag(p.summary.DaemonSetsReady, p.summary.DaemonSetsTotal),
	)
	line2 := fmt.Sprintf(
		" %sJobs:%s %s%d running%s / %s%d completed%s / %s  │  %sNamespace:%s %s%s%s",
		lblOpen, resetTag,
		valOpen, p.summary.JobsRunning, resetTag,
		okOpen, p.summary.JobsCompleted, resetTag,
		failedJobText,
		lblOpen, resetTag, valOpen, nsLabel, resetTag,
	)
	p.text.SetText(line1 + "\n" + line2)
}
