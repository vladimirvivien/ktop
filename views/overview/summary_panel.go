package overview

import (
	"fmt"
	"time"

	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
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
func (p *clusterSummaryPanel) Layout(data interface{}) {
	// Stats view (single line, with border for visual separation)
	p.statsView = tview.NewTextView()
	p.statsView.SetDynamicColors(true)
	p.statsView.SetBorder(true)
	p.statsView.SetBorderPadding(0, 0, 0, 0)

	// CPU sparkline view
	p.cpuView = tview.NewTextView()
	p.cpuView.SetDynamicColors(true)
	p.cpuView.SetTextAlign(tview.AlignCenter)
	p.cpuView.SetBorder(true)
	p.cpuView.SetBorderPadding(0, 0, 0, 0)
	p.cpuView.SetTitleAlign(tview.AlignCenter)

	// MEM sparkline view
	p.memView = tview.NewTextView()
	p.memView.SetDynamicColors(true)
	p.memView.SetTextAlign(tview.AlignCenter)
	p.memView.SetBorder(true)
	p.memView.SetBorderPadding(0, 0, 0, 0)
	p.memView.SetTitleAlign(tview.AlignCenter)

	// Horizontal flex for CPU and MEM sparklines side by side
	graphFlex := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(p.cpuView, 0, 1, false).
		AddItem(p.memView, 0, 1, false)

	// Flex layout: stacks bordered stats and graph vertically
	p.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.statsView, 3, 0, false). // 3 rows (1 content + 2 border)
		AddItem(graphFlex, 0, 1, false)    // remaining for sparklines
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
	const sparklineHeight = 5 // 7 graphFlex rows - 2 border rows = 5 (label moved to title)
	const defaultWidth = 40   // Initial width before container dimensions are known

	switch summary := data.(type) {
	case model.ClusterSummary:
		var cpuRatio, memRatio ui.Ratio

		// Check if usage metrics are actually available (non-zero)
		hasUsageMetrics := summary.UsageNodeCpuTotal.MilliValue() > 0 || summary.UsageNodeMemTotal.MilliValue() > 0

		// Initialize multi-line sparklines if needed
		if p.cpuSparkline == nil {
			p.cpuSparkline = ui.NewSparklineStateWithHeight(defaultWidth, sparklineHeight, colorKeys)
		}
		if p.memSparkline == nil {
			p.memSparkline = ui.NewSparklineStateWithHeight(defaultWidth, sparklineHeight, colorKeys)
		}

		// Get container width and resize sparklines to fill available space
		_, _, containerWidth, _ := p.cpuView.GetRect()
		if containerWidth > 2 {
			sparklineWidth := containerWidth - 2 // subtract border chars
			p.cpuSparkline.Resize(sparklineWidth)
			p.memSparkline.Resize(sparklineWidth)
		}

		// === Stats row ===
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
		p.memView.SetText(memSparklineText) // This is correct - memSparklineText

	default:
		panic(fmt.Sprintf("SummaryPanel.DrawBody: unexpected type %T", data))
	}
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
