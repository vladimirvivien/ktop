package overview

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/api/resource"
)

type nodePanel struct {
	app      *application.Application
	title    string
	root     *tview.Flex
	children []tview.Primitive
	listCols []string
	list     *tview.Table
	laidout  bool
	colMap   map[string]int // Maps column name to position index
}

func NewNodePanel(app *application.Application, title string) ui.Panel {
	p := &nodePanel{app: app, title: title}
	p.Layout(nil)
	return p
}
func (p *nodePanel) GetTitle() string {
	return p.title
}
func (p *nodePanel) Layout(_ interface{}) {
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

func (p *nodePanel) DrawHeader(data interface{}) {
	cols, ok := data.([]string)
	if !ok {
		panic(fmt.Sprintf("nodePanel.DrawHeader got unexpected data type %T", data))
	}

	// Initialize a new column map
	p.colMap = make(map[string]int)
	
	// Reserve index 0 for the legend column
	p.list.SetCell(0, 0,
		tview.NewTableCell("").
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignCenter).
			SetBackgroundColor(tcell.ColorDarkGreen).
			SetMaxWidth(1).
			SetExpansion(0).
			SetSelectable(false),
	)

	p.listCols = cols
	
	// Set column headers and build column map
	for i, col := range p.listCols {
		pos := i + 1
		p.list.SetCell(0, pos,
			tview.NewTableCell(col).
				SetTextColor(tcell.ColorWhite).
				SetAlign(tview.AlignLeft).
				SetBackgroundColor(tcell.ColorDarkGreen).
				SetExpansion(100).
				SetSelectable(false),
		)
		
		// Map column name to its position
		p.colMap[col] = pos
	}
}

func (p *nodePanel) DrawBody(data interface{}) {
	nodes, ok := data.([]model.NodeModel)
	if !ok {
		panic(fmt.Sprintf("NodePanel.DrawBody: unexpected type %T", data))
	}

	client := p.app.GetK8sClient()
	metricsDiabled := client.AssertMetricsAvailable() != nil
	var cpuRatio, memRatio ui.Ratio
	var cpuGraph, memGraph string
	var cpuMetrics, memMetrics string
	colorKeys := ui.ColorKeys{0: "green", 50: "yellow", 90: "red"}

	p.root.SetTitle(fmt.Sprintf("%s(%d) ", p.GetTitle(), len(nodes)))
	p.root.SetTitleAlign(tview.AlignLeft)

	for rowIdx, node := range nodes {
		rowIdx++ // offset for header-row
		
		// Always render the legend column
		controlLegend := ""
		if node.Controller {
			controlLegend = fmt.Sprintf("%c", ui.Icons.TrafficLight)
		}
		
		p.list.SetCell(
			rowIdx, 0,
			&tview.TableCell{
				Text:          controlLegend,
				Color:         tcell.ColorOrangeRed,
				Align:         tview.AlignCenter,
				NotSelectable: true,
			},
		)
		
		// Render each column that is included in the filtered view
		for _, colName := range p.listCols {
			colIdx, exists := p.colMap[colName]
			if !exists {
				continue
			}
			
			switch colName {
			case "NAME":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  node.Name,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)
				
			case "STATUS":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  node.Status,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)
				
			case "AGE":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  node.TimeSinceStart,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)
				
			case "VERSION":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  node.KubeletVersion,
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)
				
			case "INT/EXT IPs":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%s/%s", node.InternalIP, node.ExternalIP),
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)
				
			case "OS/ARC":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%s/%s", node.OSImage, node.Architecture),
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)
				
			case "PODS/IMGs":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%d/%d", node.PodsCount, node.ContainerImagesCount),
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)
				
			case "DISK":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%dGi", node.AllocatableStorageQty.ScaledValue(resource.Giga)),
						Color: tcell.ColorYellow,
						Align: tview.AlignLeft,
					},
				)
				
			case "CPU":
				// Calculate CPU metrics
				if metricsDiabled {
					cpuRatio = ui.GetRatio(float64(node.RequestedPodCpuQty.MilliValue()), float64(node.AllocatableCpuQty.MilliValue()))
					cpuGraph = ui.BarGraph(10, cpuRatio, colorKeys)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] %dm/%dm (%1.0f%%)",
						cpuGraph, node.RequestedPodCpuQty.MilliValue(), node.AllocatableCpuQty.MilliValue(), cpuRatio*100,
					)
				} else {
					cpuRatio = ui.GetRatio(float64(node.UsageCpuQty.MilliValue()), float64(node.AllocatableCpuQty.MilliValue()))
					cpuGraph = ui.BarGraph(10, cpuRatio, colorKeys)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] %dm/%dm (%1.0f%%)",
						cpuGraph, node.UsageCpuQty.MilliValue(), node.AllocatableCpuQty.MilliValue(), cpuRatio*100,
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
				
			case "MEM":
				// Calculate memory metrics
				if metricsDiabled {
					memRatio = ui.GetRatio(float64(node.RequestedPodMemQty.MilliValue()), float64(node.AllocatableMemQty.MilliValue()))
					memGraph = ui.BarGraph(10, memRatio, colorKeys)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] %dGi/%dGi (%1.0f%%)",
						memGraph, node.RequestedPodMemQty.ScaledValue(resource.Giga), node.AllocatableMemQty.ScaledValue(resource.Giga), memRatio*100,
					)
				} else {
					memRatio = ui.GetRatio(float64(node.UsageMemQty.MilliValue()), float64(node.AllocatableMemQty.MilliValue()))
					memGraph = ui.BarGraph(10, memRatio, colorKeys)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] %dGi/%dGi (%1.0f%%)",
						memGraph, node.UsageMemQty.ScaledValue(resource.Giga), node.AllocatableMemQty.ScaledValue(resource.Giga), memRatio*100,
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

func (p *nodePanel) DrawFooter(_ interface{}) {}

func (p *nodePanel) Clear() {
	p.list.Clear()
	p.Layout(nil)
	p.DrawHeader(p.listCols)
}

func (p *nodePanel) GetRootView() tview.Primitive {
	return p.root
}

func (p *nodePanel) GetChildrenViews() []tview.Primitive {
	return p.children
}