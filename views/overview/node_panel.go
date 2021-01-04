package overview

import (
	"fmt"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"

	"github.com/vladimirvivien/ktop/views/model"
	"github.com/vladimirvivien/ktop/ui"
)

type nodePanel struct {
	title    string
	root     *tview.Flex
	listCols []string
	list     *tview.Table
}

func NewNodePanel(title string) ui.Panel {
	p := &nodePanel{title: title, list: tview.NewTable()}
	p.Layout(nil)
	p.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.list, 0, 1, true)
	return p
}
func (p *nodePanel) GetTitle() string {
	return p.title
}
func (p *nodePanel) Layout(data interface{}) {
	p.list.SetBorder(true)
	p.list.SetBorders(false)
	p.list.SetTitle(p.GetTitle())
	p.list.SetTitleAlign(tview.AlignLeft)
	p.list.SetBorderColor(tcell.ColorWhite)
}

func (p *nodePanel) DrawHeader(data interface{}) {
	cols, ok := data.([]string)
	if !ok {
		panic(fmt.Sprintf("nodePanel.DrawHeader got unexpected data type %T", data))
	}

	p.listCols = cols
	for i, col := range p.listCols {
		p.list.SetCell(0, i,
			tview.NewTableCell(col).
				SetTextColor(tcell.ColorWhite).
				SetAlign(tview.AlignLeft).
				SetBackgroundColor(tcell.ColorDarkGreen).
				SetExpansion(100),
		)
	}
	p.list.SetFixed(1, 0)
}

func (p *nodePanel) DrawBody(data interface{}) {
	nodes, ok := data.([]model.NodeModel)
	if !ok {
		panic(fmt.Sprintf("NodePanel.DrawBody got unexpected type %T", data))
	}

	colorKeys := ui.ColorKeys{0: "green", 50: "yellow", 90: "red"}

	for i, node := range nodes {

		cpuRatio := ui.GetRatio(float64(node.CpuValue), float64(node.CpuAvailValue))
		cpuGraph := ui.BarGraph(10, cpuRatio, colorKeys)

		memRatio := ui.GetRatio(float64(node.MemValue), float64(node.MemAvailValue))
		memGraph := ui.BarGraph(10, memRatio, colorKeys)

		i++  // offset for header-row
		p.list.SetCell(
			i, 0,
			&tview.TableCell{
				Text:  node.Name,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i, 1,
			&tview.TableCell{
				Text:  node.Status,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i, 2,
			&tview.TableCell{
				Text:  node.Role,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i, 3,
			&tview.TableCell{
				Text:  node.Version,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i, 4,
			&tview.TableCell{
				Text:  fmt.Sprintf("[white][%s[white]] %-2.1f%%", cpuGraph, cpuRatio*100),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i, 5,
			&tview.TableCell{
				Text:  fmt.Sprintf("[white][%s[white]] %02.1f%%", memGraph, memRatio*100),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)
	}
}

func (p *nodePanel) DrawFooter(data interface{}) {}
func (p *nodePanel) Clear() {
	p.list.Clear()
	p.Layout(nil)
	p.DrawHeader(p.listCols)
}

func (p *nodePanel) GetView() tview.Primitive {
	return p.root
}
