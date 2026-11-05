package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ColumnSelectorConfig configures the ColumnSelectorModal
type ColumnSelectorConfig struct {
	Title          string
	AllColumns     []string
	CurrentColumns []string
	Descriptions   map[string]string
	OnApply        func(selected []string)
	OnCancel       func()
}

// ColumnSelectorModal provides an interactive checklist modal for customizing visible columns.
type ColumnSelectorModal struct {
	grid         *tview.Grid
	table        *tview.Table
	allColumns   []string
	selected     map[string]bool
	descriptions map[string]string
	onApply      func(selected []string)
	onCancel     func()
}

// NewColumnSelectorModal creates a modal dialog allowing users to toggle visible columns.
func NewColumnSelectorModal(cfg ColumnSelectorConfig) *ColumnSelectorModal {
	m := &ColumnSelectorModal{
		allColumns:   cfg.AllColumns,
		selected:     make(map[string]bool),
		descriptions: cfg.Descriptions,
		onApply:      cfg.OnApply,
		onCancel:     cfg.OnCancel,
	}

	for _, col := range cfg.CurrentColumns {
		m.selected[col] = true
	}

	m.table = tview.NewTable()
	m.table.SetSelectable(true, false)
	m.table.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorDarkCyan).Foreground(tcell.ColorWhite).Bold(true))
	m.table.SetBorder(false)

	m.renderRows()

	m.table.SetInputCapture(m.HandleInput)

	instructions := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText("[yellow]Space[-] Toggle  [yellow]Enter[-] Apply  [yellow]Esc[-] Cancel")

	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(m.table, 0, 1, true).
		AddItem(instructions, 1, 0, false)
	box.SetBorder(true).
		SetTitle(fmt.Sprintf(" Configure Columns: %s ", cfg.Title)).
		SetTitleAlign(tview.AlignCenter)

	modalHeight := len(cfg.AllColumns) + 4
	if modalHeight < 16 {
		modalHeight = 16
	}

	m.grid = tview.NewGrid().
		SetColumns(0, 58, 0).
		SetRows(0, modalHeight, 0).
		AddItem(box, 1, 1, 1, 1, 0, 0, true)

	return m
}

func (m *ColumnSelectorModal) renderRows() {
	for i, col := range m.allColumns {
		check := "[ ]"
		checkColor := tcell.ColorGray
		if m.selected[col] {
			check = tview.Escape("[X]")
			checkColor = tcell.ColorGreen
		}

		desc := ""
		if m.descriptions != nil {
			desc = m.descriptions[col]
		}

		m.table.SetCell(i, 0, tview.NewTableCell(check).
			SetTextColor(checkColor).
			SetAlign(tview.AlignLeft).
			SetSelectable(true))

		m.table.SetCell(i, 1, tview.NewTableCell(fmt.Sprintf(" %-9s", col)).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetSelectable(true))

		m.table.SetCell(i, 2, tview.NewTableCell(desc).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignLeft).
			SetExpansion(1).
			SetSelectable(true))
	}
}

// HandleInput processes keyboard navigation and toggles for the modal
func (m *ColumnSelectorModal) HandleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		if m.onCancel != nil {
			m.onCancel()
		}
		return nil
	case tcell.KeyEnter:
		if m.onApply != nil {
			m.onApply(m.getSelectedColumns())
		}
		return nil
	case tcell.KeyUp:
		row, _ := m.table.GetSelection()
		if row > 0 {
			m.table.Select(row-1, 0)
		}
		return nil
	case tcell.KeyDown:
		row, _ := m.table.GetSelection()
		if row < len(m.allColumns)-1 {
			m.table.Select(row+1, 0)
		}
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case ' ':
			row, _ := m.table.GetSelection()
			if row >= 0 && row < len(m.allColumns) {
				col := m.allColumns[row]
				// Prevent deselecting the only selected column
				if m.selected[col] && m.countSelected() <= 1 {
					return nil
				}
				m.selected[col] = !m.selected[col]
				m.renderRows()
			}
			return nil
		case 'j':
			row, _ := m.table.GetSelection()
			if row < len(m.allColumns)-1 {
				m.table.Select(row+1, 0)
			}
			return nil
		case 'k':
			row, _ := m.table.GetSelection()
			if row > 0 {
				m.table.Select(row-1, 0)
			}
			return nil
		}
	}
	return event
}

func (m *ColumnSelectorModal) countSelected() int {
	c := 0
	for _, sel := range m.selected {
		if sel {
			c++
		}
	}
	return c
}

func (m *ColumnSelectorModal) getSelectedColumns() []string {
	var result []string
	for _, col := range m.allColumns {
		if m.selected[col] {
			result = append(result, col)
		}
	}
	if len(result) == 0 && len(m.allColumns) > 0 {
		return []string{m.allColumns[0]}
	}
	return result
}

// GetRootView returns the root layout primitive to be displayed in tview.Pages
func (m *ColumnSelectorModal) GetRootView() tview.Primitive {
	return m.grid
}

// GetFocusPrimitive returns the inner table that receives focus and keyboard navigation
func (m *ColumnSelectorModal) GetFocusPrimitive() tview.Primitive {
	return m.table
}
