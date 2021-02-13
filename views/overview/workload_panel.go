package overview

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/ui"
)

type WorkloadItem struct {
	DeploymentsTotal,
	DeploymentsReady,
	DaemonSetsTotal,
	DaemonSetsReady,
	ReplicaSetsTotal,
	ReplicaSetsReady,
	PodsTotal,
	PodsReady int
}

type workloadPanel struct {
	title    string
	root     *tview.Flex
	children  []tview.Primitive
	listCols []string
	table    *tview.Table
}

func NewWorkloadPanel(title string) ui.Panel {
	p := &workloadPanel{title: title}
	p.Layout(nil)
	p.children = append(p.children, p.table)
	return p
}

func (p *workloadPanel) GetTitle() string {
	return p.title
}
func (p *workloadPanel) Layout(data interface{}) {
	p.table = tview.NewTable()
	p.table.SetBorder(true)
	p.table.SetBorders(false)
	p.table.SetTitle(p.GetTitle())
	p.table.SetTitleAlign(tview.AlignLeft)
	p.table.SetBorderColor(tcell.ColorWhite)

	p.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.table, 0, 1, true)
}

func (p *workloadPanel) DrawHeader(data interface{}) {}

func (p *workloadPanel) DrawBody(data interface{}) {
	wl, ok := data.(WorkloadItem)
	if !ok {
		panic(fmt.Sprintf("WorkloadPanel.DrawBody got unexpected type %T", data))
	}

	colorKeys := ui.ColorKeys{0: "red", 40: "yellow", 100: "green"}

	depRatio := ui.GetRatio(float64(wl.DeploymentsReady), float64(wl.DeploymentsTotal))
	depGraph := ui.BarGraph(10, depRatio, colorKeys)
	p.table.SetCell(
		0, 0,
		tview.NewTableCell("Deployments").
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetExpansion(100),
	).SetCell(
		0, 1,
		tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", depGraph, depRatio*100)).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetExpansion(100),
	)

	dsRatio := ui.GetRatio(float64(wl.DaemonSetsReady), float64(wl.DaemonSetsTotal))
	dsGraph := ui.BarGraph(10, dsRatio, colorKeys)
	p.table.SetCell(
		0, 2,
		tview.NewTableCell("Daemon sets").
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignRight).
			SetExpansion(100),
	).SetCell(
		0, 3,
		tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", dsGraph, dsRatio*100)).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetExpansion(100),
	)

	rsRatio := ui.GetRatio(float64(wl.ReplicaSetsTotal), float64(wl.ReplicaSetsTotal))
	rsGraph := ui.BarGraph(10, rsRatio, colorKeys)
	p.table.SetCell(
		0, 4,
		tview.NewTableCell("Replica sets").
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignRight).
			SetExpansion(100),
	).SetCell(
		0, 5,
		tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", rsGraph, rsRatio*100)).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetExpansion(100),
	)

	podsRatio := ui.GetRatio(float64(wl.PodsReady), float64(wl.PodsTotal))
	podsGraph := ui.BarGraph(10, podsRatio, colorKeys)
	p.table.SetCell(
		0, 6,
		tview.NewTableCell("Pods").
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignRight).
			SetExpansion(100),
	).SetCell(
		0, 7,
		tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", podsGraph, podsRatio*100)).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetExpansion(100),
	)
}

func (p *workloadPanel) DrawFooter(data interface{}) {

}

func (p *workloadPanel) Clear() {

}

func (p *workloadPanel) GetRootView() tview.Primitive {
	return p.root
}

func (p *workloadPanel) GetChildrenViews()[]tview.Primitive {
	return p.children
}