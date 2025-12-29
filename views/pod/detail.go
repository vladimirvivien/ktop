package pod

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
)

// NodeNavigationCallback is called when user wants to navigate to the pod's node
type NodeNavigationCallback func(nodeName string)

// ContainerSelectedCallback is called when user selects a container to view logs
type ContainerSelectedCallback func(namespace, podName, containerName string)

// DetailPanel displays detailed information about a pod
type DetailPanel struct {
	root    *tview.Flex
	data    *model.PodDetailData
	laidout bool

	// Track current pod to detect when pod changes (for resetting sparklines)
	currentPodKey string

	// Two-phase layout: laidout=components created, layoutBuilt=sizes applied
	layoutBuilt bool

	// Dynamic layout tracking
	lastHeightCategory   int
	lastSparklineHeight  int
	lastInfoHeaderHeight int

	// Focus management for tab cycling
	focusedChildIdx int               // Index of currently focused child (-1 = none)
	focusableItems  []tview.Primitive // Ordered list of focusable primitives
	focusablePanels []*tview.Flex     // Corresponding parent panels (for border updates)
	setAppFocus     func(p tview.Primitive)

	// Sub-panels
	infoHeaderPanel *tview.Flex
	sparklinePanel  *tview.Flex
	podDetailPanel  *tview.Flex
	containersPanel *tview.Flex

	// Tables
	leftDetailTable   *tview.Table
	middleDetailTable *tview.Table
	containersTable   *tview.Table

	// Text views for labels/annotations/resources (sorted for stable display)
	labelsTextView      *tview.TextView
	annotationsTextView *tview.TextView
	resourcesTextView   *tview.TextView

	// Sparkline row component for metrics visualization
	sparklineRow *ui.SparklineRow

	// Callbacks
	onNodeNavigate        NodeNavigationCallback
	onContainerSelected   ContainerSelectedCallback
	onBack                func()
	onFooterContextChange func(focusedPanel string)
}

// NewDetailPanel creates a new pod detail panel
func NewDetailPanel() *DetailPanel {
	p := &DetailPanel{}
	p.Layout(nil)
	return p
}

// SetOnNodeNavigate sets the callback for navigating to the pod's node
func (p *DetailPanel) SetOnNodeNavigate(callback NodeNavigationCallback) {
	p.onNodeNavigate = callback
}

// SetOnContainerSelected sets the callback for when a container is selected (for logs)
func (p *DetailPanel) SetOnContainerSelected(callback ContainerSelectedCallback) {
	p.onContainerSelected = callback
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
	if p.data != nil && p.data.PodModel != nil {
		return fmt.Sprintf("Pod: %s/%s", p.data.PodModel.Namespace, p.data.PodModel.Name)
	}
	return "Pod Detail"
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

		// Create pod detail panel (4 columns)
		p.podDetailPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
		p.podDetailPanel.SetBorder(true)
		p.podDetailPanel.SetTitle(" Pod Detail ")
		p.podDetailPanel.SetTitleAlign(tview.AlignLeft)
		p.podDetailPanel.SetBorderColor(tcell.ColorLightGray)

		// Left column: Pod info table
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

		// Resources section: TextView for aggregated requests/limits
		p.resourcesTextView = tview.NewTextView()
		p.resourcesTextView.SetDynamicColors(true)
		p.resourcesTextView.SetBorder(false)

		// 3-column layout:
		// Column 1 (Left): Pod Info table
		// Column 2 (Center): Conditions (top) + Resources (bottom)
		// Column 3 (Right): Labels (top) + Annotations (bottom)

		// Center column: vertical flex with Conditions+Events and Resources
		centerColumn := tview.NewFlex().SetDirection(tview.FlexRow)
		centerColumn.AddItem(p.middleDetailTable, 0, 1, false)  // Conditions+Events (flex)
		centerColumn.AddItem(p.resourcesTextView, 3, 0, false)  // Resources (fixed 3 rows)

		// Right column: vertical flex with Labels and Annotations
		rightColumn := tview.NewFlex().SetDirection(tview.FlexRow)
		rightColumn.AddItem(p.labelsTextView, 0, 1, false)      // Labels (flex)
		rightColumn.AddItem(p.annotationsTextView, 0, 1, false) // Annotations (flex)

		p.podDetailPanel.AddItem(p.leftDetailTable, 0, 1, false) // Col 1: Pod Info
		p.podDetailPanel.AddItem(centerColumn, 0, 1, false)      // Col 2: Conditions + Events + Resources
		p.podDetailPanel.AddItem(rightColumn, 0, 1, false)       // Col 3: Labels + Annotations

		// Create containers panel with scrollable table
		p.containersPanel = tview.NewFlex().SetDirection(tview.FlexRow)
		p.containersPanel.SetBorder(true)
		p.containersPanel.SetTitle(" Containers ")
		p.containersPanel.SetTitleAlign(tview.AlignLeft)
		p.containersPanel.SetBorderColor(tcell.ColorLightGray)

		p.containersTable = tview.NewTable()
		p.containersTable.SetFixed(1, 0) // Fixed header row
		p.containersTable.SetSelectable(false, false) // Start unselectable, enable on focus
		p.containersTable.SetBorder(false)
		p.containersTable.SetBorders(false)
		p.containersTable.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorLightGray).Foreground(tcell.ColorBlack))

		// Handle keyboard input on containers table
		p.containersTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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
				p.handleContainerSelect()
				return nil
			case tcell.KeyRune:
				switch event.Rune() {
				case 'l', 'L':
					p.handleContainerSelect()
					return nil
				case 'n', 'N':
					if p.data != nil && p.data.PodModel != nil && p.onNodeNavigate != nil {
						p.onNodeNavigate(p.data.PodModel.Node)
						return nil
					}
				}
			}
			return event
		})

		p.containersPanel.AddItem(p.containersTable, 0, 1, true)

		// Main layout: vertical flex with dynamic heights
		// Items are added in buildLayout() when dimensions are known
		p.root = tview.NewFlex().SetDirection(tview.FlexRow)

		// Don't add items here - defer to buildLayout() to avoid jitter
		// layoutBuilt tracks whether sizes have been applied
		p.layoutBuilt = false

		p.root.SetBorder(true)
		p.root.SetTitle(fmt.Sprintf(" %s Pod Detail ", ui.Icons.Package))

		// Set up focusable items for tab cycling (only containers table now)
		p.focusableItems = []tview.Primitive{p.containersTable}
		p.focusablePanels = []*tview.Flex{p.containersPanel}
		p.focusedChildIdx = 0 // Start with containers focused

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
			case tcell.KeyRune:
				if event.Rune() == 'n' || event.Rune() == 'N' {
					if p.data != nil && p.data.PodModel != nil && p.onNodeNavigate != nil {
						p.onNodeNavigate(p.data.PodModel.Node)
						return nil
					}
				}
			}
			return event
		})

		p.root.SetTitleAlign(tview.AlignCenter)
		p.laidout = true
	}
}

// handleContainerSelect handles Enter/l on the containers table
func (p *DetailPanel) handleContainerSelect() {
	row, _ := p.containersTable.GetSelection()
	containers := p.data.GetContainers()
	if row > 0 && p.data != nil && row-1 < len(containers) {
		container := containers[row-1]
		if p.onContainerSelected != nil {
			p.onContainerSelected(
				p.data.PodModel.Namespace,
				p.data.PodModel.Name,
				container.Name,
			)
		}
	}
}

// DrawHeader draws the header row
func (p *DetailPanel) DrawHeader(data interface{}) {
	// Header is part of the layout
}

// DrawBody draws the main content
func (p *DetailPanel) DrawBody(data interface{}) {
	detailData, ok := data.(*model.PodDetailData)
	if !ok {
		return
	}

	// Validate data consistency: if both PodModel and Pod exist, they must refer to the same pod
	// This prevents displaying mismatched data (e.g., one pod's title with another's labels)
	if detailData.PodModel != nil && detailData.Pod != nil {
		if detailData.PodModel.Namespace != detailData.Pod.Namespace ||
			detailData.PodModel.Name != detailData.Pod.Name {
			// Data mismatch - skip this draw to avoid showing inconsistent data
			return
		}
	}

	p.data = detailData

	// Build layout on first draw when dimensions are available
	if !p.layoutBuilt {
		p.buildLayout()
	}

	// Check if terminal size category changed and rebuild layout if needed
	p.checkAndRebuildLayout()

	// Detect if we're viewing a different pod - if so, reset sparklines
	newPodKey := ""
	if p.data.PodModel != nil {
		newPodKey = p.data.PodModel.Namespace + "/" + p.data.PodModel.Name
	}
	if newPodKey != p.currentPodKey {
		p.resetSparklines()
		p.currentPodKey = newPodKey

		// Populate sparklines from history if available
		if p.data.MetricsHistory != nil {
			if samples, ok := p.data.MetricsHistory["pod"]; ok && len(samples) > 0 {
				for _, sample := range samples {
					p.sparklineRow.UpdateCPU(sample.CPURatio, "")
					p.sparklineRow.UpdateMEM(sample.MemRatio, "")
				}
			}
		}
	}

	// Update main title with breadcrumb navigation
	if p.data.PodModel != nil {
		p.root.SetTitle(fmt.Sprintf(" %s Pods > [::b]%s[::] ", ui.Icons.Package, p.data.PodModel.Name))
	}

	// Only draw info header if it's visible (hidden at panel height ≤31)
	terminalHeight := ui.GetTerminalHeight(p.root)
	if terminalHeight > 31 {
		p.drawInfoHeader()
	}
	p.drawSparklines() // Always draw sparklines (height varies based on terminal size)
	p.drawPodDetailSection()
	p.drawContainersTable()
}

// resetSparklines clears all sparkline state for a fresh start
func (p *DetailPanel) resetSparklines() {
	p.sparklineRow.Reset()
}

// drawInfoHeader draws the condensed info header row
func (p *DetailPanel) drawInfoHeader() {
	p.infoHeaderPanel.Clear()

	if p.data == nil || p.data.PodModel == nil {
		return
	}
	pod := p.data.PodModel

	// Calculate total restarts from all containers
	totalRestarts := 0
	containers := p.data.GetContainers()
	for _, c := range containers {
		totalRestarts += int(c.RestartCount)
	}

	// Build info items
	items := []string{
		fmt.Sprintf("[gray]Status:[white] %s", pod.Status),
		fmt.Sprintf("[gray]Node:[white] %s [yellow][n][-]", pod.Node),
		fmt.Sprintf("[gray]NS:[white] %s", pod.Namespace),
		fmt.Sprintf("[gray]IP:[white] %s", pod.IP),
		fmt.Sprintf("[gray]Age:[white] %s", pod.TimeSince),
		fmt.Sprintf("[gray]Restarts:[white] %d", totalRestarts),
	}

	// Create a text view with all items on one line
	infoText := tview.NewTextView()
	infoText.SetDynamicColors(true)
	infoText.SetText("  " + strings.Join(items, "  │  "))

	p.infoHeaderPanel.AddItem(infoText, 0, 1, false)
}

// drawSparklines draws the sparkline row
func (p *DetailPanel) drawSparklines() {
	if p.data == nil || p.data.PodModel == nil {
		return
	}
	pod := p.data.PodModel

	// Update prometheus mode based on metrics source type
	prometheusMode := (p.data.MetricsSourceType == metrics.SourceTypePrometheus)
	p.sparklineRow.SetPrometheusMode(prometheusMode)

	// Calculate CPU and MEM ratios
	var cpuRatio, memRatio float64

	if pod.PodUsageCpuQty != nil && pod.NodeAllocatableCpuQty != nil && pod.NodeAllocatableCpuQty.MilliValue() > 0 {
		cpuRatio = float64(pod.PodUsageCpuQty.MilliValue()) / float64(pod.NodeAllocatableCpuQty.MilliValue())
	} else if pod.PodRequestedCpuQty != nil && pod.NodeAllocatableCpuQty != nil && pod.NodeAllocatableCpuQty.MilliValue() > 0 {
		cpuRatio = float64(pod.PodRequestedCpuQty.MilliValue()) / float64(pod.NodeAllocatableCpuQty.MilliValue())
	}

	if pod.PodUsageMemQty != nil && pod.NodeAllocatableMemQty != nil && pod.NodeAllocatableMemQty.Value() > 0 {
		memRatio = float64(pod.PodUsageMemQty.Value()) / float64(pod.NodeAllocatableMemQty.Value())
	} else if pod.PodRequestedMemQty != nil && pod.NodeAllocatableMemQty != nil && pod.NodeAllocatableMemQty.Value() > 0 {
		memRatio = float64(pod.PodRequestedMemQty.Value()) / float64(pod.NodeAllocatableMemQty.Value())
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
	cpuUsed := "n/a"
	cpuTotal := "n/a"
	if pod.PodUsageCpuQty != nil {
		cpuUsed = fmt.Sprintf("%dm", pod.PodUsageCpuQty.MilliValue())
	} else if pod.PodRequestedCpuQty != nil {
		cpuUsed = fmt.Sprintf("%dm", pod.PodRequestedCpuQty.MilliValue())
	}
	if pod.NodeAllocatableCpuQty != nil {
		cpuTotal = fmt.Sprintf("%dm", pod.NodeAllocatableCpuQty.MilliValue())
	}
	cpuTitle := fmt.Sprintf(" CPU %s/%s ([%s]%.1f%% %s[-]) %s ", cpuUsed, cpuTotal, cpuPercentColor, cpuPercent, metricsLabel, cpuTrend)

	memPercent := memRatio * 100
	memPercentColor := ui.GetResourcePercentageColor(memPercent)
	memTrend := p.sparklineRow.MEMTrend(memPercent)
	memUsed := "n/a"
	memTotal := "n/a"
	if pod.PodUsageMemQty != nil {
		memUsed = ui.FormatMemory(pod.PodUsageMemQty)
	} else if pod.PodRequestedMemQty != nil {
		memUsed = ui.FormatMemory(pod.PodRequestedMemQty)
	}
	if pod.NodeAllocatableMemQty != nil {
		memTotal = ui.FormatMemory(pod.NodeAllocatableMemQty)
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

// drawPodDetailSection draws the 3-column pod detail section
func (p *DetailPanel) drawPodDetailSection() {
	p.leftDetailTable.Clear()
	p.middleDetailTable.Clear()
	p.labelsTextView.Clear()
	p.annotationsTextView.Clear()
	p.resourcesTextView.Clear()

	if p.data == nil || p.data.PodModel == nil {
		return
	}
	pod := p.data.PodModel

	// === LEFT COLUMN: Pod Info (all detail fields) ===
	row := 0
	p.leftDetailTable.SetCell(row, 0, tview.NewTableCell("[::b]Pod Info[::-]").SetTextColor(tcell.ColorAqua).SetSelectable(false))
	row++

	if p.data.Pod != nil {
		spec := p.data.Pod.Spec
		status := p.data.Pod.Status

		// ServiceAcct
		p.addDetailRow(p.leftDetailTable, row, "ServiceAcct", spec.ServiceAccountName)
		row++

		// Priority
		if spec.Priority != nil {
			p.addDetailRow(p.leftDetailTable, row, "Priority", fmt.Sprintf("%d", *spec.Priority))
		} else {
			p.addDetailRow(p.leftDetailTable, row, "Priority", "default")
		}
		row++

		// QoS Class
		p.addDetailRow(p.leftDetailTable, row, "QoS", string(status.QOSClass))
		row++

		// DNS Policy
		p.addDetailRow(p.leftDetailTable, row, "DNSPolicy", string(spec.DNSPolicy))
		row++

		// Restart Policy
		p.addDetailRow(p.leftDetailTable, row, "RestartPol", string(spec.RestartPolicy))
		row++

		// Termination Grace Period
		termGrace := "30s"
		if spec.TerminationGracePeriodSeconds != nil {
			termGrace = fmt.Sprintf("%ds", *spec.TerminationGracePeriodSeconds)
		}
		p.addDetailRow(p.leftDetailTable, row, "TermGrace", termGrace)
		row++

		// Image Pull Policy (from first container)
		imgPull := "n/a"
		if len(spec.Containers) > 0 {
			imgPull = string(spec.Containers[0].ImagePullPolicy)
		}
		p.addDetailRow(p.leftDetailTable, row, "ImgPull", imgPull)
		row++

		// Scheduler
		scheduler := spec.SchedulerName
		if scheduler == "" {
			scheduler = "default"
		}
		p.addDetailRow(p.leftDetailTable, row, "Scheduler", scheduler)
		row++

		// Node
		p.addDetailRow(p.leftDetailTable, row, "Node", fmt.Sprintf("%s [yellow][n][-]", pod.Node))
		row++

		// Host IP
		p.addDetailRow(p.leftDetailTable, row, "HostIP", status.HostIP)
		row++

		// Pod IP
		p.addDetailRow(p.leftDetailTable, row, "PodIP", pod.IP)
		row++

		// Volumes count
		p.addDetailRow(p.leftDetailTable, row, "Volumes", fmt.Sprintf("%d", len(spec.Volumes)))
		row++

		// Tolerations count
		p.addDetailRow(p.leftDetailTable, row, "Tolerations", fmt.Sprintf("%d", len(spec.Tolerations)))
		row++

		// NodeSelector
		nodeSel := "None"
		if len(spec.NodeSelector) > 0 {
			nodeSel = fmt.Sprintf("%d keys", len(spec.NodeSelector))
		}
		p.addDetailRow(p.leftDetailTable, row, "NodeSel", nodeSel)
		row++

		// Owner Reference
		owner := "None"
		if len(p.data.Pod.OwnerReferences) > 0 {
			ref := p.data.Pod.OwnerReferences[0]
			// Abbreviate common kinds
			kind := ref.Kind
			switch kind {
			case "ReplicaSet":
				kind = "RS"
			case "DaemonSet":
				kind = "DS"
			case "StatefulSet":
				kind = "STS"
			case "Deployment":
				kind = "Deploy"
			}
			owner = fmt.Sprintf("%s/%s", kind, ref.Name)
			if len(owner) > 25 {
				owner = owner[:22] + "..."
			}
		}
		p.addDetailRow(p.leftDetailTable, row, "Owner", owner)
		row++

		// Host Namespaces (compact: Net/PID/IPC)
		boolToYN := func(b bool) string {
			if b {
				return "Y"
			}
			return "N"
		}
		hostNS := fmt.Sprintf("%s/%s/%s", boolToYN(spec.HostNetwork), boolToYN(spec.HostPID), boolToYN(spec.HostIPC))
		p.addDetailRow(p.leftDetailTable, row, "HostNS", hostNS)
		row++

		// Security Context
		runAsUser := "-"
		runAsGroup := "-"
		if spec.SecurityContext != nil {
			if spec.SecurityContext.RunAsUser != nil {
				runAsUser = fmt.Sprintf("%d", *spec.SecurityContext.RunAsUser)
			}
			if spec.SecurityContext.RunAsGroup != nil {
				runAsGroup = fmt.Sprintf("%d", *spec.SecurityContext.RunAsGroup)
			}
		}
		// Check if any container is privileged
		privileged := "N"
		for _, c := range spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				privileged = "Y"
				break
			}
		}
		security := fmt.Sprintf("%s:%s Priv:%s", runAsUser, runAsGroup, privileged)
		p.addDetailRow(p.leftDetailTable, row, "Security", security)
	} else {
		// Fallback if Pod spec not available
		p.addDetailRow(p.leftDetailTable, row, "Node", fmt.Sprintf("%s [yellow][n][-]", pod.Node))
		row++
		p.addDetailRow(p.leftDetailTable, row, "PodIP", pod.IP)
	}

	// === CENTER COLUMN TOP: Conditions ===
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

	// === CENTER COLUMN BOTTOM: Resources ===
	var resourcesBuilder strings.Builder
	resourcesBuilder.WriteString("[aqua::b]Resources[::-]\n")

	if p.data.Pod != nil {
		var totalReqCPU, totalReqMem, totalLimCPU, totalLimMem int64
		for _, c := range p.data.Pod.Spec.Containers {
			if c.Resources.Requests.Cpu() != nil {
				totalReqCPU += c.Resources.Requests.Cpu().MilliValue()
			}
			if c.Resources.Requests.Memory() != nil {
				totalReqMem += c.Resources.Requests.Memory().Value()
			}
			if c.Resources.Limits.Cpu() != nil {
				totalLimCPU += c.Resources.Limits.Cpu().MilliValue()
			}
			if c.Resources.Limits.Memory() != nil {
				totalLimMem += c.Resources.Limits.Memory().Value()
			}
		}

		// Format requests
		reqCPU := "n/a"
		reqMem := "n/a"
		if totalReqCPU > 0 {
			reqCPU = fmt.Sprintf("%dm", totalReqCPU)
		}
		if totalReqMem > 0 {
			reqMem = ui.FormatBytes(totalReqMem)
		}
		resourcesBuilder.WriteString(fmt.Sprintf("[gray]Requests:[-] %s / %s\n", reqCPU, reqMem))

		// Format limits
		limCPU := "n/a"
		limMem := "n/a"
		if totalLimCPU > 0 {
			limCPU = fmt.Sprintf("%dm", totalLimCPU)
		}
		if totalLimMem > 0 {
			limMem = ui.FormatBytes(totalLimMem)
		}
		resourcesBuilder.WriteString(fmt.Sprintf("[gray]Limits:[-]   %s / %s\n", limCPU, limMem))
	} else {
		resourcesBuilder.WriteString("[gray]n/a[-]")
	}
	p.resourcesTextView.SetText(resourcesBuilder.String())

	// === RIGHT COLUMN TOP: Labels ===
	labels := p.data.GetLabels()
	var labelsBuilder strings.Builder
	labelsBuilder.WriteString("[aqua::b]Labels[::-]\n")

	if len(labels) == 0 {
		labelsBuilder.WriteString("[gray]None[-]")
	} else {
		labelKeys := make([]string, 0, len(labels))
		for k := range labels {
			labelKeys = append(labelKeys, k)
		}
		sortStrings(labelKeys)

		for _, k := range labelKeys {
			v := labels[k]
			if len(v) > 25 {
				v = v[:22] + "..."
			}
			labelsBuilder.WriteString(fmt.Sprintf("[gray]%s:[-] %s\n", truncateKey(k), v))
		}
	}
	p.labelsTextView.SetText(labelsBuilder.String())

	// === RIGHT COLUMN BOTTOM: Annotations ===
	annotations := p.data.GetAnnotations()
	var annotationsBuilder strings.Builder
	annotationsBuilder.WriteString("[aqua::b]Annotations[::-]\n")

	if len(annotations) == 0 {
		annotationsBuilder.WriteString("[gray]None[-]")
	} else {
		annotationKeys := make([]string, 0, len(annotations))
		for k := range annotations {
			annotationKeys = append(annotationKeys, k)
		}
		sortStrings(annotationKeys)

		for _, k := range annotationKeys {
			v := annotations[k]
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
	key = strings.TrimPrefix(key, "app.kubernetes.io/")
	key = strings.TrimPrefix(key, "pod.kubernetes.io/")
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

// drawContainersTable draws the scrollable containers table
func (p *DetailPanel) drawContainersTable() {
	// Save current selection before clearing
	selectedRow, selectedCol := p.containersTable.GetSelection()

	p.containersTable.Clear()

	containers := p.data.GetContainers()

	// Update title with container count
	p.containersPanel.SetTitle(fmt.Sprintf(" Containers (%d) - [yellow]Enter/l: logs[-] ", len(containers)))

	// Draw header
	headers := []string{"NAME", "IMAGE", "STATE", "READY", "RESTARTS", "CPU", "MEM"}
	for col, header := range headers {
		expansion := 1
		if header == "IMAGE" {
			expansion = 2
		}
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorWhite).
			SetBackgroundColor(tcell.ColorDarkCyan).
			SetSelectable(false).
			SetExpansion(expansion)
		p.containersTable.SetCell(0, col, cell)
	}

	if len(containers) == 0 {
		return
	}

	// Draw containers
	for row, container := range containers {
		rowIdx := row + 1 // Offset for header

		stateColor := tcell.ColorGreen
		if container.State != "Running" {
			stateColor = tcell.ColorYellow
		}
		if container.State == "Terminated" || container.State == "CrashLoopBackOff" || container.State == "Error" {
			stateColor = tcell.ColorRed
		}

		readyStr := "No"
		readyColor := tcell.ColorRed
		if container.Ready {
			readyStr = "Yes"
			readyColor = tcell.ColorGreen
		}

		restartColor := tcell.ColorGreen
		if container.RestartCount > 0 {
			restartColor = tcell.ColorYellow
		}
		if container.RestartCount > 5 {
			restartColor = tcell.ColorRed
		}

		// Truncate image if too long
		image := container.Image
		if len(image) > 40 {
			image = image[:37] + "..."
		}

		// Format CPU: usage → request → limit → "-"
		cpuStr := "-"
		cpuColor := tcell.ColorWhite
		if container.CPUUsage != "" {
			cpuStr = container.CPUUsage
			cpuColor = tcell.ColorGreen
		} else if container.CPURequest != "" {
			cpuStr = container.CPURequest
			cpuColor = tcell.ColorGray
		} else if container.CPULimit != "" {
			cpuStr = container.CPULimit
			cpuColor = tcell.ColorGray
		}

		// Format MEM: usage → request → limit → "-"
		memStr := "-"
		memColor := tcell.ColorWhite
		if container.MemoryUsage != "" {
			memStr = container.MemoryUsage
			memColor = tcell.ColorGreen
		} else if container.MemoryRequest != "" {
			memStr = container.MemoryRequest
			memColor = tcell.ColorGray
		} else if container.MemoryLimit != "" {
			memStr = container.MemoryLimit
			memColor = tcell.ColorGray
		}

		p.containersTable.SetCell(rowIdx, 0, tview.NewTableCell(container.Name).SetTextColor(tcell.ColorWhite))
		p.containersTable.SetCell(rowIdx, 1, tview.NewTableCell(image).SetTextColor(tcell.ColorGray).SetExpansion(2).SetMaxWidth(40))
		p.containersTable.SetCell(rowIdx, 2, tview.NewTableCell(container.State).SetTextColor(stateColor))
		p.containersTable.SetCell(rowIdx, 3, tview.NewTableCell(readyStr).SetTextColor(readyColor))
		p.containersTable.SetCell(rowIdx, 4, tview.NewTableCell(fmt.Sprintf("%d", container.RestartCount)).SetTextColor(restartColor))
		p.containersTable.SetCell(rowIdx, 5, tview.NewTableCell(cpuStr).SetTextColor(cpuColor))
		p.containersTable.SetCell(rowIdx, 6, tview.NewTableCell(memStr).SetTextColor(memColor))
	}

	// Restore selection (clamped to valid range)
	maxRow := len(containers)
	if selectedRow < 1 {
		selectedRow = 1 // Minimum is first data row
	} else if selectedRow > maxRow {
		selectedRow = maxRow
	}
	p.containersTable.Select(selectedRow, selectedCol)
}

// Helper functions

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
	if p.containersTable != nil {
		p.containersTable.Clear()
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
	// Only containers table is focusable now
	return "containers"
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

	// Only containers table is focusable now - always show selection
	p.containersTable.SetSelectable(true, false)

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

// podDetailHeights holds panel heights for pod detail view
type podDetailHeights struct {
	infoHeader int
	sparklines int
	podDetail  int
}

// calculatePanelHeights returns panel heights based on terminal height
// At panel height ≤31: compact layout (no info header, smaller pod detail, compact sparklines)
// Note: Panel height is ~4-5 less than terminal height due to ktop header/footer overhead
func (p *DetailPanel) calculatePanelHeights(terminalHeight int) podDetailHeights {
	// Compact layout when panel height ≤31 (corresponds to terminal ≤35-36)
	if terminalHeight <= 31 {
		return podDetailHeights{infoHeader: 0, sparklines: 3, podDetail: 8}
	}

	// Normal layout for larger terminals
	switch ui.GetHeightCategory(terminalHeight) {
	case ui.HeightCategorySmall:
		return podDetailHeights{infoHeader: 3, sparklines: 4, podDetail: 12}
	case ui.HeightCategoryMedium:
		return podDetailHeights{infoHeader: 3, sparklines: 4, podDetail: 14}
	default:
		return podDetailHeights{infoHeader: 3, sparklines: 4, podDetail: 14}
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
	p.root.AddItem(p.podDetailPanel, heights.podDetail, 0, false)
	p.root.AddItem(p.containersPanel, 0, 1, true)

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
	p.root.AddItem(p.podDetailPanel, heights.podDetail, 0, false)
	p.root.AddItem(p.containersPanel, 0, 1, true)

	p.lastHeightCategory = currentCategory
	p.lastSparklineHeight = heights.sparklines
	p.lastInfoHeaderHeight = heights.infoHeader

	// Reset sparklines with new height if sparkline panel height changed
	if sparklineHeightChanged {
		p.resetSparklines()
	}
}
