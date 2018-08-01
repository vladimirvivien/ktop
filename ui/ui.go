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
	header         *tview.TextView
	buttons        *tview.Table
	buttonsBgColor *tcell.Color
	root           *tview.Pages

	refreshQ chan struct{}
	stopCh   chan struct{}
}

func New() *Application {
	app := tview.NewApplication()
	header := tview.NewTextView().
		SetDynamicColors(true)
	header.SetBorder(true)

	pages := tview.NewPages()
	buttons := makeButtons()

	content := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 3, 1, false).
		AddItem(pages, 0, 1, true).
		AddItem(buttons, 3, 1, false)

	app.SetRoot(content, true)

	ui := &Application{
		app:      app,
		header:   header,
		root:     pages,
		buttons:  buttons,
		refreshQ: make(chan struct{}),
		stopCh:   make(chan struct{}),
	}

	ui.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			ui.Stop()
		case tcell.KeyF1:
			ui.switchPage(0)
		case tcell.KeyF2:
			ui.switchPage(1)
		default:
			return event
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

func (ui *Application) SetHeader(title string) {
	fmt.Fprintf(ui.header, title)
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

func (ui *Application) StopChan() <-chan struct{} {
	return ui.stopCh
}

// ShowBanner displays a welcome banner
func (ui *Application) WelcomeBanner() {
	fmt.Println(`
 _    _              
| | _| |_ ___  _ __  
| |/ / __/ _ \| '_ \ 
|   <| || (_) | |_) |
|_|\_\\__\___/| .__/ 
              |_|`)
}

func (ui *Application) Start() error {
	if ui.app == nil {
		return errors.New("failed to start, tview.Application nil")
	}

	if err := ui.app.Run(); err != nil {
		return err
	}

	return nil
}

func (ui *Application) Stop() error {
	if ui.app == nil {
		return errors.New("failed to stop, tview.Application nil")
	}
	ui.app.Stop()
	fmt.Println("ktop closed")
	close(ui.stopCh)
	return nil
}

func (ui *Application) switchPage(index int) {
	if !ui.root.HasPage(PageNames[index]) {
		return
	}

	row := 0
	cols := ui.buttons.GetColumnCount()

	for i := 0; i < cols; i++ {
		cell := ui.buttons.GetCell(row, i)
		if i == index {
			cell.SetTextColor(buttonSelectedFgColor)
			cell.SetBackgroundColor(buttonSelectedBgColor)
		} else {
			cell.SetTextColor(buttonUnselectedFgColor)
			cell.SetBackgroundColor(buttonUnselectedBgColor)
		}
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
				Expansion:       1,
			},
		)
	}

	return buttons
}
