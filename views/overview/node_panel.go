package overview

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/internal/snapshot"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/api/resource"
)

// NodeSelectedCallback is called when a node is selected (Enter pressed)
type NodeSelectedCallback func(nodeName string)

type nodePanel struct {
	app         *application.Application
	title       string
	root        *tview.Flex
	children    []tview.Primitive
	listCols    []string
	list        *tview.Table
	laidout     bool
	colMap      map[string]int    // Maps column name to position index
	sortColumn  string            // Current sort column
	sortAsc     bool              // Sort direction: true=ascending, false=descending
	currentData []model.NodeModel // Store current data for re-sorting
	filter      *ui.FilterState   // Filter state for row filtering
	tableCtrl   *ui.TableController

	// Stateful sparklines for smooth sliding animation
	cpuSparklines map[string]*ui.Sparkline // key: node name
	memSparklines map[string]*ui.Sparkline

	// Callback for node selection
	onNodeSelected NodeSelectedCallback
}

func NewNodePanel(app *application.Application, title string) ui.Panel {
	p := &nodePanel{
		app:           app,
		title:         title,
		sortColumn:    "NAME", // Default sort by NAME
		sortAsc:       true,   // Default ascending
		filter:        &ui.FilterState{},
		cpuSparklines: make(map[string]*ui.Sparkline),
		memSparklines: make(map[string]*ui.Sparkline),
	}
	p.Layout(nil)
	return p
}

func (p *nodePanel) GetTitle() string {
	return p.title
}

// SetOnNodeSelected sets the callback for when a node is selected (Enter pressed)
func (p *nodePanel) SetOnNodeSelected(callback NodeSelectedCallback) {
	p.onNodeSelected = callback
}

// SetOnConfigureColumns sets the callback when 'C' is pressed
func (p *nodePanel) SetOnConfigureColumns(callback func()) {
	if p.tableCtrl != nil {
		p.tableCtrl.SetOnConfigureColumns(callback)
	}
}

// GetVisibleColumns returns currently visible column names
func (p *nodePanel) GetVisibleColumns() []string {
	return p.listCols
}

// SetVisibleColumns updates the visible columns and immediately redraws the table
func (p *nodePanel) SetVisibleColumns(cols []string) {
	p.listCols = cols
	if p.list != nil {
		p.list.Clear()
		p.DrawHeader(cols)
		if len(p.currentData) > 0 {
			p.DrawBody(p.currentData)
		}
	}
	if p.app != nil {
		p.app.Refresh()
	}
}

// getNodeNameFromRow returns the node name from the given row index
func (p *nodePanel) getNodeNameFromRow(row int) string {
	// Row 0 is header, data starts at row 1
	// We need to map from displayed row to the filtered data index
	dataIndex := row - 1 // Subtract 1 for header

	// Get filtered nodes (same logic as DrawBody)
	var filteredNodes []model.NodeModel
	if p.filter.IsFiltering() && p.filter.Text != "" {
		for _, node := range p.currentData {
			if p.filter.MatchesRow(p.getNodeCells(node)) {
				filteredNodes = append(filteredNodes, node)
			}
		}
	} else {
		filteredNodes = p.currentData
	}

	// Sort the filtered nodes the same way as DrawBody
	sortCol, sortAsc := p.GetSortParams()
	model.SortNodeModelsBy(filteredNodes, sortCol, sortAsc)

	if dataIndex >= 0 && dataIndex < len(filteredNodes) {
		return filteredNodes[dataIndex].Name
	}
	return ""
}

// getSparkline returns an existing sparkline or creates a new one
func (p *nodePanel) getSparkline(sparklines map[string]*ui.Sparkline, key string, width int, colors ui.ColorKeys) *ui.Sparkline {
	return ui.GetSparkline(sparklines, key, width, colors)
}

func (p *nodePanel) Layout(_ any) {
	if !p.laidout {
		p.list = tview.NewTable()
		p.list.SetFixed(1, 0)
		p.list.SetBorder(false)
		p.list.SetBorders(false)
		p.list.SetFocusFunc(func() {
			p.list.SetSelectable(true, false)
			p.list.SetSelectedStyle(ui.SelectedRowStyle())
			p.list.Select(1, 0) // Select row 1 (first data row), column 0
		})
		p.list.SetBlurFunc(func() {
			p.list.SetSelectable(false, false)
		})

		nodeColumnToKey := map[string]rune{
			"NAME":     'n',
			"STATUS":   'a',
			"RST":      'r',
			"IP":       'i',
			"PODS":     'p',
			"TAINTS":   't',
			"PRESSURE": 's',
			"VOLS":     'v',
			"DISK":     'd',
			"CPU":      'c',
			"MEM":      'm',
			"STALL":    'l',
		}

		p.tableCtrl = ui.NewTableController(p.list, ui.TableControllerConfig{
			DefaultSortColumn: "NAME",
			DefaultSortAsc:    true,
			ColumnToKey:       nodeColumnToKey,
			OnRedraw:          p.redrawWithFilter,
			OnSelect: func(row int) bool {
				nodeName := p.getNodeNameFromRow(row)
				if nodeName != "" && p.onNodeSelected != nil {
					p.onNodeSelected(nodeName)
					return true
				}
				return false
			},
			OnYank: func(row int) bool {
				nodeName := p.getNodeNameFromRow(row)
				if nodeName != "" {
					_ = ui.CopyToClipboard(nodeName)
					if p.app != nil {
						p.app.ShowToast(fmt.Sprintf("Copied \"%s\" to clipboard", nodeName), ui.ToastSuccess, 2*time.Second)
					}
					return true
				}
				return false
			},
		})
		p.filter = p.tableCtrl.Filter()

		p.list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			return p.tableCtrl.HandleInput(event)
		})

		p.root = tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(p.list, 0, 1, true)
		p.root.SetBorder(true)
		p.root.SetTitle(p.GetTitle())
		p.root.SetTitleAlign(tview.AlignLeft)
		p.laidout = true
	}
}

// formatColumnHeader formats a column header with keyboard shortcut hint and sort indicator
func (p *nodePanel) formatColumnHeader(col string) string {
	return p.tableCtrl.FormatColumnHeader(col)
}

func (p *nodePanel) DrawHeader(data any) {
	cols, ok := data.([]string)
	if !ok {
		panic(fmt.Sprintf("nodePanel.DrawHeader got unexpected data type %T", data))
	}

	p.tableCtrl.SetVisibleColumns(cols)

	// Initialize a new column map
	p.colMap = make(map[string]int)

	// Reserve index 0 for the legend column
	p.list.SetCell(0, 0,
		ui.StyleHeaderCell(tview.NewTableCell("").
			SetAlign(tview.AlignCenter).
			SetMaxWidth(1).
			SetExpansion(0).
			SetSelectable(true)),
	)

	p.listCols = cols

	// Set column headers and build column map
	for i, col := range p.listCols {
		pos := i + 1

		// Format column header with keyboard shortcut hint and sort indicator
		headerText := p.formatColumnHeader(col)

		p.list.SetCell(0, pos,
			ui.StyleHeaderCell(tview.NewTableCell(headerText).
				SetAlign(tview.AlignLeft).
				SetExpansion(100).
				SetSelectable(true)),
		)

		// Map column name to its position
		p.colMap[col] = pos
	}
}

// GetSortParams returns the current sort column and direction
func (p *nodePanel) GetSortParams() (string, bool) {
	return p.tableCtrl.GetSortParams()
}

// toggleSort toggles the sort column and direction
func (p *nodePanel) toggleSort(columnName string) {
	p.tableCtrl.ToggleSort(columnName)
	p.sortColumn, p.sortAsc = p.tableCtrl.GetSortParams()
}

// handleSortKey processes keyboard shortcuts for sorting
// Returns true if the key was handled, false otherwise
func (p *nodePanel) handleSortKey(key rune) bool {
	handled := p.tableCtrl.HandleSortKey(key)
	if handled {
		p.sortColumn, p.sortAsc = p.tableCtrl.GetSortParams()
	}
	return handled
}

func (p *nodePanel) DrawBody(data any) {
	var nodes []model.NodeModel
	switch v := data.(type) {
	case *snapshot.Snapshot:
		if v != nil {
			nodes = make([]model.NodeModel, len(v.Nodes))
			for i, nd := range v.Nodes {
				nodes[i] = nd.ToNodeModel()
			}
		}
	case []model.NodeModel:
		nodes = v
	default:
		panic(fmt.Sprintf("NodePanel.DrawBody: unexpected type %T", data))
	}

	// Prune sparkline entries for nodes that are no longer present
	activeNodes := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		activeNodes[node.Name] = true
	}
	ui.PruneSparklines(p.cpuSparklines, activeNodes)
	ui.PruneSparklines(p.memSparklines, activeNodes)

	// Store the data for re-sorting
	p.currentData = nodes

	// Sort nodes according to current sort state
	sortCol, sortAsc := p.GetSortParams()
	model.SortNodeModelsBy(nodes, sortCol, sortAsc)

	var cpuRatio, memRatio ui.Ratio
	var cpuGraph, memGraph string
	var cpuMetrics, memMetrics string
	colorKeys := ui.DefaultColorKeys()
	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()
	valTag := ""
	if !mono {
		valTag = "[" + ui.TagFg(pal.DataSecondary) + "]"
	}

	// Apply filter and track counts
	p.filter.TotalRows = len(nodes)
	p.filter.MatchRows = 0

	// Filter nodes if filter is active or editing
	var filteredNodes []model.NodeModel
	if p.filter.IsFiltering() && p.filter.Text != "" {
		for _, node := range nodes {
			if p.filter.MatchesRow(p.getNodeCells(node)) {
				filteredNodes = append(filteredNodes, node)
			}
		}
		p.filter.MatchRows = len(filteredNodes)
	} else {
		filteredNodes = nodes
		p.filter.MatchRows = len(nodes)
	}

	// Update title with scroll position indicator
	p.updateTitle(len(filteredNodes))

	rowIdx := 0
	for _, node := range filteredNodes {
		rowIdx++ // offset for header-row

		// Determine row color based on node status
		// Unhealthy nodes get their status color for the entire row
		rowColor := ui.GetRowColorForStatus(node.Status, "node")

		// Always render the legend column
		controlLegend := ""
		if node.Controller {
			controlLegend = ui.Icons.TrafficLight
		}

		legendColor := pal.Accent
		if mono {
			legendColor = tcell.ColorDefault
		}
		p.list.SetCell(
			rowIdx, 0,
			&tview.TableCell{
				Text:          controlLegend,
				Color:         legendColor,
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
				nameColor := rowColor
				nameAttrs := tcell.AttrNone
				if node.Unschedulable {
					if mono {
						nameColor = tcell.ColorDefault
						nameAttrs = tcell.AttrDim
					} else {
						nameColor = pal.StatusMuted
					}
				}
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:       node.Name,
						Color:      nameColor,
						Attributes: nameAttrs,
						Align:      tview.AlignLeft,
						MaxWidth:   20,
					},
				)

			case "STATUS":
				maxWidth := 8
				if mono {
					maxWidth = 10
				}
				p.list.SetCell(
					rowIdx, colIdx,
					ui.StyleStatusCell(&tview.TableCell{
						Text:     node.Status,
						Align:    tview.AlignLeft,
						MaxWidth: maxWidth,
					}, node.Status, "node"),
				)

			case "RST":
				rstColor := pal.StatusOK
				rstAttrs := tcell.AttrNone
				if node.Restarts > 0 {
					if mono {
						rstAttrs = tcell.AttrBold
					} else {
						rstColor = pal.StatusWarning
					}
				}
				if mono {
					rstColor = tcell.ColorDefault
				}
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:       fmt.Sprintf("%d", node.Restarts),
						Color:      rstColor,
						Attributes: rstAttrs,
						Align:      tview.AlignLeft,
						MaxWidth:   5,
					},
				)

			case "IP":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     node.InternalIP,
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 15,
					},
				)

			case "PODS":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     fmt.Sprintf("%d", node.PodsCount),
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 5,
					},
				)

			case "TAINTS":
				taintsColor := pal.StatusOK
				taintsAttrs := tcell.AttrNone
				if node.TaintCount > 0 {
					if mono {
						taintsAttrs = tcell.AttrBold
					} else {
						taintsColor = pal.StatusWarning
					}
				}
				if mono {
					taintsColor = tcell.ColorDefault
				}
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:       fmt.Sprintf("%d", node.TaintCount),
						Color:      taintsColor,
						Attributes: taintsAttrs,
						Align:      tview.AlignLeft,
						MaxWidth:   4,
					},
				)

			case "PRESSURE":
				var pressureText string
				pressureColor := pal.StatusOK
				pressureAttrs := tcell.AttrNone
				if len(node.Pressures) == 0 {
					pressureText = "none"
				} else {
					pressureText = strings.Join(node.Pressures, "/")
					if mono {
						pressureText = "! " + pressureText
						pressureAttrs = tcell.AttrUnderline | tcell.AttrBold
					} else {
						pressureColor = pal.StatusError
					}
				}
				if mono {
					pressureColor = tcell.ColorDefault
				}
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:       pressureText,
						Color:      pressureColor,
						Attributes: pressureAttrs,
						Align:      tview.AlignLeft,
						MaxWidth:   12,
					},
				)

			case "VOLS":
				volsText := fmt.Sprintf("%d/%d", node.VolumesInUse, node.VolumesAttached)
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     volsText,
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 5,
					},
				)

			case "DISK":
				diskText := fmt.Sprintf("%dGi", node.AllocatableStorageQty.ScaledValue(resource.Giga))
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     diskText,
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 7,
					},
				)

			case "CPU":
				graphWidth := 10
				// Get or create stateful sparkline for this node
				cpuSparkline := p.getSparkline(p.cpuSparklines, node.Name, graphWidth, colorKeys)

				var cpuVal, allocCPU int64
				if node.UsageCpuQty != nil {
					cpuVal = node.UsageCpuQty.MilliValue()
				} else if node.RequestedPodCpuQty != nil {
					cpuVal = node.RequestedPodCpuQty.MilliValue()
				}
				if node.AllocatableCpuQty != nil {
					allocCPU = node.AllocatableCpuQty.MilliValue()
				}
				cpuRatio = ui.GetRatio(float64(cpuVal), float64(allocCPU))
				cpuPercentage := float64(cpuRatio) * 100
				cpuPctTag := ""
				if !mono {
					cpuPctTag = "[" + ui.GetResourcePercentageColor(cpuPercentage) + "]"
				}
				cpuSparkline.Push(float64(cpuRatio))
				cpuGraph = cpuSparkline.RenderText()
				cpuTrend := cpuSparkline.TrendIndicator(cpuPercentage)
				cpuMetrics = fmt.Sprintf(
					"%s[%s%s] %5dm %s%5.1f%%%s %s",
					valTag, cpuGraph, valTag, cpuVal, cpuPctTag, cpuPercentage, valTag, cpuTrend,
				)

				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  cpuMetrics,
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "MEM":
				graphWidth := 10
				// Get or create stateful sparkline for this node's memory
				memSparkline := p.getSparkline(p.memSparklines, node.Name, graphWidth, colorKeys)

				memQty := node.UsageMemQty
				if memQty == nil {
					memQty = node.RequestedPodMemQty
				}
				var memVal, allocMem int64
				if memQty != nil {
					memVal = memQty.MilliValue()
				}
				if node.AllocatableMemQty != nil {
					allocMem = node.AllocatableMemQty.MilliValue()
				}
				memRatio = ui.GetRatio(float64(memVal), float64(allocMem))
				memPercentage := float64(memRatio) * 100
				memPctTag := ""
				if !mono {
					memPctTag = "[" + ui.GetResourcePercentageColor(memPercentage) + "]"
				}
				memSparkline.Push(float64(memRatio))
				memGraph = memSparkline.RenderText()
				memTrend := memSparkline.TrendIndicator(memPercentage)
				memMetrics = fmt.Sprintf(
					"%s[%s%s] %s %s%5.1f%%%s %s",
					valTag, memGraph, valTag, ui.FormatMemory(memQty), memPctTag, memPercentage, valTag, memTrend,
				)

				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  memMetrics,
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "STALL":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     renderStallCell(node.PSI, node.PSIKernelSupported),
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 12,
					},
				)
			}
		}
	}
}

func (p *nodePanel) DrawFooter(_ any) {}

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

// SetFocused implements ui.FocusablePanel - updates visual focus state
func (p *nodePanel) SetFocused(focused bool) {
	ui.SetFlexFocused(p.root, focused)
}

// getNodeCells extracts text values from a node model for filter matching
func (p *nodePanel) getNodeCells(node model.NodeModel) []string {
	pressureText := "none"
	if len(node.Pressures) > 0 {
		pressureText = strings.Join(node.Pressures, "/")
	}
	return []string{
		node.Name,
		node.Status,
		fmt.Sprintf("%d", node.Restarts),
		node.InternalIP,
		fmt.Sprintf("%d", node.PodsCount),
		fmt.Sprintf("%d", node.TaintCount),
		pressureText,
		fmt.Sprintf("%d/%d", node.VolumesInUse, node.VolumesAttached),
	}
}

// redrawWithFilter redraws the table with current filter applied
func (p *nodePanel) redrawWithFilter() {
	if len(p.currentData) > 0 {
		p.list.Clear()
		p.DrawHeader(p.listCols)
		p.DrawBody(p.currentData)
		if p.app != nil {
			p.app.Refresh()
		}
	} else {
		// No data yet, just update title
		p.updateTitle(0)
		if p.app != nil {
			p.app.Refresh()
		}
	}
}

// HasEscapableState implements ui.EscapablePanel
func (p *nodePanel) HasEscapableState() bool {
	return p.tableCtrl.HasEscapableState()
}

// IsFilterEditing returns whether the filter input is currently being edited
func (p *nodePanel) IsFilterEditing() bool {
	return p.tableCtrl.IsFilterEditing()
}

// HandleEscape implements ui.EscapablePanel - handles ESC key press
func (p *nodePanel) HandleEscape() bool {
	return p.tableCtrl.HandleEscape()
}

// updateTitle updates the panel title with scroll position indicator and filter state
func (p *nodePanel) updateTitle(totalRows int) {
	var disconnectedSuffix string
	if p.app != nil && p.app.IsAPIDisconnected() {
		disconnectedSuffix = " " + ui.ErrorMarkup("[DISCONNECTED - Press R to reconnect]")
	}
	p.root.SetTitle(p.tableCtrl.FormatTitleWithScroll("Nodes", ui.Icons.Factory, totalRows, disconnectedSuffix))
}
