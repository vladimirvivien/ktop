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
	tabIdx        int // -1 = header, 0+ = children panels
	visibleView   int
	panel         *appPanel
	refreshQ      chan struct{}
	stopCh        chan struct{}

	// Navigation stack for detail view navigation
	navStack *NavigationStack

	// Detail view callbacks
	nodeDetailCallback     func(nodeName string)
	podDetailCallback      func(namespace, podName string)
	containerLogsCallback  func(namespace, podName, containerName string)

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

	// Namespace filter callback for pod filtering
	namespaceFilterCallback func(namespace string)

	// Quit confirmation state (double-ESC to quit from Overview)
	pendingQuit     bool
	pendingQuitTime time.Time
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

// QueueUpdateDraw safely queues a UI update function to run on the main goroutine.
// Use this when updating UI from background goroutines (e.g., controller callbacks).
// The function will be executed and followed by a screen redraw.
func (app *Application) QueueUpdateDraw(f func()) {
	app.tviewApp.QueueUpdateDraw(f)
}

// SetNamespaceFilterCallback sets the callback for namespace filter changes
func (app *Application) SetNamespaceFilterCallback(callback func(namespace string)) {
	app.namespaceFilterCallback = callback
	// Also set it on the panel so it can notify when user types
	app.panel.setNamespaceFilterCallback(callback)
}

// GetNamespaceFilter returns the current namespace filter text
func (app *Application) GetNamespaceFilter() string {
	return app.panel.getNamespaceFilter()
}

// updatePanelFocus updates focus state for all child panels based on tabIdx
func (app *Application) updatePanelFocus(views []tview.Primitive) {
	// Get the main panel to access child panels with FocusablePanel interface
	if len(app.pages) == 0 {
		return
	}

	mainPanel := app.pages[0].Panel
	children := mainPanel.GetChildrenViews()

	for i, child := range children {
		// Try to get the underlying panel that implements FocusablePanel
		// We need to check if the panel (not the view) implements the interface
		if focusable, ok := app.getPanelForView(child).(ui.FocusablePanel); ok {
			focusable.SetFocused(i == app.tabIdx)
		}
	}
}

// getPanelForView returns the panel for a given view (helper for focus management)
// This is a workaround since we store views but need access to panel methods
func (app *Application) getPanelForView(view tview.Primitive) interface{} {
	// The views are children of the main panel, so we need to match them
	// to the actual panel objects. For now, we'll use a simple approach
	// since the MainPanel stores references to its child panels.
	if len(app.pages) == 0 {
		return nil
	}
	mainPanel := app.pages[0].Panel
	children := mainPanel.GetChildrenViews()

	for i, child := range children {
		if child == view {
			// Return the panel that owns this view
			// This requires MainPanel to expose its child panels
			if provider, ok := mainPanel.(interface{ GetChildPanel(int) ui.Panel }); ok {
				return provider.GetChildPanel(i)
			}
		}
	}
	return nil
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

	// Draw initial header using shared helper functions
	nsDisplay := app.getNamespaceDisplay()
	app.panel.DrawHeader(app.buildHeaderString(nsDisplay))

	app.panel.DrawFooter(app.getPageTitles()[app.visibleView])

	// Set initial focus to header panel and unfocus all child panels
	app.panel.setHeaderFocused(true)
	if len(app.pages) > 0 {
		views := app.pages[0].Panel.GetChildrenViews()
		app.updatePanelFocus(views) // With tabIdx=-1, this unfocuses all children
	}
	// Explicitly focus header to prevent tview from auto-focusing first child
	app.Focus(app.panel.getHeader())

	// Set initial footer context for Overview page with header focused
	app.updateFooterContext()

	// Set up focus restoration callback for toast dismissal
	// This ensures proper focus is restored based on tabIdx after toast goes away
	app.panel.setFocusRestorationCallback(func() {
		views := app.pages[0].Panel.GetChildrenViews()
		// Restore focus based on current tabIdx
		if app.tabIdx == -1 {
			// Header was focused - must explicitly focus header to prevent
			// tview from auto-focusing the first focusable child (cluster summary)
			app.panel.setHeaderFocused(true)
			app.updatePanelFocus(views) // Ensure children are unfocused
			app.tviewApp.SetFocus(app.panel.getHeader())
		} else if app.tabIdx >= 0 && app.tabIdx < len(views) {
			// A child panel was focused
			app.panel.setHeaderFocused(false)
			app.updatePanelFocus(views)
			app.tviewApp.SetFocus(views[app.tabIdx])
		}
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

	app.tviewApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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

		// Reset pending quit state on any non-ESC key
		if event.Key() != tcell.KeyEsc && app.pendingQuit {
			app.pendingQuit = false
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
			// Check if the currently visible panel has escapable state
			if app.visibleView >= 0 && app.visibleView < len(app.pages) {
				if escapable, ok := app.pages[app.visibleView].Panel.(ui.EscapablePanel); ok {
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
			// Check if we're on a detail page - if so, let the detail panel handle Tab
			if frontPage, _ := app.panel.pages.GetFrontPage(); frontPage != "" {
				// Detail pages are named "node_detail", "pod_detail", etc.
				// Overview pages are named "Overview", etc.
				if frontPage == "node_detail" || frontPage == "pod_detail" {
					// Pass Tab through to the detail panel
					return event
				}
			}

			views := app.pages[0].Panel.GetChildrenViews()
			// Cycle: -1 (header) -> 0 -> 1 -> 2 -> -1 (header) ...
			// -2 = no focus (initial state)
			if event.Key() == tcell.KeyTAB {
				app.tabIdx++
				if app.tabIdx >= len(views) {
					app.tabIdx = -1 // Back to header
				}
			} else {
				// Backtab - cycle in reverse
				app.tabIdx--
				if app.tabIdx < -1 {
					app.tabIdx = len(views) - 1
				}
			}

			// Update focus visuals for all panels
			app.updatePanelFocus(views)

			// Set tview focus
			if app.tabIdx == -1 {
				// Header focused - must explicitly focus header to prevent
				// tview from auto-focusing the first focusable child
				app.panel.setHeaderFocused(true)
				app.Focus(app.panel.getHeader())
			} else {
				app.panel.setHeaderFocused(false)
				app.Focus(views[app.tabIdx])
			}
			// Update footer context to reflect new focus
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
	const requiredConsecOK = 2            // Require 2 consecutive successes
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
			app.lastHealthyState = false
		}
	}
}

// buildHeaderString constructs the header text with current metrics status
// namespaceDisplay is the namespace value to show (may include filter styling)
func (app *Application) buildHeaderString(namespaceDisplay string) string {
	var hdr strings.Builder
	// Format: Context: name | K8s: version | User: user | Namespace: value | Metrics: status
	hdr.WriteString("[green]Context: [white]%s [green]| K8s: [white]%s [green]| User: [white]%s [green]| Namespace: [white]%s [green]| Metrics:")

	// Check MetricsSource health
	if app.metricsSource != nil && app.metricsSource.IsHealthy() {
		sourceInfo := app.metricsSource.GetSourceInfo()
		hdr.WriteString(fmt.Sprintf(" [white]%s", sourceInfo.Type))
	} else {
		hdr.WriteString(" [red]not connected")
	}

	client := app.GetK8sClient()

	// Truncate long values to prevent header overflow
	context := truncateString(client.ClusterName(), 25)
	user := truncateString(client.Username(), 20)
	ns := truncateString(namespaceDisplay, 16)

	return fmt.Sprintf(
		hdr.String(),
		context, client.GetServerVersion(), user, ns,
	)
}

// truncateString truncates a string for header display
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// getNamespaceDisplay returns the namespace display string based on filter state
func (app *Application) getNamespaceDisplay() string {
	// Check if filter is editing or active
	if app.panel.isNamespaceFilterEditing() {
		// Show filter input with cursor
		filterText := app.panel.getNamespaceFilter()
		if filterText == "" {
			return "[yellow]▌[-]"
		}
		return fmt.Sprintf("[yellow]%s▌[-]", filterText)
	}

	// Check if filter is active (has confirmed filter text)
	filterText := app.panel.getNamespaceFilter()
	if filterText != "" {
		return filterText // White to match header style
	}

	// No filter - show actual namespace
	namespace := app.k8sClient.Namespace()
	if namespace == k8s.AllNamespaces {
		return "[orange](all)[-]"
	}
	return namespace
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
	// Push current state to navigation stack
	app.navStack.Push(PageState{
		PageType:   PageNodeDetail,
		ResourceID: nodeName,
	})

	// Call the callback to show the detail view
	if app.nodeDetailCallback != nil {
		app.nodeDetailCallback(nodeName)
	}

	// Update footer context for detail page
	app.updateFooterContext()
}

// NavigateToPodDetail navigates to the pod detail view for the given pod
func (app *Application) NavigateToPodDetail(namespace, podName string) {
	// Push current state to navigation stack
	resourceID := namespace + "/" + podName
	app.navStack.Push(PageState{
		PageType:   PagePodDetail,
		ResourceID: resourceID,
	})

	// Call the callback to show the detail view
	if app.podDetailCallback != nil {
		app.podDetailCallback(namespace, podName)
	}

	// Update footer context for detail page
	app.updateFooterContext()
}

// SetContainerLogsCallback sets the callback for navigating to container logs view
func (app *Application) SetContainerLogsCallback(callback func(namespace, podName, containerName string)) {
	app.containerLogsCallback = callback
}

// NavigateToContainerLogs navigates to the container logs view
func (app *Application) NavigateToContainerLogs(namespace, podName, containerName string) {
	// Push current state to navigation stack
	resourceID := namespace + "/" + podName + "/" + containerName
	app.navStack.Push(PageState{
		PageType:   PageContainerLogs,
		ResourceID: resourceID,
	})

	// Call the callback to show the logs view
	if app.containerLogsCallback != nil {
		app.containerLogsCallback(namespace, podName, containerName)
	}

	// Update footer context for detail page
	app.updateFooterContext()
}

// NavigateBack navigates back to the previous page
func (app *Application) NavigateBack() bool {
	popped := app.navStack.Pop()
	if popped == nil {
		return false // Already at root
	}

	// Determine what page we're now on
	current := app.navStack.Current()
	if current == nil {
		return false
	}

	switch current.PageType {
	case PageOverview:
		// Switch back to the main overview page
		app.panel.pages.SwitchToPage(app.pages[0].Title)
		// Reset focus to the overview panel
		if len(app.pages) > 0 {
			views := app.pages[0].Panel.GetChildrenViews()
			if len(views) > 0 && app.tabIdx >= 0 && app.tabIdx < len(views) {
				app.tviewApp.SetFocus(views[app.tabIdx])
			}
		}
	case PageNodeDetail:
		// Navigate back to node detail (for nested navigation)
		if app.nodeDetailCallback != nil {
			app.nodeDetailCallback(current.ResourceID)
		}
	case PagePodDetail:
		// Navigate back to pod detail (for nested navigation)
		parts := strings.SplitN(current.ResourceID, "/", 2)
		if len(parts) == 2 && app.podDetailCallback != nil {
			app.podDetailCallback(parts[0], parts[1])
		}
	}

	// Update footer context for the page we navigated back to
	app.updateFooterContext()

	return true
}

// GetNavigationStack returns the navigation stack for breadcrumb display
func (app *Application) GetNavigationStack() *NavigationStack {
	return app.navStack
}

// IsInDetailView returns true if currently viewing a detail page
func (app *Application) IsInDetailView() bool {
	current := app.navStack.Current()
	return current != nil && current.PageType != PageOverview
}

// getActiveDetailPanel returns the currently active detail panel if any
func (app *Application) getActiveDetailPanel() ui.EscapablePanel {
	// MainPanel is always the first page
	if len(app.pages) > 0 {
		if provider, ok := app.pages[0].Panel.(interface{ GetActiveDetailPanel() ui.EscapablePanel }); ok {
			return provider.GetActiveDetailPanel()
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
	// Map tabIdx to panel names based on Overview page structure
	// Order: summary (0), nodes (1), pods (2)
	switch app.tabIdx {
	case 0:
		return "summary"
	case 1:
		return "nodes"
	case 2:
		return "pods"
	default:
		return "summary"
	}
}

// updateFooterContext updates the navigation footer based on current page and focus
func (app *Application) updateFooterContext() {
	current := app.navStack.Current()
	if current == nil {
		return
	}

	var ctx ui.FooterContext

	switch current.PageType {
	case PageOverview:
		ctx = ui.OverviewContext{FocusedPanel: app.getFocusedPanelName()}
	case PageNodeDetail:
		ctx = ui.NodeDetailContext{FocusedPanel: "events"}
	case PagePodDetail:
		ctx = ui.PodDetailContext{FocusedPanel: "events"}
	case PageContainerLogs:
		ctx = ui.ContainerDetailContext{FocusedPanel: "logs"}
	default:
		ctx = ui.OverviewContext{FocusedPanel: app.getFocusedPanelName()}
	}

	app.panel.setFooterContext(ctx)
}

// SetFooterContext sets the footer context directly (for detail panels to update)
func (app *Application) SetFooterContext(ctx ui.FooterContext) {
	app.panel.setFooterContext(ctx)
}

