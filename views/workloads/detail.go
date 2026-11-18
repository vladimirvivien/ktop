package workloads

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

// PodSelectedCallback is called when an owned pod row is selected (Enter pressed).
type PodSelectedCallback func(namespace, podName string)

// DetailPanel displays detailed status, efficiency, events, and owned pods for a workload.
type DetailPanel struct {
	root        *tview.Flex
	data        *model.WorkloadDetailData
	laidout     bool
	layoutBuilt bool

	currentWorkloadKey   string
	lastHeightCategory   int
	lastSparklineHeight  int
	lastInfoHeaderHeight int

	focusedChildIdx int
	focusableItems  []tview.Primitive
	focusablePanels []*tview.Flex
	panelNames      []string
	setAppFocus     func(p tview.Primitive)

	infoHeaderPanel     *tview.Flex
	sparklinePanel      *tview.Flex
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

	sparklineRow *ui.SparklineRow

	onPodSelected         PodSelectedCallback
	onBack                func()
	onFooterContextChange func(focusedPanel string)
	onYank                func(target string)
	getTerminalHeight     func() int
}

// NewDetailPanel creates a Workload Detail panel.
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

// SetGetTerminalHeightFunc registers a provider for current terminal height.
func (p *DetailPanel) SetGetTerminalHeightFunc(fn func() int) {
	p.getTerminalHeight = fn
}

// GetTitle returns the panel title.
func (p *DetailPanel) GetTitle() string {
	if p.data != nil {
		return fmt.Sprintf("%s: %s/%s", p.data.Kind, p.data.Namespace, p.data.Name)
	}
	return "Workload Detail"
}

// Layout initializes sub-components for the workload detail page.
func (p *DetailPanel) Layout(_ any) {
	if p.laidout {
		return
	}

	p.sparklineRow = ui.NewSparklineRow(false)

	p.infoHeaderPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
	p.infoHeaderPanel.SetBorder(true)
	p.infoHeaderPanel.SetTitle(" Info ")
	p.infoHeaderPanel.SetTitleAlign(tview.AlignLeft)
	ui.SetFlexFocused(p.infoHeaderPanel, false)

	p.sparklinePanel = tview.NewFlex().SetDirection(tview.FlexColumn)
	p.sparklinePanel.AddItem(p.sparklineRow, 0, 1, false)

	p.workloadDetailPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
	p.workloadDetailPanel.SetBorder(true)
	p.workloadDetailPanel.SetTitle(" Workload Detail ")
	p.workloadDetailPanel.SetTitleAlign(tview.AlignLeft)
	ui.SetFlexFocused(p.workloadDetailPanel, false)

	p.leftDetailTable = tview.NewTable()
	p.leftDetailTable.SetBorder(false)
	p.leftDetailTable.SetBorders(false)
	p.leftDetailTable.SetSelectable(false, false)
	ui.StyleTable(p.leftDetailTable)

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
		AddItem(p.leftDetailTable, 0, 1, false).
		AddItem(p.middleDetailTable, 0, 1, false).
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
		"NAMESPACE": 'n',
		"POD":       'p',
		"STATUS":    's',
		"READY":     'd',
		"RST":       'r',
		"CPU":       'c',
		"MEM":       'm',
		"AGE":       'a',
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
		return p.podsTableCtrl.HandleInput(event)
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
	p.eventsPanel.AddItem(p.eventsTable, 0, 1, true)

	p.bottomRow = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(p.podsPanel, 0, 1, true).
		AddItem(p.eventsPanel, 0, 1, false)

	p.root = tview.NewFlex().SetDirection(tview.FlexRow)
	p.root.SetBorder(true)
	p.root.SetTitleAlign(tview.AlignCenter)

	p.root.AddItem(p.infoHeaderPanel, 3, 0, false)
	p.root.AddItem(p.sparklinePanel, 4, 0, false)
	p.root.AddItem(p.workloadDetailPanel, 12, 0, false)
	p.root.AddItem(p.bottomRow, 0, 1, true)

	p.focusableItems = []tview.Primitive{p.podsTable, p.eventsTable}
	p.focusablePanels = []*tview.Flex{p.podsPanel, p.eventsPanel}
	p.panelNames = []string{"pods", "events"}
	p.focusedChildIdx = 0

	p.root.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if p.podsTableCtrl != nil && p.podsTableCtrl.IsFilterEditing() {
			return event
		}
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
	})

	p.laidout = true
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

	newKey := fmt.Sprintf("%s/%s/%s", detail.Kind, detail.Namespace, detail.Name)
	if newKey != p.currentWorkloadKey {
		p.sparklineRow.Reset()
		p.currentWorkloadKey = newKey
		for _, sample := range detail.MetricsHistory {
			p.sparklineRow.UpdateCPU(sample.CPURatio, "")
			p.sparklineRow.UpdateMEM(sample.MemRatio, "")
		}
	}

	p.root.SetTitle(fmt.Sprintf(" %s Workloads > [::b]%s: %s/%s[::] ", ui.Icons.Rocket, detail.Kind, detail.Namespace, detail.Name))

	if p.effectiveHeight() > 31 {
		p.drawInfoHeader()
	}
	p.drawSparklines()
	p.drawMetadataSection()
	p.drawPodsTable()
	p.drawEventsTable()
}

// InitFocus activates focus on the Owned Pods table when first opened.
func (p *DetailPanel) InitFocus() {
	p.focusedChildIdx = 0
	p.updateFocusVisuals()
}

// SetFocused implements ui.FocusablePanel.
func (p *DetailPanel) SetFocused(focused bool) {
	ui.SetFlexFocused(p.root, focused)
	if focused {
		p.updateFocusVisuals()
	}
}

// GetFocusedPanelName returns "pods" or "events".
func (p *DetailPanel) GetFocusedPanelName() string {
	if p.focusedChildIdx >= 0 && p.focusedChildIdx < len(p.panelNames) {
		return p.panelNames[p.focusedChildIdx]
	}
	return "pods"
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
	p.updateFocusVisuals()
	if p.onFooterContextChange != nil {
		p.onFooterContextChange(p.GetFocusedPanelName())
	}
}

func (p *DetailPanel) updateFocusVisuals() {
	for i, panel := range p.focusablePanels {
		ui.SetFlexFocused(panel, i == p.focusedChildIdx)
	}
	p.podsTable.SetSelectable(p.focusedChildIdx == 0, false)
	if p.focusedChildIdx == 0 {
		p.podsTable.SetSelectedStyle(ui.SelectedRowStyle())
		if p.podsTable.GetRowCount() > 1 {
			r, c := p.podsTable.GetSelection()
			if r < 1 {
				p.podsTable.Select(1, c)
			}
		}
	}
	p.eventsTable.SetSelectable(p.focusedChildIdx == 1, false)
	if p.focusedChildIdx == 1 {
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
	keyOpen, valOpen, resetTag := "", "", ""
	if !mono {
		keyOpen = "[" + ui.TagFg(pal.StatusMuted) + "]"
		valOpen = "[" + ui.TagFg(pal.DataSecondary) + "]"
		resetTag = "[-]"
	}

	effSummary := ui.FormatEfficiencyCell(p.data.Efficiency.EffPct, p.data.Efficiency.HasEfficiency)
	if p.data.Efficiency.HasCPUEfficiency || p.data.Efficiency.HasMemEfficiency {
		effSummary = fmt.Sprintf("%s %s(CPU %.0f%% / MEM %.0f%%)%s",
			effSummary, keyOpen, p.data.Efficiency.CPUEffPct, p.data.Efficiency.MemEffPct, resetTag)
	}

	items := []string{
		fmt.Sprintf("%sStatus:%s %s%s", keyOpen, valOpen, p.data.Status, resetTag),
		fmt.Sprintf("%sReplicas:%s %s%s", keyOpen, valOpen, p.data.ReadyText, resetTag),
		fmt.Sprintf("%sAge:%s %s%s", keyOpen, valOpen, p.data.Age, resetTag),
		fmt.Sprintf("%sEFF:%s %s", keyOpen, resetTag, effSummary),
	}
	if p.data.Strategy != "" {
		items = append(items, fmt.Sprintf("%sStrategy:%s %s%s", keyOpen, valOpen, p.data.Strategy, resetTag))
	} else if p.data.Owner != "" {
		items = append(items, fmt.Sprintf("%sOwner:%s %s%s", keyOpen, valOpen, p.data.Owner, resetTag))
	}

	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetText("  " + strings.Join(items, "  │  "))
	p.infoHeaderPanel.AddItem(tv, 0, 1, false)
}

func (p *DetailPanel) drawSparklines() {
	if p.data == nil {
		return
	}

	prometheusMode := p.data.MetricsSourceType == metrics.SourceTypePrometheus
	p.sparklineRow.SetPrometheusMode(prometheusMode)

	var cpuRatio, memRatio float64
	reqCPU, reqMem := p.data.TotalRequestedCPU, p.data.TotalRequestedMem
	useCPU, useMem := p.data.TotalUsageCPU, p.data.TotalUsageMem

	if useCPU != nil && reqCPU != nil && reqCPU.MilliValue() > 0 {
		cpuRatio = min(float64(useCPU.MilliValue())/float64(reqCPU.MilliValue()), 1.0)
	}
	if useMem != nil && reqMem != nil && reqMem.Value() > 0 {
		memRatio = min(float64(useMem.Value())/float64(reqMem.Value()), 1.0)
	}

	cpuLabel := fmt.Sprintf(" CPU %s / %s req %s ", ui.FormatCPU(useCPU), ui.FormatCPU(reqCPU), p.sparklineRow.CPUTrend(cpuRatio*100))
	if useCPU == nil {
		cpuLabel = fmt.Sprintf(" CPU req %s ", ui.FormatCPU(reqCPU))
	}
	memLabel := fmt.Sprintf(" MEM %s / %s req %s ", strings.TrimSpace(ui.FormatMemory(useMem)), strings.TrimSpace(ui.FormatMemory(reqMem)), p.sparklineRow.MEMTrend(memRatio*100))
	if useMem == nil {
		memLabel = fmt.Sprintf(" MEM req %s ", strings.TrimSpace(ui.FormatMemory(reqMem)))
	}

	p.sparklineRow.UpdateCPU(cpuRatio, cpuLabel)
	p.sparklineRow.UpdateMEM(memRatio, memLabel)
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
	errColor := pal.StatusError
	sectionOpen := "[::b]"
	mutedOpen, closeTag := "", ""
	if mono {
		sectionColor = tcell.ColorDefault
		mutedColor = tcell.ColorDefault
		okColor = tcell.ColorDefault
		errColor = tcell.ColorDefault
	} else {
		sectionOpen = "[" + ui.TagFg(pal.Accent) + "::b]"
		mutedOpen = "[" + ui.TagFg(pal.StatusMuted) + "]"
		closeTag = "[-]"
	}

	// Column 1: Workload Metadata & Selector
	row := 0
	p.leftDetailTable.SetCell(row, 0, ui.NewDataCell("[::b]Overview[::-]").SetTextColor(sectionColor).SetSelectable(false))
	row++
	ui.AddDetailRow(p.leftDetailTable, row, "Namespace", p.data.Namespace)
	row++
	ui.AddDetailRow(p.leftDetailTable, row, "Kind", string(p.data.Kind))
	row++
	if p.data.ServiceName != "" {
		ui.AddDetailRow(p.leftDetailTable, row, "Service", p.data.ServiceName)
		row++
	}
	if p.data.Generation > 0 {
		ui.AddDetailRow(p.leftDetailTable, row, "Generation", fmt.Sprintf("%d (obs %d)", p.data.Generation, p.data.ObservedGeneration))
		row++
	}
	if len(p.data.Selector) > 0 {
		pairs := make([]string, 0, len(p.data.Selector))
		for _, k := range slices.Sorted(maps.Keys(p.data.Selector)) {
			pairs = append(pairs, k+"="+p.data.Selector[k])
		}
		ui.AddDetailRow(p.leftDetailTable, row, "Selector", ui.Truncate(strings.Join(pairs, ","), 36))
	}

	// Column 2: Conditions
	row = 0
	p.middleDetailTable.SetCell(row, 0, ui.NewDataCell("[::b]Conditions[::-]").SetTextColor(sectionColor).SetSelectable(false))
	row++
	if len(p.data.Conditions) == 0 {
		p.middleDetailTable.SetCell(row, 0, ui.NewDataCell("None").SetTextColor(mutedColor).SetSelectable(false))
	} else {
		for _, cond := range p.data.Conditions {
			valColor := okColor
			statusText := cond.Status
			attrs := tcell.AttrNone
			if !cond.Healthy {
				valColor = errColor
				if mono {
					statusText = "! " + statusText
					attrs = tcell.AttrBold | tcell.AttrUnderline
				}
			}
			p.middleDetailTable.SetCell(row, 0, ui.NewDataCell(fmt.Sprintf("%-18s", cond.Type)).SetTextColor(mutedColor).SetSelectable(false))
			detailVal := statusText
			if cond.Reason != "" {
				detailVal = fmt.Sprintf("%s (%s)", statusText, cond.Reason)
			}
			p.middleDetailTable.SetCell(row, 1, ui.NewDataCell(ui.Truncate(detailVal, 34)).SetTextColor(valColor).SetAttributes(attrs).SetSelectable(false))
			row++
		}
	}

	// Column 3: Labels & Annotations
	p.labelsTextView.SetText(formatKeyValueSection(sectionOpen+"Labels[::-]", p.data.Labels, mutedOpen, closeTag))
	p.annotationsTextView.SetText(formatKeyValueSection(sectionOpen+"Annotations[::-]", p.data.Annotations, mutedOpen, closeTag))
}

func formatKeyValueSection(heading string, m map[string]string, mutedOpen, closeTag string) string {
	var b strings.Builder
	b.WriteString(heading + "\n")
	if len(m) == 0 {
		b.WriteString(mutedOpen + "None" + closeTag)
		return b.String()
	}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		b.WriteString(fmt.Sprintf("%s%s:%s %s\n", mutedOpen, ui.TruncateKey(k), closeTag, ui.Truncate(m[k], 28)))
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

	cols := []string{"NAMESPACE", "POD", "STATUS", "READY", "RST", "CPU", "MEM", "AGE"}
	p.podsTableCtrl.SetVisibleColumns(cols)
	for i, col := range cols {
		p.podsTable.SetCell(0, i,
			ui.StyleHeaderCell(tview.NewTableCell(p.podsTableCtrl.FormatColumnHeader(col)).
				SetAlign(tview.AlignLeft).
				SetExpansion(100).
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
			if filter.MatchesRow([]string{pod.Namespace, pod.Name, pod.Status, pod.IP}) {
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
	cellColor := pal.DataSecondary
	mutedColor := pal.StatusMuted
	okColor := pal.StatusOK
	warnColor := pal.StatusWarning
	errColor := pal.StatusError
	if mono {
		cellColor = tcell.ColorDefault
		mutedColor = tcell.ColorDefault
		okColor = tcell.ColorDefault
		warnColor = tcell.ColorDefault
		errColor = tcell.ColorDefault
	}

	for i, pod := range filtered {
		row := i + 1
		ready := fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers)
		rstColor := okColor
		if pod.Restarts > 5 {
			rstColor = errColor
		} else if pod.Restarts > 0 {
			rstColor = warnColor
		}

		p.podsTable.SetCell(row, 0, ui.NewDataCell(pod.Namespace).SetTextColor(cellColor))
		p.podsTable.SetCell(row, 1, ui.NewDataCell(pod.Name).SetTextColor(cellColor).SetMaxWidth(32))
		p.podsTable.SetCell(row, 2, ui.StyleStatusCell(ui.NewDataCell(pod.Status), pod.Status, "pod"))
		p.podsTable.SetCell(row, 3, ui.NewDataCell(ready).SetTextColor(cellColor))
		p.podsTable.SetCell(row, 4, ui.NewDataCell(fmt.Sprintf("%d", pod.Restarts)).SetTextColor(rstColor))
		p.podsTable.SetCell(row, 5, ui.NewDataCell(ui.FormatCPU(pod.PodUsageCpuQty)).SetTextColor(cellColor))
		p.podsTable.SetCell(row, 6, ui.NewDataCell(ui.FormatMemory(pod.PodUsageMemQty)).SetTextColor(cellColor))
		p.podsTable.SetCell(row, 7, ui.NewDataCell(pod.TimeSince).SetTextColor(mutedColor))
	}

	if len(filtered) > 0 && p.focusedChildIdx == 0 {
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

type workloadDetailHeights struct {
	infoHeader     int
	sparklines     int
	workloadDetail int
}

func (p *DetailPanel) calculatePanelHeights(terminalHeight int) workloadDetailHeights {
	if terminalHeight <= 31 {
		return workloadDetailHeights{infoHeader: 0, sparklines: 3, workloadDetail: 8}
	}
	switch ui.GetHeightCategory(terminalHeight) {
	case ui.HeightCategorySmall:
		return workloadDetailHeights{infoHeader: 3, sparklines: 4, workloadDetail: 10}
	case ui.HeightCategoryMedium:
		return workloadDetailHeights{infoHeader: 3, sparklines: 4, workloadDetail: 12}
	default:
		return workloadDetailHeights{infoHeader: 3, sparklines: 4, workloadDetail: 12}
	}
}

func (p *DetailPanel) renderedHeight() (int, bool) {
	_, _, w, height := p.root.GetRect()
	// tview.NewBox initializes every unrendered box to 0,0,15,10.
	if height <= 0 || (w == 15 && height == 10) {
		return 0, false
	}
	return height, true
}

func (p *DetailPanel) effectiveHeight() int {
	if h, ok := p.renderedHeight(); ok {
		return h
	}
	if p.getTerminalHeight != nil {
		if th := p.getTerminalHeight(); th > 0 {
			return th
		}
	}
	return 50
}

func (p *DetailPanel) buildLayout() {
	height, ok := p.renderedHeight()
	if !ok {
		return
	}
	h := p.calculatePanelHeights(height)
	p.applyLayout(height, h)
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
	cat := ui.GetHeightCategory(height)
	h := p.calculatePanelHeights(height)
	if cat == p.lastHeightCategory &&
		h.sparklines == p.lastSparklineHeight &&
		h.infoHeader == p.lastInfoHeaderHeight {
		return
	}
	sparkChanged := h.sparklines != p.lastSparklineHeight
	p.applyLayout(height, h)
	if sparkChanged {
		p.sparklineRow.Reset()
	}
}

func (p *DetailPanel) applyLayout(height int, h workloadDetailHeights) {
	p.root.Clear()
	if h.infoHeader > 0 {
		p.root.AddItem(p.infoHeaderPanel, h.infoHeader, 0, false)
	}
	p.root.AddItem(p.sparklinePanel, h.sparklines, 0, false)
	p.root.AddItem(p.workloadDetailPanel, h.workloadDetail, 0, false)
	p.root.AddItem(p.bottomRow, 0, 1, true)

	p.lastHeightCategory = ui.GetHeightCategory(height)
	p.lastSparklineHeight = h.sparklines
	p.lastInfoHeaderHeight = h.infoHeader
}

// unused time import guard
var _ = time.Second
