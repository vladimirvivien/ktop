package overview

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

type podPanel struct {
	app      *application.Application
	title    string
	root     *tview.Flex
	children []tview.Primitive
	listCols []string
	list     *tview.Table
	laidout  bool
	colMap   map[string]int // Maps column name to position index
}

func NewPodPanel(app *application.Application, title string) ui.Panel {
	p := &podPanel{app: app, title: title}
	p.Layout(nil)

	return p
}

func (p *podPanel) GetTitle() string {
	return p.title
}

func (p *podPanel) Layout(_ interface{}) {
	if !p.laidout {
		p.list = tview.NewTable()
		p.list.SetFixed(1, 0)
		p.list.SetBorder(false)
		p.list.SetBorders(false)
		p.list.SetFocusFunc(func() {
			p.list.SetSelectable(true, false)
			p.list.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorYellow).Foreground(tcell.ColorBlue))
		})
		p.list.SetBlurFunc(func() {
			p.list.SetSelectable(false, false)
		})

		p.root = tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(p.list, 0, 1, true)
		p.root.SetBorder(true)
		p.root.SetTitle(p.GetTitle())
		p.root.SetTitleAlign(tview.AlignLeft)
		p.laidout = true
	}
}

func (p *podPanel) DrawHeader(data interface{}) {
	cols, ok := data.([]string)
	if !ok {
		panic(fmt.Sprintf("podPanel.DrawBody got unexpected data type %T", data))
	}

	// Initialize the column map
	p.colMap = make(map[string]int)
	p.listCols = cols

	// Set column headers and build column map
	for i, col := range p.listCols {
		p.list.SetCell(0, i,
			tview.NewTableCell(col).
				SetTextColor(tcell.ColorWhite).
				SetBackgroundColor(tcell.ColorDarkGreen).
				SetAlign(tview.AlignLeft).
				SetExpansion(100).
				SetSelectable(false),
		)

		// Map column name to position
		p.colMap[col] = i
	}
	p.list.SetFixed(1, 0)
}

func (p *podPanel) DrawBody(data interface{}) {
	pods, ok := data.([]model.PodModel)
	if !ok {
		panic(fmt.Sprintf("PodPanel.DrawBody got unexpected type %T", data))
	}

	colorKeys := ui.ColorKeys{0: "green", 50: "yellow", 90: "red"}
	var cpuRatio, memRatio ui.Ratio
	var cpuGraph, memGraph string
	var cpuMetrics, memMetrics string

	p.root.SetTitle(fmt.Sprintf("%s(%d) ", p.GetTitle(), len(pods)))
	p.root.SetTitleAlign(tview.AlignLeft)

	for rowIdx, pod := range pods {
		rowIdx++ // offset for header row

		// Render each column that is included in the filtered view
		for _, colName := range p.listCols {
			colIdx, exists := p.colMap[colName]
			if !exists {
				continue
			}

			switch colName {
			case "NAMESPACE":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  pod.Namespace,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)

			case "POD":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  pod.Name,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)

			case "READY":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers),
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)

			case "STATUS":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  pod.Status,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)

			case "RESTARTS":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%d", pod.Restarts),
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)

			case "AGE":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  pod.TimeSince,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)

			case "VOLS":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%d/%d", pod.Volumes, pod.VolMounts),
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)

			case "IP":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  pod.IP,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)

			case "NODE":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  pod.Node,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)

			case "CPU":
				// Check if we have actual usage metrics (non-zero values)
				hasUsageMetrics := pod.PodUsageCpuQty != nil && pod.PodUsageCpuQty.MilliValue() > 0
				hasRequestMetrics := pod.PodRequestedCpuQty != nil && pod.PodRequestedCpuQty.MilliValue() > 0
				hasAllocatable := pod.NodeAllocatableCpuQty != nil && pod.NodeAllocatableCpuQty.MilliValue() > 0

				if hasUsageMetrics && hasAllocatable {
					// Display usage with graph: [||        ] 150m 3.8%
					cpuRatio = ui.GetRatio(float64(pod.PodUsageCpuQty.MilliValue()), float64(pod.NodeAllocatableCpuQty.MilliValue()))
					cpuGraph = ui.BarGraph(10, cpuRatio, colorKeys)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] %dm %.1f%%",
						cpuGraph, pod.PodUsageCpuQty.MilliValue(), cpuRatio*100,
					)
				} else if hasRequestMetrics && hasAllocatable {
					// Fallback: show requested with graph: [|         ] 100m 2.5%
					cpuRatio = ui.GetRatio(float64(pod.PodRequestedCpuQty.MilliValue()), float64(pod.NodeAllocatableCpuQty.MilliValue()))
					cpuGraph = ui.BarGraph(10, cpuRatio, colorKeys)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] %dm %.1f%%",
						cpuGraph, pod.PodRequestedCpuQty.MilliValue(), cpuRatio*100,
					)
				} else {
					// Zero or unavailable: show empty graph with 0m 0.0%
					cpuGraph = ui.BarGraph(10, 0, colorKeys)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] 0m 0.0%%",
						cpuGraph,
					)
				}

				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  cpuMetrics,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)

			case "MEMORY":
				// Check if we have actual usage metrics (non-zero values)
				hasUsageMetrics := pod.PodUsageMemQty != nil && pod.PodUsageMemQty.Value() > 0
				hasRequestMetrics := pod.PodRequestedMemQty != nil && pod.PodRequestedMemQty.Value() > 0
				hasAllocatable := pod.NodeAllocatableMemQty != nil && pod.NodeAllocatableMemQty.Value() > 0

				if hasUsageMetrics && hasAllocatable {
					// Display usage with graph: [||        ] 366Mi 0.5%
					memRatio = ui.GetRatio(float64(pod.PodUsageMemQty.Value()), float64(pod.NodeAllocatableMemQty.Value()))
					memGraph = ui.BarGraph(10, memRatio, colorKeys)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] %s %.1f%%",
						memGraph,
						ui.FormatMemory(pod.PodUsageMemQty),
						memRatio*100,
					)
				} else if hasRequestMetrics && hasAllocatable {
					// Fallback: show requested with graph: [|         ] 512Mi 0.5%
					memRatio = ui.GetRatio(float64(pod.PodRequestedMemQty.Value()), float64(pod.NodeAllocatableMemQty.Value()))
					memGraph = ui.BarGraph(10, memRatio, colorKeys)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] %s %.1f%%",
						memGraph,
						ui.FormatMemory(pod.PodRequestedMemQty),
						memRatio*100,
					)
				} else {
					// Zero or unavailable: show empty graph with 0Mi 0.0%
					memGraph = ui.BarGraph(10, 0, colorKeys)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] 0Mi 0.0%%",
						memGraph,
					)
				}

				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  memMetrics,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)
			}
		}
	}
}

func (p *podPanel) DrawFooter(_ interface{}) {}

func (p *podPanel) Clear() {
	p.list.Clear()
	p.Layout(nil)
	p.DrawHeader(p.listCols)
}

func (p *podPanel) GetRootView() tview.Primitive {
	return p.root
}

func (p *podPanel) GetChildrenViews() []tview.Primitive {
	return p.children
}
