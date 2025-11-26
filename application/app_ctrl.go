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
	"github.com/vladimirvivien/ktop/health"

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

	// Health state tracking for transitions
	lastHealthyState      bool
	lastMetricsSource     string
	loadingToastID        string
	loadingToastStartTime time.Time

	// API health tracking
	apiHealthTracker   *health.APIHealthTracker
	apiHealthToastID   string // Persistent toast for API health issues
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

	// Initialize API health tracker with persistent toast callback
	app.apiHealthTracker = health.NewAPIHealthTracker(func(state health.APIState, msg string) {
		// Use QueueUpdateDraw to safely update UI from callback
		tapp.QueueUpdateDraw(func() {
			switch state {
			case health.APIHealthy:
				// Connection restored - dismiss persistent toast and show brief success (no buttons)
				if app.apiHealthToastID != "" {
					app.DismissToast(app.apiHealthToastID)
					app.apiHealthToastID = ""
				}
				app.ShowToast(msg, ui.ToastSuccess, 3*time.Second)

			case health.APIUnhealthy:
				// Connection lost or retrying - show persistent toast (no buttons during retry)
				if app.apiHealthToastID != "" {
					app.DismissToast(app.apiHealthToastID)
				}
				// Duration 0 = persistent, no auto-dismiss, no buttons during retry sequence
				app.apiHealthToastID = app.ShowToast(msg, ui.ToastWarning, 0)

			case health.APIDisconnected:
				// All retries exhausted - show persistent error toast with Retry and Quit buttons
				if app.apiHealthToastID != "" {
					app.DismissToast(app.apiHealthToastID)
				}
				// Duration 0 = persistent, no auto-dismiss
				app.apiHealthToastID = app.ShowToastWithButtons(msg, ui.ToastError, 0, []string{"Retry", "Quit"})
			}
		})
	})

	// Set up callbacks for health state changes
	app.apiHealthTracker.SetOnDisconnected(func() {
		app.Refresh() // Trigger UI refresh to show zeroed values
	})

	app.apiHealthTracker.SetOnHealthy(func() {
		app.Refresh() // Trigger UI refresh when reconnected
	})

	return app
}

func (app *Application) GetK8sClient() *k8s.Client {
	return app.k8sClient
}

func (app *Application) GetMetricsSource() metrics.MetricsSource {
	return app.metricsSource
}

// GetAPIHealthTracker returns the API health tracker for controllers to report status
func (app *Application) GetAPIHealthTracker() *health.APIHealthTracker {
	return app.apiHealthTracker
}

// GetAPIHealth returns the current API health state
func (app *Application) GetAPIHealth() health.APIState {
	if app.apiHealthTracker == nil {
		return health.APIHealthy
	}
	return app.apiHealthTracker.GetState()
}

// IsAPIHealthy returns true if the API connection is healthy
func (app *Application) IsAPIHealthy() bool {
	return app.apiHealthTracker == nil || app.apiHealthTracker.IsHealthy()
}

// IsAPIDisconnected returns true if the API connection has been lost
func (app *Application) IsAPIDisconnected() bool {
	return app.apiHealthTracker != nil && app.apiHealthTracker.IsDisconnected()
}

func (app *Application) AddPage(panel ui.PanelController) {
	app.pages = append(app.pages, AppPage{Title: panel.GetTitle(), Panel: panel})
}

func (app *Application) ShowModal(view tview.Primitive) {
	app.panel.showModalView(view)
}

func (app *Application) ShowToast(message string, level ui.ToastLevel, duration time.Duration) string {
	return app.panel.showToast(message, level, duration)
}

func (app *Application) ShowToastWithButtons(message string, level ui.ToastLevel, duration time.Duration, buttons []string) string {
	return app.panel.showToastWithButtons(message, level, duration, buttons)
}

// HasActiveToast returns true if a toast modal is currently displayed
func (app *Application) HasActiveToast() bool {
	return app.panel.hasActiveToast()
}

func (app *Application) DismissToast(toastID string) {
	app.panel.dismissToast(toastID)
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
	hdr.WriteString("%s [green]API server: [white]%s [green]Version: [white]%s [green]context: [white]%s [green]User: [white]%s [green]namespace: [white]%s [green]metrics:")
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

	// Show loading toast immediately when source is initializing
	if app.metricsSource != nil {
		sourceInfo := app.metricsSource.GetSourceInfo()
		app.loadingToastStartTime = time.Now()
		app.loadingToastID = app.ShowToast(
			fmt.Sprintf("Waiting for metrics: %s...", sourceInfo.Type),
			ui.ToastInfo,
			0, // No timeout - dismiss when healthy or timeout
		)
		app.lastHealthyState = false
		app.lastMetricsSource = sourceInfo.Type
	}

	// Start periodic header refresh to update metrics status
	go app.refreshHeaderPeriodically(ctx)

	// Set toast button callback to handle Retry and Quit buttons
	app.panel.setToastButtonCallback(func(buttonLabel string) {
		switch buttonLabel {
		case "Quit":
			app.Stop()
		case "Retry":
			if app.IsAPIDisconnected() {
				app.apiHealthTracker.TryReconnect()
			}
		}
	})

	app.tviewApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// When a toast modal is displayed, let it handle all input
		// Don't process global shortcuts here - the modal handles them
		if app.HasActiveToast() {
			return event // Pass through to modal
		}

		// Handle ESC to quit (only when no modal is shown)
		if event.Key() == tcell.KeyEsc {
			app.Stop()
			return nil
		}

		// Handle 'R' key for reconnecting when API is disconnected
		if event.Key() == tcell.KeyRune && (event.Rune() == 'R' || event.Rune() == 'r') {
			if app.IsAPIDisconnected() {
				app.apiHealthTracker.TryReconnect()
				return nil
			}
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
	app.checkHealthTransition() // Check for health transitions

	// Then check periodically every 10 seconds
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			app.updateHeader()
			app.checkHealthTransition() // Check for health transitions
		}
	}
}

// updateHeader refreshes the header with current metrics status
func (app *Application) updateHeader() {
	var hdr strings.Builder
	hdr.WriteString("%s [green]API server: [white]%s [green]Version: [white]%s [green]context: [white]%s [green]User: [white]%s [green]namespace: [white]%s [green]metrics:")

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

// checkHealthTransition detects when metrics source transitions between healthy/unhealthy
// and displays appropriate toast notifications
// NOTE: Metrics toasts are suppressed when API server is unhealthy to reduce noise
func (app *Application) checkHealthTransition() {
	if app.metricsSource == nil {
		// No metrics source (fallback mode) - no toasts
		return
	}

	// IMPORTANT: If API server is unhealthy/disconnected, don't show metrics toasts
	// The API health tracker handles all connection-related notifications
	if !app.IsAPIHealthy() {
		// Just track the state silently without showing toasts
		app.lastHealthyState = app.metricsSource.IsHealthy()
		// Dismiss any pending loading toast
		if app.loadingToastID != "" {
			app.DismissToast(app.loadingToastID)
			app.loadingToastID = ""
		}
		return
	}

	currentHealthy := app.metricsSource.IsHealthy()
	sourceInfo := app.metricsSource.GetSourceInfo()

	// Check for loading timeout (15 seconds)
	if !app.lastHealthyState && !currentHealthy && app.loadingToastID != "" {
		elapsed := time.Since(app.loadingToastStartTime)
		if elapsed > 15*time.Second {
			// Dismiss loading toast
			app.DismissToast(app.loadingToastID)
			app.loadingToastID = ""

			// Show error toast with auto-dismiss
			app.ShowToast(
				fmt.Sprintf("%s metrics unavailable", sourceInfo.Type),
				ui.ToastError,
				5*time.Second,
			)
		}
		return // Don't process other transitions while in loading state
	}

	// Detect transition from unhealthy -> healthy
	if !app.lastHealthyState && currentHealthy {
		// Dismiss loading toast
		if app.loadingToastID != "" {
			app.DismissToast(app.loadingToastID)
			app.loadingToastID = ""
		}

		// Show success toast
		app.ShowToast(
			fmt.Sprintf("%s metrics connected", sourceInfo.Type),
			ui.ToastSuccess,
			3*time.Second,
		)
		app.lastHealthyState = true
	}

	// Detect transition from healthy -> unhealthy
	if app.lastHealthyState && !currentHealthy {
		// Show error toast (only on critical errors)
		app.ShowToast(
			fmt.Sprintf("%s error: connection failed", sourceInfo.Type),
			ui.ToastError,
			5*time.Second,
		)
		app.lastHealthyState = false
	}
}
