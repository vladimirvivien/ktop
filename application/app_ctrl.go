package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/buildinfo"

	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
)

type AppPage struct {
	Title string
	Panel ui.PanelController
}

type Application struct {
	namespace     string
	k8sClient     *k8s.Client
	metricsSource metrics.MetricsSource
	tviewApp      *tview.Application
	pages         []AppPage
	modals        []tview.Primitive
	pageIdx       int
	tabIdx        int
	visibleView   int
	panel         *appPanel
	refreshQ      chan struct{}
	stopCh        chan struct{}
}

func New(k8sC *k8s.Client, metricsSource metrics.MetricsSource) *Application {
	tapp := tview.NewApplication()
	app := &Application{
		k8sClient:     k8sC,
		metricsSource: metricsSource,
		namespace:     k8sC.Namespace(),
		tviewApp:      tapp,
		panel:         newPanel(tapp),
		refreshQ:      make(chan struct{}, 1),
		pageIdx:       -1,
		tabIdx:        -1,
	}
	return app
}

func (app *Application) GetK8sClient() *k8s.Client {
	return app.k8sClient
}

func (app *Application) GetMetricsSource() metrics.MetricsSource {
	return app.metricsSource
}

func (app *Application) AddPage(panel ui.PanelController) {
	app.pages = append(app.pages, AppPage{Title: panel.GetTitle(), Panel: panel})
}

func (app *Application) ShowModal(view tview.Primitive) {
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

func (app *Application) WelcomeBanner() {
	fmt.Println(`
 _    _
| | _| |_ ___  _ __
| |/ / __/ _ \| '_ \
|   <| || (_) | |_) |
|_|\_\\__\___/| .__/
              |_|`)
	fmt.Printf("Version %s\n", buildinfo.Version)
	fmt.Println("Loading cluster data...")
}

func (app *Application) setup(ctx context.Context) error {
	// setup each page panel
	for _, page := range app.pages {
		if err := page.Panel.Run(ctx); err != nil {
			return fmt.Errorf("init failed: page %s: %s", page.Title, err)
		}
	}

	// continue setup rest of UI
	app.panel.Layout(app.pages)

	var hdr strings.Builder
	hdr.WriteString("%c [green]API server: [white]%s [green]Version: [white]%s [green]context: [white]%s [green]User: [white]%s [green]namespace: [white]%s [green]metrics:")
	// Check MetricsSource health instead of blocking network call
	// This provides accurate status for both metrics-server and prometheus sources
	if app.metricsSource != nil && app.metricsSource.IsHealthy() {
		sourceInfo := app.metricsSource.GetSourceInfo()
		hdr.WriteString(fmt.Sprintf(" [white]%s", sourceInfo.Type))
	} else {
		hdr.WriteString(" [red]not connected")
	}

	namespace := app.k8sClient.Namespace()
	if namespace == k8s.AllNamespaces {
		namespace = "[orange](all)"
	}
	client := app.GetK8sClient()
	app.panel.DrawHeader(fmt.Sprintf(
		hdr.String(),
		ui.Icons.Rocket, client.RESTConfig().Host, client.GetServerVersion(), client.ClusterContext(), client.Username(), namespace,
	))

	app.panel.DrawFooter(app.getPageTitles()[app.visibleView])

	// Start periodic header refresh to update metrics status
	go app.refreshHeaderPeriodically(ctx)

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

		return event
	})

	return nil
}

func (app *Application) Run(ctx context.Context) error {

	// setup application UI
	if err := app.setup(ctx); err != nil {
		return err
	}

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
	for _, page := range app.pages {
		titles = append(titles, page.Title)
	}
	return
}

// refreshHeaderPeriodically updates the header to reflect current metrics status
// It does an immediate check after 2 seconds (to catch the first metrics fetch),
// then checks every 10 seconds thereafter
func (app *Application) refreshHeaderPeriodically(ctx context.Context) {
	// Do an immediate check after 2 seconds to catch the first metrics fetch
	time.Sleep(2 * time.Second)
	app.updateHeader()

	// Then check periodically every 10 seconds
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			app.updateHeader()
		}
	}
}

// updateHeader refreshes the header with current metrics status
func (app *Application) updateHeader() {
	var hdr strings.Builder
	hdr.WriteString("%c [green]API server: [white]%s [green]Version: [white]%s [green]context: [white]%s [green]User: [white]%s [green]namespace: [white]%s [green]metrics:")

	// Check MetricsSource health
	if app.metricsSource != nil && app.metricsSource.IsHealthy() {
		sourceInfo := app.metricsSource.GetSourceInfo()
		hdr.WriteString(fmt.Sprintf(" [white]%s", sourceInfo.Type))
	} else {
		hdr.WriteString(" [red]not connected")
	}

	namespace := app.k8sClient.Namespace()
	if namespace == k8s.AllNamespaces {
		namespace = "[orange](all)"
	}
	client := app.GetK8sClient()

	// Queue a UI update
	app.tviewApp.QueueUpdateDraw(func() {
		app.panel.DrawHeader(fmt.Sprintf(
			hdr.String(),
			ui.Icons.Rocket, client.RESTConfig().Host, client.GetServerVersion(),
			client.ClusterContext(), client.Username(), namespace,
		))
	})
}
