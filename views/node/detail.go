package node

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/internal/snapshot"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// NodeSelectedCallback is called when a pod is selected in the node detail view
type NodeSelectedCallback func(namespace, podName string)

// DetailPanel displays detailed information about a node
type DetailPanel struct {
	root    *tview.Flex
	data    *model.NodeDetailData
	laidout bool

	// Track current node to detect when node changes (for resetting sparklines)
	currentNodeName string

	// Two-phase layout: laidout=components created, layoutBuilt=sizes applied
	layoutBuilt bool

	// Dynamic layout tracking
	lastHeightCategory   int
	lastSparklineHeight  int
	lastInfoHeaderHeight int

	// Focus management for tab cycling
	focusedChildIdx int                     // Index of currently focused child (-1 = none)
	focusableItems  []tview.Primitive       // Ordered list of focusable primitives
	focusablePanels []*tview.Flex           // Corresponding parent panels (for border updates)
	setAppFocus     func(p tview.Primitive) // Callback to set tview app focus

	// Sub-panels
	infoHeaderPanel   *tview.Flex
	infoHeaderText    *tview.TextView
	sparklinePanel    *tview.Flex
	systemDetailPanel *tview.Flex
	podsPanel         *tview.Flex

	// Tables
	leftDetailTable   *tview.Table
	middleDetailTable *tview.Table
	podsTable         *tview.Table
	podsTableCtrl     *ui.TableController
	filteredPods      []*model.PodModel

	// Text views for labels/annotations (sorted for stable display)
	labelsTextView      *tview.TextView
	annotationsTextView *tview.TextView

	// Sparkline row component for metrics visualization
	sparklineRow *ui.SparklineRow

	// Callbacks
	onPodSelected         NodeSelectedCallback
	onShowYAML            func(node *corev1.Node)
	onBack                func()
	onFooterContextChange func(focusedPanel string)

	// Adaptive scaling for Net/Disk sparklines
	peakNetRate  float64
	peakDiskRate float64
}

// NewDetailPanel creates a new node detail panel
func NewDetailPanel() *DetailPanel {
	p := &DetailPanel{}
	p.Layout(nil)
	return p
}

// SetOnPodSelected sets the callback for when a pod is selected
func (p *DetailPanel) SetOnPodSelected(callback NodeSelectedCallback) {
	p.onPodSelected = callback
}

// SetOnShowYAML sets the callback for inspecting the full Node YAML.
func (p *DetailPanel) SetOnShowYAML(callback func(node *corev1.Node)) {
	p.onShowYAML = callback
}

// SetOnBack sets the callback for when user navigates back
func (p *DetailPanel) SetOnBack(callback func()) {
	p.onBack = callback
}

// SetOnFooterContextChange sets the callback for when focused panel changes
func (p *DetailPanel) SetOnFooterContextChange(callback func(focusedPanel string)) {
	p.onFooterContextChange = callback
}

// GetTitle returns the panel title
func (p *DetailPanel) GetTitle() string {
	if p.data != nil && p.data.NodeModel != nil {
		return fmt.Sprintf("Node: %s", p.data.NodeModel.Name)
	}
	return "Node Detail"
}

// Layout initializes the panel UI
func (p *DetailPanel) Layout(_ any) {
	if !p.laidout {
		// Initialize sparkline row (starts in non-prometheus mode, self-sizing)
		p.sparklineRow = ui.NewSparklineRow(false)

		// Create info header panel with a reusable text view
		p.infoHeaderPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
		p.infoHeaderPanel.SetBorder(true)
		p.infoHeaderPanel.SetTitle(" Info ")
		p.infoHeaderPanel.SetTitleAlign(tview.AlignLeft)
		ui.SetFlexFocused(p.infoHeaderPanel, false)
		p.infoHeaderText = tview.NewTextView()
		p.infoHeaderText.SetDynamicColors(true)
		p.infoHeaderPanel.AddItem(p.infoHeaderText, 0, 1, false)

		// Create sparkline panel and add sparkline row to it
		p.sparklinePanel = tview.NewFlex().SetDirection(tview.FlexColumn)
		p.sparklinePanel.AddItem(p.sparklineRow, 0, 1, false)

		// Create system detail panel (4 columns)
		p.systemDetailPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
		p.systemDetailPanel.SetBorder(true)
		p.systemDetailPanel.SetTitle(" System Detail ")
		p.systemDetailPanel.SetTitleAlign(tview.AlignLeft)
		ui.SetFlexFocused(p.systemDetailPanel, false)

		// Left column: System info table
		p.leftDetailTable = tview.NewTable()
		p.leftDetailTable.SetBorder(false)
		p.leftDetailTable.SetBorders(false)
		p.leftDetailTable.SetSelectable(false, false)
		ui.StyleTable(p.leftDetailTable)

		// Middle column: Conditions table
		p.middleDetailTable = tview.NewTable()
		p.middleDetailTable.SetBorder(false)
		p.middleDetailTable.SetBorders(false)
		p.middleDetailTable.SetSelectable(false, false)
		ui.StyleTable(p.middleDetailTable)

		// Labels column: TextView (scrollable, sorted)
		p.labelsTextView = tview.NewTextView()
		p.labelsTextView.SetDynamicColors(true)
		p.labelsTextView.SetScrollable(true)
		p.labelsTextView.SetBorder(false)

		// Annotations column: TextView (scrollable, sorted)
		p.annotationsTextView = tview.NewTextView()
		p.annotationsTextView.SetDynamicColors(true)
		p.annotationsTextView.SetScrollable(true)
		p.annotationsTextView.SetBorder(false)

		p.systemDetailPanel.AddItem(p.leftDetailTable, 0, 1, false)
		p.systemDetailPanel.AddItem(p.middleDetailTable, 0, 1, false)
		p.systemDetailPanel.AddItem(p.labelsTextView, 0, 1, false)
		p.systemDetailPanel.AddItem(p.annotationsTextView, 0, 1, false)

		// Create pods panel with scrollable table
		p.podsPanel = tview.NewFlex().SetDirection(tview.FlexRow)
		p.podsPanel.SetBorder(true)
		p.podsPanel.SetTitle(" Pods ")
		p.podsPanel.SetTitleAlign(tview.AlignLeft)
		ui.SetFlexFocused(p.podsPanel, false)

		p.podsTable = tview.NewTable()
		p.podsTable.SetFixed(1, 0)              // Fixed header row
		p.podsTable.SetSelectable(false, false) // Start unselectable, enable on focus
		p.podsTable.SetBorder(false)
		p.podsTable.SetBorders(false)
		ui.StyleTable(p.podsTable)

		podColumnToKey := map[string]rune{
			"NAMESPACE": 'n',
			"POD":       'p',
			"STATUS":    's',
			"READY":     'd',
			"RESTARTS":  'r',
			"CPU":       'c',
			"MEM":       'm',
			"AGE":       'a',
		}

		p.podsTableCtrl = ui.NewTableController(p.podsTable, ui.TableControllerConfig{
			DefaultSortColumn: "POD",
			DefaultSortAsc:    true,
			ColumnToKey:       podColumnToKey,
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
					_ = ui.CopyToClipboard(pod.Name)
					return true
				}
				return false
			},
		})

		// Handle keyboard input on pods table
		p.podsTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if p.podsTableCtrl != nil {
				if handled := p.podsTableCtrl.HandleInput(event); handled == nil {
					return nil
				}
			}
			switch event.Key() {
			case tcell.KeyTab:
				p.cycleFocus()
				return nil
			case tcell.KeyBacktab:
				p.cycleFocusReverse()
				return nil
			case tcell.KeyEscape:
				if p.onBack != nil {
					p.onBack()
					return nil
				}
			case tcell.KeyEnter:
				row, _ := p.podsTable.GetSelection()
				if row > 0 && row-1 < len(p.filteredPods) {
					pod := p.filteredPods[row-1]
					if p.onPodSelected != nil {
						p.onPodSelected(pod.Namespace, pod.Name)
						return nil
					}
				}
			case tcell.KeyRune:
				if event.Rune() == 'v' && p.onShowYAML != nil && p.data != nil && p.data.Node != nil {
					p.onShowYAML(p.data.Node)
					return nil
				}
			}
			return event
		})

		p.podsPanel.AddItem(p.podsTable, 0, 1, true)

		// Main layout: vertical flex with dynamic heights
		// Items are added in buildLayout() when dimensions are known
		p.root = tview.NewFlex().SetDirection(tview.FlexRow)

		// Don't add items here - defer to buildLayout() to avoid jitter
		// layoutBuilt tracks whether sizes have been applied
		p.layoutBuilt = false

		p.root.SetBorder(true)
		p.root.SetTitle(fmt.Sprintf(" %s Node Detail ", ui.Icons.Factory))

		// Set up focusable items for tab cycling (only pods table now)
		p.focusableItems = []tview.Primitive{p.podsTable}
		p.focusablePanels = []*tview.Flex{p.podsPanel}
		p.focusedChildIdx = 0 // Start with pods focused

		// Set up input capture on root for Tab when page first opens
		p.root.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			switch event.Key() {
			case tcell.KeyTab:
				p.cycleFocus()
				return nil
			case tcell.KeyBacktab:
				p.cycleFocusReverse()
				return nil
			case tcell.KeyEscape:
				if p.podsTableCtrl != nil && p.podsTableCtrl.HasEscapableState() {
					p.podsTableCtrl.HandleEscape()
					return nil
				}
				if p.onBack != nil {
					p.onBack()
					return nil
				}
			case tcell.KeyRune:
				if event.Rune() == 'v' && (p.podsTableCtrl == nil || !p.podsTableCtrl.IsFilterEditing()) &&
					p.onShowYAML != nil && p.data != nil && p.data.Node != nil {
					p.onShowYAML(p.data.Node)
					return nil
				}
			}
			return event
		})
		p.root.SetTitleAlign(tview.AlignCenter)
		p.laidout = true
	}
}

// DrawHeader draws the header row
func (p *DetailPanel) DrawHeader(data any) {
	// Header is part of the layout
}

// DrawBody draws the main content
func (p *DetailPanel) DrawBody(data any) {
	var detailData *model.NodeDetailData
	switch v := data.(type) {
	case *snapshot.NodeDetailProjection:
		detailData = v.ToNodeDetailData()
	case *model.NodeDetailData:
		detailData = v
	default:
		return
	}
	if detailData == nil {
		return
	}
	p.data = detailData

	// Build layout on first draw when dimensions are available
	if !p.layoutBuilt {
		p.buildLayout()
	}

	// Check if terminal size category changed and rebuild layout if needed
	p.checkAndRebuildLayout()

	// Detect if we're viewing a different node - if so, reset sparklines
	newNodeName := ""
	if p.data.NodeModel != nil {
		newNodeName = p.data.NodeModel.Name
	}
	if newNodeName != p.currentNodeName {
		p.resetSparklines()
		p.peakNetRate = 0 // Reset adaptive scaling peaks
		p.peakDiskRate = 0
		p.currentNodeName = newNodeName

		// Populate sparklines from history if available
		if len(p.data.MetricsHistory) > 0 {
			for _, sample := range p.data.MetricsHistory {
				p.sparklineRow.UpdateCPU(sample.CPURatio, "")
				p.sparklineRow.UpdateMEM(sample.MemRatio, "")
			}
		}
	}

	// Update main title with breadcrumb navigation
	if p.data.NodeModel != nil {
		p.root.SetTitle(fmt.Sprintf(" %s Nodes > [::b]%s[::] ", ui.Icons.Factory, p.data.NodeModel.Name))
	}

	// Only draw info header if it's visible (hidden at panel height ≤31)
	terminalHeight := ui.GetTerminalHeight(p.root)
	if terminalHeight > 31 {
		p.drawInfoHeader()
	}
	p.drawSparklines() // Always draw sparklines (height varies based on terminal size)
	p.drawSystemDetailSection()
	p.drawPodsTable()
}

// resetSparklines clears all sparkline state for a fresh start
func (p *DetailPanel) resetSparklines() {
	p.sparklineRow.Reset()
}

// drawInfoHeader draws the condensed info header row
func (p *DetailPanel) drawInfoHeader() {
	if p.infoHeaderText == nil {
		return
	}
	if p.data == nil || p.data.NodeModel == nil {
		p.infoHeaderText.SetText("")
		return
	}
	node := p.data.NodeModel

	// Get hostname from raw Node object
	hostname := "n/a"
	if p.data.Node != nil {
		// Get hostname from addresses
		for _, addr := range p.data.Node.Status.Addresses {
			if addr.Type == corev1.NodeHostName {
				hostname = addr.Address
				break
			}
		}
	}

	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()
	keyOpen, valOpen, resetTag := "", "", ""
	if !mono {
		keyOpen = "[" + ui.TagFg(pal.StatusMuted) + "]"
		valOpen = "[" + ui.TagFg(pal.DataSecondary) + "]"
		resetTag = "[-]"
	}

	// Build info items (MachineID moved to System Detail section)
	items := []string{
		fmt.Sprintf("%sStatus:%s %s%s", keyOpen, valOpen, node.Status, resetTag),
		fmt.Sprintf("%sRoles:%s %s%s", keyOpen, valOpen, strings.Join(node.Roles, ","), resetTag),
		fmt.Sprintf("%sAge:%s %s%s", keyOpen, valOpen, node.TimeSinceStart, resetTag),
		fmt.Sprintf("%sIP:%s %s%s", keyOpen, valOpen, node.InternalIP, resetTag),
		fmt.Sprintf("%sHostname:%s %s%s", keyOpen, valOpen, hostname, resetTag),
		ui.FormatPSIInfo(node.PSI, node.PSIKernelSupported),
	}

	p.infoHeaderText.SetText("  " + strings.Join(items, "  │  "))
}

// drawSparklines draws the sparkline row
func (p *DetailPanel) drawSparklines() {
	if p.data == nil || p.data.NodeModel == nil {
		return
	}
	node := p.data.NodeModel

	// Update prometheus mode based on metrics source type
	prometheusMode := (p.data.MetricsSourceType == metrics.SourceTypePrometheus)
	p.sparklineRow.SetPrometheusMode(prometheusMode)

	// Calculate CPU and MEM ratios
	var cpuRatio, memRatio float64

	if node.UsageCpuQty != nil && node.AllocatableCpuQty != nil && node.AllocatableCpuQty.MilliValue() > 0 {
		cpuRatio = float64(node.UsageCpuQty.MilliValue()) / float64(node.AllocatableCpuQty.MilliValue())
	} else if node.RequestedPodCpuQty != nil && node.AllocatableCpuQty != nil && node.AllocatableCpuQty.MilliValue() > 0 {
		cpuRatio = float64(node.RequestedPodCpuQty.MilliValue()) / float64(node.AllocatableCpuQty.MilliValue())
	}

	if node.UsageMemQty != nil && node.AllocatableMemQty != nil && node.AllocatableMemQty.Value() > 0 {
		memRatio = float64(node.UsageMemQty.Value()) / float64(node.AllocatableMemQty.Value())
	} else if node.RequestedPodMemQty != nil && node.AllocatableMemQty != nil && node.AllocatableMemQty.Value() > 0 {
		memRatio = float64(node.RequestedPodMemQty.Value()) / float64(node.AllocatableMemQty.Value())
	}

	// Label each resource "used" only when an active metrics source supplied
	// actual usage; otherwise label request fallback honestly as "requests".
	hasMetricsSource := prometheusMode || p.data.MetricsSourceType == metrics.SourceTypeMetricsServer
	cpuMetricsLabel := "requests"
	if hasMetricsSource && node.UsageCpuQty != nil {
		cpuMetricsLabel = "used"
	}
	memMetricsLabel := "requests"
	if hasMetricsSource && node.UsageMemQty != nil {
		memMetricsLabel = "used"
	}

	cpuPercent := cpuRatio * 100
	cpuPercentColor := ui.GetResourcePercentageColor(cpuPercent)
	cpuTrend := p.sparklineRow.CPUTrend(cpuPercent)
	cpuUsed := "0m"
	cpuTotal := "0m"
	if node.UsageCpuQty != nil {
		cpuUsed = ui.FormatCPU(node.UsageCpuQty)
	} else if node.RequestedPodCpuQty != nil {
		cpuUsed = ui.FormatCPU(node.RequestedPodCpuQty)
	}
	if node.AllocatableCpuQty != nil {
		cpuTotal = ui.FormatCPU(node.AllocatableCpuQty)
	}

	memPercent := memRatio * 100
	memPercentColor := ui.GetResourcePercentageColor(memPercent)
	memTrend := p.sparklineRow.MEMTrend(memPercent)
	memUsed := "0Mi"
	memTotal := "0Mi"
	if node.UsageMemQty != nil {
		memUsed = ui.FormatMemory(node.UsageMemQty)
	} else if node.RequestedPodMemQty != nil {
		memUsed = ui.FormatMemory(node.RequestedPodMemQty)
	}
	if node.AllocatableMemQty != nil {
		memTotal = ui.FormatMemory(node.AllocatableMemQty)
	}

	var cpuColorOpen, memColorOpen, pctClose string
	if !ui.IsMonochrome() {
		cpuColorOpen = "[" + cpuPercentColor + "]"
		memColorOpen = "[" + memPercentColor + "]"
		pctClose = "[-]"
	}
	cpuTitle := fmt.Sprintf(" CPU %s/%s (%s%.1f%% %s%s) %s ", cpuUsed, cpuTotal, cpuColorOpen, cpuPercent, cpuMetricsLabel, pctClose, cpuTrend)
	memTitle := fmt.Sprintf(" MEM %s/%s (%s%.1f%% %s%s) %s ", memUsed, memTotal, memColorOpen, memPercent, memMetricsLabel, pctClose, memTrend)

	// Update sparkline row with new values and titles
	p.sparklineRow.UpdateCPU(cpuRatio, cpuTitle)
	p.sparklineRow.UpdateMEM(memRatio, memTitle)

	// Update network/disk sparklines in prometheus mode
	if prometheusMode {
		// Network sparkline with adaptive scaling
		netTitle := fmt.Sprintf(" Net ↓%s ↑%s ",
			ui.FormatBytesRate(p.data.NetworkRxRate),
			ui.FormatBytesRate(p.data.NetworkTxRate))
		combinedNetRate := p.data.NetworkRxRate + p.data.NetworkTxRate

		// Adaptive scaling: track peak, use minimum 512 KB/s baseline
		effectiveNetMax := ui.AdaptiveRateMax(combinedNetRate, &p.peakNetRate, 512*1024)
		netNormalized := combinedNetRate / effectiveNetMax
		if netNormalized > 1 {
			netNormalized = 1
		}
		p.sparklineRow.UpdateNET(netNormalized, netTitle)

		// Disk sparkline with adaptive scaling
		diskTitle := fmt.Sprintf(" Disk R:%s W:%s ",
			ui.FormatBytesRate(p.data.DiskReadRate),
			ui.FormatBytesRate(p.data.DiskWriteRate))
		combinedDiskRate := p.data.DiskReadRate + p.data.DiskWriteRate

		// Adaptive scaling: track peak, use minimum 512 KB/s baseline
		effectiveDiskMax := ui.AdaptiveRateMax(combinedDiskRate, &p.peakDiskRate, 512*1024)
		diskNormalized := combinedDiskRate / effectiveDiskMax
		if diskNormalized > 1 {
			diskNormalized = 1
		}
		p.sparklineRow.UpdateDisk(diskNormalized, diskTitle)
	}
}

// drawSystemDetailSection draws the 4-column system detail section
func (p *DetailPanel) drawSystemDetailSection() {
	p.leftDetailTable.Clear()
	p.middleDetailTable.Clear()
	ui.StyleTable(p.leftDetailTable)
	ui.StyleTable(p.middleDetailTable)
	p.labelsTextView.Clear()
	p.annotationsTextView.Clear()

	if p.data == nil || p.data.NodeModel == nil {
		return
	}
	node := p.data.NodeModel
	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()
	sectionColor := pal.Accent
	mutedColor := pal.StatusMuted
	okColor := pal.StatusOK
	warnColor := pal.StatusWarning
	errColor := pal.StatusError
	var okOpen, errOpen, warnOpen, mutedOpen, closeTag string
	sectionOpen := "[::b]"
	if mono {
		sectionColor = tcell.ColorDefault
		mutedColor = tcell.ColorDefault
		okColor = tcell.ColorDefault
		warnColor = tcell.ColorDefault
		errColor = tcell.ColorDefault
	} else {
		okOpen = "[" + ui.TagFg(pal.StatusOK) + "]"
		errOpen = "[" + ui.TagFg(pal.StatusError) + "]"
		warnOpen = "[" + ui.TagFg(pal.StatusWarning) + "]"
		mutedOpen = "[" + ui.TagFg(pal.StatusMuted) + "]"
		sectionOpen = "[" + ui.TagFg(pal.Accent) + "::b]"
		closeTag = "[-]"
	}

	// === LEFT COLUMN: System Info ===
	row := 0
	p.leftDetailTable.SetCell(row, 0, ui.NewDataCell("[::b]System[::-]").SetTextColor(sectionColor).SetSelectable(false))
	row++

	// MachineID (truncate for display)
	machineID := "n/a"
	if p.data.Node != nil && p.data.Node.Status.NodeInfo.MachineID != "" {
		mid := p.data.Node.Status.NodeInfo.MachineID
		if len(mid) > 12 {
			machineID = mid[:12] + "..."
		} else {
			machineID = mid
		}
	}
	ui.AddDetailRow(p.leftDetailTable, row, "MachineID", machineID)
	row++

	// Truncate OS to sensible width
	osImage := node.OSImage
	if len(osImage) > 35 {
		osImage = osImage[:32] + "..."
	}
	ui.AddDetailRow(p.leftDetailTable, row, "OS", osImage)
	row++
	ui.AddDetailRow(p.leftDetailTable, row, "Arch", node.Architecture)
	row++
	ui.AddDetailRow(p.leftDetailTable, row, "Kernel", node.OSKernel)
	row++
	ui.AddDetailRow(p.leftDetailTable, row, "Kubelet", node.KubeletVersion)
	row++
	ui.AddDetailRow(p.leftDetailTable, row, "Runtime", node.ContainerRuntimeVersion)
	row++

	// Pods count
	maxPods := "n/a"
	if p.data.Node != nil {
		if allocPods, ok := p.data.Node.Status.Allocatable[corev1.ResourcePods]; ok {
			maxPods = allocPods.String()
		}
	}
	ui.AddDetailRow(p.leftDetailTable, row, "Pods", fmt.Sprintf("%d/%s", node.PodsCount, maxPods))
	row++

	ui.AddDetailRow(p.leftDetailTable, row, "Volumes", fmt.Sprintf("%d/%d", node.VolumesInUse, node.VolumesAttached))
	row++

	// Pod CIDR
	podCIDR := "n/a"
	if p.data.Node != nil && p.data.Node.Spec.PodCIDR != "" {
		podCIDR = p.data.Node.Spec.PodCIDR
	}
	ui.AddDetailRow(p.leftDetailTable, row, "CIDR", podCIDR)

	// === MIDDLE COLUMN: Conditions ===
	row = 0
	p.middleDetailTable.SetCell(row, 0, ui.NewDataCell("[::b]Conditions[::-]").SetTextColor(sectionColor).SetSelectable(false))
	row++

	conditions := p.data.GetConditions()
	for _, cond := range conditions {
		statusColor := okColor
		statusAttrs := tcell.AttrNone
		statusText := cond.Status
		if !cond.Healthy {
			statusColor = errColor
			if mono {
				statusText = "! " + statusText
				statusAttrs = tcell.AttrUnderline | tcell.AttrBold
			}
		}
		p.middleDetailTable.SetCell(row, 0, ui.NewDataCell(fmt.Sprintf("%-18s", cond.Type)).SetTextColor(mutedColor).SetSelectable(false))
		p.middleDetailTable.SetCell(row, 1, ui.NewDataCell(statusText).SetTextColor(statusColor).SetAttributes(statusAttrs).SetSelectable(false))
		row++
	}

	// Cordoned status
	cordonedValue := okOpen + "False" + closeTag
	if node.Unschedulable {
		if mono {
			cordonedValue = ui.ErrorMarkup("True")
		} else {
			cordonedValue = errOpen + "True" + closeTag
		}
	}
	p.middleDetailTable.SetCell(row, 0, ui.NewDataCell(fmt.Sprintf("%-18s", "Cordoned")).SetTextColor(mutedColor).SetSelectable(false))
	p.middleDetailTable.SetCell(row, 1, ui.NewDataCell(cordonedValue).SetSelectable(false))
	row++

	// Taints
	taintsValue := okOpen + "None" + closeTag
	if node.TaintCount > 0 {
		if mono {
			taintsValue = fmt.Sprintf("[::b]%d[::-]", node.TaintCount)
		} else {
			taintsValue = fmt.Sprintf("%s%d%s", warnOpen, node.TaintCount, closeTag)
		}
	}
	p.middleDetailTable.SetCell(row, 0, ui.NewDataCell(fmt.Sprintf("%-18s", "Taints")).SetTextColor(mutedColor).SetSelectable(false))
	p.middleDetailTable.SetCell(row, 1, ui.NewDataCell(taintsValue).SetSelectable(false))
	row++

	// Pressures
	pressureValue := okOpen + "None" + closeTag
	if len(node.Pressures) > 0 {
		joined := strings.Join(node.Pressures, ", ")
		if mono {
			pressureValue = ui.ErrorMarkup(joined)
		} else {
			pressureValue = errOpen + joined + closeTag
		}
	}
	p.middleDetailTable.SetCell(row, 0, ui.NewDataCell(fmt.Sprintf("%-18s", "Pressures")).SetTextColor(mutedColor).SetSelectable(false))
	p.middleDetailTable.SetCell(row, 1, ui.NewDataCell(pressureValue).SetSelectable(false))
	row++

	// === EVENTS SUMMARY ===
	row++
	p.middleDetailTable.SetCell(row, 0, ui.NewDataCell("[::b]Events[::-]").SetTextColor(sectionColor).SetSelectable(false))
	row++

	if p.data != nil && len(p.data.Events) > 0 {
		normalCount, warningCount := 0, 0
		var recentWarning *corev1.Event
		for i := range p.data.Events {
			e := &p.data.Events[i]
			if e.Type == "Warning" {
				warningCount++
				if recentWarning == nil {
					recentWarning = e
				}
			} else {
				normalCount++
			}
		}

		ui.AddDetailRow(p.middleDetailTable, row, "Total", fmt.Sprintf("%d", len(p.data.Events)))
		row++
		ui.AddDetailRow(p.middleDetailTable, row, "Normal", fmt.Sprintf("%d", normalCount))
		row++

		// Warning with color
		warningColor := okColor
		if warningCount > 0 {
			warningColor = warnColor
		}
		ui.AddDetailRowColor(p.middleDetailTable, row, "Warning", fmt.Sprintf("%d", warningCount), warningColor)
		row++

		if recentWarning != nil {
			reason := recentWarning.Reason
			if len(reason) > 18 {
				reason = reason[:15] + "..."
			}
			ui.AddDetailRow(p.middleDetailTable, row, "Recent", fmt.Sprintf("%s (%s)", reason, ui.FormatEventAge(*recentWarning)))
		}
	} else {
		ui.AddDetailRow(p.middleDetailTable, row, "Total", "0")
	}

	// === LABELS COLUMN (TextView with sorted keys) ===
	labels := p.data.GetLabels()
	var labelsBuilder strings.Builder
	labelsBuilder.WriteString(sectionOpen + "Labels[::-]\n")

	if len(labels) == 0 {
		labelsBuilder.WriteString(mutedOpen + "None" + closeTag)
	} else {
		for _, k := range slices.Sorted(maps.Keys(labels)) {
			v := labels[k]
			// Truncate long values
			if len(v) > 25 {
				v = v[:22] + "..."
			}
			labelsBuilder.WriteString(fmt.Sprintf("%s%s:%s %s\n", mutedOpen, ui.TruncateKey(k), closeTag, v))
		}
	}
	p.labelsTextView.SetText(labelsBuilder.String())

	// === ANNOTATIONS COLUMN (TextView with sorted keys) ===
	annotations := p.data.GetAnnotations()
	var annotationsBuilder strings.Builder
	annotationsBuilder.WriteString(sectionOpen + "Annotations[::-]\n")

	if len(annotations) == 0 {
		annotationsBuilder.WriteString(mutedOpen + "None" + closeTag)
	} else {
		for _, k := range slices.Sorted(maps.Keys(annotations)) {
			v := annotations[k]
			// Truncate long values
			if len(v) > 25 {
				v = v[:22] + "..."
			}
			annotationsBuilder.WriteString(fmt.Sprintf("%s%s:%s %s\n", mutedOpen, ui.TruncateKey(k), closeTag, v))
		}
	}
	p.annotationsTextView.SetText(annotationsBuilder.String())
}

// drawPodsTable draws the scrollable pods table
func (p *DetailPanel) drawPodsTable() {
	// Save current selection before clearing
	selectedRow, selectedCol := p.podsTable.GetSelection()

	p.podsTable.Clear()
	ui.StyleTable(p.podsTable)

	// Draw header
	headers := []string{"NAMESPACE", "POD", "STATUS", "READY", "RESTARTS", "CPU", "MEM", "AGE"}
	if p.podsTableCtrl != nil {
		p.podsTableCtrl.SetVisibleColumns(headers)
	}
	for col, header := range headers {
		headerText := header
		if p.podsTableCtrl != nil {
			headerText = p.podsTableCtrl.FormatColumnHeader(header)
		}
		cell := ui.StyleHeaderCell(tview.NewTableCell(headerText).
			SetSelectable(false).
			SetExpansion(1))
		p.podsTable.SetCell(0, col, cell)
	}

	if p.data == nil || len(p.data.PodsOnNode) == 0 {
		p.filteredPods = nil
		if p.podsTableCtrl != nil {
			p.podsPanel.SetTitle(p.podsTableCtrl.FormatTitleWithScroll("Pods", "", 0, ""))
		} else {
			p.podsPanel.SetTitle(" Pods (0) ")
		}
		return
	}

	// Filter pods if filter is active
	if p.podsTableCtrl != nil {
		filter := p.podsTableCtrl.Filter()
		filter.TotalRows = len(p.data.PodsOnNode)

		p.filteredPods = make([]*model.PodModel, 0, len(p.data.PodsOnNode))
		if filter.IsFiltering() && filter.Text != "" {
			for _, pod := range p.data.PodsOnNode {
				readyStr := fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers)
				cells := []string{
					pod.Namespace,
					pod.Name,
					pod.Status,
					readyStr,
					fmt.Sprintf("%d", pod.Restarts),
					pod.TimeSince,
				}
				if filter.MatchesRow(cells) {
					p.filteredPods = append(p.filteredPods, pod)
				}
			}
			filter.MatchRows = len(p.filteredPods)
		} else {
			p.filteredPods = append(p.filteredPods, p.data.PodsOnNode...)
			filter.MatchRows = len(p.filteredPods)
		}

		// Sort filtered pods
		sortCol, sortAsc := p.podsTableCtrl.GetSortParams()
		model.SortPodModelPointersBy(p.filteredPods, sortCol, sortAsc)

		p.podsPanel.SetTitle(p.podsTableCtrl.FormatTitleWithScroll("Pods", "", len(p.filteredPods), ""))
	} else {
		p.filteredPods = p.data.PodsOnNode
		p.podsPanel.SetTitle(fmt.Sprintf(" Pods (%d) ", len(p.filteredPods)))
	}

	// Draw pods
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
	for row, pod := range p.filteredPods {
		rowIdx := row + 1 // Offset for header

		readyStr := fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers)

		restartColor := okColor
		restartAttrs := tcell.AttrNone
		if pod.Restarts > 5 {
			restartColor = errColor
			if mono {
				restartAttrs = tcell.AttrUnderline | tcell.AttrBold
			}
		} else if pod.Restarts > 0 {
			restartColor = warnColor
			if mono {
				restartAttrs = tcell.AttrBold
			}
		}

		// CPU and Memory
		cpuStr := ui.FormatCPU(pod.PodUsageCpuQty)
		memStr := ui.FormatMemory(pod.PodUsageMemQty)

		p.podsTable.SetCell(rowIdx, 0, ui.NewDataCell(pod.Namespace).SetTextColor(cellColor))
		p.podsTable.SetCell(rowIdx, 1, ui.NewDataCell(pod.Name).SetTextColor(cellColor).SetMaxWidth(30))
		p.podsTable.SetCell(rowIdx, 2, ui.StyleStatusCell(ui.NewDataCell(pod.Status), pod.Status, "pod"))
		p.podsTable.SetCell(rowIdx, 3, ui.NewDataCell(readyStr).SetTextColor(cellColor))
		p.podsTable.SetCell(rowIdx, 4, ui.NewDataCell(fmt.Sprintf("%d", pod.Restarts)).SetTextColor(restartColor).SetAttributes(restartAttrs))
		p.podsTable.SetCell(rowIdx, 5, ui.NewDataCell(cpuStr).SetTextColor(cellColor))
		p.podsTable.SetCell(rowIdx, 6, ui.NewDataCell(memStr).SetTextColor(cellColor))
		p.podsTable.SetCell(rowIdx, 7, ui.NewDataCell(pod.TimeSince).SetTextColor(mutedColor))
	}

	// Restore selection (clamped to valid range)
	if len(p.filteredPods) > 0 {
		maxRow := len(p.filteredPods)
		if selectedRow < 1 {
			selectedRow = 1 // Minimum is first data row
		} else if selectedRow > maxRow {
			selectedRow = maxRow
		}
		p.podsTable.Select(selectedRow, selectedCol)
	}
}

// Helper functions

func formatStorage(q *resource.Quantity) string {
	if q == nil {
		return "n/a"
	}
	return fmt.Sprintf("%dGi", q.ScaledValue(resource.Giga))
}

// DrawFooter draws the footer
func (p *DetailPanel) DrawFooter(_ any) {}

// Clear clears the panel
func (p *DetailPanel) Clear() {
	if p.podsTable != nil {
		p.podsTable.Clear()
		ui.StyleTable(p.podsTable)
	}
}

// GetRootView returns the root view
func (p *DetailPanel) GetRootView() tview.Primitive {
	return p.root
}

// GetChildrenViews returns child views
func (p *DetailPanel) GetChildrenViews() []tview.Primitive {
	return nil
}

// SetAppFocus sets the callback used to focus primitives in the tview app
func (p *DetailPanel) SetAppFocus(fn func(p tview.Primitive)) {
	p.setAppFocus = fn
}

// InitFocus sets up initial focus when the page is shown
func (p *DetailPanel) InitFocus() {
	p.focusedChildIdx = 0 // Start with pods
	p.updateFocusVisuals()
}

// RestoreFocus re-applies focus to the active child panel after modal dismissal.
func (p *DetailPanel) RestoreFocus() {
	p.updateFocusVisuals()
	p.notifyFooterContextChange()
}

// cycleFocus moves focus to the next focusable child
func (p *DetailPanel) cycleFocus() {
	if len(p.focusableItems) == 0 {
		return
	}
	p.focusedChildIdx = ui.CycleFocus(p.focusedChildIdx, len(p.focusableItems), false)
	p.updateFocusVisuals()
	p.notifyFooterContextChange()
}

// cycleFocusReverse moves focus to the previous focusable child
func (p *DetailPanel) cycleFocusReverse() {
	if len(p.focusableItems) == 0 {
		return
	}
	p.focusedChildIdx = ui.CycleFocus(p.focusedChildIdx, len(p.focusableItems), true)
	p.updateFocusVisuals()
	p.notifyFooterContextChange()
}

// GetFocusedPanelName returns the name of the currently focused panel
func (p *DetailPanel) GetFocusedPanelName() string {
	// Only pods table is focusable now
	return "pods"
}

// notifyFooterContextChange calls the footer context callback if set
func (p *DetailPanel) notifyFooterContextChange() {
	if p.onFooterContextChange != nil {
		p.onFooterContextChange(p.GetFocusedPanelName())
	}
}

// updateFocusVisuals updates border colors, table selectability, and sets tview focus
func (p *DetailPanel) updateFocusVisuals() {
	// Update border colors for all focusable panels
	for i, panel := range p.focusablePanels {
		ui.SetFlexFocused(panel, i == p.focusedChildIdx)
	}

	// Only pods table is focusable now - always show selection
	p.podsTable.SetSelectable(true, false)

	// Set tview focus to the currently focused item
	if p.setAppFocus != nil && p.focusedChildIdx >= 0 && p.focusedChildIdx < len(p.focusableItems) {
		p.setAppFocus(p.focusableItems[p.focusedChildIdx])
	}
}

// SetFocused implements ui.FocusablePanel
func (p *DetailPanel) SetFocused(focused bool) {
	ui.SetFlexFocused(p.root, focused)
	if focused {
		// When the panel receives focus, update visuals for the currently focused child
		p.updateFocusVisuals()
	}
}

// HasEscapableState implements ui.EscapablePanel
func (p *DetailPanel) HasEscapableState() bool {
	return true // Always allow ESC to go back
}

// IsFilterEditing returns whether the pods table filter is being edited
func (p *DetailPanel) IsFilterEditing() bool {
	return p.podsTableCtrl != nil && p.podsTableCtrl.IsFilterEditing()
}

// HandleEscape implements ui.EscapablePanel
func (p *DetailPanel) HandleEscape() bool {
	if p.podsTableCtrl != nil && p.podsTableCtrl.HasEscapableState() {
		return p.podsTableCtrl.HandleEscape()
	}
	if p.onBack != nil {
		p.onBack()
		return true
	}
	return false
}

// nodeDetailHeights holds panel heights for node detail view
type nodeDetailHeights struct {
	infoHeader   int
	sparklines   int
	systemDetail int
}

// calculatePanelHeights returns panel heights based on terminal height
// At panel height ≤31: compact layout (no info header, smaller system detail, compact sparklines)
// Note: Panel height is ~4-5 less than terminal height due to ktop header/footer overhead
func (p *DetailPanel) calculatePanelHeights(terminalHeight int) nodeDetailHeights {
	// Compact layout when panel height ≤31 (corresponds to terminal ≤35-36)
	if terminalHeight <= 31 {
		return nodeDetailHeights{infoHeader: 0, sparklines: 3, systemDetail: 8}
	}

	// Normal layout for larger terminals
	switch ui.GetHeightCategory(terminalHeight) {
	case ui.HeightCategorySmall:
		return nodeDetailHeights{infoHeader: 3, sparklines: 4, systemDetail: 12}
	case ui.HeightCategoryMedium:
		return nodeDetailHeights{infoHeader: 3, sparklines: 4, systemDetail: 14}
	default:
		return nodeDetailHeights{infoHeader: 3, sparklines: 4, systemDetail: 14}
	}
}

// buildLayout builds the initial layout with correct sizes based on terminal dimensions.
// Called once on first DrawBody() when dimensions are available.
func (p *DetailPanel) buildLayout() {
	// Get actual dimensions - don't build until we have real values
	_, _, _, height := p.root.GetRect()
	if height <= 0 {
		// Panel not rendered yet, will try again next frame
		return
	}

	terminalHeight := height
	currentCategory := ui.GetHeightCategory(terminalHeight)
	heights := p.calculatePanelHeights(terminalHeight)

	// Build the layout with correct sizes
	p.root.Clear()
	if heights.infoHeader > 0 {
		p.root.AddItem(p.infoHeaderPanel, heights.infoHeader, 0, false)
	}
	p.root.AddItem(p.sparklinePanel, heights.sparklines, 0, false)
	p.root.AddItem(p.systemDetailPanel, heights.systemDetail, 0, false)
	p.root.AddItem(p.podsPanel, 0, 1, true)

	p.lastHeightCategory = currentCategory
	p.lastSparklineHeight = heights.sparklines
	p.lastInfoHeaderHeight = heights.infoHeader
	p.layoutBuilt = true
}

// checkAndRebuildLayout checks if terminal size category changed and rebuilds layout if needed
func (p *DetailPanel) checkAndRebuildLayout() {
	// Only check for rebuild after initial layout is built
	if !p.layoutBuilt {
		return
	}

	// Get actual dimensions
	_, _, _, height := p.root.GetRect()
	if height <= 0 {
		return
	}

	terminalHeight := height
	currentCategory := ui.GetHeightCategory(terminalHeight)
	heights := p.calculatePanelHeights(terminalHeight)

	// Only rebuild if height category, sparkline height, or info header height changed
	if currentCategory == p.lastHeightCategory &&
		heights.sparklines == p.lastSparklineHeight &&
		heights.infoHeader == p.lastInfoHeaderHeight {
		return
	}

	// Track if sparkline height is changing (need to reset sparklines)
	sparklineHeightChanged := heights.sparklines != p.lastSparklineHeight

	// Clear and rebuild the flex layout
	p.root.Clear()
	if heights.infoHeader > 0 {
		p.root.AddItem(p.infoHeaderPanel, heights.infoHeader, 0, false)
	}
	p.root.AddItem(p.sparklinePanel, heights.sparklines, 0, false)
	p.root.AddItem(p.systemDetailPanel, heights.systemDetail, 0, false)
	p.root.AddItem(p.podsPanel, 0, 1, true)

	p.lastHeightCategory = currentCategory
	p.lastSparklineHeight = heights.sparklines
	p.lastInfoHeaderHeight = heights.infoHeader

	// Reset sparklines with new height if sparkline panel height changed
	if sparklineHeightChanged {
		p.resetSparklines()
	}
}
