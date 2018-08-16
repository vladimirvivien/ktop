package deployments

import (
	"sort"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

type deployRow struct {
	name string
}

type deploymentPage struct {
	root *tview.Flex

	depListFormat string
	depListCols   []string
	depList       *tview.Table
}

func newPage() *deploymentPage {
	p := &deploymentPage{
		depListCols: []string{"NAME", "STATUS", "IP", "NODE", "CPU", "MEMORY"},
	}
	p.layout()
	return p
}

func (p *deploymentPage) layout() {
	p.depList = tview.NewTable()
	p.depList.SetBorder(true)
	p.depList.SetBorders(false)
	p.depList.SetTitle(" Deployments ")
	p.depList.SetTitleAlign(tview.AlignLeft)
	p.depList.SetBorderColor(tcell.ColorWhite)

	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.depList, 0, 1, true)

	p.root = page
}

func (p *deploymentPage) drawDepList(sortCol int, rows []deployRow) {
	if sortCol > len(rows)-1 {
		sortCol = 0
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].name < rows[j].name
	})
}
