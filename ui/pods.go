package ui

import "github.com/gizak/termui"

type podsUI struct {
	grid    *termui.Grid
	table   *termui.Table
	hotkey  int
	visible bool
}

func newPodsUI() *podsUI {
	ui := new(podsUI)
	return ui
}

func (ui *podsUI) layout() {
	// lays out
	// [tab]
	//  +-[grid]
	//     +-[table]
	ui.table = termui.NewTable()

	ui.table.FgColor = termui.ColorDefault
	ui.table.BgColor = termui.ColorDefault
	ui.table.TextAlign = termui.AlignLeft
	ui.table.Separator = false
	ui.table.PaddingLeft = 0
	ui.table.PaddingRight = 0
	ui.table.PaddingBottom = 0
	ui.table.PaddingTop = 0
	ui.table.Analysis()
	ui.table.SetSize()
	ui.table.Border = true
	ui.table.BorderLabel = "Pods"

	// layout table in grid
	ui.grid = termui.NewGrid()
	ui.grid.X, ui.grid.Y = 0, 0
	ui.grid.BgColor = termui.ThemeAttr("bg")
	ui.grid.Width = termui.TermWidth()
	ui.grid.Align()

	ui.grid.AddRows(
		termui.NewRow(
			termui.NewCol(12, 0, ui.table),
		),
	)
}

func (ui *podsUI) buffer() termui.Bufferer {
	return ui.grid
}

func (ui *podsUI) update(data [][]string) {
	ui.table.Rows = data
	ui.table.Analysis()
	ui.table.SetSize()

	ui.grid.Width = termui.TermWidth()
	ui.grid.Align()

	if len(ui.table.Rows) > 1 {
		ui.table.BgColors[0] = termui.ColorBlue
		ui.table.FgColors[0] = termui.ColorWhite | termui.AttrBold
	}
	termui.Render(ui.grid)
}

func (ui *podsUI) show() {
	termui.Clear()
	ui.visible = true
	termui.Render(ui.grid)
}

func (ui *podsUI) hide() {
	ui.visible = false
}
