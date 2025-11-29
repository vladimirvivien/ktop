package overview

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/api/resource"
)

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

	// Stateful sparklines for smooth sliding animation
	cpuSparklines map[string]*ui.SparklineState // key: node name
	memSparklines map[string]*ui.SparklineState
}

func NewNodePanel(app *application.Application, title string) ui.Panel {
	p := &nodePanel{
		app:           app,
		title:         title,
		sortColumn:    "NAME", // Default sort by NAME
		sortAsc:       true,   // Default ascending
		filter:        &ui.FilterState{},
		cpuSparklines: make(map[string]*ui.SparklineState),
		memSparklines: make(map[string]*ui.SparklineState),
	}
	p.Layout(nil)
	return p
}

func (p *nodePanel) GetTitle() string {
	return p.title
}

// getSparkline returns an existing sparkline or creates a new one
func (p *nodePanel) getSparkline(sparklines map[string]*ui.SparklineState, key string, width int, colors ui.ColorKeys) *ui.SparklineState {
	if spark, ok := sparklines[key]; ok {
		return spark
	}
	spark := ui.NewSparklineState(width, colors)
	sparklines[key] = spark
	return spark
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
			p.list.Select(1, 0) // Select row 1 (first data row), column 0
		})
		p.list.SetBlurFunc(func() {
			p.list.SetSelectable(false, false)
		})

		// Capture keyboard events for filtering and sorting
		// ESC handling: panels consume ESC when they have state (filter),
		// otherwise pass through to let global handler quit the app
		p.list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			// Handle filter edit mode input
			if p.filter.Editing {
				switch event.Key() {
				case tcell.KeyEscape:
					p.filter.Cancel()
					p.redrawWithFilter()
					return nil // Consume - don't bubble to global handler
				case tcell.KeyEnter:
					p.filter.Confirm()
					p.redrawWithFilter()
					return nil
				case tcell.KeyBackspace, tcell.KeyBackspace2:
					if p.filter.HandleBackspace() {
						p.redrawWithFilter()
					}
					return nil
				case tcell.KeyRune:
					p.filter.AppendChar(event.Rune())
					p.redrawWithFilter()
					return nil
				}
				return nil // Consume all keys in edit mode
			}

			// Normal mode - ESC clears active filter
			if event.Key() == tcell.KeyEscape && p.filter.Active {
				p.filter.Clear()
				p.redrawWithFilter()
				return nil // Consume - don't quit app
			}

			// Filter trigger
			if event.Key() == tcell.KeyRune && event.Rune() == '/' {
				p.filter.StartEditing()
				p.redrawWithFilter()
				return nil
			}

			// Handle sorting shortcuts
			if event.Key() == tcell.KeyRune {
				if p.handleSortKey(event.Rune()) {
					return nil // Event consumed
				}
			}
			return event // Pass through - let it bubble to global handler
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
	if len(col) == 0 {
		return col
	}

	// Map column name to keyboard shortcut key
	columnToKey := map[string]rune{
		"NAME":        'n',
		"STATUS":      's',
		"AGE":         'a',
		"VERSION":     'v',
		"INT/EXT IPs": 'i',
		"OS/ARC":      'o',
		"PODS/IMGs":   'p',
		"DISK":        'd',
		"CPU":         'c',
		"MEM":         'm',
	}

	// Find the shortcut key for this column
	key, exists := columnToKey[col]
	var formatted string

	if exists {
		// Find position of key character (case-insensitive)
		keyPos := -1
		colUpper := strings.ToUpper(col)
		keyUpper := strings.ToUpper(string(key))
		for i, ch := range colUpper {
			if string(ch) == keyUpper {
				keyPos = i
				break
			}
		}

		if keyPos >= 0 && keyPos < len(col) {
			// Build formatted string with highlighted character at correct position
			before := col[:keyPos]
			highlighted := string(col[keyPos])
			after := ""
			if keyPos+1 < len(col) {
				after = col[keyPos+1:]
			}
			formatted = fmt.Sprintf("%s[%s::b]%s[%s::-]%s",
				before, ui.Theme.HeaderShortcutKey, highlighted, ui.Theme.HeaderForeground, after)
		} else {
			// Fallback: highlight first character if position not found
			formatted = fmt.Sprintf("[%s::b]%c[%s::-]%s",
				ui.Theme.HeaderShortcutKey, col[0], ui.Theme.HeaderForeground, col[1:])
		}
	} else {
		// No mapping found, highlight first character
		formatted = fmt.Sprintf("[%s::b]%c[%s::-]%s",
			ui.Theme.HeaderShortcutKey, col[0], ui.Theme.HeaderForeground, col[1:])
	}

	// Add sort indicator if this is the active sort column
	if col == p.sortColumn {
		if p.sortAsc {
			formatted += " ▲" // Ascending
		} else {
			formatted += " ▼" // Descending
		}
	}

	return formatted
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
			SetSelectable(true),
	)

	p.listCols = cols

	// Set column headers and build column map
	for i, col := range p.listCols {
		pos := i + 1

		// Format column header with keyboard shortcut hint and sort indicator
		headerText := p.formatColumnHeader(col)

		p.list.SetCell(0, pos,
			tview.NewTableCell(headerText).
				SetTextColor(tcell.ColorWhite).
				SetAlign(tview.AlignLeft).
				SetBackgroundColor(tcell.ColorDarkGreen).
				SetExpansion(100).
				SetSelectable(true),
		)

		// Map column name to its position
		p.colMap[col] = pos
	}
}

// GetSortParams returns the current sort column and direction
func (p *nodePanel) GetSortParams() (string, bool) {
	return p.sortColumn, p.sortAsc
}

// toggleSort toggles the sort column and direction
func (p *nodePanel) toggleSort(columnName string) {
	if columnName == p.sortColumn {
		// Same column - toggle direction
		p.sortAsc = !p.sortAsc
	} else {
		// New column - start with ascending
		p.sortColumn = columnName
		p.sortAsc = true
	}

	// Re-sort and redraw with current data
	if len(p.currentData) > 0 {
		// Clear and redraw header with updated sort indicators
		p.list.Clear()
		p.DrawHeader(p.listCols)

		// Redraw body with re-sorted data
		p.DrawBody(p.currentData)

		// Trigger screen refresh
		if p.app != nil {
			p.app.Refresh()
		}
	}
}

// handleSortKey processes keyboard shortcuts for sorting
// Returns true if the key was handled, false otherwise
func (p *nodePanel) handleSortKey(key rune) bool {
	// Map keyboard shortcuts to column names
	keyToColumn := map[rune]string{
		'n': "NAME",
		's': "STATUS",
		'a': "AGE",
		'v': "VERSION",
		'i': "INT/EXT IPs",
		'o': "OS/ARC",
		'p': "PODS/IMGs",
		'd': "DISK",
		'c': "CPU",
		'm': "MEM",
	}

	columnName, exists := keyToColumn[key]
	if !exists {
		return false
	}

	// Check if this column is visible (when using column filtering)
	columnVisible := false
	for _, col := range p.listCols {
		if col == columnName {
			columnVisible = true
			break
		}
	}

	if !columnVisible {
		return false
	}

	// Toggle sort for this column
	p.toggleSort(columnName)
	return true
}

func (p *nodePanel) DrawBody(data interface{}) {
	nodes, ok := data.([]model.NodeModel)
	if !ok {
		panic(fmt.Sprintf("NodePanel.DrawBody: unexpected type %T", data))
	}

	// Store the data for re-sorting
	p.currentData = nodes

	// Sort nodes according to current sort state
	model.SortNodeModelsBy(nodes, p.sortColumn, p.sortAsc)

	// Check if usage metrics are available by looking at the actual data in the models
	// Don't rely on AssertMetricsAvailable() as it's cached and unreliable
	metricsDisabled := true
	if len(nodes) > 0 && nodes[0].UsageMemQty != nil && nodes[0].UsageMemQty.Value() > 0 {
		metricsDisabled = false
	}
	var cpuRatio, memRatio ui.Ratio
	var cpuGraph, memGraph string
	var cpuMetrics, memMetrics string
	colorKeys := ui.ColorKeys{0: "olivedrab", 50: "yellow", 90: "red"}

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
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "STATUS":
				// Status column: green for healthy, status color for unhealthy
				statusColor := ui.GetTcellColor(ui.GetStatusColor(node.Status, "node"))
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  node.Status,
						Color: statusColor,
						Align: tview.AlignLeft,
					},
				)

			case "AGE":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  node.TimeSinceStart,
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "VERSION":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  node.KubeletVersion,
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "INT/EXT IPs":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%s/%s", node.InternalIP, node.ExternalIP),
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "OS/ARC":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%s/%s", node.OSImage, node.Architecture),
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "PODS/IMGs":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%d/%d", node.PodsCount, node.ContainerImagesCount),
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "DISK":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%dGi", node.AllocatableStorageQty.ScaledValue(resource.Giga)),
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "CPU":
				graphWidth := 10
				// Get or create stateful sparkline for this node
				cpuSparkline := p.getSparkline(p.cpuSparklines, node.Name, graphWidth, colorKeys)

				// Calculate CPU metrics
				if metricsDisabled || node.UsageCpuQty == nil {
					cpuRatio = ui.GetRatio(float64(node.RequestedPodCpuQty.MilliValue()), float64(node.AllocatableCpuQty.MilliValue()))
					cpuPercentage := float64(cpuRatio) * 100
					cpuPercentageColor := ui.GetResourcePercentageColor(cpuPercentage)
					cpuSparkline.Push(float64(cpuRatio))
					cpuGraph = cpuSparkline.Render()
					cpuTrend := cpuSparkline.TrendIndicator(cpuPercentage)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] %5dm [%s]%5.1f%%[white] %s",
						cpuGraph, node.RequestedPodCpuQty.MilliValue(), cpuPercentageColor, cpuPercentage, cpuTrend,
					)
				} else {
					cpuRatio = ui.GetRatio(float64(node.UsageCpuQty.MilliValue()), float64(node.AllocatableCpuQty.MilliValue()))
					cpuPercentage := float64(cpuRatio) * 100
					cpuPercentageColor := ui.GetResourcePercentageColor(cpuPercentage)
					cpuSparkline.Push(float64(cpuRatio))
					cpuGraph = cpuSparkline.Render()
					cpuTrend := cpuSparkline.TrendIndicator(cpuPercentage)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] %5dm [%s]%5.1f%%[white] %s",
						cpuGraph, node.UsageCpuQty.MilliValue(), cpuPercentageColor, cpuPercentage, cpuTrend,
					)
				}

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

				// Calculate memory metrics
				if metricsDisabled || node.UsageMemQty == nil {
					memRatio = ui.GetRatio(float64(node.RequestedPodMemQty.MilliValue()), float64(node.AllocatableMemQty.MilliValue()))
					memPercentage := float64(memRatio) * 100
					memPercentageColor := ui.GetResourcePercentageColor(memPercentage)
					memSparkline.Push(float64(memRatio))
					memGraph = memSparkline.Render()
					memTrend := memSparkline.TrendIndicator(memPercentage)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] %s [%s]%5.1f%%[white] %s",
						memGraph, ui.FormatMemory(node.RequestedPodMemQty), memPercentageColor, memPercentage, memTrend,
					)
				} else {
					memRatio = ui.GetRatio(float64(node.UsageMemQty.MilliValue()), float64(node.AllocatableMemQty.MilliValue()))
					memPercentage := float64(memRatio) * 100
					memPercentageColor := ui.GetResourcePercentageColor(memPercentage)
					memSparkline.Push(float64(memRatio))
					memGraph = memSparkline.Render()
					memTrend := memSparkline.TrendIndicator(memPercentage)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] %s [%s]%5.1f%%[white] %s",
						memGraph, ui.FormatMemory(node.UsageMemQty), memPercentageColor, memPercentage, memTrend,
					)
				}

				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  memMetrics,
						Color: rowColor,
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

// SetFocused implements ui.FocusablePanel - updates visual focus state
func (p *nodePanel) SetFocused(focused bool) {
	ui.SetFlexFocused(p.root, focused)
}

// getNodeCells extracts text values from a node model for filter matching
func (p *nodePanel) getNodeCells(node model.NodeModel) []string {
	return []string{
		node.Name,
		node.Status,
		node.TimeSinceStart,
		node.KubeletVersion,
		node.InternalIP,
		node.ExternalIP,
		node.OSImage,
		node.Architecture,
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
	return p.filter.HasEscapableState()
}

// HandleEscape implements ui.EscapablePanel - handles ESC key press
func (p *nodePanel) HandleEscape() bool {
	if p.filter.Editing {
		p.filter.Cancel()
		p.redrawWithFilter()
		return true
	}
	if p.filter.Active {
		p.filter.Clear()
		p.redrawWithFilter()
		return true
	}
	return false
}

// updateTitle updates the panel title with scroll position indicator and filter state
func (p *nodePanel) updateTitle(totalRows int) {
	// Get visible area dimensions
	_, _, _, height := p.list.GetInnerRect()
	visibleRows := height - 1 // Subtract header row

	offset, _ := p.list.GetOffset()

	// Handle disconnected state
	var disconnectedSuffix string
	if p.app.IsAPIDisconnected() {
		disconnectedSuffix = " [red][DISCONNECTED - Press R to reconnect][-]"
	}

	// Calculate visible range (1-indexed for display)
	firstVisible := offset + 1 // Convert 0-indexed to 1-indexed
	lastVisible := min(offset+visibleRows, totalRows)
	if totalRows == 0 {
		firstVisible = 0
		lastVisible = 0
	}

	// Determine scroll indicators
	var scrollIndicator string
	hasAbove := offset > 0
	hasBelow := (offset + visibleRows) < totalRows

	if hasAbove && hasBelow {
		scrollIndicator = " ↑↓"
	} else if hasAbove {
		scrollIndicator = " ↑"
	} else if hasBelow {
		scrollIndicator = " ↓"
	}

	// Use filter-aware title formatting
	title := p.filter.FormatTitleWithScroll("Nodes", ui.Icons.Factory, firstVisible, lastVisible, totalRows, scrollIndicator, disconnectedSuffix)
	p.root.SetTitle(title)
}
