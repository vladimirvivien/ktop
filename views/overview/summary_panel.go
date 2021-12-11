package overview

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/duration"
)

type clusterSummaryPanel struct {
	app *application.Application
	title        string
	root         *tview.Flex
	children     []tview.Primitive
	listCols     []string
	graphTable   *tview.Table
	summaryTable *tview.Table
}

func NewClusterSummaryPanel(app *application.Application, title string) ui.Panel {
	p := &clusterSummaryPanel{app: app, title: title}
	p.Layout(nil)
	p.children = append(p.children, p.graphTable)
	return p
}

func (p *clusterSummaryPanel) GetTitle() string {
	return p.title
}
func (p *clusterSummaryPanel) Layout(data interface{}) {
	p.summaryTable = tview.NewTable()
	p.summaryTable.SetBorder(false)
	p.summaryTable.SetBorders(false)
	p.summaryTable.SetTitleAlign(tview.AlignLeft)
	p.summaryTable.SetBorderColor(tcell.ColorWhite)

	p.graphTable = tview.NewTable()
	p.graphTable.SetBorder(false)
	p.graphTable.SetBorders(false)
	p.graphTable.SetTitleAlign(tview.AlignLeft)
	p.graphTable.SetBorderColor(tcell.ColorWhite)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.summaryTable, 1, 1, true).
		AddItem(p.graphTable, 1, 1, true)
	root.SetBorder(true)
	root.SetTitle(p.GetTitle())
	root.SetTitleAlign(tview.AlignLeft)
	root.SetBorderPadding(0, 0, 0, 0)
	p.root = root
}

func (p *clusterSummaryPanel) DrawHeader(data interface{}) {}

func (p *clusterSummaryPanel) DrawBody(data interface{}) {
	colorKeys := ui.ColorKeys{0: "green", 40: "yellow", 80: "red"}
	client := p.app.GetK8sClient()
	graphSize := 40
	switch summary := data.(type) {
	case model.ClusterSummary:
		var cpuRatio, memRatio ui.Ratio
		var cpuGraph, memGraph string
		var cpuMetrics, memMetrics string
		if err := client.AssertMetricsAvailable(); err != nil { // metrics not available
			cpuRatio = ui.GetRatio(float64(summary.RequestedCpuTotal.MilliValue()), float64(summary.AllocatableCpuTotal.MilliValue()))
			cpuGraph = ui.BarGraph(graphSize, cpuRatio, colorKeys)
			cpuMetrics = fmt.Sprintf(
				"CPU: [white][%s[white]] %dm/%dm (%02.1f%% requested)",
				cpuGraph, summary.RequestedCpuTotal.MilliValue(), summary.AllocatableCpuTotal.MilliValue(), cpuRatio*100,
			)

			memRatio = ui.GetRatio(float64(summary.RequestedMemTotal.MilliValue()), float64(summary.AllocatableMemTotal.MilliValue()))
			memGraph = ui.BarGraph(graphSize, memRatio, colorKeys)
			memMetrics = fmt.Sprintf(
				"Memory: [white][%s[white]] %dGi/%dGi (%02.1f%% requested)",
				memGraph, summary.RequestedMemTotal.ScaledValue(resource.Giga), summary.AllocatableMemTotal.ScaledValue(resource.Giga), memRatio*100,
			)
		}else{

		}

		p.graphTable.SetCell(
			0, 0,
			tview.NewTableCell(cpuMetrics).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)

		p.graphTable.SetCell(
			0, 1,
			tview.NewTableCell(memMetrics).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)

		// -=-=-=-=-=-=-=-=-=-=-=-=- cluster summary table -=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-=-
		p.summaryTable.SetCell(
			0, 0,
			tview.NewTableCell(fmt.Sprintf("Uptime: [white]%s[white]", duration.HumanDuration(time.Since(summary.Uptime.Time)))).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
		p.summaryTable.SetCell(
			0, 1,
			tview.NewTableCell(fmt.Sprintf("Nodes: [white]%d", summary.NodesReady)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
		p.summaryTable.SetCell(
			0, 2,
			tview.NewTableCell(fmt.Sprintf("Namespaces: [white]%d[white]", summary.Namespaces)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
		p.summaryTable.SetCell(
			0, 3,
			tview.NewTableCell(fmt.Sprintf("Pressures: [white]%d[white]", summary.Pressures)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
		p.summaryTable.SetCell(
			0, 4,
			tview.NewTableCell(fmt.Sprintf("Pods: [white]%d[white]", summary.PodsRunning)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
		p.summaryTable.SetCell(
			0, 5,
			tview.NewTableCell(fmt.Sprintf("Containers: [white]%d", summary.ContainersRunning)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
		p.summaryTable.SetCell(
			0, 6,
			tview.NewTableCell(fmt.Sprintf("Volumes: [white]%d", summary.PVsInUse)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
		p.summaryTable.SetCell(
			0, 7,
			tview.NewTableCell(fmt.Sprintf("Deployments: [white]%d[white]", summary.DeploymentsReady)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)

		p.summaryTable.SetCell(
			0, 8,
			tview.NewTableCell(fmt.Sprintf("Replicasets: [white]%d[white]", summary.ReplicaSetsReady)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
		p.summaryTable.SetCell(
			0, 9,
			tview.NewTableCell(fmt.Sprintf("Daemonsets: [white]%d[white]", summary.DaemonSetsReady)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
		p.summaryTable.SetCell(
			0, 10,
			tview.NewTableCell(fmt.Sprintf("Statefulsets: [white]%d[white]", summary.StatefulSetsReady)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
		p.summaryTable.SetCell(
			0, 11,
			tview.NewTableCell(fmt.Sprintf("Jobs: [white]%d, [green]cron: [white]%d", summary.JobsCount, summary.CronJobsCount)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)

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
