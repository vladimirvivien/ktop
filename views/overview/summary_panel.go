package overview

import (
	"fmt"
	"time"

	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/util/duration"
)

type clusterSummaryPanel struct {
	app       *application.Application
	title     string
	root      *tview.Flex     // Outer flex with border
	statsView *tview.TextView // Stats row
	children  []tview.Primitive

	// Sparkline row component for metrics visualization
	sparklineRow *ui.SparklineRow

	// Prometheus-only views (conditionally shown)
	enhancedStats *tview.TextView // Enhanced stats row

	// Layout mode
	prometheusMode bool

	// Dynamic layout tracking
	lastTerminalHeight int
}

func NewClusterSummaryPanel(app *application.Application, title string) ui.Panel {
	p := &clusterSummaryPanel{app: app, title: title}
	p.Layout(nil)
	return p
}

func (p *clusterSummaryPanel) GetTitle() string {
	return p.title
}

func (p *clusterSummaryPanel) Layout(data interface{}) {
	// Detect metrics source type
	metricsSource := p.app.GetMetricsSource()
	if metricsSource != nil {
		info := metricsSource.GetSourceInfo()
		p.prometheusMode = (info.Type == metrics.SourceTypePrometheus)
	}

	// Get actual terminal height from application (not panel height)
	// During initial layout, app may not be fully set up - default to large
	terminalHeight := 50 // Default to medium/large
	if p.app != nil {
		terminalHeight = p.app.GetTerminalHeight()
	}
	isSmallHeight := terminalHeight <= 45 // Match HeightCategorySmall threshold
	p.lastTerminalHeight = terminalHeight

	// Stats view (single line, with border for visual separation)
	// Always create it even if hidden, for transitions to larger sizes
	p.statsView = tview.NewTextView()
	p.statsView.SetDynamicColors(true)
	p.statsView.SetBorder(true)
	p.statsView.SetBorderPadding(0, 0, 0, 0)

	// Initialize sparkline row with prometheus mode
	p.sparklineRow = ui.NewSparklineRow(p.prometheusMode)

	// Create graphFlex to hold sparkline row
	graphFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(p.sparklineRow, 0, 1, false)

	if p.prometheusMode {
		// Enhanced stats row (always create for transitions)
		p.enhancedStats = tview.NewTextView()
		p.enhancedStats.SetDynamicColors(true)
		p.enhancedStats.SetBorder(true)
		p.enhancedStats.SetBorderPadding(0, 0, 0, 0)
	}

	// Create or clear root and rebuild layout
	if p.root == nil {
		p.root = tview.NewFlex().SetDirection(tview.FlexRow)
	} else {
		p.root.Clear()
	}

	// Add items based on height category
	if isSmallHeight {
		// Small: sparklines only (no stats rows)
		p.root.AddItem(graphFlex, 0, 1, false)
	} else if p.prometheusMode {
		// Medium/Large Prometheus: stats + sparklines + enhanced stats
		p.root.AddItem(p.statsView, 3, 0, false)
		p.root.AddItem(graphFlex, 0, 1, false)
		p.root.AddItem(p.enhancedStats, 3, 0, false)
	} else {
		// Medium/Large Metrics Server: stats + sparklines
		p.root.AddItem(p.statsView, 3, 0, false)
		p.root.AddItem(graphFlex, 0, 1, false)
	}

	p.root.SetBorder(true)
	p.root.SetBorderPadding(0, 0, 0, 0)
	p.root.SetTitle(p.GetTitle())
	p.root.SetTitleAlign(tview.AlignLeft)
}

func (p *clusterSummaryPanel) DrawHeader(data interface{}) {}

// checkAndRebuildLayout checks if terminal size changed across the threshold and rebuilds layout if needed
func (p *clusterSummaryPanel) checkAndRebuildLayout() {
	terminalHeight := p.app.GetTerminalHeight()
	isSmallHeight := terminalHeight <= 45
	wasSmallHeight := p.lastTerminalHeight > 0 && p.lastTerminalHeight <= 45

	// Only rebuild if small-height state changed
	if isSmallHeight == wasSmallHeight {
		p.lastTerminalHeight = terminalHeight
		return
	}

	p.lastTerminalHeight = terminalHeight
	p.Layout(nil)
}

func (p *clusterSummaryPanel) DrawBody(data interface{}) {
	// Check if terminal size category changed and rebuild layout if needed
	p.checkAndRebuildLayout()

	// Update title with disconnected state if applicable
	if p.app.IsAPIDisconnected() {
		p.root.SetTitle(fmt.Sprintf("%s [red][DISCONNECTED - Press R to reconnect]", p.GetTitle()))
	} else {
		p.root.SetTitle(p.GetTitle())
	}

	switch summary := data.(type) {
	case model.ClusterSummary:
		var cpuRatio, memRatio ui.Ratio

		// Check if usage metrics are actually available (non-zero)
		hasUsageMetrics := summary.UsageNodeCpuTotal.MilliValue() > 0 || summary.UsageNodeMemTotal.MilliValue() > 0

		// Update prometheus mode on sparkline row
		p.sparklineRow.SetPrometheusMode(p.prometheusMode)

		// === Stats row ===
		// Color code Deploys ratio: >= 80% green, > 50% yellow, <= 50% red
		deploysColor := "green"
		if summary.DeploymentsTotal > 0 {
			deploysRatio := float64(summary.DeploymentsReady) / float64(summary.DeploymentsTotal)
			if deploysRatio <= 0.5 {
				deploysColor = "red"
			} else if deploysRatio < 0.8 {
				deploysColor = "yellow"
			}
		}

		// Color code Pods ratio: >= 80% green, > 50% yellow, <= 50% red
		podsColor := "green"
		if summary.PodsAvailable > 0 {
			podsRatio := float64(summary.PodsRunning) / float64(summary.PodsAvailable)
			if podsRatio <= 0.5 {
				podsColor = "red"
			} else if podsRatio < 0.8 {
				podsColor = "yellow"
			}
		}

		// Build PV/PVC display strings conditionally (show storage only when count > 0)
		pvDisplay := fmt.Sprintf("%d", summary.PVCount)
		if summary.PVCount > 0 {
			pvDisplay = fmt.Sprintf("%d (%s)", summary.PVCount, ui.FormatMemory(summary.PVsTotal))
		}
		pvcDisplay := fmt.Sprintf("%d", summary.PVCCount)
		if summary.PVCCount > 0 {
			pvcDisplay = fmt.Sprintf("%d (%s)", summary.PVCCount, ui.FormatMemory(summary.PVCsTotal))
		}

		// Stats format is the same for both modes
		statsText := fmt.Sprintf(
			"[yellow]Uptime: [white]%s [yellow]│ Nodes: [white]%d [yellow]│ NS: [white]%d [yellow]│ Deploys: [%s]%d/%d [yellow]│ Pods: [%s]%d/%d [yellow]│ Vols: [white]%d [yellow]│ PVs: [white]%s [yellow]│ PVCs: [white]%s",
			duration.HumanDuration(time.Since(summary.Uptime.Time)),
			summary.NodesReady,
			summary.Namespaces,
			deploysColor, summary.DeploymentsReady, summary.DeploymentsTotal,
			podsColor, summary.PodsRunning, summary.PodsAvailable,
			summary.VolumesInUse,
			pvDisplay,
			pvcDisplay,
		)
		p.statsView.SetText(statsText)

		// === CPU and MEM sparklines ===
		var cpuValue, cpuTotal int64
		var memValue, memTotal string
		var cpuLabel, memLabel string

		if !hasUsageMetrics {
			// Fallback mode: show requested resources
			cpuRatio = ui.GetRatio(float64(summary.RequestedPodCpuTotal.MilliValue()), float64(summary.AllocatableNodeCpuTotal.MilliValue()))
			cpuValue = summary.RequestedPodCpuTotal.MilliValue()
			cpuTotal = summary.AllocatableNodeCpuTotal.MilliValue()
			cpuLabel = "requested"

			memRatio = ui.GetRatio(float64(summary.RequestedPodMemTotal.MilliValue()), float64(summary.AllocatableNodeMemTotal.MilliValue()))
			memValue = ui.FormatMemory(summary.RequestedPodMemTotal)
			memTotal = ui.FormatMemory(summary.AllocatableNodeMemTotal)
			memLabel = "requested"
		} else {
			// Show actual usage
			cpuRatio = ui.GetRatio(float64(summary.UsageNodeCpuTotal.MilliValue()), float64(summary.AllocatableNodeCpuTotal.MilliValue()))
			cpuValue = summary.UsageNodeCpuTotal.MilliValue()
			cpuTotal = summary.AllocatableNodeCpuTotal.MilliValue()
			cpuLabel = "used"

			memRatio = ui.GetRatio(float64(summary.UsageNodeMemTotal.MilliValue()), float64(summary.AllocatableNodeMemTotal.MilliValue()))
			memValue = ui.FormatMemory(summary.UsageNodeMemTotal)
			memTotal = ui.FormatMemory(summary.AllocatableNodeMemTotal)
			memLabel = "used"
		}

		// Build sparkline titles
		cpuTrend := p.sparklineRow.CPUTrend(float64(cpuRatio) * 100)
		cpuTitle := fmt.Sprintf(" CPU %dm/%dm (%02.1f%% %s) %s ",
			cpuValue, cpuTotal, cpuRatio*100, cpuLabel, cpuTrend)

		memTrend := p.sparklineRow.MEMTrend(float64(memRatio) * 100)
		memTitle := fmt.Sprintf(" MEM %s/%s (%02.1f%% %s) %s ",
			memValue, memTotal, memRatio*100, memLabel, memTrend)

		// Update sparkline row
		p.sparklineRow.UpdateCPU(float64(cpuRatio), cpuTitle)
		p.sparklineRow.UpdateMEM(float64(memRatio), memTitle)

		// === Prometheus-only: Network, Disk, Enhanced Stats ===
		if p.prometheusMode {
			// Network sparkline
			netTitle := fmt.Sprintf(" Net ↓%s ↑%s ",
				ui.FormatBytesRate(summary.NetworkRxRate),
				ui.FormatBytesRate(summary.NetworkTxRate))
			combinedNetRate := summary.NetworkRxRate + summary.NetworkTxRate
			netNormalized := combinedNetRate / (128 * 1024 * 1024) // 128 MB/s baseline
			if netNormalized > 1 {
				netNormalized = 1
			}
			p.sparklineRow.UpdateNET(netNormalized, netTitle)

			// Disk sparkline
			diskTitle := fmt.Sprintf(" Disk R:%s W:%s ",
				ui.FormatBytesRate(summary.DiskReadRate),
				ui.FormatBytesRate(summary.DiskWriteRate))
			combinedDiskRate := summary.DiskReadRate + summary.DiskWriteRate
			diskNormalized := combinedDiskRate / (500 * 1024 * 1024) // 500 MB/s baseline
			if diskNormalized > 1 {
				diskNormalized = 1
			}
			p.sparklineRow.UpdateDisk(diskNormalized, diskTitle)

			// Enhanced stats
			p.drawEnhancedStats(summary)
		}

	default:
		panic(fmt.Sprintf("SummaryPanel.DrawBody: unexpected type %T", data))
	}
}

// drawEnhancedStats renders the enhanced stats row (Prometheus mode only)
func (p *clusterSummaryPanel) drawEnhancedStats(summary model.ClusterSummary) {
	if p.enhancedStats == nil {
		return
	}

	// Color-code health indicators
	restartsColor := "green"
	if summary.ContainerRestarts > 100 {
		restartsColor = "red"
	} else if summary.ContainerRestarts > 50 {
		restartsColor = "yellow"
	}

	failuresColor := "green"
	if summary.FailedPods > 0 {
		failuresColor = "red"
	}

	evictedColor := "green"
	if summary.EvictedPods > 0 {
		evictedColor = "yellow"
	}

	pressureColor := "green"
	if summary.NodePressureCount > 0 {
		pressureColor = "red"
	}

	enhancedText := fmt.Sprintf(
		"[yellow]Restarts: [%s]%d[yellow] │ Failures: [%s]%d[yellow] │ Evicted: [%s]%d[yellow] │ Pressure: [%s]%d",
		restartsColor, summary.ContainerRestarts,
		failuresColor, summary.FailedPods,
		evictedColor, summary.EvictedPods,
		pressureColor, summary.NodePressureCount,
	)
	p.enhancedStats.SetText(enhancedText)
}

func (p *clusterSummaryPanel) DrawFooter(data interface{}) {}

func (p *clusterSummaryPanel) Clear() {}

func (p *clusterSummaryPanel) GetRootView() tview.Primitive {
	return p.root
}

func (p *clusterSummaryPanel) GetChildrenViews() []tview.Primitive {
	return p.children
}

// SetFocused implements ui.FocusablePanel - updates visual focus state
func (p *clusterSummaryPanel) SetFocused(focused bool) {
	ui.SetFlexFocused(p.root, focused)
}
