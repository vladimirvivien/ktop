package application

import (
	"fmt"
	"strings"

	"github.com/vladimirvivien/ktop/ui"
)

// PageDescriptor defines metadata and focusable child panels for a top-level TUI page.
type PageDescriptor struct {
	ID         string
	Title      string
	Key        rune
	RootType   PageType
	Panel      ui.PanelController
	ChildNames []string
}

// PageSession stores independent focus and drill-down history for a registered page.
type PageSession struct {
	Descriptor PageDescriptor
	TabIdx     int
	NavStack   *NavigationStack
}

// RegisterPage adds a top-level page and initializes an isolated navigation session for it.
func (app *Application) RegisterPage(desc PageDescriptor) {
	desc.Title = strings.TrimSpace(desc.Title)
	if desc.Title == "" && desc.Panel != nil {
		desc.Title = strings.TrimSpace(desc.Panel.GetTitle())
	}
	desc.ID = strings.ToLower(strings.TrimSpace(desc.ID))
	if desc.ID == "" {
		desc.ID = strings.ToLower(desc.Title)
	}
	if desc.Key == 0 && len(app.sessions) < 9 {
		desc.Key = rune('1' + len(app.sessions))
	}
	if desc.RootType == "" {
		desc.RootType = PageType(desc.ID)
	}
	if len(desc.ChildNames) == 0 && desc.ID == "overview" {
		desc.ChildNames = []string{"nodes", "pods"}
	}

	session := PageSession{
		Descriptor: desc,
		TabIdx:     -1,
		NavStack:   NewNavigationStackWithRoot(desc.RootType),
	}
	app.pages = append(app.pages, AppPage{Title: desc.Title, Panel: desc.Panel})
	app.sessions = append(app.sessions, session)
	if len(app.sessions) == 1 {
		if app.visibleView < 0 || app.visibleView >= len(app.sessions) {
			app.visibleView = 0
		}
		app.navStack = app.sessions[0].NavStack
	}
}

// PageCount returns the number of registered top-level pages.
func (app *Application) PageCount() int {
	return len(app.sessions)
}

// ActivePageIndex returns the index of the active top-level page.
func (app *Application) ActivePageIndex() int {
	return app.visibleView
}

// ActivePageID returns the ID of the active top-level page.
func (app *Application) ActivePageID() string {
	s := app.activeSession()
	if s == nil {
		return ""
	}
	return s.Descriptor.ID
}

// FindPageIndex looks up a registered page index by case-insensitive ID or title.
func (app *Application) FindPageIndex(idOrTitle string) (int, bool) {
	target := strings.TrimSpace(idOrTitle)
	if target == "" {
		return -1, false
	}
	for i, s := range app.sessions {
		if strings.EqualFold(s.Descriptor.ID, target) || strings.EqualFold(s.Descriptor.Title, target) {
			return i, true
		}
	}
	return -1, false
}

// SetInitialPage sets the starting page by ID or title before Run() starts the event loop.
func (app *Application) SetInitialPage(idOrTitle string) error {
	idx, ok := app.FindPageIndex(idOrTitle)
	if !ok {
		return fmt.Errorf("unknown page %q", idOrTitle)
	}
	app.visibleView = idx
	app.navStack = app.sessions[idx].NavStack
	app.tabIdx = app.sessions[idx].TabIdx
	return nil
}

// SwitchToPageIndex switches the active top-level page while preserving per-page state.
func (app *Application) SwitchToPageIndex(idx int) bool {
	if idx < 0 || idx >= len(app.sessions) || idx == app.visibleView {
		return false
	}

	if out := app.activeSession(); out != nil {
		out.TabIdx = app.tabIdx
	}

	app.visibleView = idx
	in := &app.sessions[idx]
	app.tabIdx = in.TabIdx
	app.navStack = in.NavStack

	if app.panel != nil {
		app.panel.setActiveTab(idx, in.Descriptor.Title)
	}
	app.syncTabBar()

	if in.NavStack != nil && in.NavStack.CanGoBack() {
		app.presentDetailState(in.NavStack.Current())
	} else {
		app.fireDetailResetCallbacks()
		if app.panel != nil && app.panel.pages != nil {
			app.panel.pages.SwitchToPage(in.Descriptor.Title)
		}
		app.restoreActivePageFocus()
	}

	app.updateFooterContext()
	app.Refresh()
	return true
}

// SwitchToPage switches to a registered page by ID or title.
func (app *Application) SwitchToPage(idOrTitle string) bool {
	idx, ok := app.FindPageIndex(idOrTitle)
	if !ok {
		return false
	}
	return app.SwitchToPageIndex(idx)
}

// NextPage advances to the next registered top-level page, wrapping around.
func (app *Application) NextPage() bool {
	if len(app.sessions) <= 1 {
		return false
	}
	next := (app.visibleView + 1) % len(app.sessions)
	return app.SwitchToPageIndex(next)
}

// PrevPage moves to the previous registered top-level page, wrapping around.
func (app *Application) PrevPage() bool {
	if len(app.sessions) <= 1 {
		return false
	}
	prev := (app.visibleView - 1 + len(app.sessions)) % len(app.sessions)
	return app.SwitchToPageIndex(prev)
}

func (app *Application) activeSession() *PageSession {
	if app.visibleView >= 0 && app.visibleView < len(app.sessions) {
		return &app.sessions[app.visibleView]
	}
	return nil
}

func (app *Application) activeNavStack() *NavigationStack {
	if s := app.activeSession(); s != nil && s.NavStack != nil {
		return s.NavStack
	}
	return app.navStack
}

func (app *Application) activePagePanel() ui.PanelController {
	if s := app.activeSession(); s != nil {
		return s.Descriptor.Panel
	}
	if app.visibleView >= 0 && app.visibleView < len(app.pages) {
		return app.pages[app.visibleView].Panel
	}
	if len(app.pages) > 0 {
		return app.pages[0].Panel
	}
	return nil
}
