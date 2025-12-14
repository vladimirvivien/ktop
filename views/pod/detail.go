package pod

import (
	"fmt"
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

// DetailPanel displays detailed information about a pod
type DetailPanel struct {
	root     *tview.Flex
	table    *tview.Table
	data     *model.PodDetailData
	laidout  bool
	rowIndex int // Current row being written to

	// Callbacks
	onNodeNavigate NodeNavigationCallback
	onBack         func()
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

// SetOnBack sets the callback for when user navigates back
func (p *DetailPanel) SetOnBack(callback func()) {
	p.onBack = callback
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
		p.table = tview.NewTable()
		p.table.SetBorder(false)
		p.table.SetBorders(false)
		p.table.SetSelectable(false, false)

		// Set up keyboard handling
		p.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			switch event.Key() {
			case tcell.KeyEscape:
				if p.onBack != nil {
					p.onBack()
					return nil
				}
			case tcell.KeyRune:
				// 'n' to navigate to node detail
				if event.Rune() == 'n' || event.Rune() == 'N' {
					if p.data != nil && p.data.PodModel != nil && p.onNodeNavigate != nil {
						p.onNodeNavigate(p.data.PodModel.Node)
						return nil
					}
				}
			}
			return event
		})

		p.root = tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(p.table, 0, 1, true)
		p.root.SetBorder(true)
		p.root.SetTitle(" Pod Detail ")
		p.root.SetTitleAlign(tview.AlignLeft)
		p.laidout = true
	}
}

// DrawHeader draws the header row
func (p *DetailPanel) DrawHeader(data interface{}) {
	// Header is part of the table content
}

// DrawBody draws the main content
func (p *DetailPanel) DrawBody(data interface{}) {
	detailData, ok := data.(*model.PodDetailData)
	if !ok {
		return
	}
	p.data = detailData
	p.table.Clear()
	p.rowIndex = 0

	// Update title with pod name and status
	if p.data.PodModel != nil {
		status := p.data.PodModel.Status
		statusColor := ui.GetStatusColor(status, "pod")
		p.root.SetTitle(fmt.Sprintf(" [::b]Pod:[::] %s/%s [%s](%s)[-] ", p.data.PodModel.Namespace, p.data.PodModel.Name, statusColor, status))
	}

	// Draw sections
	p.drawInfoSection()
	p.drawContainersSection()
	p.drawResourcesSection()
	p.drawVolumesSection()
	p.drawConditionsSection()
	p.drawEventsSection()
	p.drawLabelsSection()
}

// drawInfoSection draws the basic pod information
func (p *DetailPanel) drawInfoSection() {
	if p.data == nil || p.data.PodModel == nil {
		return
	}
	pod := p.data.PodModel

	p.addSectionHeader("Information")

	// Basic info
	p.addKeyValue("Name", pod.Name)
	p.addKeyValue("Namespace", pod.Namespace)
	p.addKeyValue("Status", pod.Status)
	p.addKeyValue("Age", pod.TimeSince)
	p.addKeyValue("Pod IP", pod.IP)
	p.addKeyValue("Node", fmt.Sprintf("%s [yellow](press 'n' to view)[-]", pod.Node))

	// Additional info from full Pod object
	if p.data.Pod != nil {
		p.addEmptyRow()
		p.addKeyValue("QoS Class", string(p.data.Pod.Status.QOSClass))
		p.addKeyValue("Service Account", p.data.Pod.Spec.ServiceAccountName)

		// Priority
		if p.data.Pod.Spec.Priority != nil {
			p.addKeyValue("Priority", fmt.Sprintf("%d", *p.data.Pod.Spec.Priority))
		}

		// Restart policy
		p.addKeyValue("Restart Policy", string(p.data.Pod.Spec.RestartPolicy))

		// Owner references
		owners := p.data.GetOwnerReferences()
		if len(owners) > 0 {
			for i, owner := range owners {
				key := "Owner"
				if i > 0 {
					key = ""
				}
				controllerStr := ""
				if owner.Controller {
					controllerStr = " [green](controller)[-]"
				}
				p.addKeyValue(key, fmt.Sprintf("%s/%s%s", owner.Kind, owner.Name, controllerStr))
			}
		}
	}
}

// drawContainersSection draws the containers information
func (p *DetailPanel) drawContainersSection() {
	containers := p.data.GetContainers()
	if len(containers) == 0 {
		return
	}

	p.addSectionHeader(fmt.Sprintf("Containers (%d)", len(containers)))

	// Header row
	p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  NAME").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 1, tview.NewTableCell("STATE").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 2, tview.NewTableCell("READY").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 3, tview.NewTableCell("RESTARTS").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 4, tview.NewTableCell("IMAGE").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.rowIndex++

	for _, container := range containers {
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
		if len(image) > 50 {
			image = image[:47] + "..."
		}

		p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  "+container.Name).SetTextColor(tcell.ColorWhite).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 1, tview.NewTableCell(container.State).SetTextColor(stateColor).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 2, tview.NewTableCell(readyStr).SetTextColor(readyColor).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 3, tview.NewTableCell(fmt.Sprintf("%d", container.RestartCount)).SetTextColor(restartColor).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 4, tview.NewTableCell(image).SetTextColor(tcell.ColorGray).SetSelectable(false))
		p.rowIndex++

		// Show probes on separate lines if configured
		if container.LivenessProbe != "" {
			p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("").SetSelectable(false))
			p.table.SetCell(p.rowIndex, 1, tview.NewTableCell("  Liveness:").SetTextColor(tcell.ColorGray).SetSelectable(false))
			p.table.SetCell(p.rowIndex, 2, tview.NewTableCell(container.LivenessProbe).SetTextColor(tcell.ColorWhite).SetSelectable(false).SetExpansion(3))
			p.rowIndex++
		}
		if container.ReadinessProbe != "" {
			p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("").SetSelectable(false))
			p.table.SetCell(p.rowIndex, 1, tview.NewTableCell("  Readiness:").SetTextColor(tcell.ColorGray).SetSelectable(false))
			p.table.SetCell(p.rowIndex, 2, tview.NewTableCell(container.ReadinessProbe).SetTextColor(tcell.ColorWhite).SetSelectable(false).SetExpansion(3))
			p.rowIndex++
		}
	}
}

// drawResourcesSection draws the resource requests/limits section
func (p *DetailPanel) drawResourcesSection() {
	containers := p.data.GetContainers()
	if len(containers) == 0 {
		return
	}

	p.addSectionHeader("Resources")

	// Show pod-level usage with sparklines if available
	pod := p.data.PodModel
	if pod != nil {
		// Build CPU and memory history from MetricsHistory
		var cpuHistory, memHistory *metrics.ResourceHistory
		if p.data.MetricsHistory != nil {
			// Use "pod" key for aggregate metrics
			if samples, ok := p.data.MetricsHistory["pod"]; ok && len(samples) > 0 {
				cpuHistory = p.buildCPUHistory(samples)
				memHistory = p.buildMemHistory(samples)
			}
		}

		// CPU usage
		cpuUsage := ""
		cpuCapacity := ""
		cpuPercent := 0.0
		if pod.PodUsageCpuQty != nil {
			cpuUsage = fmt.Sprintf("%dm", pod.PodUsageCpuQty.MilliValue())
		}
		if pod.NodeAllocatableCpuQty != nil && pod.NodeAllocatableCpuQty.MilliValue() > 0 {
			cpuCapacity = fmt.Sprintf("%dm", pod.NodeAllocatableCpuQty.MilliValue())
			if pod.PodUsageCpuQty != nil {
				cpuPercent = float64(pod.PodUsageCpuQty.MilliValue()) / float64(pod.NodeAllocatableCpuQty.MilliValue()) * 100
			}
		}
		cpuColor := ui.GetResourcePercentageColor(cpuPercent)
		p.addResourceRowWithSparkline("CPU Usage", cpuCapacity, cpuUsage, cpuPercent, cpuColor, cpuHistory)

		// Memory usage
		memUsage := ""
		memCapacity := ""
		memPercent := 0.0
		if pod.PodUsageMemQty != nil {
			memUsage = ui.FormatMemory(pod.PodUsageMemQty)
		}
		if pod.NodeAllocatableMemQty != nil && pod.NodeAllocatableMemQty.Value() > 0 {
			memCapacity = ui.FormatMemory(pod.NodeAllocatableMemQty)
			if pod.PodUsageMemQty != nil {
				memPercent = float64(pod.PodUsageMemQty.Value()) / float64(pod.NodeAllocatableMemQty.Value()) * 100
			}
		}
		memColor := ui.GetResourcePercentageColor(memPercent)
		p.addResourceRowWithSparkline("Mem Usage", memCapacity, memUsage, memPercent, memColor, memHistory)
	}

	// Requests/Limits sub-header
	p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  [gray]Requests/Limits:[-]").SetSelectable(false))
	p.rowIndex++

	// Header row
	p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  CONTAINER").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 1, tview.NewTableCell("CPU REQ").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 2, tview.NewTableCell("CPU LIM").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 3, tview.NewTableCell("MEM REQ").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 4, tview.NewTableCell("MEM LIM").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.rowIndex++

	for _, container := range containers {
		cpuReq := container.CPURequest
		if cpuReq == "" {
			cpuReq = "-"
		}
		cpuLim := container.CPULimit
		if cpuLim == "" {
			cpuLim = "-"
		}
		memReq := container.MemoryRequest
		if memReq == "" {
			memReq = "-"
		}
		memLim := container.MemoryLimit
		if memLim == "" {
			memLim = "-"
		}

		p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  "+container.Name).SetTextColor(tcell.ColorWhite).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 1, tview.NewTableCell(cpuReq).SetTextColor(tcell.ColorWhite).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 2, tview.NewTableCell(cpuLim).SetTextColor(tcell.ColorWhite).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 3, tview.NewTableCell(memReq).SetTextColor(tcell.ColorWhite).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 4, tview.NewTableCell(memLim).SetTextColor(tcell.ColorWhite).SetSelectable(false))
		p.rowIndex++
	}
}

// buildCPUHistory converts MetricSample CPU ratios to ResourceHistory format
func (p *DetailPanel) buildCPUHistory(samples []model.MetricSample) *metrics.ResourceHistory {
	if len(samples) == 0 {
		return nil
	}

	history := &metrics.ResourceHistory{
		Resource:   metrics.ResourceCPU,
		DataPoints: make([]metrics.HistoryDataPoint, len(samples)),
		MinValue:   0,
		MaxValue:   1, // Ratios are 0-1
	}

	for i, sample := range samples {
		history.DataPoints[i] = metrics.HistoryDataPoint{
			Timestamp: time.Unix(sample.Timestamp, 0),
			Value:     sample.CPURatio, // Already 0-1 ratio
		}
	}

	return history
}

// buildMemHistory converts MetricSample Memory ratios to ResourceHistory format
func (p *DetailPanel) buildMemHistory(samples []model.MetricSample) *metrics.ResourceHistory {
	if len(samples) == 0 {
		return nil
	}

	history := &metrics.ResourceHistory{
		Resource:   metrics.ResourceMemory,
		DataPoints: make([]metrics.HistoryDataPoint, len(samples)),
		MinValue:   0,
		MaxValue:   1, // Ratios are 0-1
	}

	for i, sample := range samples {
		history.DataPoints[i] = metrics.HistoryDataPoint{
			Timestamp: time.Unix(sample.Timestamp, 0),
			Value:     sample.MemRatio, // Already 0-1 ratio
		}
	}

	return history
}

// addResourceRowWithSparkline adds a resource usage row with sparkline graph
func (p *DetailPanel) addResourceRowWithSparkline(name, capacity, usage string, percent float64, percentColor string, history *metrics.ResourceHistory) {
	colorKeys := ui.ColorKeys{0: "olivedrab", 50: "yellow", 90: "red"}

	// Use SparkGraph with history if available, otherwise fallback to empty graph
	sparkGraph := ui.SparkGraph(15, history, ui.Ratio(percent/100), colorKeys)

	// Add trend indicator if we have history
	trend := ""
	if history != nil && len(history.DataPoints) >= 2 {
		trend = ui.SparkGraphTrend(history)
	}

	percentStr := fmt.Sprintf("[%s]%5.1f%%[-]", percentColor, percent)
	capacityStr := ""
	if capacity != "" {
		capacityStr = fmt.Sprintf(" / %s", capacity)
	}

	p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  "+name+":").SetTextColor(tcell.ColorGray).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 1, tview.NewTableCell(fmt.Sprintf("[%s] %s %s", sparkGraph, percentStr, trend)).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 2, tview.NewTableCell(fmt.Sprintf("Used: %s%s", usage, capacityStr)).SetTextColor(tcell.ColorWhite).SetSelectable(false))
	p.rowIndex++
}

// drawVolumesSection draws the volumes section
func (p *DetailPanel) drawVolumesSection() {
	volumes := p.data.GetVolumes()
	if len(volumes) == 0 {
		return
	}

	p.addSectionHeader(fmt.Sprintf("Volumes (%d)", len(volumes)))

	// Header row
	p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  NAME").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 1, tview.NewTableCell("TYPE").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 2, tview.NewTableCell("MOUNT PATH").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 3, tview.NewTableCell("RO").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.rowIndex++

	for _, vol := range volumes {
		roStr := "No"
		roColor := tcell.ColorGreen
		if vol.ReadOnly {
			roStr = "Yes"
			roColor = tcell.ColorYellow
		}

		p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  "+vol.Name).SetTextColor(tcell.ColorWhite).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 1, tview.NewTableCell(vol.Type).SetTextColor(tcell.ColorGray).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 2, tview.NewTableCell(vol.MountPath).SetTextColor(tcell.ColorWhite).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 3, tview.NewTableCell(roStr).SetTextColor(roColor).SetSelectable(false))
		p.rowIndex++
	}
}

// drawConditionsSection draws the pod conditions
func (p *DetailPanel) drawConditionsSection() {
	conditions := p.data.GetConditions()
	if len(conditions) == 0 {
		return
	}

	p.addSectionHeader("Conditions")

	// Header row
	p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  TYPE").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 1, tview.NewTableCell("STATUS").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 2, tview.NewTableCell("REASON").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.rowIndex++

	for _, cond := range conditions {
		statusColor := tcell.ColorGreen
		if !cond.Healthy {
			statusColor = tcell.ColorRed
		}

		p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  "+cond.Type).SetTextColor(tcell.ColorWhite).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 1, tview.NewTableCell(cond.Status).SetTextColor(statusColor).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 2, tview.NewTableCell(cond.Reason).SetTextColor(tcell.ColorGray).SetSelectable(false))
		p.rowIndex++
	}
}

// drawEventsSection draws recent events for this pod
func (p *DetailPanel) drawEventsSection() {
	if p.data == nil || len(p.data.Events) == 0 {
		return
	}

	p.addSectionHeader(fmt.Sprintf("Events (%d)", len(p.data.Events)))

	// Header row
	p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  TYPE").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 1, tview.NewTableCell("REASON").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 2, tview.NewTableCell("MESSAGE").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.table.SetCell(p.rowIndex, 3, tview.NewTableCell("AGE").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	p.rowIndex++

	// Show last 10 events
	maxEvents := 10
	startIdx := 0
	if len(p.data.Events) > maxEvents {
		startIdx = len(p.data.Events) - maxEvents
	}

	for i := startIdx; i < len(p.data.Events); i++ {
		event := p.data.Events[i]
		typeColor := tcell.ColorGreen
		if event.Type == "Warning" {
			typeColor = tcell.ColorYellow
		}

		// Truncate message if too long
		message := event.Message
		if len(message) > 60 {
			message = message[:57] + "..."
		}

		age := formatEventAge(event)

		p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  "+event.Type).SetTextColor(typeColor).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 1, tview.NewTableCell(event.Reason).SetTextColor(tcell.ColorWhite).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 2, tview.NewTableCell(message).SetTextColor(tcell.ColorGray).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 3, tview.NewTableCell(age).SetTextColor(tcell.ColorGray).SetSelectable(false))
		p.rowIndex++
	}
}

// drawLabelsSection draws the pod labels
func (p *DetailPanel) drawLabelsSection() {
	labels := p.data.GetLabels()
	if len(labels) == 0 {
		return
	}

	p.addSectionHeader(fmt.Sprintf("Labels (%d)", len(labels)))

	for key, value := range labels {
		p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("  "+key+":").SetTextColor(tcell.ColorGray).SetSelectable(false))
		p.table.SetCell(p.rowIndex, 1, tview.NewTableCell(value).SetTextColor(tcell.ColorWhite).SetSelectable(false).SetExpansion(3))
		p.rowIndex++
	}
}

// Helper methods

func (p *DetailPanel) addSectionHeader(title string) {
	// Add a separator line before section (except for first section)
	if p.rowIndex > 0 {
		p.addSeparatorRow()
	}

	cell := tview.NewTableCell(fmt.Sprintf("[::b]▶ %s[-::-]", title)).
		SetTextColor(tcell.ColorAqua).
		SetSelectable(false).
		SetExpansion(1)
	p.table.SetCell(p.rowIndex, 0, cell)
	p.rowIndex++
}

func (p *DetailPanel) addSeparatorRow() {
	// Visual separator using dim line
	separator := tview.NewTableCell("[gray]────────────────────────────────────────────────────────────────────────────────[-]").
		SetSelectable(false).
		SetExpansion(1)
	p.table.SetCell(p.rowIndex, 0, separator)
	p.rowIndex++
}

func (p *DetailPanel) addKeyValue(key, value string) {
	keyCell := tview.NewTableCell("  " + key + ":").
		SetTextColor(tcell.ColorGray).
		SetSelectable(false)
	valueCell := tview.NewTableCell(value).
		SetTextColor(tcell.ColorWhite).
		SetSelectable(false)

	p.table.SetCell(p.rowIndex, 0, keyCell)
	p.table.SetCell(p.rowIndex, 1, valueCell)
	p.rowIndex++
}

func (p *DetailPanel) addEmptyRow() {
	p.table.SetCell(p.rowIndex, 0, tview.NewTableCell("").SetSelectable(false))
	p.rowIndex++
}

func formatEventAge(event corev1.Event) string {
	// Calculate age from event timestamp
	var eventTime time.Time
	if !event.LastTimestamp.IsZero() {
		eventTime = event.LastTimestamp.Time
	} else if !event.EventTime.IsZero() {
		eventTime = event.EventTime.Time
	} else if !event.FirstTimestamp.IsZero() {
		eventTime = event.FirstTimestamp.Time
	} else {
		return "unknown"
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
	p.table.Clear()
}

// GetRootView returns the root view
func (p *DetailPanel) GetRootView() tview.Primitive {
	return p.root
}

// GetChildrenViews returns child views
func (p *DetailPanel) GetChildrenViews() []tview.Primitive {
	return nil
}

// SetFocused implements ui.FocusablePanel
func (p *DetailPanel) SetFocused(focused bool) {
	ui.SetFlexFocused(p.root, focused)
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
