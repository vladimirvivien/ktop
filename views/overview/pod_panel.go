package overview

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

type podPanel struct {
	app         *application.Application
	title       string
	root        *tview.Flex
	children    []tview.Primitive
	listCols    []string
	list        *tview.Table
	laidout     bool
	colMap      map[string]int   // Maps column name to position index
	sortColumn  string           // Current sort column
	sortAsc     bool             // Sort direction: true=ascending, false=descending
	currentData []model.PodModel // Store current data for re-sorting
	filter      *ui.FilterState  // Filter state for row filtering
}

func NewPodPanel(app *application.Application, title string) ui.Panel {
	p := &podPanel{
		app:        app,
		title:      title,
		sortColumn: "NAMESPACE", // Default sort by NAMESPACE then NAME
		sortAsc:    true,        // Default ascending
		filter:     &ui.FilterState{},
	}
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
func (p *podPanel) formatColumnHeader(col string) string {
	if len(col) == 0 {
		return col
	}

	// Map column name to keyboard shortcut key
	columnToKey := map[string]rune{
		"NAMESPACE": 'n',
		"POD":       'p',
		"READY":     'r',
		"STATUS":    's',
		"RESTARTS":  't', // Uses 't' at position 3
		"AGE":       'a',
		"VOLS":      'v',
		"IP":        'i',
		"NODE":      'o', // Uses 'o' at position 1
		"CPU":       'c',
		"MEMORY":    'm',
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
		// Format column header with keyboard shortcut hint and sort indicator
		headerText := p.formatColumnHeader(col)

		p.list.SetCell(0, i,
			tview.NewTableCell(headerText).
				SetTextColor(tcell.ColorWhite).
				SetBackgroundColor(tcell.ColorDarkGreen).
				SetAlign(tview.AlignLeft).
				SetExpansion(100).
				SetSelectable(true),
		)

		// Map column name to position
		p.colMap[col] = i
	}
	p.list.SetFixed(1, 0)
}

// GetSortParams returns the current sort column and direction
func (p *podPanel) GetSortParams() (string, bool) {
	return p.sortColumn, p.sortAsc
}

// toggleSort toggles the sort column and direction
func (p *podPanel) toggleSort(columnName string) {
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
func (p *podPanel) handleSortKey(key rune) bool {
	// Map keyboard shortcuts to column names
	keyToColumn := map[rune]string{
		'n': "NAMESPACE",
		'p': "POD",
		'r': "READY",
		's': "STATUS",
		't': "RESTARTS",
		'a': "AGE",
		'v': "VOLS",
		'i': "IP",
		'o': "NODE",
		'c': "CPU",
		'm': "MEMORY",
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

func (p *podPanel) DrawBody(data interface{}) {
	pods, ok := data.([]model.PodModel)
	if !ok {
		panic(fmt.Sprintf("PodPanel.DrawBody got unexpected type %T", data))
	}

	// Store the data for re-sorting
	p.currentData = pods

	// Sort pods according to current sort state
	model.SortPodModelsBy(pods, p.sortColumn, p.sortAsc)

	colorKeys := ui.ColorKeys{0: "olivedrab", 50: "yellow", 90: "red"}
	var cpuRatio, memRatio ui.Ratio
	var cpuGraph, memGraph string
	var cpuMetrics, memMetrics string

	// Apply filter and track counts
	p.filter.TotalRows = len(pods)
	p.filter.MatchRows = 0

	// Filter pods if filter is active or editing
	var filteredPods []model.PodModel
	if p.filter.IsFiltering() && p.filter.Text != "" {
		for _, pod := range pods {
			if p.filter.MatchesRow(p.getPodCells(pod)) {
				filteredPods = append(filteredPods, pod)
			}
		}
		p.filter.MatchRows = len(filteredPods)
	} else {
		filteredPods = pods
		p.filter.MatchRows = len(pods)
	}

	// Update title with scroll position indicator
	p.updateTitle(len(filteredPods))

	rowIdx := 0
	for _, pod := range filteredPods {
		rowIdx++ // offset for header row

		// Determine row color based on pod status
		// Unhealthy pods get their status color for the entire row
		rowColor := ui.GetRowColorForStatus(pod.Status, "pod")

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
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "POD":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  pod.Name,
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "READY":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers),
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "STATUS":
				// Status column: green for healthy, status color for unhealthy
				statusColor := ui.GetTcellColor(ui.GetStatusColor(pod.Status, "pod"))
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  pod.Status,
						Color: statusColor,
						Align: tview.AlignLeft,
					},
				)

			case "RESTARTS":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%d", pod.Restarts),
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "AGE":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  pod.TimeSince,
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "VOLS":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  fmt.Sprintf("%d/%d", pod.Volumes, pod.VolMounts),
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "IP":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  pod.IP,
						Color: rowColor,
						Align: tview.AlignLeft,
					},
				)

			case "NODE":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:  pod.Node,
						Color: rowColor,
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
					cpuPercentage := float64(cpuRatio) * 100
					cpuPercentageColor := ui.GetResourcePercentageColor(cpuPercentage)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] %dm [%s]%.1f%%[white]",
						cpuGraph, pod.PodUsageCpuQty.MilliValue(), cpuPercentageColor, cpuPercentage,
					)
				} else if hasRequestMetrics && hasAllocatable {
					// Fallback: show requested with graph: [|         ] 100m 2.5%
					cpuRatio = ui.GetRatio(float64(pod.PodRequestedCpuQty.MilliValue()), float64(pod.NodeAllocatableCpuQty.MilliValue()))
					cpuGraph = ui.BarGraph(10, cpuRatio, colorKeys)
					cpuPercentage := float64(cpuRatio) * 100
					cpuPercentageColor := ui.GetResourcePercentageColor(cpuPercentage)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] %dm [%s]%.1f%%[white]",
						cpuGraph, pod.PodRequestedCpuQty.MilliValue(), cpuPercentageColor, cpuPercentage,
					)
				} else {
					// Zero or unavailable: show empty graph with 0m 0.0%
					cpuGraph = ui.BarGraph(10, 0, colorKeys)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] 0m [green]0.0%%[white]",
						cpuGraph,
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

			case "MEMORY":
				// Check if we have actual usage metrics (non-zero values)
				hasUsageMetrics := pod.PodUsageMemQty != nil && pod.PodUsageMemQty.Value() > 0
				hasRequestMetrics := pod.PodRequestedMemQty != nil && pod.PodRequestedMemQty.Value() > 0
				hasAllocatable := pod.NodeAllocatableMemQty != nil && pod.NodeAllocatableMemQty.Value() > 0

				if hasUsageMetrics && hasAllocatable {
					// Display usage with graph: [||        ] 366Mi 0.5%
					memRatio = ui.GetRatio(float64(pod.PodUsageMemQty.Value()), float64(pod.NodeAllocatableMemQty.Value()))
					memGraph = ui.BarGraph(10, memRatio, colorKeys)
					memPercentage := float64(memRatio) * 100
					memPercentageColor := ui.GetResourcePercentageColor(memPercentage)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] %s [%s]%.1f%%[white]",
						memGraph,
						ui.FormatMemory(pod.PodUsageMemQty),
						memPercentageColor, memPercentage,
					)
				} else if hasRequestMetrics && hasAllocatable {
					// Fallback: show requested with graph: [|         ] 512Mi 0.5%
					memRatio = ui.GetRatio(float64(pod.PodRequestedMemQty.Value()), float64(pod.NodeAllocatableMemQty.Value()))
					memGraph = ui.BarGraph(10, memRatio, colorKeys)
					memPercentage := float64(memRatio) * 100
					memPercentageColor := ui.GetResourcePercentageColor(memPercentage)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] %s [%s]%.1f%%[white]",
						memGraph,
						ui.FormatMemory(pod.PodRequestedMemQty),
						memPercentageColor, memPercentage,
					)
				} else {
					// Zero or unavailable: show empty graph with 0Mi 0.0%
					memGraph = ui.BarGraph(10, 0, colorKeys)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] 0Mi [green]0.0%%[white]",
						memGraph,
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

// getPodCells extracts text values from a pod model for filter matching
func (p *podPanel) getPodCells(pod model.PodModel) []string {
	return []string{
		pod.Namespace,
		pod.Name,
		pod.Status,
		pod.IP,
		pod.Node,
		pod.TimeSince,
	}
}

// redrawWithFilter redraws the table with current filter applied
func (p *podPanel) redrawWithFilter() {
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
func (p *podPanel) HasEscapableState() bool {
	return p.filter.HasEscapableState()
}

// HandleEscape implements ui.EscapablePanel - handles ESC key press
func (p *podPanel) HandleEscape() bool {
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
func (p *podPanel) updateTitle(totalRows int) {
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
	title := p.filter.FormatTitleWithScroll("Pods", ui.Icons.Package, firstVisible, lastVisible, totalRows, scrollIndicator, disconnectedSuffix)
	p.root.SetTitle(title)
}
