package application

import (
	"context"
	"testing"

	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/ui"
)

type stubPanel struct {
	title    string
	root     tview.Primitive
	children []tview.Primitive
	filterOn bool
}

func newStubPanel(title string, childrenCount int) *stubPanel {
	flex := tview.NewFlex()
	children := make([]tview.Primitive, 0, childrenCount)
	for i := 0; i < childrenCount; i++ {
		box := tview.NewBox()
		children = append(children, box)
		flex.AddItem(box, 0, 1, i == 0)
	}
	return &stubPanel{
		title:    title,
		root:     flex,
		children: children,
	}
}

func (s *stubPanel) Layout(any)                          {}
func (s *stubPanel) DrawHeader(any)                      {}
func (s *stubPanel) DrawBody(any)                        {}
func (s *stubPanel) DrawFooter(any)                      {}
func (s *stubPanel) Clear()                              {}
func (s *stubPanel) GetTitle() string                    { return s.title }
func (s *stubPanel) GetRootView() tview.Primitive        { return s.root }
func (s *stubPanel) GetChildrenViews() []tview.Primitive { return s.children }
func (s *stubPanel) Run(context.Context) error           { return nil }
func (s *stubPanel) IsFilterEditing() bool               { return s.filterOn }

func TestNavigationStackWithRoot(t *testing.T) {
	stack := NewNavigationStackWithRoot(PageWorkloads)
	if stack.CanGoBack() {
		t.Fatal("expected root-only stack not to allow going back")
	}
	if got := stack.Current(); got == nil || got.PageType != PageWorkloads {
		t.Fatalf("expected root PageWorkloads, got %+v", got)
	}

	stack.Push(PageState{PageType: PagePodDetail, ResourceID: "default/api-0"})
	if !stack.CanGoBack() {
		t.Fatal("expected stack with detail view to allow going back")
	}
	if stack.Depth() != 2 {
		t.Fatalf("expected depth 2, got %d", stack.Depth())
	}

	stack.Clear()
	if stack.CanGoBack() || stack.Current().PageType != PageWorkloads {
		t.Fatalf("expected Clear() to reset to PageWorkloads, got %+v", stack.Current())
	}
}

func TestFormatBreadcrumb(t *testing.T) {
	tests := []struct {
		name   string
		title  string
		states []PageState
		want   string
	}{
		{
			name:   "root only returns empty",
			title:  "Overview",
			states: []PageState{{PageType: PageOverview}},
			want:   "",
		},
		{
			name:  "node detail",
			title: "Overview",
			states: []PageState{
				{PageType: PageOverview},
				{PageType: PageNodeDetail, ResourceID: "worker-1"},
			},
			want: "Overview › node/worker-1",
		},
		{
			name:  "node to pod detail",
			title: "Overview",
			states: []PageState{
				{PageType: PageOverview},
				{PageType: PageNodeDetail, ResourceID: "worker-1"},
				{PageType: PagePodDetail, ResourceID: "default/api-0"},
			},
			want: "Overview › node/worker-1 › pod/default/api-0",
		},
		{
			name:  "workload detail deployment",
			title: "Workloads",
			states: []PageState{
				{PageType: PageWorkloads},
				{PageType: PageWorkloadDetail, ResourceID: "Deployment/default/api"},
				{PageType: PagePodDetail, ResourceID: "default/api-0"},
			},
			want: "Workloads › deploy/default/api › pod/default/api-0",
		},
		{
			name:  "pod to container logs",
			title: "Workloads",
			states: []PageState{
				{PageType: PageWorkloads},
				{PageType: PagePodDetail, ResourceID: "prod/web-1"},
				{PageType: PageContainerLogs, ResourceID: "prod/web-1/nginx"},
			},
			want: "Workloads › pod/prod/web-1 › logs/nginx",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatBreadcrumb(tc.title, tc.states); got != tc.want {
				t.Fatalf("FormatBreadcrumb() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApplication_PageRegistryAndPerPageStackIsolation(t *testing.T) {
	app := New(nil, nil)

	overviewPanel := newStubPanel("Overview", 3)
	workloadsPanel := newStubPanel("Workloads", 2)
	eventsPanel := newStubPanel("Events", 1)

	app.RegisterPage(PageDescriptor{
		ID:         "overview",
		Title:      "Overview",
		Key:        '1',
		RootType:   PageOverview,
		Panel:      overviewPanel,
		ChildNames: []string{"summary", "nodes", "pods"},
	})
	app.RegisterPage(PageDescriptor{
		ID:         "workloads",
		Title:      "Workloads",
		Key:        '2',
		RootType:   PageWorkloads,
		Panel:      workloadsPanel,
		ChildNames: []string{"summary", "deployments"},
	})
	app.RegisterPage(PageDescriptor{
		ID:         "events",
		Title:      "Events",
		Key:        '3',
		RootType:   PageEvents,
		Panel:      eventsPanel,
		ChildNames: []string{"events"},
	})

	if app.PageCount() != 3 {
		t.Fatalf("expected 3 registered pages, got %d", app.PageCount())
	}
	if idx, ok := app.FindPageIndex("Workloads"); !ok || idx != 1 {
		t.Fatalf("FindPageIndex('Workloads') = (%d, %v), want (1, true)", idx, ok)
	}
	if err := app.SetInitialPage("nonexistent"); err == nil {
		t.Fatal("expected error for unknown initial page")
	}

	if err := app.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	var presented []string
	var resetCount int
	app.SetNodeDetailCallback(func(nodeName string) {
		presented = append(presented, "node:"+nodeName)
	})
	app.SetPodDetailCallback(func(ns, pod string) {
		presented = append(presented, "pod:"+ns+"/"+pod)
	})
	app.SetDetailResetCallback(func() {
		resetCount++
	})

	// Advance focus inside Overview to pods (index 2) and drill into Node -> Pod
	app.tabIdx = 2
	app.NavigateToNodeDetail("worker-1")
	app.NavigateToPodDetail("default", "api-0")
	if !app.IsInDetailView() {
		t.Fatal("expected Overview session to be in detail view")
	}
	if got := app.panel.tabBar.Breadcrumb(); got != "Overview › node/worker-1 › pod/default/api-0" {
		t.Fatalf("unexpected Overview breadcrumb: %q", got)
	}

	// Switch to Workloads (page 2): should clear detail view, show Workloads root, empty breadcrumb
	if !app.SwitchToPageIndex(1) {
		t.Fatal("expected SwitchToPageIndex(1) to succeed")
	}
	if app.ActivePageID() != "workloads" {
		t.Fatalf("expected active page 'workloads', got %q", app.ActivePageID())
	}
	if app.IsInDetailView() {
		t.Fatal("expected Workloads session to be at root")
	}
	if resetCount != 1 {
		t.Fatalf("expected detailResetCallback to run once, got %d", resetCount)
	}
	if got := app.panel.tabBar.Breadcrumb(); got != "" {
		t.Fatalf("expected empty breadcrumb on Workloads root, got %q", got)
	}

	// Focus child 1 on Workloads and wrap via NextPage/PrevPage
	app.tabIdx = 1
	if !app.NextPage() || app.ActivePageID() != "events" {
		t.Fatalf("expected NextPage() to advance to 'events', got %q", app.ActivePageID())
	}
	if !app.PrevPage() || app.ActivePageID() != "workloads" {
		t.Fatalf("expected PrevPage() to return to 'workloads', got %q", app.ActivePageID())
	}
	if app.tabIdx != 1 {
		t.Fatalf("expected Workloads tabIdx=1 preserved, got %d", app.tabIdx)
	}

	// Switch back to Overview: Pod Detail and breadcrumb must be restored intact
	if !app.SwitchToPage("overview") {
		t.Fatal("expected SwitchToPage('overview') to succeed")
	}
	if !app.IsInDetailView() {
		t.Fatal("expected Overview detail view preserved after tab switch")
	}
	if got := app.panel.tabBar.Breadcrumb(); got != "Overview › node/worker-1 › pod/default/api-0" {
		t.Fatalf("expected restored Overview breadcrumb, got %q", got)
	}
	if got := presented[len(presented)-1]; got != "pod:default/api-0" {
		t.Fatalf("expected Pod Detail re-presented on return, last presented=%q", got)
	}

	// Unwind Overview stack cleanly with NavigateBack
	if !app.NavigateBack() {
		t.Fatal("expected first NavigateBack to return to Node Detail")
	}
	if got := presented[len(presented)-1]; got != "node:worker-1" {
		t.Fatalf("expected Node Detail on unwind, got %q", got)
	}
	if !app.NavigateBack() {
		t.Fatal("expected second NavigateBack to return to Overview root")
	}
	if app.IsInDetailView() {
		t.Fatal("expected Overview stack back at root")
	}
	if app.tabIdx != 2 {
		t.Fatalf("expected Overview tabIdx=2 preserved across detail drill-down, got %d", app.tabIdx)
	}
	if _, ok := app.panel.footerComponent.GetView().(*tview.Flex); !ok {
		t.Fatal("expected footer component view")
	}
	_ = ui.TabItem{}
}
