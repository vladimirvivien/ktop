package ui

import (
	"fmt"

	"github.com/gdamore/tcell"

	"github.com/rivo/tview"
)

type NodeRow struct {
	Name,
	Status,
	Role,
	Version,
	CPU,
	Memory string
}

type OverviewPage struct {
	root   *tview.Flex
	header *tview.TextView

	nodeListFormat string
	nodeListCols   []string
	nodeList       *tview.Table
}

func NewOverviewPage() *OverviewPage {
	p := &OverviewPage{
		nodeListCols: []string{"NAME", "STATUS", "ROLE", "VERSION", "CPU", "MEMORY"},
	}
	p.layout()
	return p
}

func (p *OverviewPage) Root() tview.Primitive {
	return p.root
}

func (p *OverviewPage) Header() tview.Primitive {
	return p.header
}

func (p *OverviewPage) NodeInfo() tview.Primitive {
	return p.nodeList
}

func (p *OverviewPage) layout() {
	p.header = tview.NewTextView().
		SetDynamicColors(true)
	p.header.SetBorder(true)
	fmt.Fprint(p.header, "[green]loading...")

	p.nodeList = tview.NewTable()
	p.nodeList.SetBorder(true)
	p.nodeList.SetBorders(false)
	p.nodeList.SetTitle("Clusters")
	p.nodeList.SetTitleAlign(tview.AlignLeft)
	p.nodeList.SetBorderColor(tcell.ColorWhite)

	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.header, 3, 1, true).
		AddItem(p.nodeList, 10, 1, true) //.

	p.root = page
}

func (p *OverviewPage) DrawHeader(host, namespace string) {
	p.header.Clear()
	fmt.Fprintf(p.header, "[green]API server: [white]%s [green]namespace: [white]%s", host, namespace)
}

func (p *OverviewPage) DrawNodeList(rows []NodeRow) {
	p.nodeList.Clear()
	p.drawNodeListHeader()
	for i, row := range rows {
		p.nodeList.SetCell(
			i+1, 0,
			tview.NewTableCell(row.Name).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.nodeList.SetCell(
			i+1, 1,
			tview.NewTableCell(row.Status).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.nodeList.SetCell(
			i+1, 2,
			tview.NewTableCell(row.Role).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.nodeList.SetCell(
			i+1, 3,
			tview.NewTableCell(row.Version).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)
	}
}

func (p *OverviewPage) drawNodeListHeader() {
	for i, col := range p.nodeListCols {
		p.nodeList.SetCell(0, i,
			tview.NewTableCell(col).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
	}
}
