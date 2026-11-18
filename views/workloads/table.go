package workloads

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
)

type workloadTableConfig[T any] struct {
	title          string
	icon           string
	columns        []string
	columnToKey    map[string]rune
	defaultSortCol string
	defaultSortAsc bool
	sortFn         func(items []T, col string, asc bool)
	namespaceFn    func(T) string
	nameFn         func(T) string
	statusFn       func(T) string
	filterCellsFn  func(T) []string
	cellFn         func(item T, col string, rowColor tcell.Color) *tview.TableCell
	onSelect       func(item T)
}

// workloadTable provides shared sorting, filtering, selection, clipboard copy,
// and focus styling across all workload resource sections.
type workloadTable[T any] struct {
	app             *application.Application
	cfg             workloadTableConfig[T]
	root            *tview.Flex
	table           *tview.Table
	children        []tview.Primitive
	tableCtrl       *ui.TableController
	allItems        []T
	filteredItems   []T
	namespaceFilter string
	colMap          map[string]int
}

func newWorkloadTable[T any](app *application.Application, cfg workloadTableConfig[T]) *workloadTable[T] {
	wt := &workloadTable[T]{
		app:    app,
		cfg:    cfg,
		colMap: make(map[string]int, len(cfg.columns)),
	}
	wt.Layout(nil)
	wt.DrawHeader(cfg.columns)
	return wt
}

func (w *workloadTable[T]) Layout(_ any) {
	if w.table != nil {
		return
	}
	w.table = tview.NewTable()
	w.table.SetFixed(1, 0)
	w.table.SetBorder(false)
	w.table.SetBorders(false)
	ui.StyleTable(w.table)
	w.table.SetFocusFunc(func() {
		w.table.SetSelectable(true, false)
		w.table.SetSelectedStyle(ui.SelectedRowStyle())
		if w.table.GetRowCount() > 1 {
			row, col := w.table.GetSelection()
			if row < 1 {
				row = 1
			}
			w.table.Select(row, col)
		}
	})
	w.table.SetBlurFunc(func() {
		w.table.SetSelectable(false, false)
	})

	w.tableCtrl = ui.NewTableController(w.table, ui.TableControllerConfig{
		DefaultSortColumn: w.cfg.defaultSortCol,
		DefaultSortAsc:    w.cfg.defaultSortAsc,
		ColumnToKey:       w.cfg.columnToKey,
		OnRedraw:          w.redraw,
		OnSelect: func(row int) bool {
			idx := row - 1
			if idx >= 0 && idx < len(w.filteredItems) && w.cfg.onSelect != nil {
				w.cfg.onSelect(w.filteredItems[idx])
				return true
			}
			return false
		},
		OnYank: func(row int) bool {
			idx := row - 1
			if idx < 0 || idx >= len(w.filteredItems) || w.cfg.nameFn == nil {
				return false
			}
			item := w.filteredItems[idx]
			target := w.cfg.nameFn(item)
			if w.cfg.namespaceFn != nil {
				if ns := w.cfg.namespaceFn(item); ns != "" {
					target = ns + "/" + target
				}
			}
			_ = ui.CopyToClipboard(target)
			if w.app != nil {
				w.app.ShowToast(fmt.Sprintf("Copied %q to clipboard", target), ui.ToastSuccess, 2*time.Second)
			}
			return true
		},
	})
	w.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return w.tableCtrl.HandleInput(event)
	})

	w.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(w.table, 0, 1, true)
	w.root.SetBorder(true)
	w.root.SetTitleAlign(tview.AlignLeft)
	ui.SetFlexFocused(w.root, false)
	w.children = []tview.Primitive{w.table}
	w.updateTitle(0)
}

func (w *workloadTable[T]) DrawHeader(data any) {
	cols, ok := data.([]string)
	if !ok {
		return
	}
	w.cfg.columns = cols
	w.tableCtrl.SetVisibleColumns(cols)
	ui.StyleTable(w.table)
	w.colMap = make(map[string]int, len(cols))

	for i, col := range cols {
		headerText := w.tableCtrl.FormatColumnHeader(col)
		w.table.SetCell(0, i,
			ui.StyleHeaderCell(tview.NewTableCell(headerText).
				SetAlign(tview.AlignLeft).
				SetExpansion(100).
				SetSelectable(false)),
		)
		w.colMap[col] = i
	}
	w.table.SetFixed(1, 0)
}

func (w *workloadTable[T]) DrawBody(data any) {
	items, ok := data.([]T)
	if !ok {
		return
	}

	selRow, selCol := w.table.GetSelection()

	w.allItems = slices.Clone(items)
	sorted := slices.Clone(items)
	sortCol, sortAsc := w.tableCtrl.GetSortParams()
	if w.cfg.sortFn != nil {
		w.cfg.sortFn(sorted, sortCol, sortAsc)
	}

	filter := w.tableCtrl.Filter()
	filter.TotalRows = len(sorted)

	nsLower := strings.ToLower(strings.TrimSpace(w.namespaceFilter))
	hasNS := nsLower != ""
	hasRowFilter := filter.IsFiltering() && filter.Text != ""

	filtered := sorted[:0]
	if hasNS || hasRowFilter {
		filtered = make([]T, 0, len(sorted))
		for _, item := range sorted {
			if hasNS && w.cfg.namespaceFn != nil && !strings.Contains(strings.ToLower(w.cfg.namespaceFn(item)), nsLower) {
				continue
			}
			if hasRowFilter && w.cfg.filterCellsFn != nil && !filter.MatchesRow(w.cfg.filterCellsFn(item)) {
				continue
			}
			filtered = append(filtered, item)
		}
	} else {
		filtered = sorted
	}

	filter.MatchRows = len(filtered)
	w.filteredItems = filtered

	w.table.Clear()
	w.DrawHeader(w.cfg.columns)
	w.updateTitle(len(filtered))

	for i, item := range filtered {
		rowIdx := i + 1
		rowColor := ui.ActivePalette().DataSecondary
		if ui.IsMonochrome() {
			rowColor = tcell.ColorDefault
		} else if w.cfg.statusFn != nil {
			rowColor = ui.GetRowColorForStatus(w.cfg.statusFn(item), "workload")
		}
		for _, col := range w.cfg.columns {
			colIdx := w.colMap[col]
			if w.cfg.cellFn != nil {
				w.table.SetCell(rowIdx, colIdx, w.cfg.cellFn(item, col, rowColor))
			}
		}
	}

	if len(filtered) > 0 && w.table.HasFocus() {
		if selRow < 1 {
			selRow = 1
		} else if selRow > len(filtered) {
			selRow = len(filtered)
		}
		w.table.Select(selRow, selCol)
	}
}

func (w *workloadTable[T]) redraw() {
	w.DrawBody(w.allItems)
	if w.app != nil {
		w.app.Refresh()
	}
}

func (w *workloadTable[T]) SetNamespaceFilter(ns string) {
	if w.namespaceFilter == ns {
		return
	}
	w.namespaceFilter = ns
	w.DrawBody(w.allItems)
}

func (w *workloadTable[T]) Count() int {
	return len(w.filteredItems)
}

func (w *workloadTable[T]) DrawFooter(_ any) {}
func (w *workloadTable[T]) Clear() {
	if w.table != nil {
		w.table.Clear()
	}
}
func (w *workloadTable[T]) GetTitle() string                    { return w.cfg.title }
func (w *workloadTable[T]) GetRootView() tview.Primitive        { return w.root }
func (w *workloadTable[T]) GetChildrenViews() []tview.Primitive { return w.children }

// TablePrimitive returns the underlying tview.Table registered as the focus target.
func (w *workloadTable[T]) TablePrimitive() tview.Primitive {
	return w.table
}

func (w *workloadTable[T]) SetFocused(focused bool) {
	ui.SetFlexFocused(w.root, focused)
}

func (w *workloadTable[T]) HasEscapableState() bool {
	return w.tableCtrl != nil && w.tableCtrl.HasEscapableState()
}

func (w *workloadTable[T]) IsFilterEditing() bool {
	return w.tableCtrl != nil && w.tableCtrl.IsFilterEditing()
}

func (w *workloadTable[T]) HandleEscape() bool {
	return w.tableCtrl != nil && w.tableCtrl.HandleEscape()
}

func (w *workloadTable[T]) updateTitle(totalRows int) {
	if w.root == nil || w.tableCtrl == nil {
		return
	}
	var disconnectedSuffix string
	if w.app != nil && w.app.IsAPIDisconnected() {
		disconnectedSuffix = " " + ui.ErrorMarkup("[DISCONNECTED - Press R to reconnect]")
	}
	w.root.SetTitle(w.tableCtrl.FormatTitleWithScroll(w.cfg.title, w.cfg.icon, totalRows, disconnectedSuffix))
}
