package overview

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	coreV1 "k8s.io/api/core/v1"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

type nodePanel struct {
	app *application.Application
	title     string
	root      *tview.Flex
	children []tview.Primitive
	listCols  []string
	list      *tview.Table
}

func NewNodePanel(app *application.Application, title string) ui.Panel {
	p := &nodePanel{app: app, title: title, list: tview.NewTable()}

	// set attributes
	p.list.SetFixed(1, 0)
	p.list.SetSelectable(true, false)

	// set handlers
	p.list.SetSelectedFunc(func(row int, col int) {
		modal := tview.NewModal().
			SetText(fmt.Sprintf("Selected {row:%d, col:%d}", row, col))
		p.root.AddItem(modal, 0,0,true)
		modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyESC {
				p.root.RemoveItem(modal)
				return nil
			}
			return event
		})

	})

	p.children = append(p.children, p.list)

	p.Layout(nil)
	p.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.list, 0, 1, true)
	return p
}
func (p *nodePanel) GetTitle() string {
	return p.title
}
func (p *nodePanel) Layout(data interface{}) {
	p.list.SetBorder(true)
	p.list.SetBorders(false)
	p.list.SetTitle(p.GetTitle())
	p.list.SetTitleAlign(tview.AlignLeft)
	p.list.SetBorderColor(tcell.ColorWhite)
}

func (p *nodePanel) DrawHeader(data interface{}) {
	cols, ok := data.([]string)
	if !ok {
		panic(fmt.Sprintf("nodePanel.DrawHeader got unexpected data type %T", data))
	}

	// legend column
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
	}

}

func (p *nodePanel) DrawBody(data interface{}) {
	colorKeys := ui.ColorKeys{0: "green", 50: "yellow", 90: "red"}
	p.Clear()
	nodeList, ok := data.(*coreV1.NodeList)
	if !ok {
		panic(fmt.Sprintf("NodePanel.DrawBody: unexpected type %T", data))
	}
	nodeItems := nodeList.Items
	for i, nodeItem := range nodeItems {
		metrics, err := p.app.GetK8sClient().GetNodeMetrics(context.Background(), nodeItem.Name)
		if err != nil {
			metrics = new(metricsV1beta1.NodeMetrics)
		}
		node := &model.NodeModel{
			Node:        nodeItem,
			NodeStatus:  nodeItem.Status,
			AvailRes:    nodeItem.Status.Allocatable,
			NodeMetrics: metrics,
		}

		cpuRatio := ui.GetRatio(float64(node.CpuUsageMillis()), float64(node.CpuAvailMillis()))
		cpuGraph := ui.BarGraph(10, cpuRatio, colorKeys)

		memRatio := ui.GetRatio(float64(node.MemUsageMillis()), float64(node.MemAvailMillis()))
		memGraph := ui.BarGraph(10, memRatio, colorKeys)

		i++ // offset for header-row
		masterLegend := ""
		if node.NodeRole() == "Master" {
			masterLegend = fmt.Sprintf("%c", ui.Icons.Plane)
		}

		p.list.SetCell(
			i, 0,
			&tview.TableCell{
				Text:          masterLegend,
				Color:         tcell.ColorOrangeRed,
				Align:         tview.AlignCenter,
				NotSelectable: true,
			},
		)

		p.list.SetCell(
			i, 1,
			&tview.TableCell{
				Text:  node.Node.Name,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 2,
			&tview.TableCell{
				Text:  node.NodeState(),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 3,
			&tview.TableCell{
				Text:  node.NodeStatus.NodeInfo.KubeletVersion,
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 4,
			&tview.TableCell{
				Text:  fmt.Sprintf("%s/%s", node.GetNodeInternalIp(), node.GetNodeExternalIp()),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 5,
			&tview.TableCell{
				Text:  fmt.Sprintf("%s;%s", node.NodeStatus.NodeInfo.OSImage, node.NodeStatus.NodeInfo.Architecture),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 6,
			&tview.TableCell{
				Text:  fmt.Sprintf("%d/%dMi", node.CpuAvail(), node.MemAvail()),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 7,
			&tview.TableCell{
				Text:  fmt.Sprintf("%dGi", node.EphStorageAvail()),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)

		p.list.SetCell(
			i, 8,
			&tview.TableCell{
				Text:  fmt.Sprintf("[white][%s[white]] %dm (%1.0f%%)", cpuGraph, node.CpuUsageMillis(), cpuRatio*100),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		).SetCell(
			i, 9,
			&tview.TableCell{
				Text:  fmt.Sprintf("[white][%s[white]] %dMi (%01.0f%%)", memGraph, node.MemUsage(), memRatio*100),
				Color: tcell.ColorYellow,
				Align: tview.AlignLeft,
			},
		)
	}
}

func (p *nodePanel) DrawFooter(data interface{}) {}
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