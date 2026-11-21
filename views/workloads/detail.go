package workloads

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/api/resource"
)

// PodSelectedCallback is called when an owned pod row is selected (Enter pressed).
type PodSelectedCallback func(namespace, podName string)

// DetailPanel displays detailed rollout/spec metadata, owned pods, and events for a workload.
type DetailPanel struct {
	root        *tview.Flex
	data        *model.WorkloadDetailData
	laidout     bool
	layoutBuilt bool

	lastHeightCategory int

	focusedChildIdx int
	focusableItems  []tview.Primitive
	focusablePanels []*tview.Flex
	panelNames      []string
	setAppFocus     func(p tview.Primitive)

	infoHeaderPanel     *tview.Flex
	workloadDetailPanel *tview.Flex
	podsPanel           *tview.Flex
	eventsPanel         *tview.Flex
	bottomRow           *tview.Flex

	leftDetailTable     *tview.Table
	middleDetailTable   *tview.Table
	labelsTextView      *tview.TextView
	annotationsTextView *tview.TextView

	podsTable     *tview.Table
	podsTableCtrl *ui.TableController
	filteredPods  []*model.PodModel
	eventsTable   *tview.Table

	onPodSelected         PodSelectedCallback
	onBack                func()
	onFooterContextChange func(focusedPanel string)
	onYank                func(target string)
	onViewYAML            func(detail *model.WorkloadDetailData)
	getTerminalHeight     func() int
}

// NewDetailPanel creates a workload detail subpage panel.
func NewDetailPanel() *DetailPanel {
	p := &DetailPanel{}
	p.Layout(nil)
	return p
}

// SetOnPodSelected registers the callback invoked when Enter is pressed on an owned pod.
func (p *DetailPanel) SetOnPodSelected(cb PodSelectedCallback) {
	p.onPodSelected = cb
}

// SetOnBack registers the back-navigation callback.
func (p *DetailPanel) SetOnBack(cb func()) {
	p.onBack = cb
}

// SetAppFocus sets the callback used to transfer tview focus.
func (p *DetailPanel) SetAppFocus(fn func(p tview.Primitive)) {
	p.setAppFocus = fn
}

// SetOnFooterContextChange registers the focus-change callback for the app footer.
func (p *DetailPanel) SetOnFooterContextChange(cb func(focusedPanel string)) {
	p.onFooterContextChange = cb
}

// SetOnYank registers the callback invoked after copying a pod name to clipboard.
func (p *DetailPanel) SetOnYank(cb func(target string)) {
	p.onYank = cb
}

// SetOnViewYAML registers the callback invoked when Enter is pressed while the Detail panel is active.
func (p *DetailPanel) SetOnViewYAML(cb func(detail *model.WorkloadDetailData)) {
	p.onViewYAML = cb
}

// SetGetTerminalHeightFunc registers a provider for current terminal height.
func (p *DetailPanel) SetGetTerminalHeightFunc(fn func() int) {
	p.getTerminalHeight = fn
}

// GetTitle returns the panel title.
func (p *DetailPanel) GetTitle() string {
	if p.data != nil {
		return fmt.Sprintf("%s > %s", p.data.Kind.PluralTitle(), p.data.Name)
	}
	return "Deployment Detail"
}

// Layout initializes sub-components for the workload detail page.
func (p *DetailPanel) Layout(_ any) {
	if p.laidout {
		return
	}

	p.infoHeaderPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
	p.infoHeaderPanel.SetBorder(true)
	p.infoHeaderPanel.SetTitle(" Info ")
	p.infoHeaderPanel.SetTitleAlign(tview.AlignLeft)
	ui.SetFlexFocused(p.infoHeaderPanel, false)

	p.workloadDetailPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
	p.workloadDetailPanel.SetBorder(true)
	p.workloadDetailPanel.SetTitle(" Deployment Detail ")
	p.workloadDetailPanel.SetTitleAlign(tview.AlignLeft)
	ui.SetFlexFocused(p.workloadDetailPanel, false)

	p.leftDetailTable = tview.NewTable()
	p.leftDetailTable.SetBorder(false)
	p.leftDetailTable.SetBorders(false)
	p.leftDetailTable.SetSelectable(false, false)
	ui.StyleTable(p.leftDetailTable)
	p.leftDetailTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			if p.data != nil && p.onViewYAML != nil {
				p.onViewYAML(p.data)
			}
			return nil
		}
		return p.handleNavKey(event)
	})

	p.middleDetailTable = tview.NewTable()
	p.middleDetailTable.SetBorder(false)
	p.middleDetailTable.SetBorders(false)
	p.middleDetailTable.SetSelectable(false, false)
	ui.StyleTable(p.middleDetailTable)

	p.labelsTextView = tview.NewTextView()
	p.labelsTextView.SetDynamicColors(true)
	p.labelsTextView.SetScrollable(true)
	p.labelsTextView.SetBorder(false)

	p.annotationsTextView = tview.NewTextView()
	p.annotationsTextView.SetDynamicColors(true)
	p.annotationsTextView.SetScrollable(true)
	p.annotationsTextView.SetBorder(false)

	rightCol := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.labelsTextView, 0, 1, false).
		AddItem(p.annotationsTextView, 0, 1, false)

	p.workloadDetailPanel.
		AddItem(p.leftDetailTable, 0, 2, true).
		AddItem(p.middleDetailTable, 0, 2, false).
		AddItem(rightCol, 0, 1, false)

	// Owned Pods panel
	p.podsPanel = tview.NewFlex().SetDirection(tview.FlexRow)
	p.podsPanel.SetBorder(true)
	p.podsPanel.SetTitle(" Owned Pods ")
	p.podsPanel.SetTitleAlign(tview.AlignLeft)
	ui.SetFlexFocused(p.podsPanel, false)

	p.podsTable = tview.NewTable()
	p.podsTable.SetFixed(1, 0)
	p.podsTable.SetSelectable(false, false)
	p.podsTable.SetBorder(false)
	p.podsTable.SetBorders(false)
	ui.StyleTable(p.podsTable)

	podColKeys := map[string]rune{
		"POD":    'p',
		"NODE":   'o',
		"STATUS": 's',
		"READY":  'd',
		"RST":    'r',
		"CPU":    'c',
		"MEM":    'm',
		"AGE":    'a',
	}
	p.podsTableCtrl = ui.NewTableController(p.podsTable, ui.TableControllerConfig{
		DefaultSortColumn: "POD",
		DefaultSortAsc:    true,
		ColumnToKey:       podColKeys,
		OnRedraw:          p.drawPodsTable,
		OnSelect: func(row int) bool {
			if row > 0 && row-1 < len(p.filteredPods) {
				pod := p.filteredPods[row-1]
				if p.onPodSelected != nil {
					p.onPodSelected(pod.Namespace, pod.Name)
					return true
				}
			}
			return false
		},
		OnYank: func(row int) bool {
			if row > 0 && row-1 < len(p.filteredPods) {
				pod := p.filteredPods[row-1]
				target := pod.Namespace + "/" + pod.Name
				_ = ui.CopyToClipboard(target)
				if p.onYank != nil {
					p.onYank(target)
				}
				return true
			}
			return false
		},
	})
	p.podsTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if p.podsTableCtrl != nil {
			if rem := p.podsTableCtrl.HandleInput(event); rem == nil {
				return nil
			}
		}
		return p.handleNavKey(event)
	})
	p.podsPanel.AddItem(p.podsTable, 0, 1, true)

	// Workload Events panel
	p.eventsPanel = tview.NewFlex().SetDirection(tview.FlexRow)
	p.eventsPanel.SetBorder(true)
	p.eventsPanel.SetTitle(" Events ")
	p.eventsPanel.SetTitleAlign(tview.AlignLeft)
	ui.SetFlexFocused(p.eventsPanel, false)

	p.eventsTable = tview.NewTable()
	p.eventsTable.SetFixed(1, 0)
	p.eventsTable.SetSelectable(false, false)
	p.eventsTable.SetBorder(false)
	p.eventsTable.SetBorders(false)
	ui.StyleTable(p.eventsTable)
	p.eventsTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return p.handleNavKey(event)
	})
	p.eventsPanel.AddItem(p.eventsTable, 0, 1, true)

	p.bottomRow = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(p.podsPanel, 0, 1, false).
		AddItem(p.eventsPanel, 0, 1, false)

	p.root = tview.NewFlex().SetDirection(tview.FlexRow)
	p.root.SetBorder(true)
	p.root.SetTitleAlign(tview.AlignCenter)

	p.root.AddItem(p.infoHeaderPanel, 3, 0, false)
	p.root.AddItem(p.workloadDetailPanel, 13, 0, true)
	p.root.AddItem(p.bottomRow, 0, 1, false)

	p.focusableItems = []tview.Primitive{p.leftDetailTable, p.podsTable, p.eventsTable}
	p.focusablePanels = []*tview.Flex{p.workloadDetailPanel, p.podsPanel, p.eventsPanel}
	p.panelNames = []string{"detail", "pods", "events"}
	p.focusedChildIdx = 0

	p.root.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if p.podsTableCtrl != nil && p.podsTableCtrl.IsFilterEditing() {
			return event
		}
		return p.handleNavKey(event)
	})

	p.laidout = true
}

func (p *DetailPanel) handleNavKey(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyTab:
		p.cycleFocus(false)
		return nil
	case tcell.KeyBacktab:
		p.cycleFocus(true)
		return nil
	case tcell.KeyEscape:
		if p.HandleEscape() {
			return nil
		}
	}
	return event
}

func (p *DetailPanel) DrawHeader(_ any) {}
func (p *DetailPanel) DrawFooter(_ any) {}

func (p *DetailPanel) Clear() {
	if p.podsTable != nil {
		p.podsTable.Clear()
		ui.StyleTable(p.podsTable)
	}
	if p.eventsTable != nil {
		p.eventsTable.Clear()
		ui.StyleTable(p.eventsTable)
	}
}

func (p *DetailPanel) GetRootView() tview.Primitive        { return p.root }
func (p *DetailPanel) GetChildrenViews() []tview.Primitive { return nil }

// DrawBody renders WorkloadDetailData into all detail sub-panels.
func (p *DetailPanel) DrawBody(data any) {
	detail, ok := data.(*model.WorkloadDetailData)
	if !ok || detail == nil {
		return
	}
	p.data = detail

	if !p.layoutBuilt {
		p.buildLayout()
	}
	p.checkAndRebuildLayout()

	p.root.SetTitle(fmt.Sprintf(" %s %s > [::b]%s[::] ", kindIcon(detail.Kind), detail.Kind.PluralTitle(), detail.Name))
	p.workloadDetailPanel.SetTitle(fmt.Sprintf(" %s Detail ", detail.Kind))

	p.drawInfoHeader()
	p.drawMetadataSection()
	p.drawPodsTable()
	p.drawEventsTable()
}

// InitFocus activates focus on the <Kind> Detail panel when first opened.
func (p *DetailPanel) InitFocus() {
	p.focusedChildIdx = 0
	p.RestoreFocus()
}

// RestoreFocus re-applies visual focus and tview focus to the currently active sub-panel.
func (p *DetailPanel) RestoreFocus() {
	p.updateFocusVisuals()
	if p.onFooterContextChange != nil {
		p.onFooterContextChange(p.GetFocusedPanelName())
	}
}

// SetFocused implements ui.FocusablePanel.
func (p *DetailPanel) SetFocused(focused bool) {
	ui.SetFlexFocused(p.root, focused)
	if focused {
		p.updateFocusVisuals()
	}
}

// GetFocusedPanelName returns "detail", "pods", or "events".
func (p *DetailPanel) GetFocusedPanelName() string {
	if p.focusedChildIdx >= 0 && p.focusedChildIdx < len(p.panelNames) {
		return p.panelNames[p.focusedChildIdx]
	}
	return "detail"
}

// IsFilterEditing reports whether the owned-pods table filter bar is editing.
func (p *DetailPanel) IsFilterEditing() bool {
	return p.podsTableCtrl != nil && p.podsTableCtrl.IsFilterEditing()
}

// HasEscapableState implements ui.EscapablePanel.
func (p *DetailPanel) HasEscapableState() bool {
	return true
}

// HandleEscape implements ui.EscapablePanel.
func (p *DetailPanel) HandleEscape() bool {
	if p.podsTableCtrl != nil && p.podsTableCtrl.HandleEscape() {
		return true
	}
	if p.onBack != nil {
		p.onBack()
		return true
	}
	return false
}

func (p *DetailPanel) cycleFocus(reverse bool) {
	if len(p.focusableItems) == 0 {
		return
	}
	p.focusedChildIdx = ui.CycleFocus(p.focusedChildIdx, len(p.focusableItems), reverse)
	p.RestoreFocus()
}

func (p *DetailPanel) updateFocusVisuals() {
	for i, panel := range p.focusablePanels {
		ui.SetFlexFocused(panel, i == p.focusedChildIdx)
	}
	p.podsTable.SetSelectable(p.focusedChildIdx == 1, false)
	if p.focusedChildIdx == 1 {
		p.podsTable.SetSelectedStyle(ui.SelectedRowStyle())
		if p.podsTable.GetRowCount() > 1 {
			r, c := p.podsTable.GetSelection()
			if r < 1 {
				p.podsTable.Select(1, c)
			}
		}
	}
	p.eventsTable.SetSelectable(p.focusedChildIdx == 2, false)
	if p.focusedChildIdx == 2 {
		p.eventsTable.SetSelectedStyle(ui.SelectedRowStyle())
		if p.eventsTable.GetRowCount() > 1 {
			r, c := p.eventsTable.GetSelection()
			if r < 1 {
				p.eventsTable.Select(1, c)
			}
		}
	}

	if p.setAppFocus != nil && p.focusedChildIdx >= 0 && p.focusedChildIdx < len(p.focusableItems) {
		p.setAppFocus(p.focusableItems[p.focusedChildIdx])
	}
}

func (p *DetailPanel) drawInfoHeader() {
	p.infoHeaderPanel.Clear()
	if p.data == nil {
		return
	}

	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()
	lblOpen, valOpen, okOpen, warnOpen, errOpen, mutedOpen, resetTag := "", "", "", "", "", "", ""
	if !mono {
		lblOpen = "[" + ui.TagFg(pal.DataPrimary) + "]"
		valOpen = "[" + ui.TagFg(pal.DataSecondary) + "]"
		okOpen = "[" + ui.TagFg(pal.StatusOK) + "]"
		warnOpen = "[" + ui.TagFg(pal.StatusWarning) + "]"
		errOpen = "[" + ui.TagFg(pal.StatusError) + "]"
		mutedOpen = "[" + ui.TagFg(pal.StatusMuted) + "]"
		resetTag = "[-]"
	}

	statusVal := p.data.Status
	if mono {
		if ui.GetStatusSeverity(p.data.Status, "workload") == ui.StatusSeverityError {
			statusVal = ui.ErrorMarkup(p.data.Status)
		}
	} else {
		statusColor := ui.GetStatusColor(p.data.Status, "workload")
		statusVal = fmt.Sprintf("[%s]%s%s", statusColor, p.data.Status, resetTag)
	}

	field := func(label, val string) string {
		return fmt.Sprintf("%s%s:%s %s", lblOpen, label, resetTag, val)
	}
	numVal := func(n int32, colorOpen string) string {
		if mono {
			return fmt.Sprintf("%d", n)
		}
		return fmt.Sprintf("%s%d%s", colorOpen, n, resetTag)
	}

	readyColor := okOpen
	if p.data.Desired == 0 {
		readyColor = mutedOpen
	} else if p.data.Ready < p.data.Desired {
		readyColor = warnOpen
	}

	items := []string{field("Status", statusVal)}

	switch p.data.Kind {
	case model.WorkloadKindDeployment:
		items = append(items,
			field("Desired", numVal(p.data.Desired, valOpen)),
			field("Ready", numVal(p.data.Ready, readyColor)),
			field("Updated", numVal(p.data.Updated, valOpen)),
			field("Available", numVal(p.data.Available, valOpen)),
		)
		if p.data.Unavailable > 0 {
			unavailText := numVal(p.data.Unavailable, errOpen)
			if mono {
				unavailText = ui.ErrorMarkup(fmt.Sprintf("%d", p.data.Unavailable))
			}
			items = append(items, field("Unavailable", unavailText))
		}
		if p.data.Paused {
			if mono {
				items = append(items, "[::b]PAUSED[::-]")
			} else {
				items = append(items, warnOpen+"PAUSED"+resetTag)
			}
		}
	case model.WorkloadKindStatefulSet:
		items = append(items,
			field("Desired", numVal(p.data.Desired, valOpen)),
			field("Ready", numVal(p.data.Ready, readyColor)),
			field("Current", numVal(p.data.Current, valOpen)),
			field("Updated", numVal(p.data.Updated, valOpen)),
		)
	case model.WorkloadKindDaemonSet:
		items = append(items,
			field("Desired", numVal(p.data.Desired, valOpen)),
			field("Ready", numVal(p.data.Ready, readyColor)),
			field("Current", numVal(p.data.Current, valOpen)),
			field("Updated", numVal(p.data.Updated, valOpen)),
		)
		if p.data.Misscheduled > 0 {
			items = append(items, field("Misscheduled", numVal(p.data.Misscheduled, warnOpen)))
		}
	case model.WorkloadKindJob:
		comp := p.data.CompletionsText
		if comp == "" {
			comp = p.data.ReadyText
		}
		items = append(items,
			field("Completions", valOpen+comp+resetTag),
			field("Active", numVal(p.data.Active, valOpen)),
			field("Succeeded", numVal(p.data.Succeeded, okOpen)),
		)
		failedColor := mutedOpen
		if p.data.Failed > 0 {
			failedColor = errOpen
		}
		items = append(items, field("Failed", numVal(p.data.Failed, failedColor)))
		if p.data.Paused {
			items = append(items, warnOpen+"SUSPENDED"+resetTag)
		}
	default:
		if p.data.ReadyText != "" {
			items = append(items, field("Replicas", valOpen+p.data.ReadyText+resetTag))
		}
	}

	if p.data.Age != "" {
		items = append(items, field("Age", valOpen+p.data.Age+resetTag))
	}

	sep := " │ "
	if !mono {
		sep = " " + lblOpen + "│" + resetTag + " "
	}

	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetWrap(false)
	if mono {
		tv.SetBackgroundColor(tcell.ColorDefault)
		tv.SetTextColor(tcell.ColorDefault)
	} else {
		tv.SetBackgroundColor(pal.CanvasBg)
		tv.SetTextColor(pal.DataSecondary)
	}
	tv.SetText(" " + strings.Join(items, sep))
	p.infoHeaderPanel.AddItem(tv, 0, 1, false)
}

func (p *DetailPanel) drawMetadataSection() {
	p.leftDetailTable.Clear()
	p.middleDetailTable.Clear()
	ui.StyleTable(p.leftDetailTable)
	ui.StyleTable(p.middleDetailTable)
	p.labelsTextView.Clear()
	p.annotationsTextView.Clear()

	if p.data == nil {
		return
	}

	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()
	sectionColor := pal.Accent
	mutedColor := pal.StatusMuted
	okColor := pal.StatusOK
	warnColor := pal.StatusWarning
	errColor := pal.StatusError
	valColor := pal.DataSecondary
	sectionOpen := "[::b]"
	keyOpen, closeTag := "", ""
	if mono {
		sectionColor = tcell.ColorDefault
		mutedColor = tcell.ColorDefault
		okColor = tcell.ColorDefault
		warnColor = tcell.ColorDefault
		errColor = tcell.ColorDefault
		valColor = tcell.ColorDefault
		p.labelsTextView.SetBackgroundColor(tcell.ColorDefault)
		p.labelsTextView.SetTextColor(tcell.ColorDefault)
		p.annotationsTextView.SetBackgroundColor(tcell.ColorDefault)
		p.annotationsTextView.SetTextColor(tcell.ColorDefault)
	} else {
		sectionOpen = "[" + ui.TagFg(pal.Accent) + "::b]"
		keyOpen = "[" + ui.TagFg(pal.DataPrimary) + "]"
		closeTag = "[-]"
		p.labelsTextView.SetBackgroundColor(pal.CanvasBg)
		p.labelsTextView.SetTextColor(pal.DataSecondary)
		p.annotationsTextView.SetBackgroundColor(pal.CanvasBg)
		p.annotationsTextView.SetTextColor(pal.DataSecondary)
	}

	addRow := func(tbl *tview.Table, row int, label, value string, color tcell.Color) {
		lblColor := pal.DataPrimary
		if mono {
			lblColor = tcell.ColorDefault
		}
		tbl.SetCell(row, 0, ui.NewDataCell(fmt.Sprintf("%-12s", label)).SetTextColor(lblColor).SetSelectable(false))
		tbl.SetCell(row, 1, ui.NewDataCell(value).SetTextColor(color).SetSelectable(false))
	}

	// Column 1: Spec, Rollout & Resource Totals
	row := 0
	p.leftDetailTable.SetCell(row, 0, ui.NewDataCell("[::b]Spec & Rollout[::-]").SetTextColor(sectionColor).SetSelectable(false))
	row++
	addRow(p.leftDetailTable, row, "Namespace", p.data.Namespace, valColor)
	row++

	strategy := p.data.StrategyDetail
	if strategy == "" {
		strategy = p.data.Strategy
	}
	if strategy != "" {
		addRow(p.leftDetailTable, row, "Strategy", ui.Truncate(strategy, 42), valColor)
		row++
	} else if p.data.Owner != "" {
		addRow(p.leftDetailTable, row, "Owner", p.data.Owner, valColor)
		row++
	}

	if p.data.Revision != "" || p.data.ActiveReplicaSet != "" {
		revText := p.data.ActiveReplicaSet
		if p.data.Revision != "" && p.data.ActiveReplicaSet != "" {
			revText = fmt.Sprintf("#%s (%s)", p.data.Revision, p.data.ActiveReplicaSet)
		} else if p.data.Revision != "" {
			revText = "#" + p.data.Revision
		}
		addRow(p.leftDetailTable, row, "Revision", ui.Truncate(revText, 42), valColor)
		row++
	}
	if p.data.ServiceName != "" {
		addRow(p.leftDetailTable, row, "Service", p.data.ServiceName, valColor)
		row++
	}
	if p.data.PodManagement != "" {
		addRow(p.leftDetailTable, row, "PodMgmt", p.data.PodManagement, valColor)
		row++
	}
	if len(p.data.VolumeClaims) > 0 {
		addRow(p.leftDetailTable, row, "Volumes", ui.Truncate(strings.Join(p.data.VolumeClaims, ", "), 42), valColor)
		row++
	}
	if p.data.Parallelism != nil || p.data.BackoffLimit != nil {
		var parts []string
		if p.data.Parallelism != nil {
			parts = append(parts, fmt.Sprintf("parallel=%d", *p.data.Parallelism))
		}
		if p.data.BackoffLimit != nil {
			parts = append(parts, fmt.Sprintf("backoff=%d", *p.data.BackoffLimit))
		}
		addRow(p.leftDetailTable, row, "Policy", strings.Join(parts, ", "), valColor)
		row++
	}
	if p.data.JobDuration != "" {
		addRow(p.leftDetailTable, row, "Duration", p.data.JobDuration, valColor)
		row++
	}
	if len(p.data.Images) > 0 {
		addRow(p.leftDetailTable, row, "Images", ui.Truncate(strings.Join(p.data.Images, ", "), 42), valColor)
		row++
	}

	cpuSummary := formatResourceTriple(p.data.TotalUsageCPU, p.data.TotalRequestedCPU, p.data.TotalLimitCPU, true)
	addRow(p.leftDetailTable, row, "CPU", cpuSummary, valColor)
	row++
	memSummary := formatResourceTriple(p.data.TotalUsageMem, p.data.TotalRequestedMem, p.data.TotalLimitMem, false)
	addRow(p.leftDetailTable, row, "MEM", memSummary, valColor)
	row++

	if len(p.data.Selector) > 0 {
		addRow(p.leftDetailTable, row, "Selector", ui.Truncate(formatSortedPairs(p.data.Selector), 42), valColor)
		row++
	}
	if len(p.data.NodeSelector) > 0 {
		addRow(p.leftDetailTable, row, "NodeSel", ui.Truncate(formatSortedPairs(p.data.NodeSelector), 42), valColor)
		row++
	}
	if p.data.ServiceAccount != "" {
		addRow(p.leftDetailTable, row, "SvcAccount", p.data.ServiceAccount, valColor)
		row++
	}

	// Column 2: Rollout State & Conditions
	row = 0
	p.middleDetailTable.SetCell(row, 0, ui.NewDataCell("[::b]Conditions & Status[::-]").SetTextColor(sectionColor).SetSelectable(false))
	row++
	if p.data.Generation > 0 {
		genColor := valColor
		if p.data.ObservedGeneration > 0 && p.data.ObservedGeneration != p.data.Generation {
			genColor = warnColor
		}
		addRow(p.middleDetailTable, row, "Generation", fmt.Sprintf("%d (observed %d)", p.data.Generation, p.data.ObservedGeneration), genColor)
		row++
	}
	if p.data.RevHistoryLimit != nil {
		addRow(p.middleDetailTable, row, "RevHistory", fmt.Sprintf("%d", *p.data.RevHistoryLimit), valColor)
		row++
	}
	if p.data.Efficiency.HasEfficiency {
		effText := fmt.Sprintf("%.0f%%", p.data.Efficiency.EffPct)
		if p.data.Efficiency.HasCPUEfficiency && p.data.Efficiency.HasMemEfficiency {
			effText = fmt.Sprintf("CPU %.0f%% / MEM %.0f%%", p.data.Efficiency.CPUEffPct, p.data.Efficiency.MemEffPct)
		}
		addRow(p.middleDetailTable, row, "Req Usage", effText, valColor)
		row++
	}

	if len(p.data.Conditions) == 0 {
		p.middleDetailTable.SetCell(row, 0, ui.NewDataCell("No conditions").SetTextColor(mutedColor).SetSelectable(false))
	} else {
		for _, cond := range p.data.Conditions {
			condColor := okColor
			statusText := cond.Status
			attrs := tcell.AttrNone
			if !cond.Healthy {
				condColor = errColor
				if mono {
					statusText = "! " + statusText
					attrs = tcell.AttrBold | tcell.AttrUnderline
				}
			}
			lblColor := pal.DataPrimary
			if mono {
				lblColor = tcell.ColorDefault
			}
			p.middleDetailTable.SetCell(row, 0, ui.NewDataCell(fmt.Sprintf("%-16s", cond.Type)).SetTextColor(lblColor).SetSelectable(false))
			detailVal := statusText
			if cond.Reason != "" {
				detailVal = fmt.Sprintf("%s (%s)", statusText, cond.Reason)
			}
			p.middleDetailTable.SetCell(row, 1, ui.NewDataCell(ui.Truncate(detailVal, 36)).SetTextColor(condColor).SetAttributes(attrs).SetSelectable(false))
			row++
			if !cond.Healthy && cond.Message != "" {
				p.middleDetailTable.SetCell(row, 0, ui.NewDataCell("").SetSelectable(false))
				p.middleDetailTable.SetCell(row, 1, ui.NewDataCell(ui.Truncate(cond.Message, 40)).SetTextColor(mutedColor).SetSelectable(false))
				row++
			}
		}
	}

	// Column 3: Labels & Annotations
	p.labelsTextView.SetText(formatKeyValueSection(sectionOpen+"Labels[::-]", p.data.Labels, keyOpen, closeTag))
	p.annotationsTextView.SetText(formatKeyValueSection(sectionOpen+"Annotations[::-]", p.data.Annotations, keyOpen, closeTag))
}

func formatResourceTriple(used, req, lim *resource.Quantity, isCPU bool) string {
	fmtQty := func(q *resource.Quantity) string {
		if isCPU {
			return ui.FormatCPU(q)
		}
		return strings.TrimSpace(ui.FormatMemory(q))
	}

	var parts []string
	if used != nil {
		parts = append(parts, fmtQty(used)+" used")
	}
	if req != nil && ((isCPU && req.MilliValue() > 0) || (!isCPU && req.Value() > 0)) {
		parts = append(parts, fmtQty(req)+" req")
	}
	if lim != nil && ((isCPU && lim.MilliValue() > 0) || (!isCPU && lim.Value() > 0)) {
		parts = append(parts, fmtQty(lim)+" lim")
	}
	if len(parts) == 0 {
		return "n/a"
	}
	return strings.Join(parts, " / ")
}

func formatSortedPairs(m map[string]string) string {
	pairs := make([]string, 0, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		pairs = append(pairs, k+"="+m[k])
	}
	return strings.Join(pairs, ",")
}

func formatKeyValueSection(heading string, m map[string]string, keyOpen, closeTag string) string {
	var b strings.Builder
	b.WriteString(heading + "\n")
	if len(m) == 0 {
		b.WriteString("None")
		return b.String()
	}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		b.WriteString(fmt.Sprintf("%s%s:%s %s\n", keyOpen, ui.TruncateKey(k), closeTag, ui.Truncate(m[k], 28)))
	}
	return b.String()
}

func (p *DetailPanel) drawPodsTable() {
	if p.data == nil {
		return
	}
	selRow, selCol := p.podsTable.GetSelection()
	p.podsTable.Clear()
	ui.StyleTable(p.podsTable)

	cols := []string{"POD", "NODE", "STATUS", "READY", "RST", "CPU", "MEM", "AGE"}
	p.podsTableCtrl.SetVisibleColumns(cols)
	for i, col := range cols {
		exp := 0
		if col == "POD" {
			exp = 1
		}
		p.podsTable.SetCell(0, i,
			ui.StyleHeaderCell(tview.NewTableCell(p.podsTableCtrl.FormatColumnHeader(col)).
				SetAlign(tview.AlignLeft).
				SetExpansion(exp).
				SetSelectable(false)),
		)
	}
	p.podsTable.SetFixed(1, 0)

	filter := p.podsTableCtrl.Filter()
	filter.TotalRows = len(p.data.OwnedPods)

	filtered := make([]*model.PodModel, 0, len(p.data.OwnedPods))
	if filter.IsFiltering() && filter.Text != "" {
		for _, pod := range p.data.OwnedPods {
			if pod == nil {
				continue
			}
			if filter.MatchesRow([]string{pod.Name, pod.Node, pod.Status, pod.IP}) {
				filtered = append(filtered, pod)
			}
		}
	} else {
		filtered = append(filtered, p.data.OwnedPods...)
	}
	filter.MatchRows = len(filtered)
	sortCol, sortAsc := p.podsTableCtrl.GetSortParams()
	model.SortPodModelPointersBy(filtered, sortCol, sortAsc)
	p.filteredPods = filtered

	p.podsPanel.SetTitle(p.podsTableCtrl.FormatTitleWithScroll("Owned Pods", ui.Icons.Package, len(filtered), ""))

	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()
	valColor := pal.DataSecondary
	okColor := pal.StatusOK
	warnColor := pal.StatusWarning
	errColor := pal.StatusError
	if mono {
		valColor = tcell.ColorDefault
		okColor = tcell.ColorDefault
		warnColor = tcell.ColorDefault
		errColor = tcell.ColorDefault
	}

	for i, pod := range filtered {
		row := i + 1
		rowColor := ui.GetRowColorForStatus(pod.Status, "pod")
		ready := fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers)
		rstColor := okColor
		if pod.Restarts > 5 {
			rstColor = errColor
		} else if pod.Restarts > 0 {
			rstColor = warnColor
		}

		metricColor := valColor
		if !mono && ui.GetStatusSeverity(pod.Status, "pod") != ui.StatusSeverityOK {
			metricColor = rowColor
		}

		cpuText := "n/a"
		cpuColor := mutedCellColor()
		if pod.PodUsageCpuQty != nil {
			cpuText = ui.FormatCPU(pod.PodUsageCpuQty)
			cpuColor = metricColor
		}
		memText := "n/a"
		memColor := mutedCellColor()
		if pod.PodUsageMemQty != nil {
			memText = strings.TrimSpace(ui.FormatMemory(pod.PodUsageMemQty))
			memColor = metricColor
		}

		p.podsTable.SetCell(row, 0, ui.NewDataCell(pod.Name).SetTextColor(rowColor).SetMaxWidth(32))
		p.podsTable.SetCell(row, 1, ui.NewDataCell(pod.Node).SetTextColor(rowColor).SetMaxWidth(18))
		p.podsTable.SetCell(row, 2, ui.StyleStatusCell(ui.NewDataCell(pod.Status), pod.Status, "pod"))
		p.podsTable.SetCell(row, 3, ui.NewDataCell(ready).SetTextColor(rowColor))
		p.podsTable.SetCell(row, 4, ui.NewDataCell(fmt.Sprintf("%d", pod.Restarts)).SetTextColor(rstColor))
		p.podsTable.SetCell(row, 5, ui.NewDataCell(cpuText).SetTextColor(cpuColor))
		p.podsTable.SetCell(row, 6, ui.NewDataCell(memText).SetTextColor(memColor))
		p.podsTable.SetCell(row, 7, ui.NewDataCell(pod.TimeSince).SetTextColor(rowColor))
	}

	if len(filtered) > 0 && p.focusedChildIdx == 1 {
		if selRow < 1 {
			selRow = 1
		} else if selRow > len(filtered) {
			selRow = len(filtered)
		}
		p.podsTable.Select(selRow, selCol)
	}
}

func (p *DetailPanel) drawEventsTable() {
	if p.data == nil {
		return
	}
	p.eventsTable.Clear()
	ui.StyleTable(p.eventsTable)

	headers := []string{"AGE", "TYPE", "REASON", "MESSAGE"}
	for i, h := range headers {
		exp := 0
		if h == "MESSAGE" {
			exp = 100
		}
		p.eventsTable.SetCell(0, i,
			ui.StyleHeaderCell(tview.NewTableCell(h).
				SetAlign(tview.AlignLeft).
				SetExpansion(exp).
				SetSelectable(false)),
		)
	}
	p.eventsTable.SetFixed(1, 0)
	p.eventsPanel.SetTitle(fmt.Sprintf(" Events (%d) ", len(p.data.Events)))

	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()
	normalColor := pal.DataSecondary
	warnColor := pal.StatusWarning
	mutedColor := pal.StatusMuted
	if mono {
		normalColor = tcell.ColorDefault
		warnColor = tcell.ColorDefault
		mutedColor = tcell.ColorDefault
	}

	if len(p.data.Events) == 0 {
		p.eventsTable.SetCell(1, 0, ui.NewDataCell("-").SetTextColor(mutedColor).SetSelectable(false))
		p.eventsTable.SetCell(1, 1, ui.NewDataCell("-").SetTextColor(mutedColor).SetSelectable(false))
		p.eventsTable.SetCell(1, 2, ui.NewDataCell("No recent events").SetTextColor(mutedColor).SetSelectable(false))
		p.eventsTable.SetCell(1, 3, ui.NewDataCell("").SetTextColor(mutedColor).SetSelectable(false))
		return
	}

	for i, evt := range p.data.Events {
		row := i + 1
		typeColor := normalColor
		typeText := evt.Type
		attrs := tcell.AttrNone
		if evt.Type == "Warning" {
			typeColor = warnColor
			if mono {
				typeText = "! " + typeText
				attrs = tcell.AttrBold
			}
		}

		p.eventsTable.SetCell(row, 0, ui.NewDataCell(ui.FormatEventAge(evt)).SetTextColor(mutedColor))
		p.eventsTable.SetCell(row, 1, ui.NewDataCell(typeText).SetTextColor(typeColor).SetAttributes(attrs))
		p.eventsTable.SetCell(row, 2, ui.NewDataCell(evt.Reason).SetTextColor(typeColor))
		p.eventsTable.SetCell(row, 3, ui.NewDataCell(evt.Message).SetTextColor(normalColor).SetExpansion(100))
	}
}

func kindIcon(kind model.WorkloadKind) string {
	switch kind {
	case model.WorkloadKindDeployment:
		return ui.Icons.Rocket
	case model.WorkloadKindStatefulSet:
		return ui.Icons.Package
	case model.WorkloadKindDaemonSet:
		return ui.Icons.Controller
	case model.WorkloadKindJob:
		return ui.Icons.Clock
	default:
		return ui.Icons.Rocket
	}
}

func (p *DetailPanel) detailSectionHeight(terminalHeight int) int {
	if terminalHeight <= 28 {
		return 9
	}
	if ui.GetHeightCategory(terminalHeight) == ui.HeightCategorySmall {
		return 11
	}
	return 13
}

func (p *DetailPanel) renderedHeight() (int, bool) {
	_, _, w, height := p.root.GetRect()
	// tview.NewBox initializes every unrendered box to 0,0,15,10.
	if height <= 0 || (w == 15 && height == 10) {
		return 0, false
	}
	return height, true
}

func (p *DetailPanel) buildLayout() {
	height, ok := p.renderedHeight()
	if !ok {
		return
	}
	p.applyLayout(height)
	p.layoutBuilt = true
}

func (p *DetailPanel) checkAndRebuildLayout() {
	if !p.layoutBuilt {
		return
	}
	height, ok := p.renderedHeight()
	if !ok {
		return
	}
	if ui.GetHeightCategory(height) == p.lastHeightCategory {
		return
	}
	p.applyLayout(height)
}

func (p *DetailPanel) applyLayout(height int) {
	p.root.Clear()
	p.root.AddItem(p.infoHeaderPanel, 3, 0, false)
	p.root.AddItem(p.workloadDetailPanel, p.detailSectionHeight(height), 0, true)
	p.root.AddItem(p.bottomRow, 0, 1, false)
	p.lastHeightCategory = ui.GetHeightCategory(height)
}
