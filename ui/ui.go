package ui

import (
	"errors"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

type UI struct {
	app     *tview.Application
	buttons *tview.Table
	root    *tview.Pages
}

func New() *UI {
	app := tview.NewApplication()
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {

		if event.Key() == tcell.KeyEscape {
			app.Stop()
		}

		return event
	})

	pages := tview.NewPages()

	buttons := tview.NewTable()
	buttons.SetBorder(true)
	buttons.SetCell(0, 1,
		&tview.TableCell{
			Text:            "Overview (F1)",
			Color:           tcell.ColorDarkBlue,
			Align:           tview.AlignLeft,
			BackgroundColor: tcell.ColorYellow,
			Expansion:       100,
		},
	).SetCell(0, 2,
		&tview.TableCell{
			Text:            "Deployments (F2)",
			Color:           tcell.ColorDimGray,
			Align:           tview.AlignLeft,
			BackgroundColor: tcell.ColorPaleGreen,
			Expansion:       100,
		},
	).SetCell(0, 3,
		&tview.TableCell{
			Text:            "Replicas (F3)",
			Color:           tcell.ColorDimGray,
			Align:           tview.AlignLeft,
			BackgroundColor: tcell.ColorPaleGreen,
			Expansion:       100,
		},
	).SetCell(0, 4,
		&tview.TableCell{
			Text:            "Pods (F4)",
			Color:           tcell.ColorDimGray,
			Align:           tview.AlignLeft,
			BackgroundColor: tcell.ColorPaleGreen,
			Expansion:       100,
		},
	).SetCell(0, 5,
		&tview.TableCell{
			Text:            "Storage (F5)",
			Color:           tcell.ColorDimGray,
			Align:           tview.AlignLeft,
			BackgroundColor: tcell.ColorPaleGreen,
			Expansion:       100,
		},
	).SetCell(0, 6,
		&tview.TableCell{
			Text:            "Services (F6)",
			Color:           tcell.ColorDimGray,
			Align:           tview.AlignLeft,
			BackgroundColor: tcell.ColorPaleGreen,
			Expansion:       100,
		},
	).SetCell(0, 7,
		&tview.TableCell{
			Text:            "Configs (F7)",
			Color:           tcell.ColorDimGray,
			Align:           tview.AlignLeft,
			BackgroundColor: tcell.ColorPaleGreen,
			Expansion:       100,
		},
	)

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(pages, 0, 1, true).
		AddItem(buttons, 3, 1, true)

	app.SetRoot(content, true)
	return &UI{
		app:  app,
		root: pages,
	}
}

func (ui *UI) Application() *tview.Application {
	return ui.app
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
