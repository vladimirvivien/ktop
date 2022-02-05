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

type podPanel struct {
	app      *application.Application
	title    string
	root     *tview.Flex
	children []tview.Primitive
	listCols []string
	list     *tview.Table
	laidout bool
}

func NewPodPanel(app *application.Application, title string) ui.Panel {
	p := &podPanel{app: app, title: title}
	p.Layout(nil)

	return p
}

func (p *podPanel) GetTitle() string {
	return p.title
}

func (p *podPanel) Layout(_ interface{}) {
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

func (p *podPanel) DrawHeader(data interface{}) {
	cols, ok := data.([]string)
	if !ok {
		panic(fmt.Sprintf("podPanel.DrawBody got unexpected data type %T", data))
	}

	p.listCols = cols
	for i, col := range p.listCols {
		p.list.SetCell(0, i,
			tview.NewTableCell(col).
				SetTextColor(tcell.ColorWhite).
				SetBackgroundColor(tcell.ColorDarkGreen).
				SetAlign(tview.AlignLeft).
				SetExpansion(100).
				SetSelectable(false),
		)
	}
	p.list.SetFixed(1, 0)
}

func (p *podPanel) DrawBody(data interface{}) {
	pods, ok := data.([]model.PodModel)
	if !ok {
		panic(fmt.Sprintf("PodPanel.DrawBody got unexpected type %T", data))
	}

	client := p.app.GetK8sClient()
	metricsDisabled := client.AssertMetricsAvailable() != nil
	colorKeys := ui.ColorKeys{0: "green", 50: "yellow", 90: "red"}
	var cpuRatio, memRatio ui.Ratio
	var cpuGraph, memGraph string
	var cpuMetrics, memMetrics string

	p.root.SetTitle(fmt.Sprintf("%s(%d) ", p.GetTitle(), len(pods)))
	p.root.SetTitleAlign(tview.AlignLeft)

	for i, pod := range pods {
		i++ // offset to n+1
		p.list.SetCell(
			i, 0,
			&tview.TableCell{
				Text:  pod.Namespace,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 1,
			&tview.TableCell{
				Text:  pod.Name,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 2,
			&tview.TableCell{
				Text:  fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 3,
			&tview.TableCell{
				Text:  pod.Status,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 4,
			&tview.TableCell{
				Text:  fmt.Sprintf("%d", pod.Restarts),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 5,
			&tview.TableCell{
				Text:  pod.TimeSince,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		// Volume
		p.list.SetCell(
			i, 6,
			&tview.TableCell{
				Text:  fmt.Sprintf("%d/%d", pod.Volumes, pod.VolMounts),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 7,
			&tview.TableCell{
				Text:  pod.IP,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 8,
			&tview.TableCell{
				Text:  pod.Node,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		if metricsDisabled {
			cpuRatio = ui.GetRatio(float64(pod.PodRequestedCpuQty.MilliValue()), float64(pod.NodeAllocatableCpuQty.MilliValue()))
			cpuGraph = ui.BarGraph(10, cpuRatio, colorKeys)
			cpuMetrics = fmt.Sprintf(
				"[white][%s[white]] %dm %02.1f%%",
				cpuGraph, pod.PodRequestedCpuQty.MilliValue(), cpuRatio*100,
			)

			memRatio = ui.GetRatio(float64(pod.PodRequestedMemQty.MilliValue()), float64(pod.NodeAllocatableMemQty.MilliValue()))
			memGraph = ui.BarGraph(10, memRatio, colorKeys)
			memMetrics = fmt.Sprintf(
				"[white][%s[white]] %dGi %02.1f%%", memGraph, pod.PodRequestedMemQty.ScaledValue(resource.Giga), memRatio*100,
			)
		} else {
			cpuRatio = ui.GetRatio(float64(pod.PodUsageCpuQty.MilliValue()), float64(pod.NodeAllocatableCpuQty.MilliValue()))
			cpuGraph = ui.BarGraph(10, cpuRatio, colorKeys)
			cpuMetrics = fmt.Sprintf("[white][%s[white]] %dm %02.1f%%", cpuGraph, pod.PodUsageCpuQty.MilliValue(), cpuRatio*100)

			memRatio = ui.GetRatio(float64(pod.PodUsageMemQty.MilliValue()), float64(pod.NodeUsageMemQty.MilliValue()))
			memGraph = ui.BarGraph(10, memRatio, colorKeys)
			memMetrics = fmt.Sprintf("[white][%s[white]] %dMi %02.1f%%", memGraph, pod.PodUsageMemQty.ScaledValue(resource.Mega), memRatio*100)
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

func (p *podPanel) DrawFooter(data interface{}) {}

func (p *podPanel) Clear() {
	p.list.Clear()
	p.Layout(nil)
	p.DrawHeader(p.listCols)
}

func (p *podPanel) GetRootView() tview.Primitive {
	return p.root
}

func (p *podPanel) GetChildrenViews() []tview.Primitive {
	return p.children
}
