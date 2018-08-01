package overview

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/gdamore/tcell"

	"github.com/rivo/tview"
)

type nodeRow struct {
	name,
	status,
	role,
	version,
	cpuUsage,
	cpuAvail string
	cpuValue,
	cpuAvailValue int64
	memUsage,
	memAvail string
	memValue,
	memAvailValue int64
}

type podRow struct {
	name,
	status,
	node,
	ip string
	podCPUValue,
	podMemValue,
	nodeCPUValue,
	nodeMemValue int64
	volumes int
}

type workloadSummary struct {
	deploymentsTotal,
	deploymentsReady,
	daemonSetsTotal,
	daemonSetsReady,
	replicaSetsTotal,
	replicaSetsReady,
	podsTotal,
	podsReady int
}

type overviewPage struct {
	root *tview.Flex

	nodeListFormat string
	nodeListCols   []string
	nodeList       *tview.Table

	workloadGrid *tview.Table

	podListFormat string
	podListCols   []string
	podList       *tview.Table
}

func newPage() *overviewPage {
	p := &overviewPage{
		nodeListCols: []string{"NAME", "STATUS", "ROLE", "VERSION", "CPU", "MEMORY"},
		podListCols:  []string{"NAME", "STATUS", "IP", "NODE", "CPU", "MEMORY"},
	}
	p.layout()
	return p
}

func (p *overviewPage) layout() {

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
		AddItem(p.nodeList, 7, 1, true).
		AddItem(p.workloadGrid, 4, 1, true).
		AddItem(p.podList, 0, 1, true)

	p.root = page
}

func (p *overviewPage) drawNodeList(sortByCol int, rows []nodeRow) {
	if sortByCol > len(rows)-1 {
		sortByCol = 0
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].name < rows[j].name
	})

	p.drawNodeListHeader()
	colorKeys := []string{"green", "yellow", "red"}

	for i, row := range rows {
		cpuRatio := ratio(float64(row.cpuValue), float64(row.cpuAvailValue))
		cpuGraph := barGraph(10, cpuRatio, getColorKey(colorKeys, cpuRatio))

		memRatio := ratio(float64(row.memValue), float64(row.memAvailValue))
		memGraph := barGraph(10, memRatio, getColorKey(colorKeys, memRatio))

		p.nodeList.SetCell(
			i+1, 0,
			&tview.TableCell{
				Text:  row.name,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i+1, 1,
			&tview.TableCell{
				Text:  row.status,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i+1, 2,
			&tview.TableCell{
				Text:  row.role,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i+1, 3,
			&tview.TableCell{
				Text:  row.version,
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

func (p *overviewPage) drawNodeListHeader() {
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

func (p *overviewPage) drawPodList(sortByCol int, rows []podRow) {
	if sortByCol > len(rows)-1 {
		sortByCol = 0
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].name < rows[j].name
	})

	colorKeys := []string{"green", "yellow", "red"}
	p.drawPodListHeader()
	for i, row := range rows {
		p.podList.SetCell(
			i+1, 0,
			tview.NewTableCell(row.name).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.podList.SetCell(
			i+1, 1,
			tview.NewTableCell(row.status).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.podList.SetCell(
			i+1, 2,
			tview.NewTableCell(row.ip).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		p.podList.SetCell(
			i+1, 3,
			tview.NewTableCell(row.node).
				SetTextColor(tcell.ColorYellow).
				SetAlign(tview.AlignLeft),
		)

		cpuRatio := ratio(float64(row.podCPUValue), float64(row.nodeCPUValue))
		cpuGraph := barGraph(10, cpuRatio, getColorKey(colorKeys, cpuRatio))
		memRatio := ratio(float64(row.podMemValue), float64(row.nodeMemValue))
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
}

func (p *overviewPage) drawPodListHeader() {
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

func (p *overviewPage) drawWorkloadGrid(wl workloadSummary) {
	colorKeys := []string{"red", "yellow", "green"}

	depRatio := ratio(float64(wl.deploymentsReady), float64(wl.deploymentsTotal))
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

	dsRatio := ratio(float64(wl.daemonSetsReady), float64(wl.daemonSetsTotal))
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

	rsRatio := ratio(float64(wl.replicaSetsTotal), float64(wl.replicaSetsTotal))
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

	podsRatio := ratio(float64(wl.podsReady), float64(wl.podsTotal))
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
