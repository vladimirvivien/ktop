package node

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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
	focusedChildIdx int              // Index of currently focused child (-1 = none)
	focusableItems  []tview.Primitive // Ordered list of focusable primitives
	focusablePanels []*tview.Flex     // Corresponding parent panels (for border updates)
	setAppFocus     func(p tview.Primitive) // Callback to set tview app focus

	// Sub-panels
	infoHeaderPanel   *tview.Flex
	sparklinePanel    *tview.Flex
	systemDetailPanel *tview.Flex
	podsPanel         *tview.Flex

	// Tables
	leftDetailTable   *tview.Table
	middleDetailTable *tview.Table
	podsTable         *tview.Table

	// Text views for labels/annotations (sorted for stable display)
	labelsTextView      *tview.TextView
	annotationsTextView *tview.TextView

	// Sparkline row component for metrics visualization
	sparklineRow *ui.SparklineRow

	// Callbacks
	onPodSelected         NodeSelectedCallback
	onBack                func()
	onFooterContextChange func(focusedPanel string)
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
func (p *DetailPanel) Layout(_ interface{}) {
	if !p.laidout {
		// Initialize sparkline row (starts in non-prometheus mode, self-sizing)
		p.sparklineRow = ui.NewSparklineRow(false)

		// Create info header panel
		p.infoHeaderPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
		p.infoHeaderPanel.SetBorder(true)
		p.infoHeaderPanel.SetTitle(" Info ")
		p.infoHeaderPanel.SetTitleAlign(tview.AlignLeft)
		p.infoHeaderPanel.SetBorderColor(tcell.ColorLightGray)

		// Create sparkline panel and add sparkline row to it
		p.sparklinePanel = tview.NewFlex().SetDirection(tview.FlexColumn)
		p.sparklinePanel.AddItem(p.sparklineRow, 0, 1, false)

		// Create system detail panel (4 columns)
		p.systemDetailPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
		p.systemDetailPanel.SetBorder(true)
		p.systemDetailPanel.SetTitle(" System Detail ")
		p.systemDetailPanel.SetTitleAlign(tview.AlignLeft)
		p.systemDetailPanel.SetBorderColor(tcell.ColorLightGray)

		// Left column: System info table
		p.leftDetailTable = tview.NewTable()
		p.leftDetailTable.SetBorder(false)
		p.leftDetailTable.SetBorders(false)
		p.leftDetailTable.SetSelectable(false, false)

		// Middle column: Conditions table
		p.middleDetailTable = tview.NewTable()
		p.middleDetailTable.SetBorder(false)
		p.middleDetailTable.SetBorders(false)
		p.middleDetailTable.SetSelectable(false, false)

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
		p.podsPanel.SetBorderColor(tcell.ColorLightGray)

		p.podsTable = tview.NewTable()
		p.podsTable.SetFixed(1, 0) // Fixed header row
		p.podsTable.SetSelectable(false, false) // Start unselectable, enable on focus
		p.podsTable.SetBorder(false)
		p.podsTable.SetBorders(false)
		p.podsTable.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorLightGray).Foreground(tcell.ColorBlack))

		// Handle keyboard input on pods table
		p.podsTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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
				if row > 0 && p.data != nil && row-1 < len(p.data.PodsOnNode) {
					pod := p.data.PodsOnNode[row-1]
					if p.onPodSelected != nil {
						p.onPodSelected(pod.Namespace, pod.Name)
						return nil
					}
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
				if p.onBack != nil {
					p.onBack()
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
func (p *DetailPanel) DrawHeader(data interface{}) {
	// Header is part of the layout
}

// DrawBody draws the main content
func (p *DetailPanel) DrawBody(data interface{}) {
	detailData, ok := data.(*model.NodeDetailData)
	if !ok {
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
	p.infoHeaderPanel.Clear()

	if p.data == nil || p.data.NodeModel == nil {
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

	// Build info items (MachineID moved to System Detail section)
	items := []string{
		fmt.Sprintf("[gray]Status:[white] %s", node.Status),
		fmt.Sprintf("[gray]Roles:[white] %s", strings.Join(node.Roles, ",")),
		fmt.Sprintf("[gray]Age:[white] %s", node.TimeSinceStart),
		fmt.Sprintf("[gray]IP:[white] %s", node.InternalIP),
		fmt.Sprintf("[gray]Hostname:[white] %s", hostname),
	}

	// Create a text view with all items on one line
	infoText := tview.NewTextView()
	infoText.SetDynamicColors(true)
	infoText.SetText("  " + strings.Join(items, "  │  "))

	p.infoHeaderPanel.AddItem(infoText, 0, 1, false)
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

	// Build titles with metrics values
	// Label is "used" for prometheus/metrics-server (actual usage), "requests" for none mode
	metricsLabel := "used"
	if !prometheusMode && p.data.MetricsSourceType != metrics.SourceTypeMetricsServer {
		metricsLabel = "requests"
	}

	cpuPercent := cpuRatio * 100
	cpuPercentColor := ui.GetResourcePercentageColor(cpuPercent)
	cpuTrend := p.sparklineRow.CPUTrend(cpuPercent)
	cpuUsed := "0m"
	cpuTotal := "0m"
	if node.UsageCpuQty != nil {
		cpuUsed = formatCPU(node.UsageCpuQty)
	} else if node.RequestedPodCpuQty != nil {
		cpuUsed = formatCPU(node.RequestedPodCpuQty)
	}
	if node.AllocatableCpuQty != nil {
		cpuTotal = formatCPU(node.AllocatableCpuQty)
	}
	cpuTitle := fmt.Sprintf(" CPU %s/%s ([%s]%.1f%% %s[-]) %s ", cpuUsed, cpuTotal, cpuPercentColor, cpuPercent, metricsLabel, cpuTrend)

	memPercent := memRatio * 100
	memPercentColor := ui.GetResourcePercentageColor(memPercent)
	memTrend := p.sparklineRow.MEMTrend(memPercent)
	memUsed := "0Mi"
	memTotal := "0Mi"
	if node.UsageMemQty != nil {
		memUsed = formatMemory(node.UsageMemQty)
	} else if node.RequestedPodMemQty != nil {
		memUsed = formatMemory(node.RequestedPodMemQty)
	}
	if node.AllocatableMemQty != nil {
		memTotal = formatMemory(node.AllocatableMemQty)
	}
	memTitle := fmt.Sprintf(" MEM %s/%s ([%s]%.1f%% %s[-]) %s ", memUsed, memTotal, memPercentColor, memPercent, metricsLabel, memTrend)

	// Update sparkline row with new values and titles
	p.sparklineRow.UpdateCPU(cpuRatio, cpuTitle)
	p.sparklineRow.UpdateMEM(memRatio, memTitle)

	// Update network/disk sparklines in prometheus mode
	if prometheusMode {
		p.sparklineRow.UpdateNET(0.01, " NET [gray]↓n/a ↑n/a[-] ")
		p.sparklineRow.UpdateDisk(0.01, " DISK [gray]R:n/a W:n/a[-] ")
	}
}

// drawSystemDetailSection draws the 4-column system detail section
func (p *DetailPanel) drawSystemDetailSection() {
	p.leftDetailTable.Clear()
	p.middleDetailTable.Clear()
	p.labelsTextView.Clear()
	p.annotationsTextView.Clear()

	if p.data == nil || p.data.NodeModel == nil {
		return
	}
	node := p.data.NodeModel

	// === LEFT COLUMN: System Info ===
	row := 0
	p.leftDetailTable.SetCell(row, 0, tview.NewTableCell("[::b]System[::-]").SetTextColor(tcell.ColorAqua).SetSelectable(false))
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
	p.addDetailRow(p.leftDetailTable, row, "MachineID", machineID)
	row++

	// Truncate OS to sensible width
	osImage := node.OSImage
	if len(osImage) > 35 {
		osImage = osImage[:32] + "..."
	}
	p.addDetailRow(p.leftDetailTable, row, "OS", osImage)
	row++
	p.addDetailRow(p.leftDetailTable, row, "Arch", node.Architecture)
	row++
	p.addDetailRow(p.leftDetailTable, row, "Kernel", node.OSKernel)
	row++
	p.addDetailRow(p.leftDetailTable, row, "Kubelet", node.KubeletVersion)
	row++
	p.addDetailRow(p.leftDetailTable, row, "Runtime", node.ContainerRuntimeVersion)
	row++

	// Pods count
	maxPods := "n/a"
	if p.data.Node != nil {
		if allocPods, ok := p.data.Node.Status.Allocatable[corev1.ResourcePods]; ok {
			maxPods = allocPods.String()
		}
	}
	p.addDetailRow(p.leftDetailTable, row, "Pods", fmt.Sprintf("%d/%s", node.PodsCount, maxPods))
	row++

	p.addDetailRow(p.leftDetailTable, row, "Volumes", fmt.Sprintf("%d/%d", node.VolumesInUse, node.VolumesAttached))
	row++

	// Pod CIDR
	podCIDR := "n/a"
	if p.data.Node != nil && p.data.Node.Spec.PodCIDR != "" {
		podCIDR = p.data.Node.Spec.PodCIDR
	}
	p.addDetailRow(p.leftDetailTable, row, "CIDR", podCIDR)

	// === MIDDLE COLUMN: Conditions ===
	row = 0
	p.middleDetailTable.SetCell(row, 0, tview.NewTableCell("[::b]Conditions[::-]").SetTextColor(tcell.ColorAqua).SetSelectable(false))
	row++

	conditions := p.data.GetConditions()
	for _, cond := range conditions {
		statusColor := tcell.ColorGreen
		if !cond.Healthy {
			statusColor = tcell.ColorRed
		}
		p.middleDetailTable.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf("%-18s", cond.Type)).SetTextColor(tcell.ColorGray).SetSelectable(false))
		p.middleDetailTable.SetCell(row, 1, tview.NewTableCell(cond.Status).SetTextColor(statusColor).SetSelectable(false))
		row++
	}

	// Cordoned status
	cordonedValue := "[green]False[-]"
	if node.Unschedulable {
		cordonedValue = "[red]True[-]"
	}
	p.middleDetailTable.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf("%-18s", "Cordoned")).SetTextColor(tcell.ColorGray).SetSelectable(false))
	p.middleDetailTable.SetCell(row, 1, tview.NewTableCell(cordonedValue).SetSelectable(false))
	row++

	// Taints
	taintsValue := "[green]None[-]"
	if node.TaintCount > 0 {
		taintsValue = fmt.Sprintf("[yellow]%d[-]", node.TaintCount)
	}
	p.middleDetailTable.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf("%-18s", "Taints")).SetTextColor(tcell.ColorGray).SetSelectable(false))
	p.middleDetailTable.SetCell(row, 1, tview.NewTableCell(taintsValue).SetSelectable(false))
	row++

	// Pressures
	pressureValue := "[green]None[-]"
	if len(node.Pressures) > 0 {
		pressureValue = "[red]" + strings.Join(node.Pressures, ", ") + "[-]"
	}
	p.middleDetailTable.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf("%-18s", "Pressures")).SetTextColor(tcell.ColorGray).SetSelectable(false))
	p.middleDetailTable.SetCell(row, 1, tview.NewTableCell(pressureValue).SetSelectable(false))
	row++

	// === EVENTS SUMMARY ===
	row++
	p.middleDetailTable.SetCell(row, 0, tview.NewTableCell("[::b]Events[::-]").SetTextColor(tcell.ColorAqua).SetSelectable(false))
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

		p.addDetailRow(p.middleDetailTable, row, "Total", fmt.Sprintf("%d", len(p.data.Events)))
		row++
		p.addDetailRow(p.middleDetailTable, row, "Normal", fmt.Sprintf("%d", normalCount))
		row++

		// Warning with color
		warningColor := tcell.ColorGreen
		if warningCount > 0 {
			warningColor = tcell.ColorYellow
		}
		p.addDetailRowColor(p.middleDetailTable, row, "Warning", fmt.Sprintf("%d", warningCount), warningColor)
		row++

		if recentWarning != nil {
			reason := recentWarning.Reason
			if len(reason) > 18 {
				reason = reason[:15] + "..."
			}
			p.addDetailRow(p.middleDetailTable, row, "Recent", fmt.Sprintf("%s (%s)", reason, formatEventAge(*recentWarning)))
		}
	} else {
		p.addDetailRow(p.middleDetailTable, row, "Total", "0")
	}

	// === LABELS COLUMN (TextView with sorted keys) ===
	labels := p.data.GetLabels()
	var labelsBuilder strings.Builder
	labelsBuilder.WriteString("[aqua::b]Labels[::-]\n")

	if len(labels) == 0 {
		labelsBuilder.WriteString("[gray]None[-]")
	} else {
		// Sort labels for consistent display
		labelKeys := make([]string, 0, len(labels))
		for k := range labels {
			labelKeys = append(labelKeys, k)
		}
		sortStrings(labelKeys)

		for _, k := range labelKeys {
			v := labels[k]
			// Truncate long values
			if len(v) > 25 {
				v = v[:22] + "..."
			}
			labelsBuilder.WriteString(fmt.Sprintf("[gray]%s:[-] %s\n", truncateKey(k), v))
		}
	}
	p.labelsTextView.SetText(labelsBuilder.String())

	// === ANNOTATIONS COLUMN (TextView with sorted keys) ===
	annotations := p.data.GetAnnotations()
	var annotationsBuilder strings.Builder
	annotationsBuilder.WriteString("[aqua::b]Annotations[::-]\n")

	if len(annotations) == 0 {
		annotationsBuilder.WriteString("[gray]None[-]")
	} else {
		// Sort annotations for consistent display
		annotationKeys := make([]string, 0, len(annotations))
		for k := range annotations {
			annotationKeys = append(annotationKeys, k)
		}
		sortStrings(annotationKeys)

		for _, k := range annotationKeys {
			v := annotations[k]
			// Truncate long values
			if len(v) > 25 {
				v = v[:22] + "..."
			}
			annotationsBuilder.WriteString(fmt.Sprintf("[gray]%s:[-] %s\n", truncateKey(k), v))
		}
	}
	p.annotationsTextView.SetText(annotationsBuilder.String())
}

// sortStrings sorts a slice of strings in place
func sortStrings(s []string) {
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

// truncateKey shortens a label/annotation key for display
func truncateKey(key string) string {
	// Remove common prefixes to save space
	key = strings.TrimPrefix(key, "kubernetes.io/")
	key = strings.TrimPrefix(key, "node.kubernetes.io/")
	key = strings.TrimPrefix(key, "topology.kubernetes.io/")
	if len(key) > 25 {
		key = key[:22] + "..."
	}
	return key
}

// addDetailRow adds a key-value row to a detail table
func (p *DetailPanel) addDetailRow(table *tview.Table, row int, key, value string) {
	// Pad key to minimum width for visual separation
	paddedKey := fmt.Sprintf("%-10s", key)
	table.SetCell(row, 0, tview.NewTableCell(paddedKey).SetTextColor(tcell.ColorGray).SetSelectable(false))
	table.SetCell(row, 1, tview.NewTableCell(value).SetTextColor(tcell.ColorWhite).SetSelectable(false))
}

// addDetailRowColor adds a key-value row with a specific value color
func (p *DetailPanel) addDetailRowColor(table *tview.Table, row int, key, value string, color tcell.Color) {
	paddedKey := fmt.Sprintf("%-10s", key)
	table.SetCell(row, 0, tview.NewTableCell(paddedKey).SetTextColor(tcell.ColorGray).SetSelectable(false))
	table.SetCell(row, 1, tview.NewTableCell(value).SetTextColor(color).SetSelectable(false))
}

// drawPodsTable draws the scrollable pods table
func (p *DetailPanel) drawPodsTable() {
	// Save current selection before clearing
	selectedRow, selectedCol := p.podsTable.GetSelection()

	p.podsTable.Clear()

	// Update title with pod count
	if p.data != nil {
		p.podsPanel.SetTitle(fmt.Sprintf(" Pods (%d) ", len(p.data.PodsOnNode)))
	}

	// Draw header
	headers := []string{"NAMESPACE", "NAME", "STATUS", "READY", "RESTARTS", "CPU", "MEM", "AGE"}
	for col, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorWhite).
			SetBackgroundColor(tcell.ColorDarkCyan).
			SetSelectable(false).
			SetExpansion(1)
		p.podsTable.SetCell(0, col, cell)
	}

	if p.data == nil || len(p.data.PodsOnNode) == 0 {
		return
	}

	// Draw pods
	for row, pod := range p.data.PodsOnNode {
		rowIdx := row + 1 // Offset for header

		statusColor := ui.GetTcellColor(ui.GetStatusColor(pod.Status, "pod"))
		readyStr := fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers)

		restartColor := tcell.ColorGreen
		if pod.Restarts > 0 {
			restartColor = tcell.ColorYellow
		}
		if pod.Restarts > 5 {
			restartColor = tcell.ColorRed
		}

		// CPU and Memory
		cpuStr := "n/a"
		memStr := "n/a"
		if pod.PodUsageCpuQty != nil {
			cpuStr = fmt.Sprintf("%dm", pod.PodUsageCpuQty.MilliValue())
		}
		if pod.PodUsageMemQty != nil {
			memStr = ui.FormatMemory(pod.PodUsageMemQty)
		}

		p.podsTable.SetCell(rowIdx, 0, tview.NewTableCell(pod.Namespace).SetTextColor(tcell.ColorWhite))
		p.podsTable.SetCell(rowIdx, 1, tview.NewTableCell(pod.Name).SetTextColor(tcell.ColorWhite).SetMaxWidth(30))
		p.podsTable.SetCell(rowIdx, 2, tview.NewTableCell(pod.Status).SetTextColor(statusColor))
		p.podsTable.SetCell(rowIdx, 3, tview.NewTableCell(readyStr).SetTextColor(tcell.ColorWhite))
		p.podsTable.SetCell(rowIdx, 4, tview.NewTableCell(fmt.Sprintf("%d", pod.Restarts)).SetTextColor(restartColor))
		p.podsTable.SetCell(rowIdx, 5, tview.NewTableCell(cpuStr).SetTextColor(tcell.ColorWhite))
		p.podsTable.SetCell(rowIdx, 6, tview.NewTableCell(memStr).SetTextColor(tcell.ColorWhite))
		p.podsTable.SetCell(rowIdx, 7, tview.NewTableCell(pod.TimeSince).SetTextColor(tcell.ColorGray))
	}

	// Restore selection (clamped to valid range)
	maxRow := len(p.data.PodsOnNode) // +1 for header, but we want max data row index
	if selectedRow < 1 {
		selectedRow = 1 // Minimum is first data row
	} else if selectedRow > maxRow {
		selectedRow = maxRow
	}
	p.podsTable.Select(selectedRow, selectedCol)
}

// Helper functions

func formatCPU(q *resource.Quantity) string {
	if q == nil {
		return "n/a"
	}
	return fmt.Sprintf("%dm", q.MilliValue())
}

func formatMemory(q *resource.Quantity) string {
	if q == nil {
		return "n/a"
	}
	return ui.FormatMemory(q)
}

func formatStorage(q *resource.Quantity) string {
	if q == nil {
		return "n/a"
	}
	return fmt.Sprintf("%dGi", q.ScaledValue(resource.Giga))
}

func formatEventAge(event corev1.Event) string {
	var eventTime time.Time
	if !event.LastTimestamp.IsZero() {
		eventTime = event.LastTimestamp.Time
	} else if !event.EventTime.IsZero() {
		eventTime = event.EventTime.Time
	} else if !event.FirstTimestamp.IsZero() {
		eventTime = event.FirstTimestamp.Time
	} else {
		return "?"
	}

	duration := time.Since(eventTime)
	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	}
	return fmt.Sprintf("%dd", int(duration.Hours()/24))
}

// DrawFooter draws the footer
func (p *DetailPanel) DrawFooter(_ interface{}) {}

// Clear clears the panel
func (p *DetailPanel) Clear() {
	if p.podsTable != nil {
		p.podsTable.Clear()
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

// cycleFocus moves focus to the next focusable child
func (p *DetailPanel) cycleFocus() {
	if len(p.focusableItems) == 0 {
		return
	}
	p.focusedChildIdx = (p.focusedChildIdx + 1) % len(p.focusableItems)
	p.updateFocusVisuals()
	p.notifyFooterContextChange()
}

// cycleFocusReverse moves focus to the previous focusable child
func (p *DetailPanel) cycleFocusReverse() {
	if len(p.focusableItems) == 0 {
		return
	}
	p.focusedChildIdx--
	if p.focusedChildIdx < 0 {
		p.focusedChildIdx = len(p.focusableItems) - 1
	}
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
		if i == p.focusedChildIdx {
			panel.SetBorderColor(tcell.ColorDodgerBlue)
		} else {
			panel.SetBorderColor(tcell.ColorLightGray)
		}
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

// HandleEscape implements ui.EscapablePanel
func (p *DetailPanel) HandleEscape() bool {
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
