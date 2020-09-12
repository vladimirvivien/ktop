package overview

import (
	"fmt"
	"sort"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/ui"
)

type PodItem struct {
	Name,
	Status,
	Node,
	IP string
	PodCPUValue,
	PodMemValue,
	NodeCPUValue,
	NodeMemValue int64
	Volumes int
}

type podPanel struct {
	title    string
	root     *tview.Flex
	listCols []string
	list     *tview.Table
}

func NewPodPanel(title string) ui.Panel {
	p := &podPanel{title: title}
	p.Layout(nil)
	return p
}
func (p *podPanel) GetTitle() string {
	return p.title
}
func (p *podPanel) Layout(data interface{}) {
	p.list = tview.NewTable()
	p.list.SetBorder(true)
	p.list.SetBorders(false)
	p.list.SetTitle(p.GetTitle())
	p.list.SetTitleAlign(tview.AlignLeft)
	p.list.SetBorderColor(tcell.ColorWhite)

	p.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.list, 0, 1, true)
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
}

func (p *podPanel) DrawBody(data interface{}) {
	rows, ok := data.([]PodItem)
	if !ok {
		panic(fmt.Sprintf("PodPanel.DrawBody got unexpected type %T", data))
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Name < rows[j].Name
	})

	colorKeys := ui.ColorKeys{0: "green", 50: "yellow", 90: "red"}

	for i, row := range rows {
		p.list.SetCell(
			i+1, 0,
			tview.NewTableCell(row.Name).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.list.SetCell(
			i+1, 1,
			tview.NewTableCell(row.Status).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.list.SetCell(
			i+1, 2,
			tview.NewTableCell(row.IP).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.list.SetCell(
			i+1, 3,
			tview.NewTableCell(row.Node).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		cpuRatio := ui.GetRatio(float64(row.PodCPUValue), float64(row.NodeCPUValue))
		cpuGraph := ui.BarGraph(10, cpuRatio, colorKeys)
		memRatio := ui.GetRatio(float64(row.PodMemValue), float64(row.NodeMemValue))
		memGraph := ui.BarGraph(10, memRatio, colorKeys)

		p.list.SetCell(
			i+1, 4,
			tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", cpuGraph, cpuRatio*100)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.list.SetCell(
			i+1, 5,
			tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", memGraph, memRatio*100)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)
	}
}

func (p *podPanel) DrawFooter(data interface{}) {

}

func (p *podPanel) Clear() {

}

func (p *podPanel) GetView() tview.Primitive {
	return p.root
}
