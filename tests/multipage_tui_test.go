package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/overview"
)

type testPagePanel struct {
	title    string
	root     *tview.Flex
	body     *tview.TextView
	children []tview.Primitive
}

func newTestPagePanel(title, bodyText string) *testPagePanel {
	tv := tview.NewTextView().SetText(bodyText)
	tv.SetBorder(true).SetTitle(" " + title + " ")
	flex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tv, 0, 1, true)
	return &testPagePanel{
		title:    title,
		root:     flex,
		body:     tv,
		children: []tview.Primitive{tv},
	}
}

func (p *testPagePanel) Layout(any)                          {}
func (p *testPagePanel) DrawHeader(any)                      {}
func (p *testPagePanel) DrawBody(any)                        {}
func (p *testPagePanel) DrawFooter(any)                      {}
func (p *testPagePanel) Clear()                              {}
func (p *testPagePanel) GetTitle() string                    { return p.title }
func (p *testPagePanel) GetRootView() tview.Primitive        { return p.root }
func (p *testPagePanel) GetChildrenViews() []tview.Primitive { return p.children }
func (p *testPagePanel) Run(context.Context) error           { return nil }

func sendAppKey(ts *ui.TestScreen, app *application.Application, key tcell.Key, ch rune, mod tcell.ModMask) {
	ev := tcell.NewEventKey(key, ch, mod)
	rem := app.HandleInput(ev)
	if rem != nil {
		if focused := app.FocusedPrimitive(); focused != nil {
			if handler := focused.InputHandler(); handler != nil {
				handler(rem, func(p tview.Primitive) {
					if p != nil {
						app.Focus(p)
					}
				})
			}
		}
	}
	ts.Draw(app.RootPrimitive())
}

func TestMultiPage_TabSwitchingAndBracketCycling(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	app := application.New(nil, nil)
	overviewPanel := overview.New(app, "Overview")
	workloadsPanel := newTestPagePanel("Workloads", "WORKLOADS_PAGE_BODY")
	eventsPanel := newTestPagePanel("Events", "EVENTS_PAGE_BODY")

	app.RegisterPage(application.PageDescriptor{
		ID:         "overview",
		Title:      "Overview",
		Key:        '1',
		RootType:   application.PageOverview,
		Panel:      overviewPanel,
		ChildNames: []string{"summary", "nodes", "pods"},
	})
	app.RegisterPage(application.PageDescriptor{
		ID:         "workloads",
		Title:      "Workloads",
		Key:        '2',
		RootType:   application.PageWorkloads,
		Panel:      workloadsPanel,
		ChildNames: []string{"workloads"},
	})
	app.RegisterPage(application.PageDescriptor{
		ID:         "events",
		Title:      "Events",
		Key:        '3',
		RootType:   application.PageEvents,
		Panel:      eventsPanel,
		ChildNames: []string{"events"},
	})

	if err := app.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	ts, err := ui.NewTestScreen(160, 45)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(app.RootPrimitive())
	initial := ts.Text()
	for _, want := range []string{"[1] Overview", "[2] Workloads", "[3] Events", "[1-3] page"} {
		if !strings.Contains(initial, want) {
			t.Fatalf("expected %q on initial multi-page screen, got:\n%s", want, initial)
		}
	}

	sendAppKey(ts, app, tcell.KeyRune, '2', tcell.ModNone)
	if app.ActivePageID() != "workloads" {
		t.Fatalf("expected ActivePageID() = 'workloads', got %q", app.ActivePageID())
	}
	if !strings.Contains(ts.Text(), "WORKLOADS_PAGE_BODY") {
		t.Fatalf("expected Workloads body after pressing '2', got:\n%s", ts.Text())
	}

	sendAppKey(ts, app, tcell.KeyRune, '3', tcell.ModNone)
	if app.ActivePageID() != "events" {
		t.Fatalf("expected ActivePageID() = 'events', got %q", app.ActivePageID())
	}
	if !strings.Contains(ts.Text(), "EVENTS_PAGE_BODY") {
		t.Fatalf("expected Events body after pressing '3', got:\n%s", ts.Text())
	}

	sendAppKey(ts, app, tcell.KeyRune, ']', tcell.ModNone)
	if app.ActivePageID() != "overview" {
		t.Fatalf("expected ']' to wrap to 'overview', got %q", app.ActivePageID())
	}

	sendAppKey(ts, app, tcell.KeyRune, '[', tcell.ModNone)
	if app.ActivePageID() != "events" {
		t.Fatalf("expected '[' to wrap to 'events', got %q", app.ActivePageID())
	}
}

func TestMultiPage_PerPageStackIsolationAndEscUnwind(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	app := application.New(nil, nil)
	overviewPanel := newTestPagePanel("Overview", "OVERVIEW_ROOT_BODY")
	workloadsPanel := newTestPagePanel("Workloads", "WORKLOADS_ROOT_BODY")

	app.RegisterPage(application.PageDescriptor{
		ID:         "overview",
		Title:      "Overview",
		Key:        '1',
		RootType:   application.PageOverview,
		Panel:      overviewPanel,
		ChildNames: []string{"overview"},
	})
	app.RegisterPage(application.PageDescriptor{
		ID:         "workloads",
		Title:      "Workloads",
		Key:        '2',
		RootType:   application.PageWorkloads,
		Panel:      workloadsPanel,
		ChildNames: []string{"workloads"},
	})

	if err := app.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	nodeDetailView := tview.NewTextView().SetText("NODE_DETAIL_BODY: worker-1")
	podDetailView := tview.NewTextView().SetText("POD_DETAIL_BODY: default/api-0")
	app.AddDetailPage("node_detail", nodeDetailView)
	app.AddDetailPage("pod_detail", podDetailView)

	activeDetail := ""
	app.SetNodeDetailCallback(func(nodeName string) {
		activeDetail = "node:" + nodeName
		nodeDetailView.SetText("NODE_DETAIL_BODY: " + nodeName)
		app.ShowDetailPage("node_detail")
		app.Focus(nodeDetailView)
	})
	app.SetPodDetailCallback(func(ns, podName string) {
		activeDetail = "pod:" + ns + "/" + podName
		podDetailView.SetText("POD_DETAIL_BODY: " + ns + "/" + podName)
		app.ShowDetailPage("pod_detail")
		app.Focus(podDetailView)
	})
	app.SetDetailResetCallback(func() {
		activeDetail = ""
	})

	ts, err := ui.NewTestScreen(160, 45)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	app.NavigateToNodeDetail("worker-1")
	ts.Draw(app.RootPrimitive())
	screenNode1 := ts.Text()
	if !strings.Contains(screenNode1, "NODE_DETAIL_BODY: worker-1") || !strings.Contains(screenNode1, "Overview › node/worker-1") {
		t.Fatalf("expected Node Detail and breadcrumb on Tab 1, got:\n%s", screenNode1)
	}

	// Switch to Tab 2: root Workloads renders with no Overview breadcrumb.
	sendAppKey(ts, app, tcell.KeyRune, '2', tcell.ModNone)
	screenTab2 := ts.Text()
	if !strings.Contains(screenTab2, "WORKLOADS_ROOT_BODY") {
		t.Fatalf("expected Workloads root body on Tab 2, got:\n%s", screenTab2)
	}
	if strings.Contains(screenTab2, "Overview › node/worker-1") {
		t.Fatalf("expected Tab 2 breadcrumb cleared, got:\n%s", screenTab2)
	}

	// Switch back to Tab 1: Node Detail and breadcrumb must be restored intact.
	sendAppKey(ts, app, tcell.KeyRune, '1', tcell.ModNone)
	screenRestored := ts.Text()
	if activeDetail != "node:worker-1" || !strings.Contains(screenRestored, "NODE_DETAIL_BODY: worker-1") || !strings.Contains(screenRestored, "Overview › node/worker-1") {
		t.Fatalf("expected Node Detail and breadcrumb restored on return to Tab 1, activeDetail=%q, got:\n%s", activeDetail, screenRestored)
	}

	// Unwind depth-1 stack via ESC: Node Detail -> Overview root.
	sendAppKey(ts, app, tcell.KeyEsc, 0, tcell.ModNone)
	if app.IsInDetailView() {
		t.Fatal("expected ESC to unwind back to Overview root")
	}
	screenRoot := ts.Text()
	if !strings.Contains(screenRoot, "OVERVIEW_ROOT_BODY") {
		t.Fatalf("expected Overview root body after ESC, got:\n%s", screenRoot)
	}
}

func TestMultiPage_FilterInputPassthrough(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	app := application.New(nil, nil)
	app.RegisterPage(application.PageDescriptor{
		ID:       "overview",
		Title:    "Overview",
		Key:      '1',
		RootType: application.PageOverview,
		Panel:    newTestPagePanel("Overview", "OVERVIEW_ROOT_BODY"),
	})
	app.RegisterPage(application.PageDescriptor{
		ID:       "workloads",
		Title:    "Workloads",
		Key:      '2',
		RootType: application.PageWorkloads,
		Panel:    newTestPagePanel("Workloads", "WORKLOADS_ROOT_BODY"),
	})

	if err := app.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	ts, err := ui.NewTestScreen(160, 45)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)
	ts.Draw(app.RootPrimitive())

	// Initial focus is on Header (-1). Enter namespace filter edit mode with '/'.
	sendAppKey(ts, app, tcell.KeyRune, '/', tcell.ModNone)
	for _, r := range "21[]" {
		sendAppKey(ts, app, tcell.KeyRune, r, tcell.ModNone)
	}
	sendAppKey(ts, app, tcell.KeyEnter, 0, tcell.ModNone)

	if app.ActivePageID() != "overview" {
		t.Fatalf("expected digit/bracket keys during filter editing not to switch pages, active=%q", app.ActivePageID())
	}
	if got := app.GetNamespaceFilter(); got != "21[]" {
		t.Fatalf("expected namespace filter '21[]', got %q", got)
	}
}

func TestMultiPage_InitialPageAndHelpModal(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	app := application.New(nil, nil)
	app.RegisterPage(application.PageDescriptor{
		ID:       "overview",
		Title:    "Overview",
		Key:      '1',
		RootType: application.PageOverview,
		Panel:    newTestPagePanel("Overview", "OVERVIEW_ROOT_BODY"),
	})
	app.RegisterPage(application.PageDescriptor{
		ID:       "workloads",
		Title:    "Workloads",
		Key:      '2',
		RootType: application.PageWorkloads,
		Panel:    newTestPagePanel("Workloads", "WORKLOADS_ROOT_BODY"),
	})
	app.RegisterPage(application.PageDescriptor{
		ID:       "events",
		Title:    "Events",
		Key:      '3',
		RootType: application.PageEvents,
		Panel:    newTestPagePanel("Events", "EVENTS_ROOT_BODY"),
	})

	if err := app.SetInitialPage("workloads"); err != nil {
		t.Fatalf("SetInitialPage failed: %v", err)
	}
	if err := app.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	ts, err := ui.NewTestScreen(160, 45)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)
	ts.Draw(app.RootPrimitive())

	if app.ActivePageID() != "workloads" {
		t.Fatalf("expected initial active page 'workloads', got %q", app.ActivePageID())
	}
	if !strings.Contains(ts.Text(), "WORKLOADS_ROOT_BODY") {
		t.Fatalf("expected Workloads body rendered at startup, got:\n%s", ts.Text())
	}

	sendAppKey(ts, app, tcell.KeyRune, '?', tcell.ModNone)
	if !app.HasActiveModalPage() {
		t.Fatal("expected '?' to open HelpModal")
	}
	helpScreen := ts.Text()
	for _, want := range []string{"Keyboard Shortcuts — Workloads", "1-3", "Overview, Workloads, Events"} {
		if !strings.Contains(helpScreen, want) {
			t.Fatalf("expected %q in HelpModal, got:\n%s", want, helpScreen)
		}
	}

	sendAppKey(ts, app, tcell.KeyEsc, 0, tcell.ModNone)
	if app.HasActiveModalPage() {
		t.Fatal("expected ESC to dismiss HelpModal")
	}
}
