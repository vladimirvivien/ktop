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
	p := &nodePanel{title: title}
	p.Layout(nil)
	return p
}
func (p *nodePanel) GetTitle() string {
	return p.title
}
func (p *nodePanel) Layout(data interface{}) {
	p.list = tview.NewTable()
	p.list.SetBorder(true)
	p.list.SetBorders(false)
	p.list.SetTitle(p.GetTitle())
	p.list.SetTitleAlign(tview.AlignLeft)
	p.list.SetBorderColor(tcell.ColorWhite)

	p.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.list, 0, 1, true)
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
}

func (p *nodePanel) DrawBody(data interface{}) {
	store, ok := data.(*model.Store)
	if !ok {
		panic(fmt.Sprintf("NodePanel.DrawBody got unexpected store type %T", data))
	}

	colorKeys := ui.ColorKeys{0: "green", 50: "yellow", 90: "red"}

	for i, key := range store.Keys() {
		r, found := store.Get(key)
		if !found {
			continue
		}
		row, ok := r.(model.NodeModel)
		if !ok {
			panic(fmt.Sprintf("NodePanel.DrawBody got unexpected model type %T", r))
		}

		cpuRatio := ui.GetRatio(float64(row.CpuValue), float64(row.CpuAvailValue))
		cpuGraph := ui.BarGraph(10, cpuRatio, colorKeys)

		memRatio := ui.GetRatio(float64(row.MemValue), float64(row.MemAvailValue))
		memGraph := ui.BarGraph(10, memRatio, colorKeys)

		if !found {
			continue
		}
		i++
		p.list.SetCell(
			i, 0,
			&tview.TableCell{
				Text:  row.Name,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i, 1,
			&tview.TableCell{
				Text:  row.Status,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i, 2,
			&tview.TableCell{
				Text:  row.Role,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i, 3,
			&tview.TableCell{
				Text:  row.Version,
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
func (p *nodePanel) Clear() {}

func (p *nodePanel) GetView() tview.Primitive {
	return p.root
}
