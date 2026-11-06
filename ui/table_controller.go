package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TableControllerConfig defines setup parameters for TableController
type TableControllerConfig struct {
	DefaultSortColumn  string
	DefaultSortAsc     bool
	ColumnToKey        map[string]rune
	OnRedraw           func()
	OnSelect           func(row int) bool // Callback for Enter key on selected row. Return true if consumed.
	OnYank             func(row int) bool // Callback for 'y' key to copy row data. Return true if consumed.
	OnConfigureColumns func()             // Callback for '=' column configuration key.
}

// TableController manages sorting, filtering, and header indicators for tabular panels
type TableController struct {
	table              *tview.Table
	sortColumn         string
	sortAsc            bool
	columnToKey        map[string]rune
	keyToColumn        map[rune]string
	filter             *FilterState
	visibleCols        []string
	onRedraw           func()
	onSelect           func(row int) bool
	onYank             func(row int) bool
	onConfigureColumns func()
}

// NewTableController initializes a TableController
func NewTableController(table *tview.Table, cfg TableControllerConfig) *TableController {
	tc := &TableController{
		table:              table,
		sortColumn:         cfg.DefaultSortColumn,
		sortAsc:            cfg.DefaultSortAsc,
		columnToKey:        cfg.ColumnToKey,
		keyToColumn:        make(map[rune]string),
		filter:             &FilterState{},
		onRedraw:           cfg.OnRedraw,
		onSelect:           cfg.OnSelect,
		onYank:             cfg.OnYank,
		onConfigureColumns: cfg.OnConfigureColumns,
	}
	for col, key := range cfg.ColumnToKey {
		tc.keyToColumn[key] = col
	}
	return tc
}

// GetSortParams returns current sort column and ascending direction
func (tc *TableController) GetSortParams() (string, bool) {
	return tc.sortColumn, tc.sortAsc
}

// SetSortParams updates the sort column and ascending direction
func (tc *TableController) SetSortParams(col string, asc bool) {
	tc.sortColumn = col
	tc.sortAsc = asc
}

// Filter returns the underlying FilterState
func (tc *TableController) Filter() *FilterState {
	return tc.filter
}

// SetVisibleColumns sets the visible column subset for filtering shortcut visibility
func (tc *TableController) SetVisibleColumns(cols []string) {
	tc.visibleCols = cols
}

// GetVisibleColumns returns the visible column subset
func (tc *TableController) GetVisibleColumns() []string {
	return tc.visibleCols
}

// SetOnConfigureColumns sets the callback invoked when 'C' is pressed
func (tc *TableController) SetOnConfigureColumns(cb func()) {
	tc.onConfigureColumns = cb
}

// ToggleSort toggles the sort direction or switches sort column
func (tc *TableController) ToggleSort(columnName string) {
	if columnName == tc.sortColumn {
		tc.sortAsc = !tc.sortAsc
	} else {
		tc.sortColumn = columnName
		tc.sortAsc = true
	}
	if tc.onRedraw != nil {
		tc.onRedraw()
	}
}

// HandleSortKey handles a single rune key for column sorting
func (tc *TableController) HandleSortKey(key rune) bool {
	col, exists := tc.keyToColumn[key]
	if !exists {
		return false
	}
	if len(tc.visibleCols) > 0 && !slices.Contains(tc.visibleCols, col) {
		return false
	}
	tc.ToggleSort(col)
	return true
}

// FormatColumnHeader formats a column header with keyboard shortcut hint and sort indicator
func (tc *TableController) FormatColumnHeader(col string) string {
	if len(col) == 0 {
		return col
	}

	key, exists := tc.columnToKey[col]
	var formatted string

	if exists {
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
			before := col[:keyPos]
			highlighted := string(col[keyPos])
			after := ""
			if keyPos+1 < len(col) {
				after = col[keyPos+1:]
			}
			formatted = fmt.Sprintf("%s[%s::b]%s[%s::-]%s",
				before, Theme.HeaderShortcutKey, highlighted, Theme.HeaderForeground, after)
		} else {
			formatted = fmt.Sprintf("[%s::b]%c[%s::-]%s",
				Theme.HeaderShortcutKey, col[0], Theme.HeaderForeground, col[1:])
		}
	} else {
		formatted = fmt.Sprintf("[%s::b]%c[%s::-]%s",
			Theme.HeaderShortcutKey, col[0], Theme.HeaderForeground, col[1:])
	}

	if col == tc.sortColumn {
		if tc.sortAsc {
			formatted += " ▲"
		} else {
			formatted += " ▼"
		}
	}

	return formatted
}

// FormatTitleWithScroll formats a panel title with scroll indicators and filter summary
func (tc *TableController) FormatTitleWithScroll(entityName, icon string, totalRows int, disconnectedSuffix string) string {
	tc.filter.TotalRows = totalRows
	if tc.table == nil {
		return tc.filter.FormatTitle(entityName, icon)
	}

	_, _, _, height := tc.table.GetInnerRect()
	visibleRows := height - 1 // Subtract header row
	offset, _ := tc.table.GetOffset()

	firstVisible := offset + 1
	lastVisible := min(offset+visibleRows, totalRows)
	if totalRows == 0 {
		firstVisible = 0
		lastVisible = 0
	}

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

	return tc.filter.FormatTitleWithScroll(entityName, icon, firstVisible, lastVisible, totalRows, scrollIndicator, disconnectedSuffix)
}

// HandleInput processes keyboard events for filtering, sorting, and row selection
func (tc *TableController) HandleInput(event *tcell.EventKey) *tcell.EventKey {
	// Filter edit mode
	if tc.filter.Editing {
		switch event.Key() {
		case tcell.KeyEscape:
			tc.filter.Cancel()
			if tc.onRedraw != nil {
				tc.onRedraw()
			}
			return nil
		case tcell.KeyEnter:
			tc.filter.Confirm()
			if tc.onRedraw != nil {
				tc.onRedraw()
			}
			return nil
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if tc.filter.HandleBackspace() {
				if tc.onRedraw != nil {
					tc.onRedraw()
				}
			}
			return nil
		case tcell.KeyRune:
			tc.filter.AppendChar(event.Rune())
			if tc.onRedraw != nil {
				tc.onRedraw()
			}
			return nil
		}
		return nil
	}

	// Normal mode - ESC clears active filter
	if event.Key() == tcell.KeyEscape && tc.filter.Active {
		tc.filter.Clear()
		if tc.onRedraw != nil {
			tc.onRedraw()
		}
		return nil
	}

	// Filter trigger '/'
	if event.Key() == tcell.KeyRune && event.Rune() == '/' {
		tc.filter.StartEditing()
		if tc.onRedraw != nil {
			tc.onRedraw()
		}
		return nil
	}

	// Row selection Enter
	if event.Key() == tcell.KeyEnter && tc.onSelect != nil && tc.table != nil {
		row, _ := tc.table.GetSelection()
		if row > 0 {
			if tc.onSelect(row) {
				return nil
			}
		}
	}

	// Column configuration trigger '='
	if event.Key() == tcell.KeyRune && event.Rune() == '=' && tc.onConfigureColumns != nil {
		tc.onConfigureColumns()
		return nil
	}

	// Clipboard copy / yank trigger 'y'
	if event.Key() == tcell.KeyRune && (event.Rune() == 'y' || event.Rune() == 'Y') && tc.onYank != nil && tc.table != nil {
		row, _ := tc.table.GetSelection()
		if row > 0 {
			if tc.onYank(row) {
				return nil
			}
		}
	}

	// Sorting shortcut keys
	if event.Key() == tcell.KeyRune {
		if tc.HandleSortKey(event.Rune()) {
			return nil
		}
	}

	return event
}

// HandleEscape handles ESC key, returning true if it consumed an editing/active filter
func (tc *TableController) HandleEscape() bool {
	if tc.filter.Editing {
		tc.filter.Cancel()
		if tc.onRedraw != nil {
			tc.onRedraw()
		}
		return true
	}
	if tc.filter.Active {
		tc.filter.Clear()
		if tc.onRedraw != nil {
			tc.onRedraw()
		}
		return true
	}
	return false
}

// HasEscapableState checks if filter has active or editing state
func (tc *TableController) HasEscapableState() bool {
	return tc.filter.HasEscapableState()
}

// IsFilterEditing returns true if the filter is currently in edit mode (user is typing)
func (tc *TableController) IsFilterEditing() bool {
	if tc == nil || tc.filter == nil {
		return false
	}
	return tc.filter.Editing
}

// GetSparkline returns an existing sparkline from the cache or creates a new one
func GetSparkline(sparklines map[string]*Sparkline, key string, width int, colors ColorKeys) *Sparkline {
	if spark, ok := sparklines[key]; ok {
		spark.Resize(width)
		return spark
	}
	spark := NewSparkline().SetDimensions(width, 1).SetColorKeys(colors)
	sparklines[key] = spark
	return spark
}

// PruneSparklines removes sparklines whose keys are not in the activeKeys set
func PruneSparklines(sparklines map[string]*Sparkline, activeKeys map[string]bool) {
	for k := range sparklines {
		if !activeKeys[k] {
			delete(sparklines, k)
		}
	}
}
