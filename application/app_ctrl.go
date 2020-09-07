package application

import (
	"errors"
	"fmt"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/ui"
)

type Application struct {
	k8sClient *k8s.Client
	tviewApp  *tview.Application
	appui     *appPanel
	refreshQ  chan struct{}
	stopCh    chan struct{}
}

func New(k8sClient *k8s.Client) *Application {
	app := &Application{
		k8sClient: k8sClient,
		tviewApp:  tview.NewApplication(),
		refreshQ:  make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
	}

	app.appui = newAppPanel(app.tviewApp)
	app.setHeader()

	app.tviewApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			app.Stop()
		case tcell.KeyF1:
			app.appui.switchToPage(0)
		case tcell.KeyF2:
			app.appui.switchToPage(1)
		default:
			return event
		}

		return event
	})

	// setup refresh queue
	go func() {
		for range app.refreshQ {
			app.tviewApp.Draw()
		}
	}()

	return app
}

func (app *Application) setHeader() {
	app.appui.DrawHeader(fmt.Sprintf(
		"%c [green]API server: [white]%s [green]namespace: [white]%s",
		ui.Icons.Rocket,
		app.k8sClient.Config.Host,
		app.k8sClient.Namespace,
	))
}

func (app *Application) GetK8sClient() *k8s.Client {
	return app.k8sClient
}

func (app *Application) AddPage(title string, page tview.Primitive) {
	app.appui.addPage(title, page)
}

func (app *Application) Focus(t tview.Primitive) {
	app.tviewApp.SetFocus(t)
}

func (app *Application) Refresh() {
	app.refreshQ <- struct{}{}
}

func (app *Application) ShowPage(i int) {
	app.appui.switchToPage(i)
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

func (app *Application) Run() error {
	if app.tviewApp == nil {
		return errors.New("failed to start, tview.Application nil")
	}
	app.k8sClient.InformerFactory.Start(app.GetStopChan())
	return app.tviewApp.Run()
}

func (app *Application) Stop() error {
	if app.tviewApp == nil {
		return errors.New("failed to stop, tview.Application nil")
	}
	app.tviewApp.Stop()
	close(app.stopCh)
	fmt.Println("ktop finished")
	return nil
}
