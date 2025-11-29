package application

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/buildinfo"
	"github.com/vladimirvivien/ktop/ui"
)

var (
	buttonUnselectedBgColor = tcell.ColorPaleGreen
	buttonUnselectedFgColor = tcell.ColorDarkBlue
	buttonSelectedBgColor   = tcell.ColorBlue
	buttonSelectedFgColor   = tcell.ColorWhite
)

type appPanel struct {
	tviewApp *tview.Application
	title    string
	header   *tview.Table
	pages    *tview.Pages
	footer   *tview.Table
	modals   []tview.Primitive
	root     *tview.Pages // CHANGED: from *tview.Flex to *tview.Pages

	// Toast tracking
	currentToastID      string
	toastMutex          sync.Mutex
	toastButtonCallback ui.ToastCallback // Callback for toast button presses

	// Namespace filter
	namespaceFilter         *ui.FilterState
	namespaceFilterCallback func(namespace string) // Callback when namespace filter changes
	headerFocused           bool                   // Whether header panel has focus

	// Focus restoration callback - called after toast is dismissed
	focusRestorationCallback func()
}

func newPanel(app *tview.Application) *appPanel {
	p := &appPanel{
		title:           "ktop",
		tviewApp:        app,
		namespaceFilter: &ui.FilterState{},
	}
	return p
}

func (p *appPanel) GetTitle() string {
	return p.title
}

func (p *appPanel) Layout(data interface{}) {
	p.header = tview.NewTable()
	p.header.SetBorder(false)
	p.header.SetBorders(false)

	p.header.SetBorder(true)
	p.pages = tview.NewPages()
	p.footer = tview.NewTable()
	p.footer.SetBorder(true)

	// Existing layout
	mainLayout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.header, 3, 1, true). // header - focusable for input handling
		AddItem(p.pages, 0, 1, true)   // body
		// TODO show footer when multi-page is implemented
		//AddItem(p.footer, 3, 1, false)  // footer

	// NEW: Wrap in Pages for toast layering
	p.root = tview.NewPages()
	p.root.AddPage("main", mainLayout, true, true)

	p.tviewApp.SetRoot(p.root, true)

	// add pages
	pages, ok := data.([]AppPage)
	if !ok {
		panic(fmt.Sprintf("application.Layout got unexpected data type: %T", data))
	}

	// setup page and page buttons in footer
	for i, page := range pages {
		p.pages.AddPage(page.Title, page.Panel.GetRootView(), true, false)
		p.footer.SetCell(0, i,
			&tview.TableCell{
				Text:            fmt.Sprintf("  %s (F%d)  ", page.Title, i+1),
				Color:           buttonUnselectedFgColor,
				Align:           tview.AlignCenter,
				BackgroundColor: buttonUnselectedBgColor,
				Expansion:       0,
			},
		)
	}
}

func (p *appPanel) DrawHeader(data interface{}) {
	header, ok := data.(string)
	if !ok {
		panic(fmt.Sprintf("application.Drawheader got unexpected type %T", data))
	}

	// Header string already includes namespace filter state (handled by getNamespaceDisplay)
	p.header.SetCell(
		0, 0,
		tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignLeft).
			SetExpansion(100),
	)

	p.header.SetCell(
		0, 1,
		tview.NewTableCell("ktop: "+buildinfo.Version).
			SetTextColor(tcell.ColorWhite).
			SetAlign(tview.AlignRight).
			SetExpansion(100),
	)
}

func (p *appPanel) DrawBody(data interface{}) {}

func (p *appPanel) DrawFooter(data interface{}) {
	title, ok := data.(string)
	if !ok {
		panic(fmt.Sprintf("application.DrawBody got unexpected data type: %T", data))
	}
	p.switchToPage(title)
}

func (p *appPanel) Clear() {}

func (p *appPanel) GetRootView() tview.Primitive {
	//return p.pages
	return p.root
}

func (p *appPanel) GetChildrenViews() []tview.Primitive {
	return []tview.Primitive{p.header, p.pages, p.footer}
}

func (p *appPanel) switchToPage(title string) {

	row := 0
	cols := p.footer.GetColumnCount()

	for i := 0; i < cols; i++ {
		cell := p.footer.GetCell(row, i)
		if strings.HasPrefix(strings.TrimSpace(cell.Text), title) {
			cell.SetTextColor(buttonSelectedFgColor)
			cell.SetBackgroundColor(buttonSelectedBgColor)
		} else {
			cell.SetTextColor(buttonUnselectedFgColor)
			cell.SetBackgroundColor(buttonUnselectedBgColor)
		}
	}
	p.pages.SwitchToPage(title)
}

func (p *appPanel) showModalView(t tview.Primitive) {
	p.tviewApp.SetRoot(t, false)
}

// setToastButtonCallback sets the callback for toast button presses
func (p *appPanel) setToastButtonCallback(callback ui.ToastCallback) {
	p.toastButtonCallback = callback
}

// showToast displays a toast notification with auto-dismiss (no buttons)
func (p *appPanel) showToast(message string, level ui.ToastLevel, duration time.Duration) string {
	return p.showToastWithButtons(message, level, duration, nil)
}

// showToastWithButtons displays a toast notification with buttons
func (p *appPanel) showToastWithButtons(message string, level ui.ToastLevel, duration time.Duration, buttons []string) string {
	p.toastMutex.Lock()
	defer p.toastMutex.Unlock()

	// Dismiss current toast (replace old with new)
	if p.currentToastID != "" {
		p.root.RemovePage(p.currentToastID)
	}

	toastID := fmt.Sprintf("toast-%d", time.Now().UnixNano())

	// Create callback that dismisses toast and calls user callback
	// Run in goroutine to avoid blocking the modal's event handler
	wrappedCallback := func(buttonLabel string) {
		go func() {
			// Dismiss toast first
			p.tviewApp.QueueUpdateDraw(func() {
				p.dismissToastInternal(toastID)
			})
			// Then call the user callback
			if p.toastButtonCallback != nil {
				p.toastButtonCallback(buttonLabel)
			}
		}()
	}

	var toast *tview.Modal
	if len(buttons) > 0 {
		// Create toast with buttons
		toast = ui.NewToastWithButtons(message, level, buttons, wrappedCallback)
	} else {
		// Create toast without buttons but with ESC handler for quitting
		toast = ui.NewToastWithEscHandler(message, level, wrappedCallback)
	}

	// Add to pages with unique ID
	p.root.AddPage(toastID, toast, true, true)
	p.currentToastID = toastID

	// Ensure the modal has focus so it can receive key input
	p.tviewApp.SetFocus(toast)

	// Auto-dismiss after duration (0 = no timeout, manual dismiss only)
	if duration > 0 {
		go func() {
			time.Sleep(duration)
			p.tviewApp.QueueUpdateDraw(func() {
				p.dismissToastInternal(toastID)
			})
		}()
	}

	return toastID
}

// dismissToast removes a toast notification by ID (public, acquires lock)
func (p *appPanel) dismissToast(toastID string) {
	p.toastMutex.Lock()
	defer p.toastMutex.Unlock()
	p.dismissToastInternal(toastID)
}

// dismissToastInternal removes a toast notification by ID (internal, no lock)
// Must be called from within QueueUpdateDraw or with toastMutex held
func (p *appPanel) dismissToastInternal(toastID string) {
	if p.currentToastID == toastID {
		p.root.RemovePage(toastID)
		p.currentToastID = ""
		// Switch back to main page
		p.root.SwitchToPage("main")
		// Call focus restoration callback to restore proper panel focus
		// This avoids tview's auto-focus propagation which would focus the first child
		if p.focusRestorationCallback != nil {
			p.focusRestorationCallback()
		} else {
			// Fallback: focus root (may trigger undesired focus propagation)
			p.tviewApp.SetFocus(p.root)
		}
	}
}

// setFocusRestorationCallback sets the callback to restore focus after toast dismissal
func (p *appPanel) setFocusRestorationCallback(callback func()) {
	p.focusRestorationCallback = callback
}

// hasActiveToast returns true if a toast is currently displayed
func (p *appPanel) hasActiveToast() bool {
	p.toastMutex.Lock()
	defer p.toastMutex.Unlock()
	return p.currentToastID != ""
}

// setHeaderFocused updates the header panel's visual focus state
func (p *appPanel) setHeaderFocused(focused bool) {
	p.headerFocused = focused
	if focused {
		p.header.SetBorderColor(ui.FocusBorderColor())
	} else {
		p.header.SetBorderColor(ui.UnfocusBorderColor())
	}
}

// isHeaderFocused returns whether the header has focus
func (p *appPanel) isHeaderFocused() bool {
	return p.headerFocused
}

// getHeader returns the header table primitive for focus management
func (p *appPanel) getHeader() tview.Primitive {
	return p.header
}

// setNamespaceFilterCallback sets the callback for namespace filter changes
func (p *appPanel) setNamespaceFilterCallback(callback func(namespace string)) {
	p.namespaceFilterCallback = callback
}

// getNamespaceFilter returns the current namespace filter text
func (p *appPanel) getNamespaceFilter() string {
	return p.namespaceFilter.Text
}

// isNamespaceFilterEditing returns whether the namespace filter is in edit mode
func (p *appPanel) isNamespaceFilterEditing() bool {
	return p.namespaceFilter.Editing
}

// handleHeaderKey processes keyboard input when header is focused
// Returns true if the key was handled
func (p *appPanel) handleHeaderKey(event *tcell.EventKey) bool {
	// Handle namespace filter edit mode
	if p.namespaceFilter.Editing {
		switch event.Key() {
		case tcell.KeyEscape:
			p.namespaceFilter.Cancel()
			return true
		case tcell.KeyEnter:
			p.namespaceFilter.Confirm()
			// Notify callback of filter change
			if p.namespaceFilterCallback != nil {
				p.namespaceFilterCallback(p.namespaceFilter.Text)
			}
			return true
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			if p.namespaceFilter.HandleBackspace() {
				// Live update: notify callback as user types
				if p.namespaceFilterCallback != nil {
					p.namespaceFilterCallback(p.namespaceFilter.Text)
				}
			}
			return true
		case tcell.KeyRune:
			p.namespaceFilter.AppendChar(event.Rune())
			// Live update: notify callback as user types
			if p.namespaceFilterCallback != nil {
				p.namespaceFilterCallback(p.namespaceFilter.Text)
			}
			return true
		}
		return true // Consume all keys in edit mode
	}

	// Normal mode - '/' starts namespace filter editing
	if event.Key() == tcell.KeyRune && event.Rune() == '/' {
		p.namespaceFilter.StartEditing()
		return true
	}

	return false
}

// hasEscapableHeaderState returns true if header has state that ESC should clear
func (p *appPanel) hasEscapableHeaderState() bool {
	return p.namespaceFilter.HasEscapableState()
}

// handleHeaderEscape handles ESC when header is focused
func (p *appPanel) handleHeaderEscape() bool {
	if p.namespaceFilter.Editing {
		p.namespaceFilter.Cancel()
		return true
	}
	if p.namespaceFilter.Active {
		p.namespaceFilter.Clear()
		// Notify callback of filter cleared
		if p.namespaceFilterCallback != nil {
			p.namespaceFilterCallback("")
		}
		return true
	}
	return false
}
