package overview

import (
	"fmt"
	"time"

	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/internal/snapshot"
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

	// Adaptive scaling for Net/Disk sparklines
	peakNetRate  float64
	peakDiskRate float64
}

func NewClusterSummaryPanel(app *application.Application, title string) ui.Panel {
	p := &clusterSummaryPanel{app: app, title: title}
	p.Layout(nil)
	return p
}

func (p *clusterSummaryPanel) GetTitle() string {
	return p.title
}

func (p *clusterSummaryPanel) Layout(data any) {
	// Detect metrics source type
	var metricsSource metrics.MetricsSource
	if p.app != nil {
		metricsSource = p.app.GetMetricsSource()
	}
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

func (p *clusterSummaryPanel) DrawHeader(data any) {}

// checkAndRebuildLayout checks if terminal size changed across the threshold and rebuilds layout if needed
func (p *clusterSummaryPanel) checkAndRebuildLayout() {
	if p.app == nil {
		return
	}
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

func (p *clusterSummaryPanel) DrawBody(data any) {
	// Check if terminal size category changed and rebuild layout if needed
	p.checkAndRebuildLayout()

	// Update title with disconnected state if applicable
	if p.app != nil && p.app.IsAPIDisconnected() {
		if ui.IsMonochrome() {
			p.root.SetTitle(fmt.Sprintf("%s %s", p.GetTitle(), ui.ErrorMarkup("[DISCONNECTED - Press R to reconnect]")))
		} else {
			errTag := ui.TagFg(ui.ActivePalette().StatusError)
			p.root.SetTitle(fmt.Sprintf("%s [%s][DISCONNECTED - Press R to reconnect]", p.GetTitle(), errTag))
		}
	} else {
		p.root.SetTitle(p.GetTitle())
	}

	if snap, ok := data.(*snapshot.Snapshot); ok && snap != nil {
		data = snap.Summary.ToClusterSummary(snap.Cluster)
	}

	switch summary := data.(type) {
	case model.ClusterSummary:
		var cpuRatio, memRatio ui.Ratio

		// Evaluate CPU and Memory usage availability independently so available
		// memory gauges do not force missing CPU rates to render as 0.0% used.
		hasCPUUsage := summary.HasUsageCPU || (summary.UsageNodeCpuTotal != nil && summary.UsageNodeCpuTotal.MilliValue() > 0)
		hasMemUsage := summary.HasUsageMem || (summary.UsageNodeMemTotal != nil && summary.UsageNodeMemTotal.MilliValue() > 0)

		// Update prometheus mode on sparkline row
		p.sparklineRow.SetPrometheusMode(p.prometheusMode)

		// === Stats row ===
		pal := ui.ActivePalette()
		mono := ui.IsMonochrome()

		// Color code Deploys ratio: >= 80% ok, 50-80% warning, <= 50% error
		deploysCol := pal.GaugeLow
		if summary.DeploymentsTotal > 0 {
			deploysRatio := float64(summary.DeploymentsReady) / float64(summary.DeploymentsTotal)
			if deploysRatio <= 0.5 {
				deploysCol = pal.GaugeHigh
			} else if deploysRatio < 0.8 {
				deploysCol = pal.GaugeMedium
			}
		}

		// Color code Pods ratio: >= 80% ok, 50-80% warning, <= 50% error
		podsCol := pal.GaugeLow
		if summary.PodsAvailable > 0 {
			podsRatio := float64(summary.PodsRunning) / float64(summary.PodsAvailable)
			if podsRatio <= 0.5 {
				podsCol = pal.GaugeHigh
			} else if podsRatio < 0.8 {
				podsCol = pal.GaugeMedium
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

		var statsText string
		if mono {
			deploysStr := fmt.Sprintf("%d/%d", summary.DeploymentsReady, summary.DeploymentsTotal)
			if deploysCol == pal.GaugeHigh && summary.DeploymentsTotal > 0 {
				deploysStr = ui.ErrorMarkup(deploysStr)
			}
			podsStr := fmt.Sprintf("%d/%d", summary.PodsRunning, summary.PodsAvailable)
			if podsCol == pal.GaugeHigh && summary.PodsAvailable > 0 {
				podsStr = ui.ErrorMarkup(podsStr)
			}
			statsText = fmt.Sprintf(
				"Health: %s │ Uptime: %s │ Nodes: %d │ NS: %d │ Deploys: %s │ Pods: %s │ Vols: %d │ PVs: %s │ PVCs: %s",
				ui.FormatHealthScore(summary.HealthScore),
				duration.HumanDuration(time.Since(summary.Uptime.Time)),
				summary.NodesReady,
				summary.Namespaces,
				deploysStr,
				podsStr,
				summary.VolumesInUse,
				pvDisplay,
				pvcDisplay,
			)
		} else {
			lblTag := ui.TagFg(pal.DataPrimary)
			valTag := ui.TagFg(pal.DataSecondary)
			statsText = fmt.Sprintf(
				"[%s]Health: %s [%s]│ Uptime: [%s]%s [%s]│ Nodes: [%s]%d [%s]│ NS: [%s]%d [%s]│ Deploys: [%s]%d/%d [%s]│ Pods: [%s]%d/%d [%s]│ Vols: [%s]%d [%s]│ PVs: [%s]%s [%s]│ PVCs: [%s]%s",
				lblTag, ui.FormatHealthScore(summary.HealthScore),
				lblTag, valTag, duration.HumanDuration(time.Since(summary.Uptime.Time)),
				lblTag, valTag, summary.NodesReady,
				lblTag, valTag, summary.Namespaces,
				lblTag, ui.TagFg(deploysCol), summary.DeploymentsReady, summary.DeploymentsTotal,
				lblTag, ui.TagFg(podsCol), summary.PodsRunning, summary.PodsAvailable,
				lblTag, valTag, summary.VolumesInUse,
				lblTag, valTag, pvDisplay,
				lblTag, valTag, pvcDisplay,
			)
		}
		p.statsView.SetText(statsText)

		// === CPU and MEM sparklines ===
		var cpuValue, cpuTotal int64
		var memValue, memTotal string
		var cpuLabel, memLabel string

		if summary.AllocatableNodeCpuTotal != nil {
			cpuTotal = summary.AllocatableNodeCpuTotal.MilliValue()
		}
		if hasCPUUsage {
			if summary.UsageNodeCpuTotal != nil {
				cpuValue = summary.UsageNodeCpuTotal.MilliValue()
			}
			cpuLabel = "used"
		} else {
			if summary.RequestedPodCpuTotal != nil {
				cpuValue = summary.RequestedPodCpuTotal.MilliValue()
			}
			cpuLabel = "requested"
		}
		cpuRatio = ui.GetRatio(float64(cpuValue), float64(cpuTotal))

		memTotal = ui.FormatMemory(summary.AllocatableNodeMemTotal)
		if hasMemUsage {
			var memUsedMilli int64
			if summary.UsageNodeMemTotal != nil {
				memUsedMilli = summary.UsageNodeMemTotal.MilliValue()
			}
			var allocMemMilli int64
			if summary.AllocatableNodeMemTotal != nil {
				allocMemMilli = summary.AllocatableNodeMemTotal.MilliValue()
			}
			memRatio = ui.GetRatio(float64(memUsedMilli), float64(allocMemMilli))
			memValue = ui.FormatMemory(summary.UsageNodeMemTotal)
			memLabel = "used"
		} else {
			var memReqMilli int64
			if summary.RequestedPodMemTotal != nil {
				memReqMilli = summary.RequestedPodMemTotal.MilliValue()
			}
			var allocMemMilli int64
			if summary.AllocatableNodeMemTotal != nil {
				allocMemMilli = summary.AllocatableNodeMemTotal.MilliValue()
			}
			memRatio = ui.GetRatio(float64(memReqMilli), float64(allocMemMilli))
			memValue = ui.FormatMemory(summary.RequestedPodMemTotal)
			memLabel = "requested"
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
			// Network sparkline with adaptive scaling
			netTitle := fmt.Sprintf(" Net ↓%s ↑%s ",
				ui.FormatBytesRate(summary.NetworkRxRate),
				ui.FormatBytesRate(summary.NetworkTxRate))
			combinedNetRate := summary.NetworkRxRate + summary.NetworkTxRate

			// Adaptive scaling: track peak, use minimum 512 KB/s baseline
			if combinedNetRate > p.peakNetRate {
				p.peakNetRate = combinedNetRate
			}
			effectiveNetMax := p.peakNetRate
			if effectiveNetMax < 512*1024 { // Min 512 KB/s baseline
				effectiveNetMax = 512 * 1024
			}
			netNormalized := combinedNetRate / effectiveNetMax
			if netNormalized > 1 {
				netNormalized = 1
			}
			p.sparklineRow.UpdateNET(netNormalized, netTitle)

			// Disk sparkline with adaptive scaling
			diskTitle := fmt.Sprintf(" Disk R:%s W:%s ",
				ui.FormatBytesRate(summary.DiskReadRate),
				ui.FormatBytesRate(summary.DiskWriteRate))
			combinedDiskRate := summary.DiskReadRate + summary.DiskWriteRate

			// Adaptive scaling: track peak, use minimum 512 KB/s baseline
			if combinedDiskRate > p.peakDiskRate {
				p.peakDiskRate = combinedDiskRate
			}
			effectiveDiskMax := p.peakDiskRate
			if effectiveDiskMax < 512*1024 { // Min 512 KB/s baseline
				effectiveDiskMax = 512 * 1024
			}
			diskNormalized := combinedDiskRate / effectiveDiskMax
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

	pal := ui.ActivePalette()
	stallText := formatSummaryStall(summary)

	if ui.IsMonochrome() {
		failuresStr := fmt.Sprintf("%d", summary.FailedPods)
		if summary.FailedPods > 0 {
			failuresStr = ui.ErrorMarkup(failuresStr)
		}
		pressureStr := fmt.Sprintf("%d", summary.NodePressureCount)
		if summary.NodePressureCount > 0 {
			pressureStr = ui.ErrorMarkup(pressureStr)
		}
		restartsStr := fmt.Sprintf("%d", summary.ContainerRestarts)
		if summary.ContainerRestarts > 100 {
			restartsStr = ui.ErrorMarkup(restartsStr)
		}
		enhancedText := fmt.Sprintf(
			"Restarts: %s │ Failures: %s │ Evicted: %d │ Pressure: %s │ Stall: %s",
			restartsStr,
			failuresStr,
			summary.EvictedPods,
			pressureStr,
			stallText,
		)
		p.enhancedStats.SetText(enhancedText)
		return
	}

	restartsCol := pal.GaugeLow
	if summary.ContainerRestarts > 100 {
		restartsCol = pal.GaugeHigh
	} else if summary.ContainerRestarts > 50 {
		restartsCol = pal.GaugeMedium
	}

	failuresCol := pal.GaugeLow
	if summary.FailedPods > 0 {
		failuresCol = pal.GaugeHigh
	}

	evictedCol := pal.GaugeLow
	if summary.EvictedPods > 0 {
		evictedCol = pal.GaugeMedium
	}

	pressureCol := pal.GaugeLow
	if summary.NodePressureCount > 0 {
		pressureCol = pal.GaugeHigh
	}

	lblTag := ui.TagFg(pal.DataPrimary)
	enhancedText := fmt.Sprintf(
		"[%s]Restarts: [%s]%d[%s] │ Failures: [%s]%d[%s] │ Evicted: [%s]%d[%s] │ Pressure: [%s]%d[%s] │ Stall: %s",
		lblTag, ui.TagFg(restartsCol), summary.ContainerRestarts,
		lblTag, ui.TagFg(failuresCol), summary.FailedPods,
		lblTag, ui.TagFg(evictedCol), summary.EvictedPods,
		lblTag, ui.TagFg(pressureCol), summary.NodePressureCount,
		lblTag, stallText,
	)
	p.enhancedStats.SetText(enhancedText)
}

// formatSummaryStall renders the cluster-wide PSI cell for the summary strip.
// Shows the dominant axis across nodes (max of CPU/MEM/IO cluster averages),
// with the same format and color thresholds as the per-row STALL column. The
// beta caveat for K8s versions below 1.36 (GA) is documented in docs/psi.md.
func formatSummaryStall(s model.ClusterSummary) string {
	psi := metrics.PSIMetrics{
		CPUStallPct: s.AvgNodeCPUStallPct,
		MemStallPct: s.AvgNodeMemStallPct,
		IOStallPct:  s.AvgNodeIOStallPct,
	}
	return renderStallCell(psi, true)
}

func (p *clusterSummaryPanel) DrawFooter(data any) {}

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
