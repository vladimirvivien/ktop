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
}

func NewPodPanel(app *application.Application, title string) ui.Panel {
	p := &podPanel{
		app:        app,
		title:      title,
		sortColumn: "NAMESPACE", // Default sort by NAMESPACE then NAME
		sortAsc:    true,        // Default ascending
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

		// Capture keyboard events for sorting shortcuts
		p.list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyRune {
				if p.handleSortKey(event.Rune()) {
					return nil // Event consumed
				}
			}
			return event // Pass through unhandled events
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
