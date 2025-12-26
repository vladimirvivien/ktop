package container

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	corev1 "k8s.io/api/core/v1"
)

// DetailPanel displays detailed information about a container including its logs
type DetailPanel struct {
	root    *tview.Flex
	laidout bool

	// Dynamic layout tracking
	lastHeightCategory   int
	lastInfoHeaderHeight int

	// Container identity
	namespace     string
	podName       string
	containerName string

	// Container data (from pod)
	containerSpec   *corev1.Container
	containerStatus *corev1.ContainerStatus
	pod             *corev1.Pod

	// Container metrics (usage)
	cpuUsage string
	memUsage string

	// UI components - Info header
	infoHeaderPanel *tview.Flex

	// UI components - Container Detail section
	containerDetailPanel *tview.Flex
	leftDetailTable      *tview.Table
	centerDetailTable    *tview.Table
	rightDetailTable     *tview.Table

	// UI components - Log section
	logControlPanel *tview.Flex
	logsView        *tview.TextView

	// Log state
	following  bool
	timestamps bool
	wrapText   bool
	tailLines  int64
	lineCount  int

	// Streaming control
	cancelFunc context.CancelFunc
	streamMu   sync.Mutex

	// Focus management for Tab cycling
	focusedChildIdx int               // 0 = Container Detail, 1 = Logs
	focusableItems  []tview.Primitive // Ordered list of focusable primitives
	focusablePanels []*tview.Flex     // Corresponding parent panels (for border updates)
	setAppFocus     func(p tview.Primitive)

	// Callbacks
	onBack            func()
	onShowSpec        func(namespace, podName, containerName string, containerSpec *corev1.Container)
	getLogStream      func(ctx context.Context, namespace, podName string, opts k8s.LogOptions) (io.ReadCloser, error)
	getPod            func(ctx context.Context, namespace, podName string) (*corev1.Pod, error)
	getPodMetrics     func(ctx context.Context, namespace, podName string) (*metrics.PodMetrics, error)
	queueUpdate       func(func())
	getTerminalHeight func() int
}

// NewDetailPanel creates a new container detail panel
func NewDetailPanel() *DetailPanel {
	p := &DetailPanel{
		root:      tview.NewFlex().SetDirection(tview.FlexRow),
		following: true,  // Default to following
		wrapText:  true,  // Default to wrapped
		tailLines: 1000,  // Default tail lines
	}

	p.setupInputCapture()
	return p
}

// SetOnBack sets the callback for when user wants to go back
func (p *DetailPanel) SetOnBack(callback func()) {
	p.onBack = callback
}

// SetOnShowSpec sets the callback for when user wants to view container spec
func (p *DetailPanel) SetOnShowSpec(callback func(namespace, podName, containerName string, containerSpec *corev1.Container)) {
	p.onShowSpec = callback
}

// SetAppFocus sets the function to change application focus
func (p *DetailPanel) SetAppFocus(fn func(p tview.Primitive)) {
	p.setAppFocus = fn
}

// SetLogStreamFunc sets the function to get log streams
func (p *DetailPanel) SetLogStreamFunc(fn func(ctx context.Context, namespace, podName string, opts k8s.LogOptions) (io.ReadCloser, error)) {
	p.getLogStream = fn
}

// SetGetPodFunc sets the function to fetch pod data
func (p *DetailPanel) SetGetPodFunc(fn func(ctx context.Context, namespace, podName string) (*corev1.Pod, error)) {
	p.getPod = fn
}

// SetQueueUpdateFunc sets the function for queuing UI updates
func (p *DetailPanel) SetQueueUpdateFunc(fn func(func())) {
	p.queueUpdate = fn
}

// SetGetPodMetricsFunc sets the function to fetch pod metrics
func (p *DetailPanel) SetGetPodMetricsFunc(fn func(ctx context.Context, namespace, podName string) (*metrics.PodMetrics, error)) {
	p.getPodMetrics = fn
}

// SetGetTerminalHeightFunc sets the function to get actual terminal height
// Deprecated: This is no longer used for layout decisions which now use panel height directly.
// Kept for backward compatibility.
func (p *DetailPanel) SetGetTerminalHeightFunc(fn func() int) {
	p.getTerminalHeight = fn
}

// GetRootView returns the root view for this panel
func (p *DetailPanel) GetRootView() tview.Primitive {
	return p.root
}

// ShowContainer displays detail and logs for the specified container
func (p *DetailPanel) ShowContainer(namespace, podName, containerName string) {
	p.streamMu.Lock()

	// Cancel any existing stream
	if p.cancelFunc != nil {
		p.cancelFunc()
		p.cancelFunc = nil
	}

	p.namespace = namespace
	p.podName = podName
	p.containerName = containerName
	p.lineCount = 0
	p.cpuUsage = ""
	p.memUsage = ""

	p.streamMu.Unlock()

	// Build UI if not laid out
	if !p.laidout {
		p.buildLayout()
		p.laidout = true
	}

	// Check if terminal size changed and rebuild layout if needed
	p.checkAndRebuildLayout()

	// Fetch pod data to get container spec and status
	p.fetchContainerData()

	// Update title
	p.updateTitle()

	// Only draw info header if it's visible (hidden at panel height ≤31, which corresponds to terminal ≤35)
	_, _, _, panelHeight := p.root.GetRect()
	if panelHeight > 31 {
		p.drawInfoHeader()
	}

	// Draw container detail section
	p.drawContainerDetail()

	// Update log control panel
	p.updateLogControlPanel()

	// Clear logs view and start streaming
	p.logsView.Clear()
	p.startLogStream(false)
}

func (p *DetailPanel) buildLayout() {
	// === Info Header Panel (3 rows) ===
	p.infoHeaderPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
	p.infoHeaderPanel.SetBorder(true)
	p.infoHeaderPanel.SetTitle(" Info ")
	p.infoHeaderPanel.SetTitleAlign(tview.AlignLeft)
	p.infoHeaderPanel.SetBorderColor(tcell.ColorLightGray)

	// === Container Detail Section (12 rows) ===
	p.containerDetailPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
	p.containerDetailPanel.SetBorder(true)
	p.containerDetailPanel.SetTitle(" Container Detail ")
	p.containerDetailPanel.SetTitleAlign(tview.AlignCenter)
	p.containerDetailPanel.SetBorderColor(tcell.ColorLightGray)

	// Left column: Container Info table
	p.leftDetailTable = tview.NewTable()
	p.leftDetailTable.SetBorder(false)
	p.leftDetailTable.SetBorders(false)
	p.leftDetailTable.SetSelectable(false, false)

	// Center column: State Details table
	p.centerDetailTable = tview.NewTable()
	p.centerDetailTable.SetBorder(false)
	p.centerDetailTable.SetBorders(false)
	p.centerDetailTable.SetSelectable(false, false)

	// Right column: Probes & Resources table
	p.rightDetailTable = tview.NewTable()
	p.rightDetailTable.SetBorder(false)
	p.rightDetailTable.SetBorders(false)
	p.rightDetailTable.SetSelectable(false, false)

	p.containerDetailPanel.AddItem(p.leftDetailTable, 0, 1, false)
	p.containerDetailPanel.AddItem(p.centerDetailTable, 0, 1, false)
	p.containerDetailPanel.AddItem(p.rightDetailTable, 0, 1, false)

	// === Log Control Panel (3 rows) ===
	p.logControlPanel = tview.NewFlex().SetDirection(tview.FlexColumn)
	p.logControlPanel.SetBorder(true)
	p.logControlPanel.SetTitle(" Log Control ")
	p.logControlPanel.SetTitleAlign(tview.AlignLeft)
	p.logControlPanel.SetBorderColor(tcell.ColorLightGray)

	// === Logs View (remaining space) ===
	p.logsView = tview.NewTextView()
	p.logsView.SetDynamicColors(true)
	p.logsView.SetScrollable(true)
	p.logsView.SetWrap(p.wrapText)
	p.logsView.SetBorder(true)
	p.logsView.SetTitle(" Logs ")
	p.logsView.SetTitleAlign(tview.AlignLeft)
	p.logsView.SetBorderColor(tcell.ColorLightGray)

	// Assemble main layout with dynamic heights
	// Use default height (50) during initial layout since root may not be rendered yet
	p.root.Clear()
	terminalHeight := 50 // Default to medium during initial layout
	heights := p.calculatePanelHeights(terminalHeight)
	p.lastHeightCategory = ui.GetHeightCategory(terminalHeight)
	p.lastInfoHeaderHeight = heights.infoHeader

	p.root.AddItem(p.infoHeaderPanel, heights.infoHeader, 0, false)
	p.root.AddItem(p.containerDetailPanel, heights.containerDetail, 0, false)
	p.root.AddItem(p.logControlPanel, heights.logControl, 0, false)
	p.root.AddItem(p.logsView, 0, 1, true) // Logs: remaining space (flex)

	p.root.SetBorder(true)
	p.root.SetTitleAlign(tview.AlignCenter)

	// Set up focusable items for Tab cycling
	// Index 0: Container Detail panel, Index 1: Logs view
	p.focusableItems = []tview.Primitive{p.containerDetailPanel, p.logsView}
	p.focusablePanels = []*tview.Flex{p.containerDetailPanel, nil} // logsView is TextView, not Flex
	p.focusedChildIdx = 1                                          // Default focus on logs

	// Initialize focus visuals
	p.updateFocusVisuals()
}

func (p *DetailPanel) fetchContainerData() {
	if p.getPod == nil {
		return
	}

	ctx := context.Background()
	pod, err := p.getPod(ctx, p.namespace, p.podName)
	if err != nil {
		return
	}
	p.pod = pod

	// Find container spec
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == p.containerName {
			p.containerSpec = &pod.Spec.Containers[i]
			break
		}
	}
	// Check init containers if not found
	if p.containerSpec == nil {
		for i := range pod.Spec.InitContainers {
			if pod.Spec.InitContainers[i].Name == p.containerName {
				p.containerSpec = &pod.Spec.InitContainers[i]
				break
			}
		}
	}

	// Find container status
	for i := range pod.Status.ContainerStatuses {
		if pod.Status.ContainerStatuses[i].Name == p.containerName {
			p.containerStatus = &pod.Status.ContainerStatuses[i]
			break
		}
	}
	// Check init container statuses if not found
	if p.containerStatus == nil {
		for i := range pod.Status.InitContainerStatuses {
			if pod.Status.InitContainerStatuses[i].Name == p.containerName {
				p.containerStatus = &pod.Status.InitContainerStatuses[i]
				break
			}
		}
	}

	// Fetch container metrics
	p.cpuUsage = ""
	p.memUsage = ""
	if p.getPodMetrics != nil {
		if podMetrics, err := p.getPodMetrics(ctx, p.namespace, p.podName); err == nil && podMetrics != nil {
			// Try actual container name first
			for _, cm := range podMetrics.Containers {
				if cm.Name == p.containerName {
					if cm.CPUUsage != nil {
						p.cpuUsage = fmt.Sprintf("%dm", cm.CPUUsage.MilliValue())
					}
					if cm.MemoryUsage != nil {
						p.memUsage = strings.TrimSpace(ui.FormatMemory(cm.MemoryUsage))
					}
					break
				}
			}
			// Fallback to "main" for single-container static pods (aggregate metrics)
			if p.cpuUsage == "" && p.memUsage == "" && p.pod != nil && len(p.pod.Spec.Containers) == 1 {
				for _, cm := range podMetrics.Containers {
					if cm.Name == "main" {
						if cm.CPUUsage != nil {
							p.cpuUsage = fmt.Sprintf("%dm", cm.CPUUsage.MilliValue())
						}
						if cm.MemoryUsage != nil {
							p.memUsage = strings.TrimSpace(ui.FormatMemory(cm.MemoryUsage))
						}
						break
					}
				}
			}
		}
	}
}

func (p *DetailPanel) updateTitle() {
	// Use breadcrumb navigation format: Pods > pod-name > container-name
	p.root.SetTitle(fmt.Sprintf(" %s Pods > %s > [::b]%s[::] ", ui.Icons.Drum, p.podName, p.containerName))
}

// drawInfoHeader draws the condensed info header row (similar to Pod detail)
func (p *DetailPanel) drawInfoHeader() {
	p.infoHeaderPanel.Clear()

	// Get status
	status := "Unknown"
	if p.containerStatus != nil {
		if p.containerStatus.State.Running != nil {
			status = "Running"
		} else if p.containerStatus.State.Waiting != nil {
			status = p.containerStatus.State.Waiting.Reason
			if status == "" {
				status = "Waiting"
			}
		} else if p.containerStatus.State.Terminated != nil {
			status = p.containerStatus.State.Terminated.Reason
			if status == "" {
				status = "Terminated"
			}
		}
	}

	// Get node name
	nodeName := "n/a"
	if p.pod != nil {
		nodeName = p.pod.Spec.NodeName
	}

	// Get pod IP
	podIP := "n/a"
	if p.pod != nil && p.pod.Status.PodIP != "" {
		podIP = p.pod.Status.PodIP
	}

	// Calculate age from container start time or pod creation
	age := "n/a"
	if p.containerStatus != nil && p.containerStatus.State.Running != nil && !p.containerStatus.State.Running.StartedAt.IsZero() {
		age = formatDuration(time.Since(p.containerStatus.State.Running.StartedAt.Time))
	} else if p.pod != nil && !p.pod.CreationTimestamp.IsZero() {
		age = formatDuration(time.Since(p.pod.CreationTimestamp.Time))
	}

	// Get restart count
	restarts := 0
	if p.containerStatus != nil {
		restarts = int(p.containerStatus.RestartCount)
	}

	// Build info items
	items := []string{
		fmt.Sprintf("[gray]Status:[white] %s", status),
		fmt.Sprintf("[gray]Node:[white] %s", nodeName),
		fmt.Sprintf("[gray]NS:[white] %s", p.namespace),
		fmt.Sprintf("[gray]IP:[white] %s", podIP),
		fmt.Sprintf("[gray]Age:[white] %s", age),
		fmt.Sprintf("[gray]Restarts:[white] %d", restarts),
	}

	// Create a text view with all items on one line
	infoText := tview.NewTextView()
	infoText.SetDynamicColors(true)
	infoText.SetText("  " + strings.Join(items, "  │  "))

	p.infoHeaderPanel.AddItem(infoText, 0, 1, false)
}

// formatDuration formats a duration into a human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func (p *DetailPanel) drawContainerDetail() {
	p.leftDetailTable.Clear()
	p.centerDetailTable.Clear()
	p.rightDetailTable.Clear()

	// === LEFT COLUMN: Container Info ===
	row := 0
	p.addDetailHeader(p.leftDetailTable, row, "Container Info")
	row++

	if p.containerSpec != nil {
		// Image
		image := p.containerSpec.Image
		if len(image) > 35 {
			image = image[:32] + "..."
		}
		p.addDetailRow(p.leftDetailTable, row, "Image", image)
		row++

		// Image Pull Policy
		p.addDetailRow(p.leftDetailTable, row, "ImgPull", string(p.containerSpec.ImagePullPolicy))
		row++

		// Working Directory
		workDir := p.containerSpec.WorkingDir
		if workDir == "" {
			workDir = "(default)"
		}
		p.addDetailRow(p.leftDetailTable, row, "WorkDir", workDir)
		row++

		// Command
		cmd := "(default)"
		if len(p.containerSpec.Command) > 0 {
			cmd = strings.Join(p.containerSpec.Command, " ")
			if len(cmd) > 30 {
				cmd = cmd[:27] + "..."
			}
		}
		p.addDetailRow(p.leftDetailTable, row, "Command", cmd)
		row++

		// Args
		args := "(none)"
		if len(p.containerSpec.Args) > 0 {
			args = strings.Join(p.containerSpec.Args, " ")
			if len(args) > 30 {
				args = args[:27] + "..."
			}
		}
		p.addDetailRow(p.leftDetailTable, row, "Args", args)
		row++

		// Ports
		if len(p.containerSpec.Ports) > 0 {
			var ports []string
			for _, port := range p.containerSpec.Ports {
				portStr := fmt.Sprintf("%d/%s", port.ContainerPort, port.Protocol)
				if port.Name != "" {
					portStr = fmt.Sprintf("%s(%s)", portStr, port.Name)
				}
				ports = append(ports, portStr)
			}
			portsStr := strings.Join(ports, ", ")
			if len(portsStr) > 30 {
				portsStr = portsStr[:27] + "..."
			}
			p.addDetailRow(p.leftDetailTable, row, "Ports", portsStr)
		} else {
			p.addDetailRow(p.leftDetailTable, row, "Ports", "(none)")
		}
		row++

		// EnvVars (count only per user request)
		p.addDetailRow(p.leftDetailTable, row, "EnvVars", fmt.Sprintf("%d", len(p.containerSpec.Env)))
		row++

		// VolMounts (count only)
		p.addDetailRow(p.leftDetailTable, row, "VolMounts", fmt.Sprintf("%d", len(p.containerSpec.VolumeMounts)))
	}

	// === CENTER COLUMN: State Details ===
	row = 0
	p.addDetailHeader(p.centerDetailTable, row, "State Details")
	row++

	if p.containerStatus != nil {
		// State
		state := "Unknown"
		stateColor := tcell.ColorGray
		if p.containerStatus.State.Running != nil {
			state = "Running"
			stateColor = tcell.ColorGreen
		} else if p.containerStatus.State.Waiting != nil {
			state = "Waiting"
			stateColor = tcell.ColorYellow
		} else if p.containerStatus.State.Terminated != nil {
			state = "Terminated"
			stateColor = tcell.ColorRed
		}
		p.addDetailRowColor(p.centerDetailTable, row, "State", state, stateColor)
		row++

		// Started At
		startedAt := "n/a"
		if p.containerStatus.State.Running != nil && !p.containerStatus.State.Running.StartedAt.IsZero() {
			startedAt = p.containerStatus.State.Running.StartedAt.Format("2006-01-02 15:04")
		}
		p.addDetailRow(p.centerDetailTable, row, "Started", startedAt)
		row++

		// Ready
		readyStr := "No"
		readyColor := tcell.ColorRed
		if p.containerStatus.Ready {
			readyStr = "Yes"
			readyColor = tcell.ColorGreen
		}
		p.addDetailRowColor(p.centerDetailTable, row, "Ready", readyStr, readyColor)
		row++

		// Restarts
		restartColor := tcell.ColorGreen
		if p.containerStatus.RestartCount > 0 {
			restartColor = tcell.ColorYellow
		}
		if p.containerStatus.RestartCount > 5 {
			restartColor = tcell.ColorRed
		}
		p.addDetailRowColor(p.centerDetailTable, row, "Restarts", fmt.Sprintf("%d", p.containerStatus.RestartCount), restartColor)
		row++

		// ExitCode (if terminated)
		exitCode := "n/a"
		if p.containerStatus.State.Terminated != nil {
			exitCode = fmt.Sprintf("%d", p.containerStatus.State.Terminated.ExitCode)
		} else if p.containerStatus.LastTerminationState.Terminated != nil {
			exitCode = fmt.Sprintf("%d (last)", p.containerStatus.LastTerminationState.Terminated.ExitCode)
		}
		p.addDetailRow(p.centerDetailTable, row, "ExitCode", exitCode)
		row++

		// Reason
		reason := "n/a"
		if p.containerStatus.State.Waiting != nil && p.containerStatus.State.Waiting.Reason != "" {
			reason = p.containerStatus.State.Waiting.Reason
		} else if p.containerStatus.State.Terminated != nil && p.containerStatus.State.Terminated.Reason != "" {
			reason = p.containerStatus.State.Terminated.Reason
		}
		if len(reason) > 25 {
			reason = reason[:22] + "..."
		}
		p.addDetailRow(p.centerDetailTable, row, "Reason", reason)
		row++

		// Message
		message := "n/a"
		if p.containerStatus.State.Waiting != nil && p.containerStatus.State.Waiting.Message != "" {
			message = p.containerStatus.State.Waiting.Message
		} else if p.containerStatus.State.Terminated != nil && p.containerStatus.State.Terminated.Message != "" {
			message = p.containerStatus.State.Terminated.Message
		}
		if len(message) > 25 {
			message = message[:22] + "..."
		}
		p.addDetailRow(p.centerDetailTable, row, "Message", message)
	}

	// === RIGHT COLUMN: Probes & Resources ===
	row = 0
	p.addDetailHeader(p.rightDetailTable, row, "Probes")
	row++

	if p.containerSpec != nil {
		// Liveness Probe
		p.addDetailRow(p.rightDetailTable, row, "Liveness", p.formatProbe(p.containerSpec.LivenessProbe))
		row++

		// Readiness Probe
		p.addDetailRow(p.rightDetailTable, row, "Readiness", p.formatProbe(p.containerSpec.ReadinessProbe))
		row++

		// Startup Probe
		p.addDetailRow(p.rightDetailTable, row, "Startup", p.formatProbe(p.containerSpec.StartupProbe))
		row++

		// Blank row
		row++

		// Resources header
		p.addDetailHeader(p.rightDetailTable, row, "Resources")
		row++

		// CPU Usage (actual)
		cpuUse := p.cpuUsage
		cpuUseColor := tcell.ColorGreen
		if cpuUse == "" {
			cpuUse = "n/a"
			cpuUseColor = tcell.ColorGray
		}
		p.addDetailRowColor(p.rightDetailTable, row, "CPU Use", cpuUse, cpuUseColor)
		row++

		// Mem Usage (actual)
		memUse := p.memUsage
		memUseColor := tcell.ColorGreen
		if memUse == "" {
			memUse = "n/a"
			memUseColor = tcell.ColorGray
		}
		p.addDetailRowColor(p.rightDetailTable, row, "Mem Use", memUse, memUseColor)
		row++

		// CPU Request
		cpuReq := "n/a"
		if p.containerSpec.Resources.Requests.Cpu() != nil && !p.containerSpec.Resources.Requests.Cpu().IsZero() {
			cpuReq = fmt.Sprintf("%dm", p.containerSpec.Resources.Requests.Cpu().MilliValue())
		}
		p.addDetailRow(p.rightDetailTable, row, "CPU Req", cpuReq)
		row++

		// CPU Limit
		cpuLim := "n/a"
		if p.containerSpec.Resources.Limits.Cpu() != nil && !p.containerSpec.Resources.Limits.Cpu().IsZero() {
			cpuLim = fmt.Sprintf("%dm", p.containerSpec.Resources.Limits.Cpu().MilliValue())
		}
		p.addDetailRow(p.rightDetailTable, row, "CPU Lim", cpuLim)
		row++

		// Memory Request
		memReq := "n/a"
		if p.containerSpec.Resources.Requests.Memory() != nil && !p.containerSpec.Resources.Requests.Memory().IsZero() {
			memReq = ui.FormatBytes(p.containerSpec.Resources.Requests.Memory().Value())
		}
		p.addDetailRow(p.rightDetailTable, row, "Mem Req", memReq)
		row++

		// Memory Limit
		memLim := "n/a"
		if p.containerSpec.Resources.Limits.Memory() != nil && !p.containerSpec.Resources.Limits.Memory().IsZero() {
			memLim = ui.FormatBytes(p.containerSpec.Resources.Limits.Memory().Value())
		}
		p.addDetailRow(p.rightDetailTable, row, "Mem Lim", memLim)
	}
}

func (p *DetailPanel) formatProbe(probe *corev1.Probe) string {
	if probe == nil {
		return "None"
	}

	if probe.HTTPGet != nil {
		port := probe.HTTPGet.Port.String()
		return fmt.Sprintf("HTTP:%s%s", port, probe.HTTPGet.Path)
	}
	if probe.TCPSocket != nil {
		return fmt.Sprintf("TCP:%s", probe.TCPSocket.Port.String())
	}
	if probe.Exec != nil {
		cmd := strings.Join(probe.Exec.Command, " ")
		if len(cmd) > 20 {
			cmd = cmd[:17] + "..."
		}
		return fmt.Sprintf("Exec:%s", cmd)
	}
	if probe.GRPC != nil {
		return fmt.Sprintf("GRPC:%d", probe.GRPC.Port)
	}

	return "Configured"
}

func (p *DetailPanel) addDetailHeader(table *tview.Table, row int, title string) {
	table.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf("[::b]%s[::-]", title)).SetTextColor(tcell.ColorAqua).SetSelectable(false))
}

func (p *DetailPanel) addDetailRow(table *tview.Table, row int, key, value string) {
	paddedKey := fmt.Sprintf("%-10s", key)
	table.SetCell(row, 0, tview.NewTableCell(paddedKey).SetTextColor(tcell.ColorGray).SetSelectable(false))
	table.SetCell(row, 1, tview.NewTableCell(value).SetTextColor(tcell.ColorWhite).SetSelectable(false))
}

func (p *DetailPanel) addDetailRowColor(table *tview.Table, row int, key, value string, color tcell.Color) {
	paddedKey := fmt.Sprintf("%-10s", key)
	table.SetCell(row, 0, tview.NewTableCell(paddedKey).SetTextColor(tcell.ColorGray).SetSelectable(false))
	table.SetCell(row, 1, tview.NewTableCell(value).SetTextColor(color).SetSelectable(false))
}

func (p *DetailPanel) updateLogControlPanel() {
	p.logControlPanel.Clear()

	// Container info
	containerInfo := tview.NewTextView()
	containerInfo.SetDynamicColors(true)
	containerInfo.SetText(fmt.Sprintf("[yellow]Container:[white] %s", p.containerName))

	// Pod info
	podInfo := tview.NewTextView()
	podInfo.SetDynamicColors(true)
	podInfo.SetText(fmt.Sprintf("[yellow]Pod:[white] %s", p.podName))

	// Namespace info
	nsInfo := tview.NewTextView()
	nsInfo.SetDynamicColors(true)
	nsInfo.SetText(fmt.Sprintf("[yellow]NS:[white] %s", p.namespace))

	// Lines info
	linesInfo := tview.NewTextView()
	linesInfo.SetDynamicColors(true)
	linesInfo.SetText(fmt.Sprintf("[yellow]Lines:[white] %d", p.lineCount))

	// Status info
	statusInfo := tview.NewTextView()
	statusInfo.SetDynamicColors(true)
	followStatus := "[gray]PAUSED[-]"
	if p.following {
		followStatus = "[green]FOLLOWING[-]"
	}
	statusInfo.SetText(followStatus)

	// Shortcuts
	shortcuts := tview.NewTextView()
	shortcuts.SetDynamicColors(true)
	shortcuts.SetTextAlign(tview.AlignRight)
	shortcuts.SetText("[yellow]Tab[white]:select [yellow]f[white]:follow [yellow]p[white]:prev [yellow]t[white]:time [yellow]w[white]:wrap [yellow]g/G[white]:top/btm [yellow]ESC[white]:back")

	p.logControlPanel.AddItem(containerInfo, 0, 1, false)
	p.logControlPanel.AddItem(podInfo, 0, 1, false)
	p.logControlPanel.AddItem(nsInfo, 0, 1, false)
	p.logControlPanel.AddItem(linesInfo, 12, 0, false)
	p.logControlPanel.AddItem(statusInfo, 12, 0, false)
	p.logControlPanel.AddItem(shortcuts, 0, 2, false)
}

func (p *DetailPanel) setupInputCapture() {
	p.root.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			p.stopStream()
			if p.onBack != nil {
				p.onBack()
			}
			return nil

		case tcell.KeyTab:
			p.cycleFocus()
			return nil

		case tcell.KeyEnter:
			// Enter shows spec when Container Detail panel is focused
			if p.focusedChildIdx == 0 {
				p.showSpec()
				return nil
			}

		case tcell.KeyRune:
			switch event.Rune() {
			case 's', 'S':
				// Show spec when Container Detail panel is focused
				if p.focusedChildIdx == 0 {
					p.showSpec()
					return nil
				}
			case 'f', 'F':
				p.toggleFollow()
				return nil
			case 'p', 'P':
				p.showPreviousLogs()
				return nil
			case 't', 'T':
				p.toggleTimestamps()
				return nil
			case 'w', 'W':
				p.toggleWrap()
				return nil
			case 'g':
				p.logsView.ScrollToBeginning()
				return nil
			case 'G':
				p.logsView.ScrollToEnd()
				return nil
			}
		}
		return event
	})
}

func (p *DetailPanel) toggleFollow() {
	p.streamMu.Lock()
	p.following = !p.following
	p.streamMu.Unlock()

	if p.following {
		p.startLogStream(false)
	} else {
		p.stopStream()
	}

	p.updateLogControlPanel()
	p.queueRedraw()
}

func (p *DetailPanel) showPreviousLogs() {
	p.stopStream()
	p.logsView.Clear()
	p.lineCount = 0
	p.startLogStream(true)
}

func (p *DetailPanel) toggleTimestamps() {
	p.streamMu.Lock()
	p.timestamps = !p.timestamps
	p.streamMu.Unlock()

	p.stopStream()
	p.logsView.Clear()
	p.lineCount = 0
	p.startLogStream(false)
}

func (p *DetailPanel) toggleWrap() {
	p.wrapText = !p.wrapText
	p.logsView.SetWrap(p.wrapText)
	p.queueRedraw()
}

func (p *DetailPanel) startLogStream(previous bool) {
	if p.getLogStream == nil {
		p.appendLog("[red]Error: log stream function not configured")
		return
	}

	p.streamMu.Lock()
	if p.cancelFunc != nil {
		p.cancelFunc()
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancelFunc = cancel

	opts := k8s.LogOptions{
		Container:  p.containerName,
		Follow:     p.following && !previous,
		Previous:   previous,
		Timestamps: p.timestamps,
		TailLines:  p.tailLines,
	}

	namespace := p.namespace
	podName := p.podName
	p.streamMu.Unlock()

	go func() {
		stream, err := p.getLogStream(ctx, namespace, podName, opts)
		if err != nil {
			p.appendLog(fmt.Sprintf("[red]Error getting logs: %v", err))
			return
		}
		defer stream.Close()

		scanner := bufio.NewScanner(stream)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := scanner.Text()
				p.appendLog(p.formatLogLine(line))
			}
		}

		if err := scanner.Err(); err != nil {
			select {
			case <-ctx.Done():
				// Context cancelled, don't report error
			default:
				p.appendLog(fmt.Sprintf("[yellow]Stream ended: %v", err))
			}
		}
	}()
}

func (p *DetailPanel) stopStream() {
	p.streamMu.Lock()
	defer p.streamMu.Unlock()

	if p.cancelFunc != nil {
		p.cancelFunc()
		p.cancelFunc = nil
	}
}

func (p *DetailPanel) formatLogLine(line string) string {
	// Color timestamps if present (RFC3339 format: 2024-01-15T10:30:00Z)
	if p.timestamps && len(line) > 30 && line[4] == '-' && line[7] == '-' {
		if idx := strings.Index(line, " "); idx > 0 && idx < 35 {
			timestamp := line[:idx]
			rest := line[idx+1:]
			return fmt.Sprintf("[gray]%s[white] %s", timestamp, rest)
		}
	}

	// Color error/warning lines
	lineLower := strings.ToLower(line)
	if strings.Contains(lineLower, "error") || strings.Contains(lineLower, "fatal") {
		return "[red]" + line
	}
	if strings.Contains(lineLower, "warn") {
		return "[yellow]" + line
	}

	return line
}

func (p *DetailPanel) appendLog(line string) {
	p.streamMu.Lock()
	p.lineCount++
	count := p.lineCount
	following := p.following
	p.streamMu.Unlock()

	if p.queueUpdate != nil {
		p.queueUpdate(func() {
			fmt.Fprintln(p.logsView, line)
			if following {
				p.logsView.ScrollToEnd()
			}
			// Update line count in control panel periodically
			if count%100 == 0 {
				p.updateLogControlPanel()
			}
		})
	}
}

func (p *DetailPanel) queueRedraw() {
	if p.queueUpdate != nil {
		p.queueUpdate(func() {
			// Just trigger a redraw
		})
	}
}

// cycleFocus cycles focus between Container Detail and Logs panels
func (p *DetailPanel) cycleFocus() {
	if len(p.focusableItems) == 0 {
		return
	}

	p.focusedChildIdx = (p.focusedChildIdx + 1) % len(p.focusableItems)
	p.updateFocusVisuals()

	// Set application focus to the new item
	if p.setAppFocus != nil {
		p.setAppFocus(p.focusableItems[p.focusedChildIdx])
	}
}

// updateFocusVisuals updates border colors to indicate focused panel
func (p *DetailPanel) updateFocusVisuals() {
	// Container Detail panel: dodgerblue when focused, lightgray otherwise
	if p.containerDetailPanel != nil {
		if p.focusedChildIdx == 0 {
			p.containerDetailPanel.SetBorderColor(tcell.ColorDodgerBlue)
			p.containerDetailPanel.SetTitle(" Container Detail [yellow][s][white]:spec ")
		} else {
			p.containerDetailPanel.SetBorderColor(tcell.ColorLightGray)
			p.containerDetailPanel.SetTitle(" Container Detail ")
		}
	}

	// Logs view: dodgerblue when focused, lightgray otherwise
	if p.logsView != nil {
		if p.focusedChildIdx == 1 {
			p.logsView.SetBorderColor(tcell.ColorDodgerBlue)
		} else {
			p.logsView.SetBorderColor(tcell.ColorLightGray)
		}
	}
}

// showSpec triggers the spec view if container detail is focused
func (p *DetailPanel) showSpec() {
	if p.focusedChildIdx != 0 {
		return // Only works when Container Detail panel is focused
	}

	if p.onShowSpec != nil && p.containerSpec != nil {
		p.onShowSpec(p.namespace, p.podName, p.containerName, p.containerSpec)
	}
}

// Cleanup stops any active streams
func (p *DetailPanel) Cleanup() {
	p.stopStream()
}

// SetFollowing sets the follow mode
func (p *DetailPanel) SetFollowing(follow bool) {
	p.streamMu.Lock()
	p.following = follow
	p.streamMu.Unlock()
}

// SetTailLines sets the number of tail lines to fetch
func (p *DetailPanel) SetTailLines(lines int64) {
	p.tailLines = lines
}

// Focus returns focus handling hint
func (p *DetailPanel) Focus() {
	// logsView handles focus
}

// RefreshData refreshes the container data without restarting log stream
func (p *DetailPanel) RefreshData() {
	// Check if terminal size category changed and rebuild layout if needed
	p.checkAndRebuildLayout()

	p.fetchContainerData()
	p.updateTitle()

	// Only draw info header if it's visible (hidden at panel height ≤31, which corresponds to terminal ≤35)
	_, _, _, panelHeight := p.root.GetRect()
	if panelHeight > 31 {
		p.drawInfoHeader()
	}

	p.drawContainerDetail()
	p.updateLogControlPanel()
}

// UpdateMetrics updates the CPU and memory usage values and redraws.
// This is called from the main goroutine with pre-fetched values.
func (p *DetailPanel) UpdateMetrics(cpuUsage, memUsage string) {
	// Check if terminal size changed and rebuild layout if needed
	p.checkAndRebuildLayout()

	p.cpuUsage = cpuUsage
	p.memUsage = memUsage

	// Draw info header if visible (panel height > 31)
	_, _, _, panelHeight := p.root.GetRect()
	if panelHeight > 31 {
		p.drawInfoHeader()
	}

	p.drawContainerDetail()
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
	p.stopStream()
	if p.onBack != nil {
		p.onBack()
		return true
	}
	return false
}

// containerDetailHeights holds panel heights for container detail view
type containerDetailHeights struct {
	infoHeader      int
	containerDetail int
	logControl      int
}

// calculatePanelHeights returns panel heights based on panel height
// At panel height ≤31: compact layout (no info header)
// Note: Panel height is ~4-5 less than terminal height due to ktop header/footer overhead
func (p *DetailPanel) calculatePanelHeights(panelHeight int) containerDetailHeights {
	// Compact layout when panel height ≤31 (corresponds to terminal ≤35-36)
	if panelHeight <= 31 {
		return containerDetailHeights{infoHeader: 0, containerDetail: 8, logControl: 3}
	}

	// Normal layout for larger panels
	switch ui.GetHeightCategory(panelHeight) {
	case ui.HeightCategorySmall:
		return containerDetailHeights{infoHeader: 3, containerDetail: 8, logControl: 3}
	case ui.HeightCategoryMedium:
		return containerDetailHeights{infoHeader: 3, containerDetail: 10, logControl: 3}
	default:
		return containerDetailHeights{infoHeader: 3, containerDetail: 12, logControl: 3}
	}
}

// checkAndRebuildLayout checks if terminal size category changed and rebuilds layout if needed
func (p *DetailPanel) checkAndRebuildLayout() {
	// Get actual dimensions - don't rebuild until we have real values
	_, _, _, height := p.root.GetRect()
	if height <= 0 {
		// Panel not rendered yet, skip rebuild - initial layout will be used
		return
	}

	panelHeight := height // Use actual height from GetRect
	currentCategory := ui.GetHeightCategory(panelHeight)
	heights := p.calculatePanelHeights(panelHeight)

	// Only rebuild if height category or info header height changed
	if currentCategory == p.lastHeightCategory && heights.infoHeader == p.lastInfoHeaderHeight {
		return
	}

	// Clear and rebuild the flex layout
	p.root.Clear()
	if heights.infoHeader > 0 {
		p.root.AddItem(p.infoHeaderPanel, heights.infoHeader, 0, false)
	}
	p.root.AddItem(p.containerDetailPanel, heights.containerDetail, 0, false)
	p.root.AddItem(p.logControlPanel, heights.logControl, 0, false)
	p.root.AddItem(p.logsView, 0, 1, true)

	p.lastHeightCategory = currentCategory
	p.lastInfoHeaderHeight = heights.infoHeader
}
