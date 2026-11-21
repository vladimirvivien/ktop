package workloads

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestWorkloadsPage_RendersTablesSummaryAndDetail(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	app := application.New(nil, nil)
	page := New(app, "Workloads")
	app.RegisterPage(application.PageDescriptor{
		ID:         "workloads",
		Title:      "Workloads",
		Key:        '1',
		RootType:   application.PageWorkloads,
		Panel:      page,
		ChildNames: []string{"deployments", "statefulsets", "daemonsets", "jobs"},
	})
	if err := app.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	snap := model.WorkloadsSnapshot{
		HasMetrics: true,
		Deployments: []model.DeploymentModel{
			{
				Namespace: "default", Name: "web", Desired: 3, Ready: 3, Updated: 3, Available: 3, Age: "2d", Status: "Ready",
				Usage:      model.WorkloadUsage{HasCPU: true, CPU: resource.NewMilliQuantity(250, resource.DecimalSI), HasMem: true, Mem: resource.NewQuantity(128*1024*1024, resource.BinarySI)},
				Efficiency: model.WorkloadEfficiency{HasEfficiency: true, EffPct: 72, HasCPUEfficiency: true, CPUEffPct: 60, HasMemEfficiency: true, MemEffPct: 72},
			},
			{
				Namespace: "prod", Name: "api", Desired: 2, Ready: 1, Updated: 2, Available: 1, Age: "5h", Status: "Progressing",
				Usage:      model.WorkloadUsage{HasCPU: true, CPU: resource.NewMilliQuantity(80, resource.DecimalSI), HasMem: true, Mem: resource.NewQuantity(64*1024*1024, resource.BinarySI)},
				Efficiency: model.WorkloadEfficiency{HasEfficiency: true, EffPct: 18},
			},
		},
		StatefulSets: []model.StatefulSetModel{
			{
				Namespace: "prod", Name: "pg", Desired: 3, Ready: 3, Age: "10d", Status: "Ready",
				Usage: model.WorkloadUsage{HasCPU: true, CPU: resource.NewMilliQuantity(500, resource.DecimalSI), HasMem: true, Mem: resource.NewQuantity(512*1024*1024, resource.BinarySI)},
			},
		},
		DaemonSets: []model.DaemonSetModel{
			{
				Namespace: "kube-system", Name: "proxy", Desired: 3, Current: 3, Ready: 3, Age: "20d", Status: "Ready",
				Usage: model.WorkloadUsage{HasCPU: true, CPU: resource.NewMilliQuantity(45, resource.DecimalSI), HasMem: true, Mem: resource.NewQuantity(32*1024*1024, resource.BinarySI)},
			},
		},
		Jobs: []model.JobModel{
			{Namespace: "batch", Name: "nightly", CompletionsDesired: 1, HasCompletions: true, Succeeded: 1, Age: "1h", Status: "Complete"},
		},
	}

	page.applySnapshot(snap)

	ts, err := ui.NewTestScreen(160, 48)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(app.RootPrimitive())
	text := ts.Text()
	for _, want := range []string{
		"Summary",
		"1/2 ready",
		"Deployments", "web", "250m", "128Mi", "api", "80m", "64Mi",
		"StatefulSets", "pg", "500m", "512Mi",
		"DaemonSets", "proxy", "3/3", "45m", "32Mi",
		"Jobs", "nightly", "Complete",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q on Workloads screen, got:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"Workload Summary", "UP ", "AVAIL", "DESIRED", "CURRENT", "EFF"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("did not expect %q on Workloads screen, got:\n%s", unwanted, text)
		}
	}

	// Verify Summary renders as a single line without a Namespace field.
	var foundSingleLineSummary bool
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "Deployments:") &&
			strings.Contains(line, "StatefulSets:") &&
			strings.Contains(line, "DaemonSets:") &&
			strings.Contains(line, "Jobs:") {
			foundSingleLineSummary = true
			if strings.Contains(line, "Namespace:") {
				t.Fatalf("expected Namespace omitted from Workloads Summary line, got %q", line)
			}
		}
	}
	if !foundSingleLineSummary {
		t.Fatalf("expected single-line Workloads Summary containing Deployments, StatefulSets, DaemonSets, and Jobs, got:\n%s", text)
	}

	// Verify 2-column layout places Deployments alongside StatefulSets and DaemonSets alongside Jobs.
	var foundTopRow, foundBottomRow bool
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "Deployments") && strings.Contains(line, "StatefulSets") {
			foundTopRow = true
		}
		if strings.Contains(line, "DaemonSets") && strings.Contains(line, "Jobs") {
			foundBottomRow = true
		}
	}
	if !foundTopRow || !foundBottomRow {
		t.Fatalf("expected 2-column equal grid layout (foundTopRow=%v, foundBottomRow=%v), got:\n%s", foundTopRow, foundBottomRow, text)
	}

	// Verify namespace filtering updates visible rows and summary counts.
	page.SetNamespaceFilter("prod")
	ts.Draw(app.RootPrimitive())
	filteredText := ts.Text()
	if strings.Contains(filteredText, "nightly") {
		t.Fatalf("expected batch/nightly excluded under prod namespace filter, got:\n%s", filteredText)
	}
	if !strings.Contains(filteredText, "api") || !strings.Contains(filteredText, "0/1 ready") {
		t.Fatalf("expected prod workload counts after filter, got:\n%s", filteredText)
	}

	// Verify DetailPanel renders kind-specific title, reworked Info box, Deployment Detail, owned pods, and events.
	detail := NewDetailPanel()
	detail.SetFocused(true)
	detail.InitFocus()
	var yamlOpened bool
	detail.SetOnViewYAML(func(d *model.WorkloadDetailData) {
		if d != nil && d.Name == "web" {
			yamlOpened = true
		}
	})
	detail.DrawBody(&model.WorkloadDetailData{
		Kind:               model.WorkloadKindDeployment,
		Namespace:          "default",
		Name:               "web",
		Status:             "Ready",
		Desired:            3,
		Ready:              3,
		Updated:            3,
		Available:          3,
		Strategy:           "RollingUpdate",
		StrategyDetail:     "RollingUpdate (25% surge / 25% unavail)",
		Revision:           "4",
		ActiveReplicaSet:   "web-7d9f8c",
		Images:             []string{"nginx:1.27"},
		Age:                "2d",
		Generation:         4,
		ObservedGeneration: 4,
		Efficiency:         snap.Deployments[0].Efficiency,
		Selector:           map[string]string{"app": "web"},
		Labels:             map[string]string{"tier": "frontend"},
		TotalUsageCPU:      resource.NewMilliQuantity(250, resource.DecimalSI),
		TotalRequestedCPU:  resource.NewMilliQuantity(500, resource.DecimalSI),
		TotalLimitCPU:      resource.NewMilliQuantity(1000, resource.DecimalSI),
		TotalUsageMem:      resource.NewQuantity(128*1024*1024, resource.BinarySI),
		TotalRequestedMem:  resource.NewQuantity(256*1024*1024, resource.BinarySI),
		OwnedPods: []*model.PodModel{
			{Namespace: "default", Name: "web-pod-1", Node: "worker-1", Status: "Running", ReadyContainers: 1, TotalContainers: 1, TimeSince: "2d"},
		},
		Events: []corev1.Event{
			{Reason: "ScalingReplicaSet", Type: "Normal", Message: "Scaled up replica set web-1 to 3", LastTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
		},
	})
	ts.Draw(detail.GetRootView())
	detailText := ts.Text()
	for _, want := range []string{
		"Deployments > web",
		"Deployment Detail",
		"Desired: 3",
		"Ready: 3",
		"Updated: 3",
		"Available: 3",
		"RollingUpdate (25% surge / 25% unavail)",
		"#4 (web-7d9f8c)",
		"nginx:1.27",
		"250m used / 500m req / 1000m lim",
		"web-pod-1",
		"worker-1",
		"ScalingReplicaSet",
	} {
		if !strings.Contains(detailText, want) {
			t.Fatalf("expected %q in Deployment Detail view, got:\n%s", want, detailText)
		}
	}
	if strings.Contains(detailText, "Workload Detail") || strings.Contains(detailText, "Workloads >") {
		t.Fatalf("did not expect generic Workload Detail heading, got:\n%s", detailText)
	}

	// Verify Enter while Deployment Detail is active triggers YAML viewer callback.
	if handler := detail.leftDetailTable.GetInputCapture(); handler != nil {
		handler(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	}
	if !yamlOpened {
		t.Fatal("expected Enter on active Deployment Detail to invoke YAML modal callback")
	}
}

func TestWorkloadsPage_ColorConsistencyWithOverview(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})
	rt, err := ui.ResolveAndApplyTheme("dark", "truecolor")
	if err != nil {
		t.Fatalf("ResolveAndApplyTheme failed: %v", err)
	}
	pal := rt.Palette

	readyRowColor := ui.GetRowColorForStatus("Ready", "workload")
	readyDep := model.DeploymentModel{
		Namespace: "default", Name: "web", Desired: 2, Ready: 2, Age: "2d", Status: "Ready",
		Usage: model.WorkloadUsage{
			HasCPU: true, CPU: resource.NewMilliQuantity(120, resource.DecimalSI),
			HasMem: true, Mem: resource.NewQuantity(64*1024*1024, resource.BinarySI),
		},
	}

	if got := renderDeploymentCell(readyDep, "READY", readyRowColor).Color; got != pal.DataPrimary {
		t.Fatalf("READY cell color = %v, want DataPrimary (%v)", got, pal.DataPrimary)
	}
	if got := renderDeploymentCell(readyDep, "AGE", readyRowColor).Color; got != pal.DataPrimary {
		t.Fatalf("AGE cell color = %v, want DataPrimary (%v)", got, pal.DataPrimary)
	}
	if got := renderDeploymentCell(readyDep, "CPU", readyRowColor).Color; got != pal.DataSecondary {
		t.Fatalf("CPU cell color = %v, want DataSecondary (%v)", got, pal.DataSecondary)
	}
	if got := renderDeploymentCell(readyDep, "MEM", readyRowColor).Color; got != pal.DataSecondary {
		t.Fatalf("MEM cell color = %v, want DataSecondary (%v)", got, pal.DataSecondary)
	}
	if got := renderDeploymentCell(readyDep, "STATUS", readyRowColor).Color; got != pal.StatusOK {
		t.Fatalf("STATUS cell color = %v, want StatusOK (%v)", got, pal.StatusOK)
	}

	// Scaled 0 recedes to muted gray instead of warning yellow.
	if sev := ui.GetStatusSeverity("Scaled 0", "workload"); sev != ui.StatusSeverityUnknown {
		t.Fatalf("Scaled 0 severity = %v, want StatusSeverityUnknown", sev)
	}
	if got := ui.GetRowColorForStatus("Scaled 0", "workload"); got != pal.StatusMuted {
		t.Fatalf("Scaled 0 row color = %v, want StatusMuted (%v)", got, pal.StatusMuted)
	}

	// Running Job AGE matches its row color (StatusInfo) instead of muted gray.
	runningRowColor := ui.GetRowColorForStatus("Running", "workload")
	runningJob := model.JobModel{Namespace: "batch", Name: "etl", Age: "30s", Status: "Running"}
	if got := renderJobCell(runningJob, "AGE", runningRowColor).Color; got != runningRowColor {
		t.Fatalf("Running Job AGE color = %v, want rowColor (%v)", got, runningRowColor)
	}

	// Summary labels use DataPrimary matching Overview Cluster Summary.
	sp := newSummaryPanel(" Summary ")
	sp.DrawBody(model.WorkloadSummaryModel{DeploymentsReady: 2, DeploymentsTotal: 2, JobsRunning: 1, JobsCompleted: 1})
	ts, err := ui.NewTestScreen(120, 3)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)
	ts.Draw(sp.GetRootView())
	cells, w, _ := ts.Screen().GetContents()
	row1 := ts.Lines()[1]
	depIdx := strings.Index(row1, "Deployments:")
	if depIdx < 0 {
		t.Fatalf("missing Deployments: label in summary row %q", row1)
	}
	labelFG, _, _ := cells[w+depIdx].Style.Decompose()
	if labelFG != pal.DataPrimary {
		t.Fatalf("Summary label color = %v, want DataPrimary (%v)", labelFG, pal.DataPrimary)
	}
}
