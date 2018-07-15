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
	CPUUsage,
	MemUsage string
	CPUValue,
	MemValue int64
}

type PodRow struct {
	Name,
	Status,
	Ready,
	Image,
	CPUUsage,
	MemUsage string
	CPUValue,
	MemValue int64
}

type OverviewPage struct {
	root   *tview.Flex
	header *tview.TextView

	nodeListFormat string
	nodeListCols   []string
	nodeList       *tview.Table

	podListFormat string
	podListCols   []string
	podList       *tview.Table
}

func NewOverviewPage() *OverviewPage {
	p := &OverviewPage{
		nodeListCols: []string{"NAME", "STATUS", "ROLE", "VERSION", "CPU", "MEMORY"},
		podListCols:  []string{"NAME", "STATUS", "READY", "IMAGE", "CPU", "MEMORY"},
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
	p.nodeList.SetTitle(" Cluster Nodes ")
	p.nodeList.SetTitleAlign(tview.AlignLeft)
	p.nodeList.SetBorderColor(tcell.ColorWhite)

	p.podList = tview.NewTable()
	p.podList.SetBorder(true)
	p.podList.SetBorders(false)
	p.podList.SetTitle(" Pods ")
	p.podList.SetTitleAlign(tview.AlignLeft)
	p.nodeList.SetBorderColor(tcell.ColorWhite)

	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.header, 3, 1, true).
		AddItem(p.nodeList, 7, 1, true).
		AddItem(p.podList, 0, 1, true)

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

		p.nodeList.SetCell(
			i+1, 4,
			tview.NewTableCell(row.CPUUsage).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.nodeList.SetCell(
			i+1, 5,
			tview.NewTableCell(row.MemUsage).
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

func (p *OverviewPage) DrawPodList(rows []PodRow) {
	p.podList.Clear()
	p.drawPodListHeader()
	for i, row := range rows {
		p.podList.SetCell(
			i+1, 0,
			tview.NewTableCell(row.Name).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.podList.SetCell(
			i+1, 1,
			tview.NewTableCell(row.Status).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.podList.SetCell(
			i+1, 2,
			tview.NewTableCell(row.Ready).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.podList.SetCell(
			i+1, 3,
			tview.NewTableCell(row.Image).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.podList.SetCell(
			i+1, 4,
			tview.NewTableCell(row.CPUUsage).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.podList.SetCell(
			i+1, 5,
			tview.NewTableCell(row.MemUsage).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)
	}
}

func (p *OverviewPage) drawPodListHeader() {
	for i, col := range p.podListCols {
		p.podList.SetCell(0, i,
			tview.NewTableCell(col).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
	}
}
