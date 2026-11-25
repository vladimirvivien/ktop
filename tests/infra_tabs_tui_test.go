package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/collector"
	"github.com/vladimirvivien/ktop/internal/snapshot"
	"github.com/vladimirvivien/ktop/output"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	"github.com/vladimirvivien/ktop/views/network"
	"github.com/vladimirvivien/ktop/views/overview"
	"github.com/vladimirvivien/ktop/views/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMultiPage_FiveTabsStorageAndNetworkNavigation(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	app := application.New(nil, nil)
	overviewPanel := overview.New(app, "Overview")
	workloadsPanel := newTestPagePanel("Workloads", "WORKLOADS_PAGE_BODY")
	eventsPanel := newTestPagePanel("Events", "EVENTS_PAGE_BODY")
	storagePanel := storage.New(app, "Storage")
	networkPanel := network.New(app, "Network")

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
	app.RegisterPage(application.PageDescriptor{
		ID:         "storage",
		Title:      "Storage",
		Key:        '4',
		RootType:   application.PageStorage,
		Panel:      storagePanel,
		ChildNames: []string{"pvcs", "pvs"},
	})
	app.RegisterPage(application.PageDescriptor{
		ID:         "network",
		Title:      "Network",
		Key:        '5',
		RootType:   application.PageNetwork,
		Panel:      networkPanel,
		ChildNames: []string{"services", "ingresses"},
	})

	if err := app.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	now := metav1.NewTime(time.Now().UTC().Add(-1 * time.Hour))
	storagePanel.DrawBody(model.StorageSnapshot{
		Summary: model.StorageSummary{PVCsBound: 1, PVCsTotal: 1, PVsBound: 1, PVsTotal: 1, StorageClasses: 1},
		PVCs: []model.PVCModel{
			{Namespace: "prod", Name: "pg-data", Status: "Bound", CapacityBytes: 1024, CreationTime: now, Age: "1h"},
		},
		PVs: []model.PVModel{
			{Name: "pv-prod-1", Status: "Bound", CapacityBytes: 1024, ClaimRef: "prod/pg-data", CreationTime: now, Age: "1h"},
		},
	})
	networkPanel.DrawBody(model.NetworkSnapshot{
		Summary: model.NetworkSummary{ServicesTotal: 1, ZeroEndpointServices: 1, IngressesTotal: 1, HasMetrics: true},
		Services: []model.ServiceModel{
			{Namespace: "prod", Name: "api-svc", Type: "ClusterIP", ClusterIP: "10.0.0.5", ReadyEndpoints: 0, TotalEndpoints: 2, HasEndpoints: true, CreationTime: now, Age: "1h"},
		},
		Ingresses: []model.IngressModel{
			{Namespace: "prod", Name: "edge", ClassName: "nginx", Hosts: []string{"api.example.com"}, Ports: "80, 443", CreationTime: now, Age: "1h"},
		},
	})

	ts, err := ui.NewTestScreen(170, 42)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(app.RootPrimitive())
	initial := ts.Text()
	for _, want := range []string{"[1] Overview", "[2] Workloads", "[3] Events", "[4] Storage", "[5] Network", "[1-5] page"} {
		if !strings.Contains(initial, want) {
			t.Fatalf("expected %q on initial 5-tab screen, got:\n%s", want, initial)
		}
	}

	sendAppKey(ts, app, tcell.KeyRune, '4', tcell.ModNone)
	if app.ActivePageID() != "storage" {
		t.Fatalf("expected ActivePageID() = 'storage', got %q", app.ActivePageID())
	}
	for _, want := range []string{"PersistentVolumeClaims", "pg-data", "pv-prod-1"} {
		if !strings.Contains(ts.Text(), want) {
			t.Fatalf("expected %q on Storage tab, got:\n%s", want, ts.Text())
		}
	}

	sendAppKey(ts, app, tcell.KeyRune, '5', tcell.ModNone)
	if app.ActivePageID() != "network" {
		t.Fatalf("expected ActivePageID() = 'network', got %q", app.ActivePageID())
	}
	for _, want := range []string{"Services", "api-svc", "0/2", "Ingresses", "api.example.com"} {
		if !strings.Contains(ts.Text(), want) {
			t.Fatalf("expected %q on Network tab, got:\n%s", want, ts.Text())
		}
	}

	sendAppKey(ts, app, tcell.KeyRune, '>', tcell.ModShift)
	if app.ActivePageID() != "overview" {
		t.Fatalf("expected '>' to wrap from network to overview, got %q", app.ActivePageID())
	}

	sendAppKey(ts, app, tcell.KeyRune, '<', tcell.ModShift)
	if app.ActivePageID() != "network" {
		t.Fatalf("expected '<' to wrap from overview to network, got %q", app.ActivePageID())
	}
}

func TestNoUIJSONOutput_IncludesStorageAndNetworkSections(t *testing.T) {
	agg := &collector.Aggregate{
		Metadata: collector.ReportMetadata{KtopVersion: "v0.7.9", SampleCount: 1},
		Cluster:  snapshot.ClusterInfo{Context: "test", MetricsSource: "prometheus", Ready: true},
		Storage: &snapshot.StorageData{
			Summary: snapshot.StorageSummaryData{PVCsBound: 1, PVCsTotal: 1, PVsBound: 1, PVsTotal: 1, StorageClasses: 1},
			PVCs:    []snapshot.PVCData{{Namespace: "prod", Name: "pg-data", Status: "Bound", CapacityBytes: 1024, FillPct: 50, HasUsage: true}},
			PVs:     []snapshot.PVData{{Name: "pv-1", Status: "Bound", CapacityBytes: 1024, ClaimRef: "prod/pg-data"}},
		},
		Network: &snapshot.NetworkData{
			Summary:  snapshot.NetworkSummaryData{ServicesTotal: 1, ZeroEndpointServices: 1, IngressesTotal: 1, HasMetrics: true},
			Services: []snapshot.ServiceData{{Namespace: "prod", Name: "api", Type: "ClusterIP", ReadyEndpoints: 0, TotalEndpoints: 1, HasEndpoints: true, HasEndpointAlert: true}},
			Ingresses: []snapshot.IngressData{
				{Namespace: "prod", Name: "edge", ClassName: "nginx", Hosts: []string{"api.example.com"}, Ports: "80, 443"},
			},
		},
	}

	formatter, err := output.NewFormatter("json")
	if err != nil {
		t.Fatalf("NewFormatter failed: %v", err)
	}
	var buf bytes.Buffer
	if err := formatter.WriteFinal(&buf, agg); err != nil {
		t.Fatalf("WriteFinal failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON emitted: %v\n%s", err, buf.String())
	}
	if _, ok := decoded["storage"]; !ok {
		t.Fatalf("expected 'storage' key in JSON output:\n%s", buf.String())
	}
	if _, ok := decoded["network"]; !ok {
		t.Fatalf("expected 'network' key in JSON output:\n%s", buf.String())
	}
}
