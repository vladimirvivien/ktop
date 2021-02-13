package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/ui"
)

type AppPage struct {
	Title string
	Panel ui.PanelController
}

type Application struct {
	namespace   string
	k8sClient   *k8s.Client
	tviewApp    *tview.Application
	pages       []AppPage
	modals      []tview.Primitive
	pageIdx     int
	tabIdx      int
	visibleView int
	panel       *appPanel
	refreshQ    chan struct{}
	stopCh      chan struct{}
}

func New(k8sC *k8s.Client) *Application {
	tapp := tview.NewApplication()
	app := &Application{
		k8sClient: k8sC,
		namespace: k8sC.Namespace(),
		tviewApp: tapp,
		panel:    newPanel(tapp),
		refreshQ: make(chan struct{}, 1),
		pageIdx:  -1,
		tabIdx:   -1,
	}
	return app
}

func (app *Application) GetK8sClient() *k8s.Client {
	return app.k8sClient
}

func (app *Application) AddPage(panel ui.PanelController) {
	app.pages = append(app.pages, AppPage{Title: panel.GetTitle(), Panel: panel})
}

func (app *Application)ShowModal(view tview.Primitive) {
	app.panel.showModalView(view)
}

func (app *Application) Focus(t tview.Primitive) {
	app.tviewApp.SetFocus(t)
}

func (app *Application) Refresh() {
	app.refreshQ <- struct{}{}
}

func (app *Application) ShowPanel(i int) {
	app.visibleView = i
}

func (app *Application) GetStopChan() <-chan struct{} {
	return app.stopCh
}

// ShowBanner displays a welcome banner
func (app *Application) WelcomeBanner() {
	fmt.Println(`
 _    _              
| | _| |_ ___  _ __  
| |/ / __/ _ \| '_ \ 
|   <| || (_) | |_) |
|_|\_\\__\___/| .__/ 
              |_|`)
	fmt.Println("Version 0.1.0-alpha.1")
}

func (app *Application) setup() error {
	// setup each page panel
	for _, page := range app.pages {
		if err := page.Panel.Run(); err != nil {
			return fmt.Errorf("init failed: page %s: %s", page.Title, err)
		}
	}

	// continue setup rest of UI
	app.panel.Layout(app.pages)

	var hdr strings.Builder
	hdr.WriteString("%c [green]API server: [white]%s [green]namespace: [white]%s [green] metrics:")
	if err := app.k8sClient.AssertMetricsAvailable(); err != nil {
		hdr.WriteString(" [red]not connected")
	} else {
		hdr.WriteString(" [white]connected")
	}

	app.panel.DrawHeader(fmt.Sprintf(
		hdr.String(),
		ui.Icons.Rocket, app.k8sClient.Config().Host, app.k8sClient.Namespace(),
	))

	app.panel.DrawFooter(app.getPageTitles()[app.visibleView])

	app.tviewApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			app.Stop()
		}

		if event.Key() == tcell.KeyTAB {
			views := app.pages[0].Panel.GetChildrenViews()
			app.tabIdx++
			app.Focus(views[app.tabIdx])
			if app.tabIdx == len(views)-1 {
				app.tabIdx = -1
			}
		}

		if event.Key() < tcell.KeyF1 || event.Key() > tcell.KeyF12 {
			return event
		}

		keyPos := event.Key() - tcell.KeyF1
		titles := app.getPageTitles()
		if (keyPos >= 0 || keyPos <= 9) && (int(keyPos) <= len(titles)-1) {
			app.panel.switchToPage(app.getPageTitles()[keyPos])
		}

		// start app gui

		return event
	})

	return nil
}

func (app *Application) Run(ctx context.Context) error {

	// setup application UI
	if err := app.setup(); err != nil {
		return err
	}

	// setup refresh queue
	go func() {
		for range app.refreshQ {
			app.tviewApp.Draw()
		}
	}()

	return app.tviewApp.EnableMouse(true).Run()
}

func (app *Application) Stop() error {
	if app.tviewApp == nil {
		return errors.New("failed to stop, tview.Application nil")
	}
	app.tviewApp.Stop()
	fmt.Println("ktop finished")
	return nil
}

func (app *Application) getPageTitles() (titles []string) {
	for _, page := range app.pages {
		titles = append(titles, page.Title)
	}
	return
}
