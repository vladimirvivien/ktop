package ui

import (
	"errors"
	"fmt"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

var (
	PageNames = []string{
		"Overview",
		"Deployments",
	}

	buttonUnselectedBgColor = tcell.ColorPaleGreen
	buttonUnselectedFgColor = tcell.ColorDarkBlue
	buttonSelectedBgColor   = tcell.ColorBlue
	buttonSelectedFgColor   = tcell.ColorWhite
)

type Application struct {
	app            *tview.Application
	buttons        *tview.Table
	buttonsBgColor *tcell.Color
	root           *tview.Pages

	refreshQ chan struct{}
}

func New() *Application {
	app := tview.NewApplication()
	pages := tview.NewPages()
	buttons := makeButtons()

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pages, 0, 1, true).
		AddItem(buttons, 3, 1, true)

	app.SetRoot(content, true)

	ui := &Application{
		app:      app,
		root:     pages,
		buttons:  buttons,
		refreshQ: make(chan struct{}),
	}

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {

		switch event.Key() {
		case tcell.KeyEsc:
			app.Stop()
		case tcell.KeyF1:
			ui.switchPage(0)
		case tcell.KeyF2:
			ui.switchPage(1)
		}

		return event
	})

	// setup refresh queue
	go func() {
		for range ui.refreshQ {
			ui.app.Draw()
		}
	}()

	return ui
}

func (ui *Application) TviewApplication() *tview.Application {
	return ui.app
}

func (ui *Application) AddPage(title string, page tview.Primitive) {
	if ui.root == nil {
		return
	}

	// add new page invisible.
	ui.root.AddPage(title, page, true, false)
}

func (ui *Application) Focus(t tview.Primitive) {
	if ui.app == nil {
		return
	}
	ui.app.SetFocus(t)
}

func (ui *Application) Reresh() {
	ui.refreshQ <- struct{}{}
}

func (ui *Application) ViewPage(index int) {
	ui.switchPage(index)
}

func (ui *Application) Start() error {
	if ui.app == nil {
		return errors.New("tview.Application nil")
	}

	if err := ui.app.Run(); err != nil {
		return err
	}

	return nil
}

func (ui *Application) switchPage(index int) {
	row := 1
	cols := ui.buttons.GetColumnCount()

	for i := 0; i < cols-1; i++ {
		cell := ui.buttons.GetCell(row, i)
		if i == index {
			cell.SetTextColor(buttonSelectedFgColor)
			cell.SetBackgroundColor(buttonSelectedBgColor)
		} else {
			cell.SetTextColor(buttonUnselectedFgColor)
			cell.SetBackgroundColor(buttonUnselectedBgColor)
		}

		ui.buttons.SetCell(row, i, cell)
	}

	ui.root.SwitchToPage(PageNames[index])
	ui.Reresh()
}

func makeButtons() *tview.Table {
	buttons := tview.NewTable()
	buttons.SetBorder(true)

	for i := 0; i < len(PageNames); i++ {
		buttons.SetCell(0, i,
			&tview.TableCell{
				Text:            fmt.Sprintf("%s (F%d)", PageNames[i], i+1),
				Color:           buttonUnselectedFgColor,
				Align:           tview.AlignCenter,
				BackgroundColor: buttonUnselectedBgColor,
				Expansion:       100,
			},
		)
	}

	return buttons
}
