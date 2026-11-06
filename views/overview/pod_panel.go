package overview

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

// PodSelectedCallback is called when a pod is selected (Enter pressed)
type PodSelectedCallback func(namespace, podName string)

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
	namespaceFilter string       // Global namespace filter from header
	tableCtrl   *ui.TableController

	// Stateful sparklines for smooth sliding animation
	cpuSparklines map[string]*ui.Sparkline // key: "namespace/podname"
	memSparklines map[string]*ui.Sparkline

	// Callback for pod selection
	onPodSelected PodSelectedCallback
}

func NewPodPanel(app *application.Application, title string) ui.Panel {
	p := &podPanel{
		app:           app,
		title:         title,
		sortColumn:    "NAMESPACE", // Default sort by NAMESPACE then NAME
		sortAsc:       true,        // Default ascending
		filter:        &ui.FilterState{},
		cpuSparklines: make(map[string]*ui.Sparkline),
		memSparklines: make(map[string]*ui.Sparkline),
	}
	p.Layout(nil)

	return p
}

func (p *podPanel) GetTitle() string {
	return p.title
}

// SetOnPodSelected sets the callback for when a pod is selected (Enter pressed)
func (p *podPanel) SetOnPodSelected(callback PodSelectedCallback) {
	p.onPodSelected = callback
}

// SetOnConfigureColumns sets the callback when 'C' is pressed
func (p *podPanel) SetOnConfigureColumns(callback func()) {
	if p.tableCtrl != nil {
		p.tableCtrl.SetOnConfigureColumns(callback)
	}
}

// GetVisibleColumns returns currently visible column names
func (p *podPanel) GetVisibleColumns() []string {
	return p.listCols
}

// SetVisibleColumns updates the visible columns and immediately redraws the table
func (p *podPanel) SetVisibleColumns(cols []string) {
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

// getPodInfoFromRow returns the namespace and pod name from the given row index
func (p *podPanel) getPodInfoFromRow(row int) (namespace, podName string) {
	// Row 0 is header, data starts at row 1
	// We need to map from displayed row to the filtered data index
	dataIndex := row - 1 // Subtract 1 for header

	// Get filtered pods (same logic as DrawBody)
	var filteredPods []model.PodModel
	if p.filter.IsFiltering() && p.filter.Text != "" {
		for _, pod := range p.currentData {
			if p.filter.MatchesRow(p.getPodCells(pod)) {
				filteredPods = append(filteredPods, pod)
			}
		}
	} else {
		filteredPods = p.currentData
	}

	// Sort the filtered pods the same way as DrawBody
	sortCol, sortAsc := p.GetSortParams()
	model.SortPodModelsBy(filteredPods, sortCol, sortAsc)

	if dataIndex >= 0 && dataIndex < len(filteredPods) {
		return filteredPods[dataIndex].Namespace, filteredPods[dataIndex].Name
	}
	return "", ""
}

// getSparkline returns an existing sparkline or creates a new one
func (p *podPanel) getSparkline(sparklines map[string]*ui.Sparkline, key string, width int, colors ui.ColorKeys) *ui.Sparkline {
	return ui.GetSparkline(sparklines, key, width, colors)
}

func (p *podPanel) Layout(_ any) {
	if !p.laidout {
		p.list = tview.NewTable()
		p.list.SetFixed(1, 0)
		p.list.SetBorder(false)
		p.list.SetBorders(false)
		p.list.SetFocusFunc(func() {
			p.list.SetSelectable(true, false)
			p.list.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorLightGray).Foreground(tcell.ColorBlack))
			p.list.Select(1, 0) // Select row 1 (first data row), column 0
		})
		p.list.SetBlurFunc(func() {
			p.list.SetSelectable(false, false)
		})

		podColumnToKey := map[string]rune{
			"NAMESPACE": 'n',
			"POD":       'p',
			"READY":     'r',
			"STATUS":    's',
			"RST":       't',
			"AGE":       'a',
			"VOLS":      'v',
			"IP":        'i',
			"NODE":      'o',
			"CPU":       'c',
			"MEMORY":    'm',
			"STALL":     'l',
		}

		p.tableCtrl = ui.NewTableController(p.list, ui.TableControllerConfig{
			DefaultSortColumn: "NAMESPACE",
			DefaultSortAsc:    true,
			ColumnToKey:       podColumnToKey,
			OnRedraw:          p.redrawWithFilter,
			OnSelect: func(row int) bool {
				namespace, podName := p.getPodInfoFromRow(row)
				if namespace != "" && podName != "" && p.onPodSelected != nil {
					p.onPodSelected(namespace, podName)
					return true
				}
				return false
			},
			OnYank: func(row int) bool {
				_, podName := p.getPodInfoFromRow(row)
				if podName != "" {
					_ = ui.CopyToClipboard(podName)
					if p.app != nil {
						p.app.ShowToast(fmt.Sprintf("Copied \"%s\" to clipboard", podName), ui.ToastSuccess, 2*time.Second)
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
func (p *podPanel) formatColumnHeader(col string) string {
	return p.tableCtrl.FormatColumnHeader(col)
}

func (p *podPanel) DrawHeader(data any) {
	cols, ok := data.([]string)
	if !ok {
		panic(fmt.Sprintf("podPanel.DrawBody got unexpected data type %T", data))
	}

	p.tableCtrl.SetVisibleColumns(cols)
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
				SetBackgroundColor(tcell.ColorDarkCyan).
				SetAlign(tview.AlignLeft).
				SetExpansion(100).
				SetSelectable(true),
		)

		// Map column name to position
		p.colMap[col] = i
	}
	p.list.SetFixed(1, 0)
}

// SetNamespaceFilter sets the runtime namespace filter
func (p *podPanel) SetNamespaceFilter(ns string) {
	p.namespaceFilter = ns
}

// GetSortParams returns the current sort column and direction
func (p *podPanel) GetSortParams() (string, bool) {
	return p.tableCtrl.GetSortParams()
}

// toggleSort toggles the sort column and direction
func (p *podPanel) toggleSort(columnName string) {
	p.tableCtrl.ToggleSort(columnName)
	p.sortColumn, p.sortAsc = p.tableCtrl.GetSortParams()
}

// handleSortKey processes keyboard shortcuts for sorting
// Returns true if the key was handled, false otherwise
func (p *podPanel) handleSortKey(key rune) bool {
	handled := p.tableCtrl.HandleSortKey(key)
	if handled {
		p.sortColumn, p.sortAsc = p.tableCtrl.GetSortParams()
	}
	return handled
}

func (p *podPanel) DrawBody(data any) {
	pods, ok := data.([]model.PodModel)
	if !ok {
		panic(fmt.Sprintf("PodPanel.DrawBody got unexpected type %T", data))
	}

	// Prune sparkline entries for pods that are no longer present
	activePods := make(map[string]bool, len(pods))
	for _, pod := range pods {
		activePods[pod.Namespace+"/"+pod.Name] = true
	}
	ui.PruneSparklines(p.cpuSparklines, activePods)
	ui.PruneSparklines(p.memSparklines, activePods)

	// Store the data for re-sorting
	p.currentData = pods

	// Sort pods according to current sort state
	sortCol, sortAsc := p.GetSortParams()
	model.SortPodModelsBy(pods, sortCol, sortAsc)

	colorKeys := ui.ColorKeys{0: "olivedrab", 50: "yellow", 90: "red"}
	var cpuRatio, memRatio ui.Ratio
	var cpuGraph, memGraph string
	var cpuMetrics, memMetrics string

	// Apply filter and track counts
	p.filter.TotalRows = len(pods)
	p.filter.MatchRows = 0

	// Filter pods if namespace filter or row filter is active
	var filteredPods []model.PodModel
	filterLower := strings.ToLower(p.namespaceFilter)
	hasNSFilter := filterLower != ""
	hasRowFilter := p.filter.IsFiltering() && p.filter.Text != ""

	if hasNSFilter || hasRowFilter {
		for _, pod := range pods {
			if hasNSFilter && !strings.Contains(strings.ToLower(pod.Namespace), filterLower) {
				continue
			}
			if hasRowFilter && !p.filter.MatchesRow(p.getPodCells(pod)) {
				continue
			}
			filteredPods = append(filteredPods, pod)
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
						Text:     pod.Namespace,
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 14,
					},
				)

			case "POD":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     pod.Name,
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 36,
					},
				)

			case "READY":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     fmt.Sprintf("%d/%d", pod.ReadyContainers, pod.TotalContainers),
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 5,
					},
				)

			case "STATUS":
				// Status column: green for healthy, status color for unhealthy
				statusColor := ui.GetTcellColor(ui.GetStatusColor(pod.Status, "pod"))
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     pod.Status,
						Color:    statusColor,
						Align:    tview.AlignLeft,
						MaxWidth: 11,
					},
				)

			case "RST":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     fmt.Sprintf("%d", pod.Restarts),
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 4,
					},
				)

			case "AGE":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     pod.TimeSince,
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 5,
					},
				)

			case "VOLS":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     fmt.Sprintf("%d/%d", pod.Volumes, pod.VolMounts),
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 5,
					},
				)

			case "IP":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     pod.IP,
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 15,
					},
				)

			case "NODE":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     pod.Node,
						Color:    rowColor,
						Align:    tview.AlignLeft,
						MaxWidth: 16,
					},
				)

			case "CPU":
				graphWidth := 10
				// Check if we have actual usage metrics (non-zero values)
				hasUsageMetrics := pod.PodUsageCpuQty != nil && pod.PodUsageCpuQty.MilliValue() > 0
				hasRequestMetrics := pod.PodRequestedCpuQty != nil && pod.PodRequestedCpuQty.MilliValue() > 0
				hasAllocatable := pod.NodeAllocatableCpuQty != nil && pod.NodeAllocatableCpuQty.MilliValue() > 0

				// Get or create stateful sparkline for this pod
				podKey := pod.Namespace + "/" + pod.Name
				cpuSparkline := p.getSparkline(p.cpuSparklines, podKey, graphWidth, colorKeys)

				if hasUsageMetrics && hasAllocatable {
					cpuRatio = ui.GetRatio(float64(pod.PodUsageCpuQty.MilliValue()), float64(pod.NodeAllocatableCpuQty.MilliValue()))
					cpuPercentage := float64(cpuRatio) * 100
					cpuPercentageColor := ui.GetResourcePercentageColor(cpuPercentage)
					// Push current value to sparkline (smooth sliding)
					cpuSparkline.Push(float64(cpuRatio))
					cpuGraph = cpuSparkline.RenderText()
					cpuTrend := cpuSparkline.TrendIndicator(cpuPercentage)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] %5dm [%s]%5.1f%%[white] %s",
						cpuGraph, pod.PodUsageCpuQty.MilliValue(), cpuPercentageColor, cpuPercentage, cpuTrend,
					)
				} else if hasRequestMetrics && hasAllocatable {
					cpuRatio = ui.GetRatio(float64(pod.PodRequestedCpuQty.MilliValue()), float64(pod.NodeAllocatableCpuQty.MilliValue()))
					cpuPercentage := float64(cpuRatio) * 100
					cpuPercentageColor := ui.GetResourcePercentageColor(cpuPercentage)
					// Push current value to sparkline (smooth sliding)
					cpuSparkline.Push(float64(cpuRatio))
					cpuGraph = cpuSparkline.RenderText()
					cpuTrend := cpuSparkline.TrendIndicator(cpuPercentage)
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] %5dm [%s]%5.1f%%[white] %s",
						cpuGraph, pod.PodRequestedCpuQty.MilliValue(), cpuPercentageColor, cpuPercentage, cpuTrend,
					)
				} else {
					// Zero or unavailable: push zero to sparkline
					cpuSparkline.Push(0)
					cpuGraph = cpuSparkline.RenderText()
					cpuTrend := cpuSparkline.TrendIndicator(0) // 0% when no data
					cpuMetrics = fmt.Sprintf(
						"[white][%s[white]] %5dm [green]%5.1f%%[white] %s",
						cpuGraph, 0, 0.0, cpuTrend,
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
				graphWidth := 10
				// Check if we have actual usage metrics (non-zero values)
				hasUsageMetrics := pod.PodUsageMemQty != nil && pod.PodUsageMemQty.Value() > 0
				hasRequestMetrics := pod.PodRequestedMemQty != nil && pod.PodRequestedMemQty.Value() > 0
				hasAllocatable := pod.NodeAllocatableMemQty != nil && pod.NodeAllocatableMemQty.Value() > 0

				// Get or create stateful sparkline for this pod's memory
				podKey := pod.Namespace + "/" + pod.Name
				memSparkline := p.getSparkline(p.memSparklines, podKey, graphWidth, colorKeys)

				if hasUsageMetrics && hasAllocatable {
					memRatio = ui.GetRatio(float64(pod.PodUsageMemQty.Value()), float64(pod.NodeAllocatableMemQty.Value()))
					memPercentage := float64(memRatio) * 100
					memPercentageColor := ui.GetResourcePercentageColor(memPercentage)
					// Push current value to sparkline (smooth sliding)
					memSparkline.Push(float64(memRatio))
					memGraph = memSparkline.RenderText()
					memTrend := memSparkline.TrendIndicator(memPercentage)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] %s [%s]%5.1f%%[white] %s",
						memGraph,
						ui.FormatMemory(pod.PodUsageMemQty),
						memPercentageColor, memPercentage, memTrend,
					)
				} else if hasRequestMetrics && hasAllocatable {
					memRatio = ui.GetRatio(float64(pod.PodRequestedMemQty.Value()), float64(pod.NodeAllocatableMemQty.Value()))
					memPercentage := float64(memRatio) * 100
					memPercentageColor := ui.GetResourcePercentageColor(memPercentage)
					// Push current value to sparkline (smooth sliding)
					memSparkline.Push(float64(memRatio))
					memGraph = memSparkline.RenderText()
					memTrend := memSparkline.TrendIndicator(memPercentage)
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] %s [%s]%5.1f%%[white] %s",
						memGraph,
						ui.FormatMemory(pod.PodRequestedMemQty),
						memPercentageColor, memPercentage, memTrend,
					)
				} else {
					// Zero or unavailable: push zero to sparkline
					memSparkline.Push(0)
					memGraph = memSparkline.RenderText()
					memTrend := memSparkline.TrendIndicator(0) // 0% when no data
					memMetrics = fmt.Sprintf(
						"[white][%s[white]] %s [green]%5.1f%%[white] %s",
						memGraph, "   0Mi", 0.0, memTrend,
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

			case "STALL":
				p.list.SetCell(
					rowIdx, colIdx,
					&tview.TableCell{
						Text:     renderStallCell(pod.PSI, pod.NodePSIKernelSupported),
						Align:    tview.AlignLeft,
						MaxWidth: 12,
					},
				)
			}
		}
	}
}

func (p *podPanel) DrawFooter(_ any) {}

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

// SetFocused implements ui.FocusablePanel - updates visual focus state
func (p *podPanel) SetFocused(focused bool) {
	ui.SetFlexFocused(p.root, focused)
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
	return p.tableCtrl.HasEscapableState()
}

// HandleEscape implements ui.EscapablePanel - handles ESC key press
func (p *podPanel) HandleEscape() bool {
	return p.tableCtrl.HandleEscape()
}

// updateTitle updates the panel title with scroll position indicator and filter state
func (p *podPanel) updateTitle(totalRows int) {
	var disconnectedSuffix string
	if p.app != nil && p.app.IsAPIDisconnected() {
		disconnectedSuffix = " [red][DISCONNECTED - Press R to reconnect][-]"
	}
	p.root.SetTitle(p.tableCtrl.FormatTitleWithScroll("Pods", ui.Icons.Package, totalRows, disconnectedSuffix))
}
