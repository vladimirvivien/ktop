package overview

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

type podPanel struct {
	app *application.Application
	title    string
	root     *tview.Flex
	children []tview.Primitive
	listCols []string
	list     *tview.Table
}

func NewPodPanel(app *application.Application, title string) ui.Panel {
	p := &podPanel{app: app, title: title, list: tview.NewTable()}
	p.Layout(nil)
	p.children = append(p.children, p.list)
	p.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.list, 0, 1, true)
	return p
}

func (p *podPanel) GetTitle() string {
	return p.title
}

func (p *podPanel) Layout(data interface{}) {
	p.list.SetBorder(true)
	p.list.SetBorders(false)
	p.list.SetTitle(p.GetTitle())
	p.list.SetTitleAlign(tview.AlignLeft)
	p.list.SetBorderColor(tcell.ColorWhite)
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
				SetExpansion(100),
		)
	}
	p.list.SetFixed(1, 0)
}

func (p *podPanel) DrawBody(data interface{}) {
	pods, ok := data.([]model.PodModel)
	if !ok {
		panic(fmt.Sprintf("PodPanel.DrawBody got unexpected type %T", data))
	}

	colorKeys := ui.ColorKeys{0: "green", 50: "yellow", 90: "red"}

	for i, pod := range pods {

		p.list.SetCell(
			i+1, 0,
			tview.NewTableCell(pod.Namespace).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.list.SetCell(
			i+1, 1,
			tview.NewTableCell(pod.Name).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.list.SetCell(
			i+1, 2,
			tview.NewTableCell(fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignCenter),
		)

		p.list.SetCell(
			i+1, 3,
			tview.NewTableCell(pod.Status).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.list.SetCell(
			i+1, 4,
			tview.NewTableCell(fmt.Sprintf("%d",pod.Restarts)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignCenter),
		)

		p.list.SetCell(
			i+1, 5,
			tview.NewTableCell(pod.TimeSince).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.list.SetCell(
			i+1, 6,
			tview.NewTableCell(pod.IP).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.list.SetCell(
			i+1, 7,
			tview.NewTableCell(pod.Node).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		cpuRatio := ui.GetRatio(float64(pod.PodCPUValue), float64(pod.NodeCPUValue))
		cpuGraph := ui.BarGraph(10, cpuRatio, colorKeys)
		memRatio := ui.GetRatio(float64(pod.PodMemValue), float64(pod.NodeMemValue))
		memGraph := ui.BarGraph(10, memRatio, colorKeys)

		p.list.SetCell(
			i+1, 8,
			tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", cpuGraph, cpuRatio*100)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.list.SetCell(
			i+1, 9,
			tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", memGraph, memRatio*100)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
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