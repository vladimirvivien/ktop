package ui

import (
	"errors"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

type UI struct {
	app  *tview.Application
	root *tview.Pages
}

func New() *UI {
	app := tview.NewApplication()
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' {
			app.Stop()
		}
		return event
	})
	root := tview.NewPages()
	app.SetRoot(root, true)
	return &UI{
		app:  app,
		root: root,
	}
}

func (ui *UI) AddPage(title string, page tview.Primitive) {
	if ui.root == nil {
		return
	}
	ui.root.AddPage(title, page, true, true)
}

func (ui *UI) Focus(t tview.Primitive) {
	if ui.app == nil {
		return
	}
	ui.app.SetFocus(t)
}

func (ui *UI) Start() error {
	if ui.app == nil {
		return errors.New("tview.Application nil")
	}
	if err := ui.app.Run(); err != nil {
		return err
	}

	return nil
}
