package node

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

	// Focus management for tab cycling
	focusedChildIdx int              // Index of currently focused child (-1 = none)
	focusableItems  []tview.Primitive // Ordered list of focusable primitives
	focusablePanels []*tview.Flex     // Corresponding parent panels (for border updates)
	setAppFocus     func(p tview.Primitive) // Callback to set tview app focus

	// Sub-panels
	infoHeaderPanel   *tview.Flex
	sparklinePanel    *tview.Flex
	systemDetailPanel *tview.Flex
	eventsPanel       *tview.Flex
	podsPanel         *tview.Flex

	// Tables
	leftDetailTable   *tview.Table
	middleDetailTable *tview.Table
	eventsTable       *tview.Table
	podsTable         *tview.Table

	// Text views for labels/annotations (sorted for stable display)
	labelsTextView      *tview.TextView
	annotationsTextView *tview.TextView

	// Stateful sparklines for metrics
	cpuSparkline  *ui.SparklineState
	memSparkline  *ui.SparklineState
	netSparkline  *ui.SparklineState
	diskSparkline *ui.SparklineState

	// Callbacks
	onPodSelected       NodeSelectedCallback
	onBack              func()
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
		colorKeys := ui.ColorKeys{0: "olivedrab", 50: "yellow", 90: "red"}

		// Initialize stateful sparklines (width=20, height=3 for tall multi-line)
		p.cpuSparkline = ui.NewSparklineStateWithHeight(20, 3, colorKeys)
		p.memSparkline = ui.NewSparklineStateWithHeight(20, 3, colorKeys)
		p.netSparkline = ui.NewSparklineStateWithHeight(20, 3, colorKeys)
		p.diskSparkline = ui.NewSparklineStateWithHeight(20, 3, colorKeys)

		// Create info header panel
		p.infoHeaderPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
		p.infoHeaderPanel.SetBorder(true)
		p.infoHeaderPanel.SetTitle(" Info ")
		p.infoHeaderPanel.SetTitleAlign(tview.AlignLeft)
		p.infoHeaderPanel.SetBorderColor(tcell.ColorWhite)

		// Create sparkline panel (4 columns)
		p.sparklinePanel = tview.NewFlex().SetDirection(tview.FlexColumn)

		// Create system detail panel (4 columns)
		p.systemDetailPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
		p.systemDetailPanel.SetBorder(true)
		p.systemDetailPanel.SetTitle(" System Detail ")
		p.systemDetailPanel.SetTitleAlign(tview.AlignLeft)
		p.systemDetailPanel.SetBorderColor(tcell.ColorWhite)

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

		// Create events panel with table
		p.eventsPanel = tview.NewFlex().SetDirection(tview.FlexRow)
		p.eventsPanel.SetBorder(true)
		p.eventsPanel.SetTitle(" Events ")
		p.eventsPanel.SetTitleAlign(tview.AlignLeft)
		p.eventsPanel.SetBorderColor(tcell.ColorWhite)

		p.eventsTable = tview.NewTable()
		p.eventsTable.SetFixed(1, 0) // Fixed header row
		p.eventsTable.SetBorder(false)
		p.eventsTable.SetBorders(false)
		p.eventsTable.SetSelectable(true, false) // Enable row selection for scrolling
		p.eventsTable.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlue))

		p.eventsPanel.AddItem(p.eventsTable, 0, 1, false)

		// Create pods panel with scrollable table
		p.podsPanel = tview.NewFlex().SetDirection(tview.FlexRow)
		p.podsPanel.SetBorder(true)
		p.podsPanel.SetTitle(" Pods ")
		p.podsPanel.SetTitleAlign(tview.AlignLeft)
		p.podsPanel.SetBorderColor(tcell.ColorWhite)

		p.podsTable = tview.NewTable()
		p.podsTable.SetFixed(1, 0) // Fixed header row
		p.podsTable.SetSelectable(true, false)
		p.podsTable.SetBorder(false)
		p.podsTable.SetBorders(false)
		p.podsTable.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlue))

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

		// Main layout: vertical flex
		p.root = tview.NewFlex().SetDirection(tview.FlexRow)
		p.root.AddItem(p.infoHeaderPanel, 3, 0, false)       // Info header: 3 rows
		p.root.AddItem(p.sparklinePanel, 5, 0, false)        // Sparklines: 5 rows
		p.root.AddItem(p.systemDetailPanel, 10, 0, false)    // System Detail: 10 rows
		p.root.AddItem(p.eventsPanel, 10, 0, false)          // Events: 10 rows
		p.root.AddItem(p.podsPanel, 0, 1, true)              // Pods: remaining space (flex)

		p.root.SetBorder(true)
		p.root.SetTitle(fmt.Sprintf(" %s Node Detail ", ui.Icons.Factory))

		// Set up focusable items for tab cycling
		// Order: Events -> Pods (the two scrollable/selectable tables)
		p.focusableItems = []tview.Primitive{p.eventsTable, p.podsTable}
		p.focusablePanels = []*tview.Flex{p.eventsPanel, p.podsPanel}
		p.focusedChildIdx = 0 // Start with events focused

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

		// Set up input capture on events table for Tab and ESC
		p.eventsTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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
				p.cpuSparkline.Push(sample.CPURatio)
				p.memSparkline.Push(sample.MemRatio)
			}
		}
	}

	// Update main title with breadcrumb navigation
	if p.data.NodeModel != nil {
		p.root.SetTitle(fmt.Sprintf(" %s Nodes > [::b]%s[::] ", ui.Icons.Factory, p.data.NodeModel.Name))
	}

	p.drawInfoHeader()
	p.drawSparklines()
	p.drawSystemDetailSection()
	p.drawEventsTable()
	p.drawPodsTable()
}

// resetSparklines clears all sparkline state for a fresh start
func (p *DetailPanel) resetSparklines() {
	colorKeys := ui.ColorKeys{0: "olivedrab", 50: "yellow", 90: "red"}
	p.cpuSparkline = ui.NewSparklineStateWithHeight(20, 3, colorKeys)
	p.memSparkline = ui.NewSparklineStateWithHeight(20, 3, colorKeys)
	p.netSparkline = ui.NewSparklineStateWithHeight(20, 3, colorKeys)
	p.diskSparkline = ui.NewSparklineStateWithHeight(20, 3, colorKeys)
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

// drawSparklines draws the 4-column sparkline row
func (p *DetailPanel) drawSparklines() {
	p.sparklinePanel.Clear()

	if p.data == nil || p.data.NodeModel == nil {
		return
	}
	node := p.data.NodeModel

	// Update sparkline states with new data
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

	p.cpuSparkline.Push(cpuRatio)
	p.memSparkline.Push(memRatio)
	// Network and disk sparklines - push small values so they render (data not available from metrics server)
	p.netSparkline.Push(0.01) // Small non-zero value to show baseline
	p.diskSparkline.Push(0.01)

	// Build titles with metrics values (like summary panel)
	cpuPercent := cpuRatio * 100
	cpuPercentColor := ui.GetResourcePercentageColor(cpuPercent)
	cpuTrend := p.cpuSparkline.TrendIndicator(cpuPercent)
	cpuUsed := "0m"
	cpuTotal := "0m"
	if node.UsageCpuQty != nil {
		cpuUsed = formatCPU(node.UsageCpuQty)
	}
	if node.AllocatableCpuQty != nil {
		cpuTotal = formatCPU(node.AllocatableCpuQty)
	}
	cpuTitle := fmt.Sprintf(" CPU %s/%s ([%s]%.1f%% used[-]) %s ", cpuUsed, cpuTotal, cpuPercentColor, cpuPercent, cpuTrend)

	memPercent := memRatio * 100
	memPercentColor := ui.GetResourcePercentageColor(memPercent)
	memTrend := p.memSparkline.TrendIndicator(memPercent)
	memUsed := "0Mi"
	memTotal := "0Mi"
	if node.UsageMemQty != nil {
		memUsed = formatMemory(node.UsageMemQty)
	}
	if node.AllocatableMemQty != nil {
		memTotal = formatMemory(node.AllocatableMemQty)
	}
	memTitle := fmt.Sprintf(" MEM %s/%s ([%s]%.1f%% used[-]) %s ", memUsed, memTotal, memPercentColor, memPercent, memTrend)

	// Create 4 sparkline columns with proper formatting
	cpuPanel := p.createSparklineColumn(cpuTitle, p.cpuSparkline)
	memPanel := p.createSparklineColumn(memTitle, p.memSparkline)
	netPanel := p.createSparklineColumn(" NET [gray]↓n/a ↑n/a[-] ", p.netSparkline)
	diskPanel := p.createSparklineColumn(" DISK [gray]R:n/a W:n/a[-] ", p.diskSparkline)

	p.sparklinePanel.AddItem(cpuPanel, 0, 1, false)
	p.sparklinePanel.AddItem(memPanel, 0, 1, false)
	p.sparklinePanel.AddItem(netPanel, 0, 1, false)
	p.sparklinePanel.AddItem(diskPanel, 0, 1, false)
}

// createSparklineColumn creates a bordered sparkline column with title containing metrics
func (p *DetailPanel) createSparklineColumn(title string, sparkline *ui.SparklineState) *tview.Flex {
	panel := tview.NewFlex().SetDirection(tview.FlexRow)
	panel.SetBorder(true)
	panel.SetTitle(title)
	panel.SetTitleAlign(tview.AlignCenter)
	panel.SetBorderColor(tcell.ColorWhite)

	// Create text view for sparkline
	textView := tview.NewTextView()
	textView.SetDynamicColors(true)
	textView.SetTextAlign(tview.AlignLeft)

	// Get panel width and resize sparkline to fill available space
	_, _, panelWidth, _ := p.sparklinePanel.GetInnerRect()
	if panelWidth > 0 {
		// Each of 4 panels gets 1/4 of the width, minus 2 for left/right border
		sparklineWidth := (panelWidth / 4) - 2
		if sparklineWidth > 10 {
			sparkline.Resize(sparklineWidth)
		}
	}

	// Just render the sparkline graph (values are in the title)
	textView.SetText(sparkline.Render())
	panel.AddItem(textView, 0, 1, false)

	return panel
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

// drawEventsTable draws the events table
func (p *DetailPanel) drawEventsTable() {
	// Save current selection before clearing
	selectedRow, selectedCol := p.eventsTable.GetSelection()

	p.eventsTable.Clear()

	// Update title with event count
	eventCount := 0
	if p.data != nil {
		eventCount = len(p.data.Events)
	}
	p.eventsPanel.SetTitle(fmt.Sprintf(" Events (%d) ", eventCount))

	// Draw header
	headers := []string{"TYPE", "REASON", "MESSAGE", "AGE", "COUNT"}
	for col, header := range headers {
		expansion := 1
		if header == "MESSAGE" {
			expansion = 4 // Give MESSAGE more space
		}
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetBackgroundColor(tcell.ColorDarkGreen).
			SetSelectable(false).
			SetExpansion(expansion)
		p.eventsTable.SetCell(0, col, cell)
	}

	if p.data == nil || len(p.data.Events) == 0 {
		// Show "No events" message
		p.eventsTable.SetCell(1, 0, tview.NewTableCell("").SetSelectable(false))
		p.eventsTable.SetCell(1, 1, tview.NewTableCell("").SetSelectable(false))
		p.eventsTable.SetCell(1, 2, tview.NewTableCell("[gray]No events[-]").SetSelectable(false))
		return
	}

	// Draw all events (table is scrollable)
	for i, event := range p.data.Events {
		rowIdx := i + 1 // Offset for header

		// Type color
		typeColor := tcell.ColorGreen
		if event.Type == "Warning" {
			typeColor = tcell.ColorYellow
		}

		// Truncate message if too long
		message := event.Message
		if len(message) > 80 {
			message = message[:77] + "..."
		}

		age := formatEventAge(event)
		count := fmt.Sprintf("%d", event.Count)

		p.eventsTable.SetCell(rowIdx, 0, tview.NewTableCell(event.Type).SetTextColor(typeColor))
		p.eventsTable.SetCell(rowIdx, 1, tview.NewTableCell(event.Reason).SetTextColor(tcell.ColorWhite))
		p.eventsTable.SetCell(rowIdx, 2, tview.NewTableCell(message).SetTextColor(tcell.ColorGray).SetExpansion(4))
		p.eventsTable.SetCell(rowIdx, 3, tview.NewTableCell(age).SetTextColor(tcell.ColorGray))
		p.eventsTable.SetCell(rowIdx, 4, tview.NewTableCell(count).SetTextColor(tcell.ColorWhite))
	}

	// Restore selection (clamped to valid range)
	maxRow := len(p.data.Events) // +1 for header, but we want max data row index
	if selectedRow < 1 {
		selectedRow = 1 // Minimum is first data row
	} else if selectedRow > maxRow {
		selectedRow = maxRow
	}
	p.eventsTable.Select(selectedRow, selectedCol)
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
			SetTextColor(tcell.ColorYellow).
			SetBackgroundColor(tcell.ColorDarkGreen).
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
	p.focusedChildIdx = 0 // Start with events
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
	// Index 0 = events, Index 1 = pods
	switch p.focusedChildIdx {
	case 0:
		return "events"
	case 1:
		return "pods"
	default:
		return "events"
	}
}

// notifyFooterContextChange calls the footer context callback if set
func (p *DetailPanel) notifyFooterContextChange() {
	if p.onFooterContextChange != nil {
		p.onFooterContextChange(p.GetFocusedPanelName())
	}
}

// updateFocusVisuals updates border colors and sets tview focus
func (p *DetailPanel) updateFocusVisuals() {
	// Update border colors for all focusable panels
	for i, panel := range p.focusablePanels {
		if i == p.focusedChildIdx {
			panel.SetBorderColor(tcell.ColorYellow)
		} else {
			panel.SetBorderColor(tcell.ColorWhite)
		}
	}

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
