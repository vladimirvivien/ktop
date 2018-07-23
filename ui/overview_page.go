package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/gdamore/tcell"

	"github.com/rivo/tview"
)

type NodeRow struct {
	Name,
	Status,
	Role,
	Version,
	CPUUsage,
	CPUAvail string
	CPUValue,
	CPUAvailValue int64
	MemUsage,
	MemAvail string
	MemValue,
	MemAvailValue int64
}

type PodRow struct {
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

type WorkloadSummary struct {
	DeploymentsTotal,
	DeploymentsReady,
	DaemonSetsTotal,
	DaemonSetsReady,
	ReplicaSetsTotal,
	ReplicaSetsReady,
	PodsTotal,
	PodsReady int
}

type OverviewPage struct {
	app    *tview.Application
	root   *tview.Flex
	header *tview.TextView

	nodeListFormat string
	nodeListCols   []string
	nodeList       *tview.Table

	workloadGrid *tview.Table

	podListFormat string
	podListCols   []string
	podList       *tview.Table
}

func NewOverviewPage(app *tview.Application) *OverviewPage {
	p := &OverviewPage{
		app:          app,
		nodeListCols: []string{"NAME", "STATUS", "ROLE", "VERSION", "CPU", "MEMORY"},
		podListCols:  []string{"NAME", "STATUS", "IP", "NODE", "CPU", "MEMORY"},
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

func (p *OverviewPage) NodeList() tview.Primitive {
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
	p.nodeList.SetTitle(" Cluster ")
	p.nodeList.SetTitleAlign(tview.AlignLeft)
	p.nodeList.SetBorderColor(tcell.ColorWhite)

	p.workloadGrid = tview.NewTable()
	p.workloadGrid.SetBorder(true)
	p.workloadGrid.SetBorders(false)
	p.workloadGrid.SetTitle(" Running Workload ")
	p.workloadGrid.SetTitleAlign(tview.AlignLeft)
	p.workloadGrid.SetBorderColor(tcell.ColorWhite)

	p.podList = tview.NewTable()
	p.podList.SetBorder(true)
	p.podList.SetBorders(false)
	p.podList.SetTitle(" Pods ")
	p.podList.SetTitleAlign(tview.AlignLeft)
	p.nodeList.SetBorderColor(tcell.ColorWhite)

	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.header, 3, 1, true).
		AddItem(p.nodeList, 7, 1, true).
		AddItem(p.workloadGrid, 4, 1, true).
		AddItem(p.podList, 0, 1, true)

	p.root = page
}

func (p *OverviewPage) DrawHeader(host, namespace string) {
	p.header.Clear()
	fmt.Fprintf(p.header, "[green]API server: [white]%s [green]namespace: [white]%s", host, namespace)
	p.app.Draw()
}

func (p *OverviewPage) DrawNodeList(sortByCol int, rows []NodeRow) {
	if sortByCol > len(rows)-1 {
		sortByCol = 0
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Name < rows[j].Name
	})

	p.drawNodeListHeader()
	colorKeys := []string{"green", "yellow", "red"}

	for i, row := range rows {
		cpuRatio := ratio(float64(row.CPUValue), float64(row.CPUAvailValue))
		cpuGraph := barGraph(10, cpuRatio, getColorKey(colorKeys, cpuRatio))

		memRatio := ratio(float64(row.MemValue), float64(row.MemAvailValue))
		memGraph := barGraph(10, memRatio, getColorKey(colorKeys, memRatio))

		p.nodeList.SetCell(
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
	p.app.Draw()
}

func (p *OverviewPage) UpdateNodeMetricsRow(row NodeRow) {
	for i := 0; i < p.nodeList.GetRowCount(); i++ {
		cell := p.nodeList.GetCell(i, 0)
		if cell.Text == row.Name {
			cpuCell := p.nodeList.GetCell(i, 4)
			if cpuCell != nil {
				cpuCell.Text = row.CPUUsage
			}

			memCell := p.nodeList.GetCell(i, 5)
			if memCell != nil {
				memCell.Text = row.MemUsage
			}
		}
	}
}

func (p *OverviewPage) drawNodeListHeader() {
	for i, col := range p.nodeListCols {
		p.nodeList.SetCell(0, i,
			tview.NewTableCell(col).
				SetTextColor(tcell.ColorWhite).
				SetAlign(tview.AlignLeft).
				SetBackgroundColor(tcell.ColorDarkGreen).
				SetExpansion(100),
		)
	}
}

func (p *OverviewPage) DrawPodList(sortByCol int, rows []PodRow) {
	if sortByCol > len(rows)-1 {
		sortByCol = 0
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Name < rows[j].Name
	})

	colorKeys := []string{"green", "yellow", "red"}
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
			tview.NewTableCell(row.IP).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.podList.SetCell(
			i+1, 3,
			tview.NewTableCell(row.Node).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		cpuRatio := ratio(float64(row.PodCPUValue), float64(row.NodeCPUValue))
		cpuGraph := barGraph(10, cpuRatio, getColorKey(colorKeys, cpuRatio))
		memRatio := ratio(float64(row.PodMemValue), float64(row.NodeMemValue))
		memGraph := barGraph(10, memRatio, getColorKey(colorKeys, memRatio))

		p.podList.SetCell(
			i+1, 4,
			tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", cpuGraph, cpuRatio*100)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.podList.SetCell(
			i+1, 5,
			tview.NewTableCell(fmt.Sprintf("[white][%s[white]] %02.1f%%", memGraph, memRatio*100)).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)
	}
	p.app.Draw()
}

func (p *OverviewPage) drawPodListHeader() {
	for i, col := range p.podListCols {
		p.podList.SetCell(0, i,
			tview.NewTableCell(col).
				SetTextColor(tcell.ColorWhite).
				SetBackgroundColor(tcell.ColorDarkGreen).
				SetAlign(tview.AlignLeft).
				SetExpansion(100),
		)
	}
}

func (p *OverviewPage) DrawWorkloadGrid(wl WorkloadSummary) {
	colorKeys := []string{"red", "yellow", "green"}

	depRatio := ratio(float64(wl.DeploymentsReady), float64(wl.DeploymentsTotal))
	depGraph := barGraph(10, depRatio, getColorKey(colorKeys, depRatio))
	p.workloadGrid.SetCell(
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

	dsRatio := ratio(float64(wl.DaemonSetsReady), float64(wl.DaemonSetsTotal))
	dsGraph := barGraph(10, dsRatio, getColorKey(colorKeys, dsRatio))
	p.workloadGrid.SetCell(
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

	rsRatio := ratio(float64(wl.ReplicaSetsTotal), float64(wl.ReplicaSetsTotal))
	rsGraph := barGraph(10, rsRatio, getColorKey(colorKeys, rsRatio))
	p.workloadGrid.SetCell(
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

	podsRatio := ratio(float64(wl.PodsReady), float64(wl.PodsTotal))
	podsGraph := barGraph(10, podsRatio, getColorKey(colorKeys, podsRatio))
	p.workloadGrid.SetCell(
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

	p.app.Draw()
}

func barGraph(scale int, ratio float64, color string) string {

	normVal := ratio * float64(scale)
	graphVal := int(math.Ceil(normVal))

	var graph strings.Builder

	// nothing to graph
	if normVal == 0 {
		graph.WriteString("[")
		graph.WriteString("silver")
		graph.WriteString("]")
		for j := 0; j < (scale - graphVal); j++ {
			graph.WriteString(".")
		}
		return graph.String()
	}

	graph.WriteString("[")
	graph.WriteString(color)
	graph.WriteString("]")

	for i := 0; i < int(math.Min(float64(scale), float64(graphVal))); i++ {
		graph.WriteString("|")
	}

	for j := 0; j < (scale - graphVal); j++ {
		graph.WriteString(" ")
	}
	return graph.String()
}

func getColorKey(colors []string, ratio float64) string {
	count := len(colors)
	for i, color := range colors {
		window := float64(i+1) / float64(count)
		if ratio <= window {
			return color
		}
	}
	return ""
}

func ratio(val0, val1 float64) float64 {
	if val1 <= 0 {
		return 0
	}
	return val0 / val1
}
