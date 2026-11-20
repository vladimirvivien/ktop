package workloads

import (
	"fmt"

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

	lblOpen, valOpen, mutedOpen, okOpen, infoOpen, warnOpen, errOpen, resetTag := "", "", "", "", "", "", "", ""
	if !mono {
		lblOpen = "[" + ui.TagFg(pal.DataPrimary) + "]"
		valOpen = "[" + ui.TagFg(pal.DataSecondary) + "]"
		mutedOpen = "[" + ui.TagFg(pal.StatusMuted) + "]"
		okOpen = "[" + ui.TagFg(pal.StatusOK) + "]"
		infoOpen = "[" + ui.TagFg(pal.StatusInfo) + "]"
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
			return mutedOpen + "0/0" + resetTag
		}
		if ready < total {
			return warnOpen + text + resetTag
		}
		return okOpen + text + resetTag
	}

	runningJobText := fmt.Sprintf("%d running", p.summary.JobsRunning)
	if !mono {
		if p.summary.JobsRunning > 0 {
			runningJobText = infoOpen + runningJobText + resetTag
		} else {
			runningJobText = mutedOpen + runningJobText + resetTag
		}
	}

	completedJobText := fmt.Sprintf("%d completed", p.summary.JobsCompleted)
	if !mono {
		if p.summary.JobsCompleted > 0 {
			completedJobText = okOpen + completedJobText + resetTag
		} else {
			completedJobText = mutedOpen + completedJobText + resetTag
		}
	}

	failedJobText := fmt.Sprintf("%d failed", p.summary.JobsFailed)
	if p.summary.JobsFailed > 0 {
		if mono {
			failedJobText = ui.ErrorMarkup(failedJobText)
		} else {
			failedJobText = errOpen + failedJobText + resetTag
		}
	} else if !mono {
		failedJobText = mutedOpen + failedJobText + resetTag
	}

	sep := " │ "
	slash := " / "
	if !mono {
		sep = " " + lblOpen + "│" + resetTag + " "
		slash = " " + valOpen + "/" + resetTag + " "
	}

	line := fmt.Sprintf(
		" %sDeployments:%s %s%s%sStatefulSets:%s %s%s%sDaemonSets:%s %s%s%sJobs:%s %s%s%s%s%s",
		lblOpen, resetTag, countTag(p.summary.DeploymentsReady, p.summary.DeploymentsTotal), sep,
		lblOpen, resetTag, countTag(p.summary.StatefulSetsReady, p.summary.StatefulSetsTotal), sep,
		lblOpen, resetTag, countTag(p.summary.DaemonSetsReady, p.summary.DaemonSetsTotal), sep,
		lblOpen, resetTag,
		runningJobText, slash,
		completedJobText, slash,
		failedJobText,
	)
	p.text.SetText(line)
}
