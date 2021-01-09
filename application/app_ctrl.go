package application

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"

	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/ui"
)

type ApplicationPanel struct {
	Title string
	View  tview.Primitive
}

type Application struct {
	k8sClient *k8s.Client
	tviewApp  *tview.Application
	views     []ApplicationPanel
	visibleView int
	panel     *appPanel
	refreshQ  chan struct{}
	stopCh    chan struct{}
}

func New(k8sClient *k8s.Client) *Application {
	tapp := tview.NewApplication()
	app := &Application{
		k8sClient: k8sClient,
		tviewApp:  tapp,
		panel:     newPanel(tapp),
		refreshQ:  make(chan struct{}, 1),
	}
	return app
}

func (app *Application) GetK8sClient() *k8s.Client {
	return app.k8sClient
}

func (app *Application) AddPanel(panel ui.PanelController) {
	title := panel.GetTitle()
	if err := panel.Run(); err != nil {
		panic(fmt.Sprintf("application.AddPanel failed: %s", err))
	}
	app.views = append(app.views, ApplicationPanel{Title: title, View: panel.GetView()})
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

func (app *Application) Init() {
	app.panel.Layout(app.views)

	var hdr strings.Builder
	hdr.WriteString("%c [green]API server: [white]%s [green]namespace: [white]%s [green] metrics:")
	if err := app.k8sClient.AssertMetricsAvailable(); err != nil {
		hdr.WriteString(" [red]server not available")
	}else{
		hdr.WriteString(" [white]server connected")
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

		if event.Key() < tcell.KeyF1 || event.Key() > tcell.KeyF12 {
			return event
		}

		keyPos := event.Key() - tcell.KeyF1
		titles := app.getPageTitles()
		if (keyPos >= 0 || keyPos <= 9) && (int(keyPos) <= len(titles)-1) {
			app.panel.switchToPage(app.getPageTitles()[keyPos])
		}
		return event
	})

}

func (app *Application) Run() error {

	// setup refresh queue
	go func() {
		for range app.refreshQ {
			app.tviewApp.Draw()
		}
	}()

	return app.tviewApp.Run()
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
	for _, page := range app.views {
		titles = append(titles, page.Title)
	}
	return
}
