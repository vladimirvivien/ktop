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
				Efficiency: model.WorkloadEfficiency{HasEfficiency: true, EffPct: 72, HasCPUEfficiency: true, CPUEffPct: 60, HasMemEfficiency: true, MemEffPct: 72},
			},
			{
				Namespace: "prod", Name: "api", Desired: 2, Ready: 1, Updated: 2, Available: 1, Age: "5h", Status: "Progressing",
				Efficiency: model.WorkloadEfficiency{HasEfficiency: true, EffPct: 18},
			},
		},
		StatefulSets: []model.StatefulSetModel{
			{Namespace: "prod", Name: "pg", Desired: 3, Ready: 3, Age: "10d", Status: "Ready"},
		},
		DaemonSets: []model.DaemonSetModel{
			{Namespace: "kube-system", Name: "proxy", Desired: 3, Current: 3, Ready: 3, Age: "20d", Status: "Ready"},
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
		"Deployments", "web", "72%", "api", "18%",
		"StatefulSets", "pg",
		"DaemonSets", "proxy",
		"Jobs", "nightly", "Complete",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q on Workloads screen, got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Workload Summary") {
		t.Fatalf("expected Summary title instead of Workload Summary, got:\n%s", text)
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

	// Verify DetailPanel renders workload details, owned pods, and events.
	detail := NewDetailPanel()
	detail.SetFocused(true)
	detail.InitFocus()
	detail.DrawBody(&model.WorkloadDetailData{
		Kind:              model.WorkloadKindDeployment,
		Namespace:         "default",
		Name:              "web",
		Status:            "Ready",
		ReadyText:         "3/3 ready",
		Strategy:          "RollingUpdate",
		Age:               "2d",
		Efficiency:        snap.Deployments[0].Efficiency,
		Selector:          map[string]string{"app": "web"},
		Labels:            map[string]string{"tier": "frontend"},
		TotalRequestedCPU: resource.NewMilliQuantity(500, resource.DecimalSI),
		TotalRequestedMem: resource.NewQuantity(256*1024*1024, resource.DecimalSI),
		OwnedPods: []*model.PodModel{
			{Namespace: "default", Name: "web-pod-1", Status: "Running", ReadyContainers: 1, TotalContainers: 1, TimeSince: "2d"},
		},
		Events: []corev1.Event{
			{Reason: "ScalingReplicaSet", Type: "Normal", Message: "Scaled up replica set web-1 to 3", LastTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute))},
		},
	})
	ts.Draw(detail.GetRootView())
	detailText := ts.Text()
	for _, want := range []string{"web-pod-1", "ScalingReplicaSet", "RollingUpdate", "72%"} {
		if !strings.Contains(detailText, want) {
			t.Fatalf("expected %q in Workload Detail view, got:\n%s", want, detailText)
		}
	}
	_ = tcell.ColorDefault
}
