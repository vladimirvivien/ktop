package deployments

import (
	"fmt"
	"sort"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/ui"
)

type deploymentPage struct {
	root *tview.Flex

	workloadView *tview.Table

	depListFormat string
	depListCols   []string
	depList       *tview.Table
}

func newPage() *deploymentPage {
	p := &deploymentPage{
		depListCols: []string{"NAME", "NAMESPACE", "PODS", "CPU", "MEMORY", "AGE"},
	}
	p.layout()
	return p
}

func (p *deploymentPage) layout() {
	p.workloadView = tview.NewTable()
	p.workloadView.SetBorder(true)
	p.workloadView.SetBorders(false)
	p.workloadView.SetTitle(" CPU/Memory ")
	p.workloadView.SetTitleAlign(tview.AlignLeft)
	p.workloadView.SetBorderColor(tcell.ColorWhite)

	p.depList = tview.NewTable()
	p.depList.SetBorder(true)
	p.depList.SetBorders(false)
	p.depList.SetTitle(" Deployments ")
	p.depList.SetTitleAlign(tview.AlignLeft)
	p.depList.SetBorderColor(tcell.ColorWhite)

	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.workloadView, 4, 1, true).
		AddItem(p.depList, 0, 1, true)

	p.root = page
}

func (p *deploymentPage) drawUsage(use usage) {
	colorKeys := ui.ColorKeys{0: "red", 40: "yellow", 100: "green"}
	cpuRatio := ui.GetRatio(float64(use.cpuUsage), float64(use.cpuAvail))
	cpuGraph := ui.BarGraph(50, cpuRatio, colorKeys)
	memRatio := ui.GetRatio(float64(use.memUsage), float64(use.memAvail))
	memGraph := ui.BarGraph(50, memRatio, colorKeys)

	p.workloadView.SetCell(
		0, 0,
		tview.NewTableCell("CPU").
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetExpansion(100),
	).SetCell(
		0, 1,
		tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", cpuGraph, cpuRatio*100)).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetExpansion(100),
	)

	p.workloadView.SetCell(
		0, 2,
		tview.NewTableCell("Memory").
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignRight).
			SetExpansion(100),
	).SetCell(
		0, 3,
		tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", memGraph, memRatio*100)).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetExpansion(100),
	)
}

func (p *deploymentPage) drawDeploymentList(sortByCol int, rows []deployRow) {
	if sortByCol > len(rows)-1 {
		sortByCol = 0
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].name < rows[j].name
	})

	//colorKeys := ui.ColorKeys{0: "green", 50: "yellow", 90: "red"}

	// draw header
	for i, col := range p.depListCols {
		p.depList.SetCell(0, i,
			tview.NewTableCell(col).
				SetTextColor(tcell.ColorWhite).
				SetBackgroundColor(tcell.ColorDarkGreen).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
	}

	// draw data
	for i, row := range rows {
		p.depList.SetCell(
			i+1, 0,
			tview.NewTableCell(row.name).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)
	}
}
