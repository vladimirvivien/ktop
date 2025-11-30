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
	cpuView   *tview.TextView // CPU sparkline
	memView   *tview.TextView // MEM sparkline
	children  []tview.Primitive

	// Stateful sparklines for smooth sliding animation
	cpuSparkline *ui.SparklineState
	memSparkline *ui.SparklineState

	// Prometheus-only views (conditionally shown)
	netView       *tview.TextView // Network I/O sparkline
	diskView      *tview.TextView // Disk I/O sparkline
	enhancedStats *tview.TextView // Enhanced stats row

	// Prometheus-only sparklines
	netSparkline  *ui.SparklineState
	diskSparkline *ui.SparklineState

	// Layout mode
	prometheusMode bool
}

func NewClusterSummaryPanel(app *application.Application, title string) ui.Panel {
	p := &clusterSummaryPanel{app: app, title: title}
	p.Layout(nil)
	p.children = append(p.children, p.cpuView, p.memView)
	return p
}

func (p *clusterSummaryPanel) GetTitle() string {
	return p.title
}

// createSparklineView creates a styled text view for sparklines
func (p *clusterSummaryPanel) createSparklineView() *tview.TextView {
	view := tview.NewTextView()
	view.SetDynamicColors(true)
	view.SetTextAlign(tview.AlignCenter)
	view.SetBorder(true)
	view.SetBorderPadding(0, 0, 0, 0)
	view.SetTitleAlign(tview.AlignCenter)
	return view
}

func (p *clusterSummaryPanel) Layout(data interface{}) {
	// Detect metrics source type
	metricsSource := p.app.GetMetricsSource()
	if metricsSource != nil {
		info := metricsSource.GetSourceInfo()
		p.prometheusMode = (info.Type == metrics.SourceTypePrometheus)
	}

	// Stats view (single line, with border for visual separation)
	p.statsView = tview.NewTextView()
	p.statsView.SetDynamicColors(true)
	p.statsView.SetBorder(true)
	p.statsView.SetBorderPadding(0, 0, 0, 0)

	// CPU and MEM sparkline views (always shown)
	p.cpuView = p.createSparklineView()
	p.memView = p.createSparklineView()

	if p.prometheusMode {
		// === PROMETHEUS LAYOUT: 4 sparklines on one row + enhanced stats ===

		// Network and Disk sparkline views
		p.netView = p.createSparklineView()
		p.diskView = p.createSparklineView()

		// All 4 sparklines side-by-side, each 25% width
		graphFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(p.cpuView, 0, 1, false).  // 25%
			AddItem(p.memView, 0, 1, false).  // 25%
			AddItem(p.netView, 0, 1, false).  // 25%
			AddItem(p.diskView, 0, 1, false)  // 25%

		// Enhanced stats row
		p.enhancedStats = tview.NewTextView()
		p.enhancedStats.SetDynamicColors(true)
		p.enhancedStats.SetBorder(true)
		p.enhancedStats.SetBorderPadding(0, 0, 0, 0)

		// Full layout: stats + 4 sparklines + enhanced stats
		p.root = tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(p.statsView, 3, 0, false).     // Stats row (3 rows with border)
			AddItem(graphFlex, 0, 1, false).       // 4 sparklines (flexible)
			AddItem(p.enhancedStats, 3, 0, false)  // Enhanced stats (3 rows with border)
	} else {
		// === METRICS SERVER LAYOUT: 2 sparklines ===

		// Horizontal flex for CPU and MEM sparklines side by side
		graphFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(p.cpuView, 0, 1, false). // 50%
			AddItem(p.memView, 0, 1, false)  // 50%

		// Flex layout: stacks bordered stats and graph vertically
		p.root = tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(p.statsView, 3, 0, false). // 3 rows (1 content + 2 border)
			AddItem(graphFlex, 0, 1, false)    // remaining for sparklines
	}

	p.root.SetBorder(true)
	p.root.SetBorderPadding(0, 0, 0, 0)
	p.root.SetTitle(p.GetTitle())
	p.root.SetTitleAlign(tview.AlignLeft)
}

func (p *clusterSummaryPanel) DrawHeader(data interface{}) {}

func (p *clusterSummaryPanel) DrawBody(data interface{}) {
	// Update title with disconnected state if applicable
	if p.app.IsAPIDisconnected() {
		p.root.SetTitle(fmt.Sprintf("%s [red][DISCONNECTED - Press R to reconnect]", p.GetTitle()))
	} else {
		p.root.SetTitle(p.GetTitle())
	}

	colorKeys := ui.ColorKeys{0: "olivedrab", 40: "yellow", 80: "red"}
	const defaultWidth = 40 // Initial width before container dimensions are known

	// Sparkline height - use actual view height (minus 2 for border)
	_, _, _, viewHeight := p.cpuView.GetRect()
	sparklineHeight := viewHeight - 2 // subtract top/bottom border
	if sparklineHeight < 1 {
		sparklineHeight = 1
	}

	switch summary := data.(type) {
	case model.ClusterSummary:
		var cpuRatio, memRatio ui.Ratio

		// Check if usage metrics are actually available (non-zero)
		hasUsageMetrics := summary.UsageNodeCpuTotal.MilliValue() > 0 || summary.UsageNodeMemTotal.MilliValue() > 0

		// Initialize multi-line sparklines if needed, or recreate if height changed
		if p.cpuSparkline == nil || p.cpuSparkline.Height() != sparklineHeight {
			p.cpuSparkline = ui.NewSparklineStateWithHeight(defaultWidth, sparklineHeight, colorKeys)
		}
		if p.memSparkline == nil || p.memSparkline.Height() != sparklineHeight {
			p.memSparkline = ui.NewSparklineStateWithHeight(defaultWidth, sparklineHeight, colorKeys)
		}

		// Get container width and resize sparklines to fill available space
		_, _, cpuContainerWidth, _ := p.cpuView.GetRect()
		if cpuContainerWidth > 2 {
			p.cpuSparkline.Resize(cpuContainerWidth - 2)
		}
		_, _, memContainerWidth, _ := p.memView.GetRect()
		if memContainerWidth > 2 {
			p.memSparkline.Resize(memContainerWidth - 2)
		}

		// === Stats row ===
		if p.prometheusMode {
			// Prometheus mode: show container count instead of jobs/sets
			statsText := fmt.Sprintf(
				"[yellow]Uptime: [white]%s [yellow]│ Nodes: [white]%d [yellow]│ Pods: [white]%d/%d [yellow]│ Deploys: [white]%d/%d [yellow]│ NS: [white]%d [yellow]│ Containers: [white]%d",
				duration.HumanDuration(time.Since(summary.Uptime.Time)),
				summary.NodesReady,
				summary.PodsRunning, summary.PodsAvailable,
				summary.DeploymentsReady, summary.DeploymentsTotal,
				summary.Namespaces,
				summary.ContainerCount,
			)
			p.statsView.SetText(statsText)
		} else {
			// Metrics Server mode: original stats
			statsText := fmt.Sprintf(
				"[yellow]Uptime: [white]%s [yellow]│ Nodes: [white]%d [yellow]│ Pods: [white]%d/%d [yellow]│ Deploys: [white]%d/%d [yellow]│ NS: [white]%d [yellow]│ Jobs: [white]%d [yellow]│ Sets: [white]r%d d%d s%d",
				duration.HumanDuration(time.Since(summary.Uptime.Time)),
				summary.NodesReady,
				summary.PodsRunning, summary.PodsAvailable,
				summary.DeploymentsReady, summary.DeploymentsTotal,
				summary.Namespaces,
				summary.JobsCount,
				summary.ReplicaSetsReady, summary.DaemonSetsReady, summary.StatefulSetsReady,
			)
			p.statsView.SetText(statsText)
		}

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

		// Push values and render sparklines
		p.cpuSparkline.Push(float64(cpuRatio))
		cpuTrend := p.cpuSparkline.TrendIndicator(float64(cpuRatio) * 100)
		cpuSparklineText := p.cpuSparkline.Render() // Multi-line with \n

		p.memSparkline.Push(float64(memRatio))
		memTrend := p.memSparkline.TrendIndicator(float64(memRatio) * 100)
		memSparklineText := p.memSparkline.Render() // Multi-line with \n

		// CPU view: title + sparkline
		cpuTitle := fmt.Sprintf(" CPU %dm/%dm (%02.1f%% %s) %s ",
			cpuValue, cpuTotal, cpuRatio*100, cpuLabel, cpuTrend)
		p.cpuView.SetTitle(cpuTitle)
		p.cpuView.SetText(cpuSparklineText)

		// MEM view: title + sparkline
		memTitle := fmt.Sprintf(" MEM %s/%s (%02.1f%% %s) %s ",
			memValue, memTotal, memRatio*100, memLabel, memTrend)
		p.memView.SetTitle(memTitle)
		p.memView.SetText(memSparklineText)

		// === Prometheus-only: Network, Disk, Enhanced Stats ===
		if p.prometheusMode {
			p.drawNetworkSparkline(summary, sparklineHeight, colorKeys)
			p.drawDiskSparkline(summary, sparklineHeight, colorKeys)
			p.drawEnhancedStats(summary)
		}

	default:
		panic(fmt.Sprintf("SummaryPanel.DrawBody: unexpected type %T", data))
	}
}

// drawNetworkSparkline renders the network I/O sparkline (Prometheus mode only)
func (p *clusterSummaryPanel) drawNetworkSparkline(summary model.ClusterSummary, height int, colorKeys ui.ColorKeys) {
	if p.netView == nil {
		return
	}

	const defaultWidth = 40

	// Initialize sparkline if needed, or recreate if height changed
	if p.netSparkline == nil || p.netSparkline.Height() != height {
		p.netSparkline = ui.NewSparklineStateWithHeight(defaultWidth, height, colorKeys)
	}

	// Resize to fit container
	_, _, containerWidth, _ := p.netView.GetRect()
	if containerWidth > 2 {
		p.netSparkline.Resize(containerWidth - 2)
	}

	// Format title with rates
	netTitle := fmt.Sprintf(" Net ↓%s ↑%s ",
		ui.FormatBytesRate(summary.NetworkRxRate),
		ui.FormatBytesRate(summary.NetworkTxRate))
	p.netView.SetTitle(netTitle)

	// Normalize combined rate (against 1 Gbps baseline = 128 MB/s)
	combinedRate := summary.NetworkRxRate + summary.NetworkTxRate
	normalized := combinedRate / (128 * 1024 * 1024) // 128 MB/s baseline
	if normalized > 1 {
		normalized = 1
	}
	p.netSparkline.Push(normalized)
	p.netView.SetText(p.netSparkline.Render())
}

// drawDiskSparkline renders the disk I/O sparkline (Prometheus mode only)
func (p *clusterSummaryPanel) drawDiskSparkline(summary model.ClusterSummary, height int, colorKeys ui.ColorKeys) {
	if p.diskView == nil {
		return
	}

	const defaultWidth = 40

	// Initialize sparkline if needed, or recreate if height changed
	if p.diskSparkline == nil || p.diskSparkline.Height() != height {
		p.diskSparkline = ui.NewSparklineStateWithHeight(defaultWidth, height, colorKeys)
	}

	// Resize to fit container
	_, _, containerWidth, _ := p.diskView.GetRect()
	if containerWidth > 2 {
		p.diskSparkline.Resize(containerWidth - 2)
	}

	// Format title with rates
	diskTitle := fmt.Sprintf(" Disk R:%s W:%s ",
		ui.FormatBytesRate(summary.DiskReadRate),
		ui.FormatBytesRate(summary.DiskWriteRate))
	p.diskView.SetTitle(diskTitle)

	// Normalize combined rate (against 500 MB/s baseline)
	combinedRate := summary.DiskReadRate + summary.DiskWriteRate
	normalized := combinedRate / (500 * 1024 * 1024) // 500 MB/s baseline
	if normalized > 1 {
		normalized = 1
	}
	p.diskSparkline.Push(normalized)
	p.diskView.SetText(p.diskSparkline.Render())
}

// drawEnhancedStats renders the enhanced stats row (Prometheus mode only)
func (p *clusterSummaryPanel) drawEnhancedStats(summary model.ClusterSummary) {
	if p.enhancedStats == nil {
		return
	}

	// Color-code health indicators
	restartsColor := "green"
	if summary.ContainerRestarts1h > 10 {
		restartsColor = "red"
	} else if summary.ContainerRestarts1h > 5 {
		restartsColor = "yellow"
	}

	oomColor := "green"
	if summary.OOMKillCount > 0 {
		oomColor = "red"
	}

	pressureColor := "green"
	if summary.NodePressureCount > 0 {
		pressureColor = "red"
	}

	throttledColor := "green"
	if summary.CPUThrottledPercent > 20 {
		throttledColor = "red"
	} else if summary.CPUThrottledPercent > 10 {
		throttledColor = "yellow"
	}

	enhancedText := fmt.Sprintf(
		"[yellow]Restarts: [%s]%d (1h)[yellow] │ OOMKills: [%s]%d[yellow] │ Pressure: [%s]%d[yellow] │ Throttled: [%s]%.1f%%",
		restartsColor, summary.ContainerRestarts1h,
		oomColor, summary.OOMKillCount,
		pressureColor, summary.NodePressureCount,
		throttledColor, summary.CPUThrottledPercent,
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
