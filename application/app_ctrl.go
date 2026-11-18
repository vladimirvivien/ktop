package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	sessions      []PageSession
	modals        []tview.Primitive
	pageIdx       int
	tabIdx        int // -1 = header, 0+ = children panels
	visibleView   int
	panel         *appPanel
	refreshQ      chan struct{}
	stopCh        chan struct{}

	// Navigation stack for active page (synced with active session)
	navStack *NavigationStack

	// Detail view callbacks
	nodeDetailCallback     func(nodeName string)
	podDetailCallback      func(namespace, podName string)
	workloadDetailCallback func(kind, namespace, name string)
	containerLogsCallback  func(namespace, podName, containerName string)
	detailResetCallback    func()
	detailResetListeners   []func()

	// Health state tracking for transitions
	lastHealthyState      bool
	lastMetricsSource     string
	loadingToastID        string
	loadingToastStartTime time.Time

	// Metrics health debouncing (prevents flapping during server restart)
	metricsConsecOK      int       // Consecutive successful health checks
	metricsLastErrorTime time.Time // Time of last error (for minimum unhealthy duration)

	// API health tracking
	apiHealthTracker *health.APIHealthTracker
	apiHealthToastID string // Persistent toast for API health issues

	// Namespace filter callbacks for multi-page pod/resource filtering
	namespaceFilterCallback  func(namespace string)
	namespaceFilterListeners []func(namespace string)

	// Quit confirmation state (double-ESC to quit from root page)
	pendingQuit     bool
	pendingQuitTime time.Time
}

func New(k8sC *k8s.Client, metricsSource metrics.MetricsSource) *Application {
	tapp := tview.NewApplication()
	ns := k8s.AllNamespaces
	if k8sC != nil {
		ns = k8sC.Namespace()
	}
	app := &Application{
		k8sClient:     k8sC,
		metricsSource: metricsSource,
		namespace:     ns,
		tviewApp:      tapp,
		panel:         newPanel(tapp),
		refreshQ:      make(chan struct{}, 1),
		pageIdx:       -1,
		tabIdx:        -1, // -1 = header (default focus), 0+ = children panels
		navStack:      NewNavigationStack(),
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

// GetTerminalHeight returns the current terminal height
func (app *Application) GetTerminalHeight() int {
	if app.panel == nil {
		return 50 // Default to medium during initialization
	}
	root := app.panel.GetRootView()
	// Check if root is actually usable (not a nil interface or nil pointer)
	if root == nil {
		return 50
	}
	// Use type assertion to check for nil pointer wrapped in interface
	if pages, ok := root.(*tview.Pages); ok && pages == nil {
		return 50
	}
	return ui.GetTerminalHeight(root)
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
	return app == nil || app.apiHealthTracker == nil || app.apiHealthTracker.IsHealthy()
}

// IsAPIDisconnected returns true if the API connection has been lost
func (app *Application) IsAPIDisconnected() bool {
	return app != nil && app.apiHealthTracker != nil && app.apiHealthTracker.IsDisconnected()
}

func (app *Application) AddPage(panel ui.PanelController) {
	title := ""
	if panel != nil {
		title = panel.GetTitle()
	}
	app.RegisterPage(PageDescriptor{
		Title: title,
		Panel: panel,
	})
}

func (app *Application) ShowModal(view tview.Primitive) {
	app.panel.showModalView(view)
}

// ShowModalPage displays a named modal overlay page on top of the UI
func (app *Application) ShowModalPage(name string, view tview.Primitive) {
	app.panel.showModalPage(name, view)
}

// DismissModalPage dismisses the named modal overlay page
func (app *Application) DismissModalPage(name string) {
	app.panel.dismissModalPage(name)
}

// HasActiveModalPage returns true if a modal overlay page is currently displayed
func (app *Application) HasActiveModalPage() bool {
	return app.panel.hasActiveModalPage()
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
	select {
	case app.refreshQ <- struct{}{}:
	default:
	}
}

// QueueUpdateDraw safely queues a UI update function to run on the main goroutine.
// Use this when updating UI from background goroutines (e.g., controller callbacks).
// The function will be executed and followed by a screen redraw.
func (app *Application) QueueUpdateDraw(f func()) {
	app.tviewApp.QueueUpdateDraw(f)
}

// SetNamespaceFilterCallback sets the primary callback for namespace filter changes.
func (app *Application) SetNamespaceFilterCallback(callback func(namespace string)) {
	app.namespaceFilterCallback = callback
	if app.panel != nil {
		app.panel.setNamespaceFilterCallback(app.dispatchNamespaceFilter)
	}
}

// AddNamespaceFilterListener registers an additional listener notified when the header namespace filter changes.
func (app *Application) AddNamespaceFilterListener(callback func(namespace string)) {
	if callback == nil {
		return
	}
	app.namespaceFilterListeners = append(app.namespaceFilterListeners, callback)
	if app.panel != nil {
		app.panel.setNamespaceFilterCallback(app.dispatchNamespaceFilter)
	}
}

func (app *Application) dispatchNamespaceFilter(namespace string) {
	if app.namespaceFilterCallback != nil {
		app.namespaceFilterCallback(namespace)
	}
	for _, listener := range app.namespaceFilterListeners {
		listener(namespace)
	}
	for _, page := range app.pages {
		if target, ok := page.Panel.(interface{ SetNamespaceFilter(string) }); ok {
			target.SetNamespaceFilter(namespace)
		}
	}
}

// GetNamespaceFilter returns the current namespace filter text
func (app *Application) GetNamespaceFilter() string {
	return app.panel.getNamespaceFilter()
}

// updatePanelFocus updates focus state for all child panels based on tabIdx
func (app *Application) updatePanelFocus(views []tview.Primitive) {
	activePanel := app.activePagePanel()
	if activePanel == nil {
		return
	}
	children := activePanel.GetChildrenViews()

	for i, child := range children {
		if focusable, ok := app.getPanelForView(child).(ui.FocusablePanel); ok {
			focusable.SetFocused(i == app.tabIdx)
		}
	}
}

// getPanelForView returns the panel for a given view (helper for focus management)
func (app *Application) getPanelForView(view tview.Primitive) interface{} {
	activePanel := app.activePagePanel()
	if activePanel == nil {
		return nil
	}
	children := activePanel.GetChildrenViews()

	for i, child := range children {
		if child == view {
			if provider, ok := activePanel.(interface{ GetChildPanel(int) ui.Panel }); ok {
				return provider.GetChildPanel(i)
			}
		}
	}
	return nil
}

func (app *Application) restoreActivePageFocus() {
	if app.panel == nil {
		return
	}
	if app.IsInDetailView() {
		if panel := app.getActiveDetailPanel(); panel != nil {
			if rootProvider, ok := panel.(interface{ GetRootView() tview.Primitive }); ok {
				app.panel.setHeaderFocused(false)
				app.Focus(rootProvider.GetRootView())
				return
			}
		}
	}

	activePanel := app.activePagePanel()
	var views []tview.Primitive
	if activePanel != nil {
		views = activePanel.GetChildrenViews()
	}

	if app.tabIdx == -1 || len(views) == 0 {
		app.tabIdx = -1
		app.panel.setHeaderFocused(true)
		app.updatePanelFocus(views)
		app.Focus(app.panel.getHeader())
		return
	}

	if app.tabIdx >= len(views) {
		app.tabIdx = len(views) - 1
	}
	app.panel.setHeaderFocused(false)
	app.updatePanelFocus(views)
	app.Focus(views[app.tabIdx])
}

func (app *Application) syncTabBar() {
	if app.panel == nil {
		return
	}
	s := app.activeSession()
	if s == nil {
		app.panel.setBreadcrumb("")
		return
	}
	app.panel.setActiveTab(app.visibleView, "")
	if s.NavStack == nil {
		app.panel.setBreadcrumb("")
		return
	}
	app.panel.setBreadcrumb(FormatBreadcrumb(s.Descriptor.Title, s.NavStack.Breadcrumb()))
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

// Setup initializes pages, layout, and input capture without entering the blocking event loop.
func (app *Application) Setup(ctx context.Context) error {
	return app.setup(ctx)
}

// RootPrimitive returns the top-level root primitive for rendering and simulation testing.
func (app *Application) RootPrimitive() tview.Primitive {
	if app.panel == nil {
		return nil
	}
	return app.panel.GetRootView()
}

// HandleInput dispatches a key event through the application's global input capture handler.
func (app *Application) HandleInput(event *tcell.EventKey) *tcell.EventKey {
	return app.handleInputEvent(event)
}

// FocusedPrimitive returns the primitive currently holding focus in tview.
func (app *Application) FocusedPrimitive() tview.Primitive {
	if app.tviewApp == nil {
		return nil
	}
	return app.tviewApp.GetFocus()
}

func (app *Application) setup(ctx context.Context) error {
	if len(app.sessions) > 0 {
		if app.visibleView < 0 || app.visibleView >= len(app.sessions) {
			app.visibleView = 0
		}
		app.navStack = app.sessions[app.visibleView].NavStack
		app.tabIdx = app.sessions[app.visibleView].TabIdx
	}

	// setup each page panel
	for _, page := range app.pages {
		if page.Panel == nil {
			continue
		}
		if err := page.Panel.Run(ctx); err != nil {
			return fmt.Errorf("init failed: page %s: %s", page.Title, err)
		}
	}

	if app.k8sClient != nil {
		if err := app.k8sClient.Controller().Start(ctx, 10*time.Second); err != nil {
			return fmt.Errorf("init failed: k8s controller: %w", err)
		}
	}

	// continue setup rest of UI
	if len(app.sessions) > 0 {
		app.panel.Layout(app.sessions)
	} else {
		app.panel.Layout(app.pages)
	}

	// Draw initial header using shared helper functions
	nsDisplay := app.getNamespaceDisplay()
	app.panel.DrawHeader(app.buildHeaderString(nsDisplay))

	if s := app.activeSession(); s != nil {
		app.panel.setActiveTab(app.visibleView, s.Descriptor.Title)
	} else if titles := app.getPageTitles(); app.visibleView >= 0 && app.visibleView < len(titles) {
		app.panel.setActiveTab(app.visibleView, titles[app.visibleView])
	}
	app.syncTabBar()

	// Set initial focus to header panel and unfocus all child panels
	app.restoreActivePageFocus()

	// Set initial footer context for active page
	app.updateFooterContext()

	// Set up focus restoration callback for toast and modal dismissal
	app.panel.setFocusRestorationCallback(func() {
		app.restoreActivePageFocus()
	})

	// Set up event-driven metrics health monitoring (replaces polling)
	if app.metricsSource != nil {
		sourceInfo := app.metricsSource.GetSourceInfo()
		app.lastMetricsSource = sourceInfo.Type

		// Register health callback for instant updates
		app.metricsSource.SetHealthCallback(func(healthy bool, info metrics.SourceInfo) {
			app.tviewApp.QueueUpdateDraw(func() {
				app.handleMetricsHealthChange(healthy, info)
			})
		})

		// Check health state and update UI accordingly
		// This handles the race where health changed between initial header draw
		// and callback registration
		if !app.metricsSource.IsHealthy() {
			app.loadingToastStartTime = time.Now()
			app.loadingToastID = app.ShowToast(
				fmt.Sprintf("Waiting for metrics: %s...", sourceInfo.Type),
				ui.ToastInfo,
				0, // No timeout - dismiss when healthy or timeout
			)
			app.lastHealthyState = false

			// One-shot timeout check (15 seconds) instead of polling
			time.AfterFunc(15*time.Second, func() {
				app.tviewApp.QueueUpdateDraw(func() {
					if app.loadingToastID != "" && app.metricsSource != nil && !app.metricsSource.IsHealthy() {
						app.DismissToast(app.loadingToastID)
						app.loadingToastID = ""
						sourceInfo := app.metricsSource.GetSourceInfo()
						app.ShowToast(
							fmt.Sprintf("%s metrics unavailable", sourceInfo.Type),
							ui.ToastError,
							5*time.Second,
						)
					}
				})
			})
		} else {
			app.lastHealthyState = true
			// Redraw header to ensure it reflects the healthy state
			// This handles the race where health transitioned before callback was registered
			nsDisplay := app.getNamespaceDisplay()
			app.panel.DrawHeader(app.buildHeaderString(nsDisplay))
		}
	}

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

	app.tviewApp.SetInputCapture(app.handleInputEvent)

	return nil
}

func (app *Application) handleInputEvent(event *tcell.EventKey) *tcell.EventKey {
	// When a toast modal is displayed, handle ESC at app level
	// This is necessary because tview.Modal with no buttons cannot receive focus,
	// so the modal's SetInputCapture never gets called for key events.
	if app.HasActiveToast() {
		if event.Key() == tcell.KeyEsc {
			app.panel.handleToastEsc()
			return nil // Consume the event
		}
		return event // Pass other keys through
	}

	// When a modal page (e.g. column selector) is displayed, pass key events
	// directly to the modal widget so it can handle Esc, Enter, Space, etc.
	// Do NOT intercept ESC for app quit.
	if app.HasActiveModalPage() {
		return event
	}

	// Reset pending quit state on any non-ESC key
	if event.Key() != tcell.KeyEsc && app.pendingQuit {
		app.pendingQuit = false
	}

	// Handle '?' and F1 for help overlay modal (when not actively editing text)
	if !app.isFilterEditing() && (event.Key() == tcell.KeyF1 || (event.Key() == tcell.KeyRune && event.Rune() == '?')) {
		app.ShowHelpModal()
		return nil
	}

	// Handle top-level page switching when not editing a filter input
	if !app.isFilterEditing() && event.Key() == tcell.KeyRune {
		r := event.Rune()
		if r >= '1' && r <= '9' {
			idx := int(r - '1')
			if idx < app.PageCount() {
				app.SwitchToPageIndex(idx)
				return nil
			}
		} else if app.PageCount() > 1 {
			switch r {
			case ']':
				app.NextPage()
				return nil
			case '[':
				app.PrevPage()
				return nil
			}
		}
	}

	// Handle ESC - check if header or visible panel has state to clear first
	// Global handler runs BEFORE focused widget, so we must check panels here
	if event.Key() == tcell.KeyEsc {
		// Check if we're in a detail view
		if app.IsInDetailView() {
			// First check if the detail panel has escapable state (e.g., filter mode)
			if panel := app.getActiveDetailPanel(); panel != nil {
				if escapable, ok := panel.(ui.EscapablePanel); ok {
					if escapable.HandleEscape() {
						app.Refresh()
						return nil // Panel handled ESC (e.g., closed filter)
					}
				}
			}
			// No state to clear - navigate back
			app.NavigateBack()
			app.Refresh()
			return nil
		}

		// Check if header has escapable state first (when header is focused)
		if app.tabIdx == -1 && app.panel.hasEscapableHeaderState() {
			if app.panel.handleHeaderEscape() {
				// Use updateHeaderDirect (not updateHeader) because we're
				// already on the main goroutine. QueueUpdateDraw would deadlock.
				app.updateHeaderDirect()
				app.Refresh()
				return nil // Header handled ESC, don't quit
			}
		}
		// Check if the currently active root panel has escapable state
		if activePanel := app.activePagePanel(); activePanel != nil {
			if escapable, ok := activePanel.(ui.EscapablePanel); ok {
				if escapable.HandleEscape() {
					app.Refresh()
					return nil // Panel handled ESC, don't quit
				}
			}
		}
		// No panel had state to clear - handle quit confirmation
		if app.pendingQuit && time.Since(app.pendingQuitTime) < 2*time.Second {
			// Second ESC within timeout - quit
			app.Stop()
			return nil
		}

		// First ESC - show confirmation toast
		app.pendingQuit = true
		app.pendingQuitTime = time.Now()
		app.ShowToast("Press ESC again to quit", ui.ToastInfo, 2*time.Second)

		// Reset state after toast timeout
		go func() {
			time.Sleep(2 * time.Second)
			app.tviewApp.QueueUpdateDraw(func() {
				app.pendingQuit = false
			})
		}()
		return nil
	}

	// Handle 'R' key for reconnecting when API is disconnected
	if event.Key() == tcell.KeyRune && (event.Rune() == 'R' || event.Rune() == 'r') {
		if app.IsAPIDisconnected() {
			app.apiHealthTracker.TryReconnect()
			return nil
		}
	}

	if event.Key() == tcell.KeyTAB || event.Key() == tcell.KeyBacktab {
		// Let detail pages manage internal Tab cycling across their own subpanels
		if app.IsInDetailView() {
			return event
		}

		var views []tview.Primitive
		if activePanel := app.activePagePanel(); activePanel != nil {
			views = activePanel.GetChildrenViews()
		}

		// Cycle: -1 (header) -> 0 -> ... -> len(views)-1 -> -1 (header)
		if len(views) == 0 {
			app.tabIdx = -1
		} else if event.Key() == tcell.KeyTAB {
			app.tabIdx++
			if app.tabIdx >= len(views) {
				app.tabIdx = -1
			}
		} else {
			app.tabIdx--
			if app.tabIdx < -1 {
				app.tabIdx = len(views) - 1
			}
		}

		app.updatePanelFocus(views)

		if app.tabIdx == -1 || len(views) == 0 {
			app.panel.setHeaderFocused(true)
			app.Focus(app.panel.getHeader())
		} else {
			app.panel.setHeaderFocused(false)
			app.Focus(views[app.tabIdx])
		}
		app.updateFooterContext()
		app.Refresh()
		return nil
	}

	// Handle keyboard input when header is focused
	if app.tabIdx == -1 {
		if app.panel.handleHeaderKey(event) {
			// Use updateHeaderDirect (not updateHeader) because we're
			// already on the main goroutine. QueueUpdateDraw would deadlock.
			app.updateHeaderDirect()
			app.Refresh()
			return nil
		}
	}

	return event
}

func (app *Application) Run(ctx context.Context) error {

	// setup application UI
	if err := app.setup(ctx); err != nil {
		slog.Error("application setup failed", "error", err)
		return err
	}

	// setup refresh queue
	go func() {
		for range app.refreshQ {
			app.QueueUpdateDraw(func() {})
		}
	}()

	slog.Info("tui started", "pages", app.getPageTitles())
	return app.tviewApp.Run()
}

func (app *Application) Stop() error {
	if app.tviewApp == nil {
		return errors.New("failed to stop, tview.Application nil")
	}
	app.tviewApp.Stop()
	slog.Info("ktop stopped")
	return nil
}

func (app *Application) getPageTitles() (titles []string) {
	for _, page := range app.pages {
		titles = append(titles, page.Title)
	}
	return
}

// handleMetricsHealthChange is called via callback when metrics source health changes
// This replaces the polling-based refreshHeaderPeriodically for instant responsiveness
func (app *Application) handleMetricsHealthChange(healthy bool, info metrics.SourceInfo) {
	// IMPORTANT: If API server is unhealthy/disconnected, don't show metrics toasts
	// The API health tracker handles all connection-related notifications
	if !app.IsAPIHealthy() {
		app.lastHealthyState = healthy
		app.metricsConsecOK = 0
		if app.loadingToastID != "" {
			app.DismissToast(app.loadingToastID)
			app.loadingToastID = ""
		}
		return
	}

	// Debouncing constants (prevents flapping during server restart)
	const requiredConsecOK = 2               // Require 2 consecutive successes
	const minUnhealthyTime = 5 * time.Second // Must be unhealthy for at least 5s before recovery

	// Update header immediately
	nsDisplay := app.getNamespaceDisplay()
	app.panel.DrawHeader(app.buildHeaderString(nsDisplay))

	if healthy {
		app.metricsConsecOK++

		// Check debouncing conditions before declaring recovered
		if !app.lastHealthyState {
			// For initial startup (never had an error), accept first success immediately.
			// For recovery after error, apply debouncing to prevent flapping.
			// The health callback only fires on state changes, so debouncing that
			// requires multiple callbacks won't work for initial startup.
			isInitialStartup := app.metricsLastErrorTime.IsZero()

			if !isInitialStartup {
				// Recovery case: apply debouncing
				// Must have enough consecutive successes
				if app.metricsConsecOK < requiredConsecOK {
					return // Wait for more successes
				}

				// Must have been unhealthy long enough (prevents flapping from cached responses)
				if time.Since(app.metricsLastErrorTime) < minUnhealthyTime {
					return // Too soon after last error
				}
			}

			// Declare healthy - either initial startup or debouncing passed
			if app.loadingToastID != "" {
				app.DismissToast(app.loadingToastID)
				app.loadingToastID = ""
			}
			app.ShowToast(
				fmt.Sprintf("%s metrics connected", info.Type),
				ui.ToastSuccess,
				3*time.Second,
			)
			slog.Info("metrics source healthy", "source", info.Type)
			app.lastHealthyState = true
		}
	} else {
		// Reset consecutive OK counter on error
		app.metricsConsecOK = 0
		app.metricsLastErrorTime = time.Now()

		// Detect transition from healthy -> unhealthy
		if app.lastHealthyState {
			app.ShowToast(
				fmt.Sprintf("%s error: connection failed", info.Type),
				ui.ToastError,
				5*time.Second,
			)
			slog.Warn("metrics source unhealthy", "source", info.Type)
			app.lastHealthyState = false
		}
	}
}

// buildHeaderString constructs the header text with current metrics status
// namespaceDisplay is the namespace value to show (may include filter styling)
func (app *Application) buildHeaderString(namespaceDisplay string) string {
	client := app.GetK8sClient()
	contextName := "n/a"
	serverVersion := "n/a"
	user := "n/a"
	if client != nil {
		contextName = ui.Truncate(client.ClusterName(), 25)
		serverVersion = client.GetServerVersion()
		user = ui.Truncate(client.Username(), 20)
	}
	ns := namespaceDisplay

	var metricsStr string
	if app.metricsSource != nil && app.metricsSource.IsHealthy() {
		sourceInfo := app.metricsSource.GetSourceInfo()
		if ui.IsMonochrome() {
			metricsStr = sourceInfo.Type
		} else {
			valTag := ui.TagFg(ui.ActivePalette().DataSecondary)
			metricsStr = fmt.Sprintf("[%s]%s", valTag, sourceInfo.Type)
		}
	} else {
		if ui.IsMonochrome() {
			metricsStr = ui.ErrorMarkup("not connected")
		} else {
			errTag := ui.TagFg(ui.ActivePalette().StatusError)
			metricsStr = fmt.Sprintf("[%s]not connected", errTag)
		}
	}

	if ui.IsMonochrome() {
		return fmt.Sprintf(
			"Context: %s | K8s: %s | User: %s | Namespace: %s | Metrics: %s",
			contextName, serverVersion, user, ns, metricsStr,
		)
	}

	p := ui.ActivePalette()
	lblTag := ui.TagFg(p.GaugeLow)
	valTag := ui.TagFg(p.DataSecondary)
	return fmt.Sprintf(
		"[%s]Context: [%s]%s [%s]| K8s: [%s]%s [%s]| User: [%s]%s [%s]| Namespace: [%s]%s [%s]| Metrics: %s",
		lblTag, valTag, contextName,
		lblTag, valTag, serverVersion,
		lblTag, valTag, user,
		lblTag, valTag, ns,
		lblTag, metricsStr,
	)
}

// getNamespaceDisplay returns the namespace display string based on filter state
func (app *Application) getNamespaceDisplay() string {
	mono := ui.IsMonochrome()
	p := ui.ActivePalette()

	// Check if filter is editing or active
	if app.panel.isNamespaceFilterEditing() {
		filterText := ui.Truncate(app.panel.getNamespaceFilter(), 15)
		if mono {
			return fmt.Sprintf("[::b]%s▌[::-]", filterText)
		}
		keyTag := ui.TagFg(p.DataPrimary)
		if filterText == "" {
			return fmt.Sprintf("[%s]▌[-]", keyTag)
		}
		return fmt.Sprintf("[%s]%s▌[-]", keyTag, filterText)
	}

	// Check if filter is active (has confirmed filter text)
	filterText := app.panel.getNamespaceFilter()
	if filterText != "" {
		return ui.Truncate(filterText, 16)
	}

	// No filter - show actual namespace
	namespace := k8s.AllNamespaces
	if app.k8sClient != nil {
		namespace = app.k8sClient.Namespace()
	}
	if namespace == k8s.AllNamespaces {
		if mono {
			return "(all)"
		}
		return fmt.Sprintf("[%s](all)[-]", ui.TagFg(p.GaugeElevated))
	}
	return ui.Truncate(namespace, 16)
}

// updateHeader refreshes the header with current metrics status
// Uses QueueUpdateDraw - safe to call from any goroutine
func (app *Application) updateHeader() {
	nsDisplay := app.getNamespaceDisplay()
	headerStr := app.buildHeaderString(nsDisplay)
	app.tviewApp.QueueUpdateDraw(func() {
		app.panel.DrawHeader(headerStr)
	})
}

// updateHeaderDirect refreshes the header directly without QueueUpdateDraw
// MUST only be called from the main UI goroutine (e.g., inside SetInputCapture)
func (app *Application) updateHeaderDirect() {
	nsDisplay := app.getNamespaceDisplay()
	headerStr := app.buildHeaderString(nsDisplay)
	app.panel.DrawHeader(headerStr)
}

// SetDetailResetCallback registers a callback invoked when returning from detail views to a root page.
func (app *Application) SetDetailResetCallback(callback func()) {
	app.detailResetCallback = callback
}

// AddDetailResetListener registers an additional callback invoked when returning to a root page.
func (app *Application) AddDetailResetListener(callback func()) {
	if callback != nil {
		app.detailResetListeners = append(app.detailResetListeners, callback)
	}
}

func (app *Application) fireDetailResetCallbacks() {
	if app.detailResetCallback != nil {
		app.detailResetCallback()
	}
	for _, fn := range app.detailResetListeners {
		fn()
	}
}

// SetWorkloadDetailCallback sets the callback for navigating to workload detail view.
func (app *Application) SetWorkloadDetailCallback(callback func(kind, namespace, name string)) {
	app.workloadDetailCallback = callback
}

// NavigateToWorkloadDetail navigates to the workload detail view for the given workload.
func (app *Application) NavigateToWorkloadDetail(kind, namespace, name string) {
	resourceID := kind + "/" + namespace + "/" + name
	if stack := app.activeNavStack(); stack != nil {
		stack.Push(PageState{
			PageType:   PageWorkloadDetail,
			ResourceID: resourceID,
		})
	}

	if app.workloadDetailCallback != nil {
		app.workloadDetailCallback(kind, namespace, name)
	}

	app.syncTabBar()
	app.updateFooterContext()
}

// SetNodeDetailCallback sets the callback for navigating to node detail view
func (app *Application) SetNodeDetailCallback(callback func(nodeName string)) {
	app.nodeDetailCallback = callback
}

// SetPodDetailCallback sets the callback for navigating to pod detail view
func (app *Application) SetPodDetailCallback(callback func(namespace, podName string)) {
	app.podDetailCallback = callback
}

// NavigateToNodeDetail navigates to the node detail view for the given node
func (app *Application) NavigateToNodeDetail(nodeName string) {
	if stack := app.activeNavStack(); stack != nil {
		stack.Push(PageState{
			PageType:   PageNodeDetail,
			ResourceID: nodeName,
		})
	}

	if app.nodeDetailCallback != nil {
		app.nodeDetailCallback(nodeName)
	}

	app.syncTabBar()
	app.updateFooterContext()
}

// NavigateToPodDetail navigates to the pod detail view for the given pod
func (app *Application) NavigateToPodDetail(namespace, podName string) {
	resourceID := namespace + "/" + podName
	if stack := app.activeNavStack(); stack != nil {
		stack.Push(PageState{
			PageType:   PagePodDetail,
			ResourceID: resourceID,
		})
	}

	if app.podDetailCallback != nil {
		app.podDetailCallback(namespace, podName)
	}

	app.syncTabBar()
	app.updateFooterContext()
}

// SetContainerLogsCallback sets the callback for navigating to container logs view
func (app *Application) SetContainerLogsCallback(callback func(namespace, podName, containerName string)) {
	app.containerLogsCallback = callback
}

// NavigateToContainerLogs navigates to the container logs view
func (app *Application) NavigateToContainerLogs(namespace, podName, containerName string) {
	resourceID := namespace + "/" + podName + "/" + containerName
	if stack := app.activeNavStack(); stack != nil {
		stack.Push(PageState{
			PageType:   PageContainerLogs,
			ResourceID: resourceID,
		})
	}

	if app.containerLogsCallback != nil {
		app.containerLogsCallback(namespace, podName, containerName)
	}

	app.syncTabBar()
	app.updateFooterContext()
}

func (app *Application) presentDetailState(state *PageState) {
	if state == nil {
		return
	}
	switch state.PageType {
	case PageNodeDetail:
		if app.nodeDetailCallback != nil {
			app.nodeDetailCallback(state.ResourceID)
		}
	case PagePodDetail:
		parts := strings.SplitN(state.ResourceID, "/", 2)
		if len(parts) == 2 && app.podDetailCallback != nil {
			app.podDetailCallback(parts[0], parts[1])
		}
	case PageWorkloadDetail:
		parts := strings.SplitN(state.ResourceID, "/", 3)
		if len(parts) == 3 && app.workloadDetailCallback != nil {
			app.workloadDetailCallback(parts[0], parts[1], parts[2])
		}
	case PageContainerLogs:
		parts := strings.SplitN(state.ResourceID, "/", 3)
		if len(parts) == 3 && app.containerLogsCallback != nil {
			app.containerLogsCallback(parts[0], parts[1], parts[2])
		}
	}
}

// NavigateBack navigates back to the previous page on the active tab's stack.
func (app *Application) NavigateBack() bool {
	stack := app.activeNavStack()
	if stack == nil {
		return false
	}
	popped := stack.Pop()
	if popped == nil {
		return false // Already at root
	}

	current := stack.Current()
	if current == nil {
		return false
	}

	if !stack.CanGoBack() {
		app.fireDetailResetCallbacks()
		if app.panel != nil && app.panel.pages != nil {
			if s := app.activeSession(); s != nil && s.Descriptor.Title != "" {
				app.panel.pages.SwitchToPage(s.Descriptor.Title)
			} else if len(app.pages) > 0 {
				idx := app.visibleView
				if idx < 0 || idx >= len(app.pages) {
					idx = 0
				}
				app.panel.pages.SwitchToPage(app.pages[idx].Title)
			}
		}
		app.restoreActivePageFocus()
	} else {
		app.presentDetailState(current)
	}

	app.syncTabBar()
	app.updateFooterContext()

	return true
}

// GetNavigationStack returns the navigation stack for the active page.
func (app *Application) GetNavigationStack() *NavigationStack {
	return app.activeNavStack()
}

// IsInDetailView returns true if the active page has an open detail view on its stack.
func (app *Application) IsInDetailView() bool {
	stack := app.activeNavStack()
	return stack != nil && stack.CanGoBack()
}

// getActiveDetailPanel returns the currently active detail panel if any
func (app *Application) getActiveDetailPanel() ui.EscapablePanel {
	if active := app.activePagePanel(); active != nil {
		if provider, ok := active.(interface{ GetActiveDetailPanel() ui.EscapablePanel }); ok {
			if detail := provider.GetActiveDetailPanel(); detail != nil {
				return detail
			}
		}
	}
	if stack := app.activeNavStack(); stack != nil && stack.CanGoBack() {
		if current := stack.Current(); current != nil && current.PageType == PageWorkloadDetail {
			return nil
		}
	}
	for _, page := range app.pages {
		if provider, ok := page.Panel.(interface{ GetActiveDetailPanel() ui.EscapablePanel }); ok {
			if detail := provider.GetActiveDetailPanel(); detail != nil {
				return detail
			}
		}
	}
	return nil
}

// AddDetailPage adds a detail page to the application's pages widget
func (app *Application) AddDetailPage(name string, page tview.Primitive) {
	app.panel.addDetailPage(name, page)
}

// ShowDetailPage switches to a detail page
func (app *Application) ShowDetailPage(name string) {
	app.panel.showDetailPage(name)
}

// GetPagesWidget returns the inner pages widget (for detail views to add themselves)
func (app *Application) GetPagesWidget() *tview.Pages {
	return app.panel.getPagesWidget()
}

// getFocusedPanelName returns the name of the currently focused panel
func (app *Application) getFocusedPanelName() string {
	if app.tabIdx == -1 {
		return "header"
	}
	if s := app.activeSession(); s != nil && app.tabIdx >= 0 && app.tabIdx < len(s.Descriptor.ChildNames) {
		return s.Descriptor.ChildNames[app.tabIdx]
	}
	switch app.tabIdx {
	case 0:
		return "nodes"
	case 1:
		return "pods"
	default:
		return "nodes"
	}
}

const helpModalID = "help-modal"

// ShowHelpModal displays the keyboard shortcut help overlay modal
func (app *Application) ShowHelpModal() {
	if app.HasActiveModalPage() {
		return
	}

	pageName := "Overview"
	if s := app.activeSession(); s != nil && s.Descriptor.Title != "" {
		pageName = s.Descriptor.Title
	}
	focusedPanel := app.getFocusedPanelName()
	if stack := app.activeNavStack(); stack != nil && stack.CanGoBack() {
		if current := stack.Current(); current != nil {
			switch current.PageType {
			case PageNodeDetail:
				pageName = "Node Detail"
				focusedPanel = "node_detail"
			case PagePodDetail:
				pageName = "Pod Detail"
				focusedPanel = "pod_detail"
			case PageWorkloadDetail:
				pageName = "Workload Detail"
				focusedPanel = "pods"
				if detail := app.getActiveDetailPanel(); detail != nil {
					if fp, ok := detail.(interface{ GetFocusedPanelName() string }); ok {
						focusedPanel = fp.GetFocusedPanelName()
					}
				}
			case PageContainerLogs:
				pageName = "Container Logs"
				focusedPanel = "logs"
			case PageContainerSpec:
				pageName = "Container Spec"
				focusedPanel = "detail"
			}
		}
	}

	modal := ui.NewHelpModal(ui.HelpModalConfig{
		CurrentPage:  pageName,
		FocusedPanel: focusedPanel,
		PageTitles:   app.getPageTitles(),
		OnClose: func() {
			app.DismissModalPage(helpModalID)
		},
	})

	app.ShowModalPage(helpModalID, modal.GetRootView())
}

// isFilterEditing returns true if any panel or header is currently editing filter text
func (app *Application) isFilterEditing() bool {
	if app.panel != nil && app.panel.isNamespaceFilterEditing() {
		return true
	}
	if app.IsInDetailView() {
		if panel := app.getActiveDetailPanel(); panel != nil {
			if fe, ok := panel.(interface{ IsFilterEditing() bool }); ok {
				return fe.IsFilterEditing()
			}
		}
		return false
	}
	if activePanel := app.activePagePanel(); activePanel != nil {
		if fe, ok := activePanel.(interface{ IsFilterEditing() bool }); ok {
			return fe.IsFilterEditing()
		}
	}
	return false
}

// updateFooterContext updates the navigation footer based on current page and focus
func (app *Application) updateFooterContext() {
	if app.panel == nil {
		return
	}
	stack := app.activeNavStack()
	if stack == nil {
		return
	}
	current := stack.Current()
	if current == nil {
		return
	}

	var ctx ui.FooterContext
	pageCount := app.PageCount()

	if !stack.CanGoBack() {
		switch current.PageType {
		case PageOverview:
			ctx = ui.OverviewContext{
				FocusedPanel: app.getFocusedPanelName(),
				PageCount:    pageCount,
			}
		case PageWorkloads:
			ctx = ui.WorkloadsContext{
				FocusedPanel: app.getFocusedPanelName(),
				PageCount:    pageCount,
			}
		default:
			ctx = ui.PageContext{
				FocusedPanel: app.getFocusedPanelName(),
				PageCount:    pageCount,
			}
		}
		app.panel.setFooterContext(ctx)
		return
	}

	switch current.PageType {
	case PageNodeDetail:
		ctx = ui.NodeDetailContext{FocusedPanel: "events"}
	case PagePodDetail:
		ctx = ui.PodDetailContext{FocusedPanel: "events"}
	case PageWorkloadDetail:
		focused := "pods"
		if detail := app.getActiveDetailPanel(); detail != nil {
			if fp, ok := detail.(interface{ GetFocusedPanelName() string }); ok {
				focused = fp.GetFocusedPanelName()
			}
		}
		ctx = ui.WorkloadDetailContext{FocusedPanel: focused}
	case PageContainerLogs:
		ctx = ui.ContainerDetailContext{FocusedPanel: "logs"}
	default:
		ctx = ui.PageContext{
			FocusedPanel: app.getFocusedPanelName(),
			PageCount:    pageCount,
		}
	}

	app.panel.setFooterContext(ctx)
}

// SetFooterContext sets the footer context directly (for detail panels to update)
func (app *Application) SetFooterContext(ctx ui.FooterContext) {
	app.panel.setFooterContext(ctx)
}
