package events

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

func sampleEventsSnapshot(now time.Time) model.EventsSnapshot {
	evts := []model.EventModel{
		{
			UID:               "u1",
			Namespace:         "prod",
			Name:              "api-0.warn1",
			Type:              "Warning",
			Reason:            "BackOff",
			Message:           "Back-off restarting failed container app in pod api-0_prod",
			InvolvedKind:      "Pod",
			InvolvedNamespace: "prod",
			InvolvedName:      "api-0",
			Source:            "kubelet (node-1)",
			Count:             7,
			FirstSeen:         now.Add(-15 * time.Minute),
			LastSeen:          now.Add(-20 * time.Second),
		},
		{
			UID:               "u2",
			Namespace:         "default",
			Name:              "cfg.norm1",
			Type:              "Normal",
			Reason:            "ConfigReloaded",
			Message:           "Configuration checksum updated cleanly",
			InvolvedKind:      "ConfigMap",
			InvolvedNamespace: "default",
			InvolvedName:      "app-config",
			Source:            "config-controller",
			Count:             1,
			FirstSeen:         now.Add(-5 * time.Minute),
			LastSeen:          now.Add(-2 * time.Minute),
		},
	}
	return model.EventsSnapshot{
		Summary: model.ComputeEventsSummary(evts),
		Events:  evts,
	}
}

func TestEventsPage_RenderFilterTogglesAndModal(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	app := application.New(nil, nil)
	page := New(app, "Events")
	app.RegisterPage(application.PageDescriptor{
		ID:         "events",
		Title:      "Events",
		Key:        '1',
		RootType:   application.PageEvents,
		Panel:      page,
		ChildNames: []string{"events"},
	})
	if err := app.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	now := time.Now().UTC()
	snap := sampleEventsSnapshot(now)
	page.applySnapshot(snap)

	ts, err := ui.NewTestScreen(150, 36)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(app.RootPrimitive())
	text := ts.Text()
	for _, want := range []string{
		"Summary",
		"Events: 2",
		"Warnings: 1",
		"Normal: 1",
		"Top: BackOff (1)",
		"ALL · LIVE",
		"Pod/api-0",
		"ConfigMap/app-config",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q on Events screen, got:\n%s", want, text)
		}
	}

	// Toggle Warnings-only via 'w'.
	page.handleTableInput(tcell.NewEventKey(tcell.KeyRune, 'w', tcell.ModNone))
	if !page.WarningsOnly() {
		t.Fatal("expected WarningsOnly true after pressing 'w'")
	}
	if len(page.displayed) != 1 || page.displayed[0].Reason != "BackOff" {
		t.Fatalf("expected only warning event displayed, got %+v", page.displayed)
	}
	ts.Draw(app.RootPrimitive())
	if !strings.Contains(ts.Text(), "WARNINGS ONLY · LIVE") {
		t.Fatalf("expected WARNINGS ONLY badge, got:\n%s", ts.Text())
	}

	// Toggle back to All.
	page.handleTableInput(tcell.NewEventKey(tcell.KeyRune, 'w', tcell.ModNone))
	if page.WarningsOnly() || len(page.displayed) != 2 {
		t.Fatalf("expected WarningsOnly false and 2 events, got warn=%v len=%d", page.WarningsOnly(), len(page.displayed))
	}

	// Verify '/' filter editing suppresses 'w' and 'f' toggles.
	page.handleTableInput(tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone))
	page.handleTableInput(tcell.NewEventKey(tcell.KeyRune, 'w', tcell.ModNone))
	page.handleTableInput(tcell.NewEventKey(tcell.KeyRune, 'f', tcell.ModNone))
	if page.WarningsOnly() || page.Paused() {
		t.Fatalf("expected w/f ignored during filter editing, got warn=%v paused=%v", page.WarningsOnly(), page.Paused())
	}
	page.HandleEscape()
	page.HandleEscape()

	// Pause via 'f', feed a new event, verify table stays frozen while summary tracks +1 new.
	page.handleTableInput(tcell.NewEventKey(tcell.KeyRune, 'f', tcell.ModNone))
	if !page.Paused() {
		t.Fatal("expected Paused true after pressing 'f'")
	}
	next := snap
	next.Events = append([]model.EventModel{
		{
			UID:               "u3",
			Namespace:         "prod",
			Name:              "api-0.warn2",
			Type:              "Warning",
			Reason:            "FailedScheduling",
			Message:           "0/3 nodes available",
			InvolvedKind:      "Pod",
			InvolvedNamespace: "prod",
			InvolvedName:      "api-1",
			Count:             1,
			LastSeen:          now,
		},
	}, snap.Events...)
	next.Summary = model.ComputeEventsSummary(next.Events)
	page.applySnapshot(next)

	if len(page.displayed) != 2 {
		t.Fatalf("expected displayed rows frozen at 2 while paused, got %d", len(page.displayed))
	}
	ts.Draw(app.RootPrimitive())
	if !strings.Contains(ts.Text(), "PAUSED (+1 new)") {
		t.Fatalf("expected PAUSED (+1 new) badge, got:\n%s", ts.Text())
	}

	// Unpause via 'f' and verify buffered snapshot applies immediately.
	page.handleTableInput(tcell.NewEventKey(tcell.KeyRune, 'f', tcell.ModNone))
	if page.Paused() || len(page.displayed) != 3 {
		t.Fatalf("expected live mode with 3 displayed events after unpause, got paused=%v len=%d", page.Paused(), len(page.displayed))
	}

	// Verify namespace filter synchronization.
	page.SetNamespaceFilter("default")
	if len(page.displayed) != 1 || page.displayed[0].InvolvedName != "app-config" {
		t.Fatalf("expected namespace filter 'default' to leave only app-config, got %+v", page.displayed)
	}
	page.SetNamespaceFilter("")

	// Verify Event Detail modal shows [g] Inspect for Pod and omits it for ConfigMap.
	podModal := NewDetailModal(DetailModalConfig{Event: snap.Events[0]})
	if !podModal.CanJump() {
		t.Fatal("expected Pod event modal to allow jump")
	}
	ts.Draw(podModal.GetRootView())
	podModalText := ts.Text()
	for _, want := range []string{"Event Detail: Warning / BackOff", "Back-off restarting failed container", "Inspect Pod"} {
		if !strings.Contains(podModalText, want) {
			t.Fatalf("expected %q in Pod Event Detail modal, got:\n%s", want, podModalText)
		}
	}

	cmModal := NewDetailModal(DetailModalConfig{Event: snap.Events[1]})
	if cmModal.CanJump() {
		t.Fatal("expected ConfigMap event modal to disallow jump")
	}
	ts.Draw(cmModal.GetRootView())
	if strings.Contains(ts.Text(), "Inspect ConfigMap") {
		t.Fatalf("did not expect Inspect hint for ConfigMap modal, got:\n%s", ts.Text())
	}
}

func TestApplication_JumpToObjectCrossTab(t *testing.T) {
	app := application.New(nil, nil)

	overviewDummy := New(app, "Overview")
	workloadsDummy := New(app, "Workloads")
	eventsPage := New(app, "Events")

	app.RegisterPage(application.PageDescriptor{ID: "overview", Title: "Overview", Key: '1', RootType: application.PageOverview, Panel: overviewDummy, ChildNames: []string{"nodes", "pods"}})
	app.RegisterPage(application.PageDescriptor{ID: "workloads", Title: "Workloads", Key: '2', RootType: application.PageWorkloads, Panel: workloadsDummy, ChildNames: []string{"deployments"}})
	app.RegisterPage(application.PageDescriptor{ID: "events", Title: "Events", Key: '3', RootType: application.PageEvents, Panel: eventsPage, ChildNames: []string{"events"}})
	if err := app.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	var gotPod, gotNode, gotWL string
	app.SetPodDetailCallback(func(ns, name string) { gotPod = ns + "/" + name })
	app.SetNodeDetailCallback(func(name string) { gotNode = name })
	app.SetWorkloadDetailCallback(func(kind, ns, name string) { gotWL = kind + "/" + ns + "/" + name })

	// Start on Events tab (index 2).
	app.SwitchToPageIndex(2)
	if app.ActivePageID() != "events" {
		t.Fatalf("expected active page events, got %s", app.ActivePageID())
	}

	// Pod jump opens modal in place without leaving Events tab.
	if ok, reason := app.JumpToObject("Pod", "prod", "api-0"); !ok {
		t.Fatalf("expected Pod jump ok, got reason=%q", reason)
	}
	if gotPod != "prod/api-0" || app.ActivePageID() != "events" {
		t.Fatalf("expected Pod modal over events tab, gotPod=%q page=%q", gotPod, app.ActivePageID())
	}

	// Node jump switches to Overview tab and opens Node Detail.
	if ok, reason := app.JumpToObject("Node", "", "worker-1"); !ok {
		t.Fatalf("expected Node jump ok, got reason=%q", reason)
	}
	if gotNode != "worker-1" || app.ActivePageID() != "overview" {
		t.Fatalf("expected Node detail on overview tab, gotNode=%q page=%q", gotNode, app.ActivePageID())
	}

	// Deployment jump switches to Workloads tab and opens Workload Detail.
	app.SwitchToPageIndex(2)
	if ok, reason := app.JumpToObject("Deployment", "prod", "api"); !ok {
		t.Fatalf("expected Deployment jump ok, got reason=%q", reason)
	}
	if gotWL != "Deployment/prod/api" || app.ActivePageID() != "workloads" {
		t.Fatalf("expected Deployment detail on workloads tab, gotWL=%q page=%q", gotWL, app.ActivePageID())
	}

	// Unsupported kind returns false with message.
	if ok, reason := app.JumpToObject("ConfigMap", "default", "cfg"); ok || reason == "" {
		t.Fatalf("expected ConfigMap jump false with reason, got ok=%v reason=%q", ok, reason)
	}
}
