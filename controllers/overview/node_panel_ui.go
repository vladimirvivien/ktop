package overview

import (
	"fmt"
	"sort"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/ui"
)

type NodeItem struct {
	Name,
	Status,
	Role,
	Version,
	CpuUsage,
	CpuAvail string
	CpuValue,
	CpuAvailValue int64
	MemUsage,
	MemAvail string
	MemValue,
	MemAvailValue int64
}

type nodePanel struct {
	title    string
	root     *tview.Flex
	listCols []string
	list     *tview.Table
}

func NewNodePanel(title string) ui.Panel {
	p := &nodePanel{title: title}
	p.Layout()
	return p
}
func (p *nodePanel) GetTitle() string {
	return p.title
}
func (p *nodePanel) Layout() {
	p.list = tview.NewTable()
	p.list.SetBorder(true)
	p.list.SetBorders(false)
	p.list.SetTitle(p.GetTitle())
	p.list.SetTitleAlign(tview.AlignLeft)
	p.list.SetBorderColor(tcell.ColorWhite)

	p.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.list, 7, 1, true)
}

func (p *nodePanel) DrawHeader(cols ...string) {
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
	rows, ok := data.([]NodeItem)
	if !ok {
		panic("type mismatched for NodePanel.DrawBody")
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Name < rows[j].Name
	})

	colorKeys := ui.ColorKeys{0: "green", 50: "yellow", 90: "red"}

	for i, row := range rows {
		cpuRatio := ui.GetRatio(float64(row.CpuValue), float64(row.CpuAvailValue))
		cpuGraph := ui.BarGraph(10, cpuRatio, colorKeys)

		memRatio := ui.GetRatio(float64(row.MemValue), float64(row.MemAvailValue))
		memGraph := ui.BarGraph(10, memRatio, colorKeys)

		p.list.SetCell(
			i+1, 0,
			&tview.TableCell{
				Text:  row.Name,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i+1, 1,
			&tview.TableCell{
				Text:  row.Status,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i+1, 2,
			&tview.TableCell{
				Text:  row.Role,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i+1, 3,
			&tview.TableCell{
				Text:  row.Version,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i+1, 4,
			&tview.TableCell{
				Text:  fmt.Sprintf("[white][%s[white]] %-2.1f%%", cpuGraph, cpuRatio*100),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i+1, 5,
			&tview.TableCell{
				Text:  fmt.Sprintf("[white][%s[white]] %02.1f%%", memGraph, memRatio*100),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)
	}
}

func (p *nodePanel) DrawFooter(cols ...string) {

}

func (p *nodePanel) Clear() {

}

func (p *nodePanel) GetView() tview.Primitive {
	return p.root
}
