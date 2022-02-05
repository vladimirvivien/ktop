package overview

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/api/resource"
)

type nodePanel struct {
	app      *application.Application
	title    string
	root     *tview.Flex
	children []tview.Primitive
	listCols []string
	list     *tview.Table
	laidout bool
}

func NewNodePanel(app *application.Application, title string) ui.Panel {
	p := &nodePanel{app: app, title: title}
	p.Layout(nil)
	return p
}
func (p *nodePanel) GetTitle() string {
	return p.title
}
func (p *nodePanel) Layout(_ interface{}) {
	if !p.laidout {
		p.list = tview.NewTable()
		p.list.SetFixed(1, 0)
		p.list.SetBorder(false)
		p.list.SetBorders(false)
		p.list.SetFocusFunc(func() {
			p.list.SetSelectable(true, false)
			p.list.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlue))
		})
		p.list.SetBlurFunc(func() {
			p.list.SetSelectable(false, false)
		})

		p.root = tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(p.list, 0, 1, true)
		p.root.SetBorder(true)
		p.root.SetTitle(p.GetTitle())
		p.root.SetTitleAlign(tview.AlignLeft)
		p.laidout = true
	}
}

func (p *nodePanel) DrawHeader(data interface{}) {
	cols, ok := data.([]string)
	if !ok {
		panic(fmt.Sprintf("nodePanel.DrawHeader got unexpected data type %T", data))
	}

	// legend column
	p.list.SetCell(0, 0,
		tview.NewTableCell("").
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignCenter).
			SetBackgroundColor(tcell.ColorDarkGreen).
			SetMaxWidth(1).
			SetExpansion(0).
			SetSelectable(false),
	)

	p.listCols = cols
	for i, col := range p.listCols {
		pos := i + 1
		p.list.SetCell(0, pos,
			tview.NewTableCell(col).
				SetTextColor(tcell.ColorWhite).
				SetAlign(tview.AlignLeft).
				SetBackgroundColor(tcell.ColorDarkGreen).
				SetExpansion(100).
				SetSelectable(false),
		)
	}

}

func (p *nodePanel) DrawBody(data interface{}) {
	nodes, ok := data.([]model.NodeModel)
	if !ok {
		panic(fmt.Sprintf("NodePanel.DrawBody: unexpected type %T", data))
	}

	client := p.app.GetK8sClient()
	metricsDiabled := client.AssertMetricsAvailable() != nil
	var cpuRatio, memRatio ui.Ratio
	var cpuGraph, memGraph string
	var cpuMetrics, memMetrics string
	colorKeys := ui.ColorKeys{0: "green", 50: "yellow", 90: "red"}

	p.root.SetTitle(fmt.Sprintf("%s(%d) ", p.GetTitle(), len(nodes)))
	p.root.SetTitleAlign(tview.AlignLeft)

	for i, node := range nodes {
		i++ // offset for header-row
		controlLegend := ""
		if node.Controller {
			controlLegend = fmt.Sprintf("%c", ui.Icons.TrafficLight)
		}

		// legend
		p.list.SetCell(
			i, 0,
			&tview.TableCell{
				Text:          controlLegend,
				Color:         tcell.ColorOrangeRed,
				Align:         tview.AlignCenter,
				NotSelectable: true,
			},
		)

		// name
		p.list.SetCell(
			i, 1,
			&tview.TableCell{
				Text:  node.Name,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 2,
			&tview.TableCell{
				Text:  node.Status,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 3,
			&tview.TableCell{
				Text:  node.TimeSinceStart,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 4,
			&tview.TableCell{
				Text:  node.KubeletVersion,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 5,
			&tview.TableCell{
				Text:  fmt.Sprintf("%s/%s", node.InternalIP, node.ExternalIP),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 6,
			&tview.TableCell{
				Text:  fmt.Sprintf("%s/%s", node.OSImage, node.Architecture),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 7,
			&tview.TableCell{
				Text:  fmt.Sprintf("%d/%d", node.PodsCount, node.ContainerImagesCount),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		// Disk
		p.list.SetCell(
			i, 8,
			&tview.TableCell{
				Text:  fmt.Sprintf("%dGi", node.AllocatableStorageQty.ScaledValue(resource.Giga)),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		if metricsDiabled {
			cpuRatio = ui.GetRatio(float64(node.RequestedPodCpuQty.MilliValue()), float64(node.AllocatableCpuQty.MilliValue()))
			cpuGraph = ui.BarGraph(10, cpuRatio, colorKeys)
			cpuMetrics = fmt.Sprintf(
				"[white][%s[white]] %dm/%dm (%1.0f%%)",
				cpuGraph, node.RequestedPodCpuQty.MilliValue(), node.AllocatableCpuQty.MilliValue(), cpuRatio*100,
			)

			memRatio = ui.GetRatio(float64(node.RequestedPodMemQty.MilliValue()), float64(node.AllocatableMemQty.MilliValue()))
			memGraph = ui.BarGraph(10, memRatio, colorKeys)
			memMetrics = fmt.Sprintf(
				"[white][%s[white]] %dGi/%dGi (%1.0f%%)",
				memGraph, node.RequestedPodMemQty.ScaledValue(resource.Giga), node.AllocatableMemQty.ScaledValue(resource.Giga), memRatio*100,
			)
		} else {
			cpuRatio = ui.GetRatio(float64(node.UsageCpuQty.MilliValue()), float64(node.AllocatableCpuQty.MilliValue()))
			cpuGraph = ui.BarGraph(10, cpuRatio, colorKeys)
			cpuMetrics = fmt.Sprintf(
				"[white][%s[white]] %dm/%dm (%1.0f%%)",
				cpuGraph, node.UsageCpuQty.MilliValue(), node.AllocatableCpuQty.MilliValue(), cpuRatio*100,
			)

			memRatio = ui.GetRatio(float64(node.UsageMemQty.MilliValue()), float64(node.AllocatableMemQty.MilliValue()))
			memGraph = ui.BarGraph(10, memRatio, colorKeys)
			memMetrics = fmt.Sprintf(
				"[white][%s[white]] %dGi/%dGi (%1.0f%%)",
				memGraph, node.UsageMemQty.ScaledValue(resource.Giga), node.AllocatableMemQty.ScaledValue(resource.Giga), memRatio*100,
			)
		}

		p.list.SetCell(
			i, 9,
			&tview.TableCell{
				Text:  cpuMetrics,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 10,
			&tview.TableCell{
				Text:  memMetrics,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)
	}
}

func (p *nodePanel) DrawFooter(_ interface{}) {}

func (p *nodePanel) Clear() {
	p.list.Clear()
	p.Layout(nil)
	p.DrawHeader(p.listCols)
}

func (p *nodePanel) GetRootView() tview.Primitive {
	return p.root
}

func (p *nodePanel) GetChildrenViews() []tview.Primitive {
	return p.children
}
