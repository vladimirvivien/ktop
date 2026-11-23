package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	"github.com/vladimirvivien/ktop/views/overview"
	"github.com/vladimirvivien/ktop/views/workloads"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWorkloadsTUI_EndToEndNavigationAndDrillDown(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	depUID := types.UID("dep-1")
	rsUID := types.UID("rs-1")
	replicas := int32(2)
	completions := int32(1)
	now := metav1.Now()

	fakeKube := fake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api", UID: depUID, CreationTimestamp: now},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			},
			Status: appsv1.DeploymentStatus{Replicas: 2, ReadyReplicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default", Name: "api-abc", UID: rsUID,
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "api", UID: depUID}},
			},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Namespace: "db", Name: "pg", CreationTimestamp: now},
			Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
			Status:     appsv1.StatefulSetStatus{Replicas: 2, ReadyReplicas: 2},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "proxy", CreationTimestamp: now},
			Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, CurrentNumberScheduled: 1, NumberReady: 1},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Namespace: "batch", Name: "nightly", CreationTimestamp: now},
			Spec:       batchv1.JobSpec{Completions: &completions},
			Status:     batchv1.JobStatus{Succeeded: 1},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default", Name: "api-abc-1", Labels: map[string]string{"app": "api"},
				OwnerReferences:   []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "api-abc", UID: rsUID}},
				CreationTimestamp: now,
			},
			Spec: corev1.PodSpec{NodeName: "worker-1", Containers: []corev1.Container{{Name: "app", Image: "app:v1"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:  "app",
					Ready: true,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				}},
			},
		},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "default", Name: "api-evt"},
			InvolvedObject: corev1.ObjectReference{Kind: "Deployment", Name: "api", Namespace: "default"},
			Type:           "Normal",
			Reason:         "ScalingReplicaSet",
			Message:        "Scaled up replica set api-abc to 2",
			LastTimestamp:  now,
		},
	)

	client := k8s.NewWithClient(fakeKube, k8s.AllNamespaces, "test-ctx", &version.Info{GitVersion: "v1.32.0"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Note: Test calls RefreshWorkloads directly below after waiting for informers,
	// avoiding QueueUpdateDraw blocking when tview's Run() loop is not running.
	app := application.New(client, nil)
	overviewPanel := overview.New(app, "Overview")
	workloadsPanel := workloads.New(app, "Workloads")

	app.RegisterPage(application.PageDescriptor{
		ID:         "overview",
		Title:      "Overview",
		Key:        '1',
		RootType:   application.PageOverview,
		Panel:      overviewPanel,
		ChildNames: []string{"nodes", "pods"},
	})
	app.RegisterPage(application.PageDescriptor{
		ID:         "workloads",
		Title:      "Workloads",
		Key:        '2',
		RootType:   application.PageWorkloads,
		Panel:      workloadsPanel,
		ChildNames: []string{"deployments", "statefulsets", "daemonsets", "jobs"},
	})

	if err := client.Controller().StartInformers(ctx, 10*time.Second, 5*time.Second); err != nil {
		t.Fatalf("StartInformers failed: %v", err)
	}
	if err := app.Setup(ctx); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Wait briefly for all workload informers (including batch/v1 Jobs under -race) to populate.
	var snap model.WorkloadsSnapshot
	deadline := time.Now().Add(3 * time.Second)
	for {
		var err error
		snap, err = client.Controller().GetWorkloadsSnapshot(ctx)
		if err != nil {
			t.Fatalf("GetWorkloadsSnapshot failed: %v", err)
		}
		if len(snap.Deployments) > 0 && len(snap.StatefulSets) > 0 && len(snap.DaemonSets) > 0 && len(snap.Jobs) > 0 {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	workloadsPanel.DrawBody(snap)

	ts, err := ui.NewTestScreen(160, 48)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(app.RootPrimitive())
	if !strings.Contains(ts.Text(), "[1] Overview") || !strings.Contains(ts.Text(), "[2] Workloads") {
		t.Fatalf("expected Overview and Workloads tabs on screen, got:\n%s", ts.Text())
	}

	// Verify Overview Tab cycle skips Cluster Summary: Header -> Nodes -> Pods -> Header.
	sendAppKey(ts, app, tcell.KeyTab, 0, tcell.ModNone)
	if !overviewPanel.GetChildrenViews()[0].HasFocus() {
		t.Fatalf("expected first Tab on Overview to skip Cluster Summary and focus Nodes panel")
	}
	sendAppKey(ts, app, tcell.KeyTab, 0, tcell.ModNone)
	if !overviewPanel.GetChildrenViews()[1].HasFocus() {
		t.Fatalf("expected second Tab on Overview to focus Pods panel")
	}
	sendAppKey(ts, app, tcell.KeyTab, 0, tcell.ModNone) // Wrap back to Header

	// Press '2' to switch to Workloads tab.
	sendAppKey(ts, app, tcell.KeyRune, '2', tcell.ModNone)
	if app.ActivePageID() != "workloads" {
		t.Fatalf("expected ActivePageID()='workloads', got %q", app.ActivePageID())
	}
	wScreen := ts.Text()
	for _, want := range []string{"Summary", "Deployments", "api", "StatefulSets", "pg", "DaemonSets", "proxy", "Jobs", "nightly"} {
		if !strings.Contains(wScreen, want) {
			t.Fatalf("expected %q on Workloads screen, got:\n%s", want, wScreen)
		}
	}

	// First Tab on Workloads skips Summary and lands directly on Deployments table.
	sendAppKey(ts, app, tcell.KeyTab, 0, tcell.ModNone)
	if app.FocusedPrimitive() != workloadsPanel.GetChildrenViews()[0] {
		t.Fatalf("expected first Tab on Workloads to skip Summary and focus Deployments table")
	}
	sendAppKey(ts, app, tcell.KeyEnter, 0, tcell.ModNone)

	detailScreen := ts.Text()
	for _, want := range []string{"Workloads › deploy/default/api", "Deployments > api", "Deployment Detail", "Desired: 2", "Ready: 2", "api-abc-1", "ScalingReplicaSet"} {
		if !strings.Contains(detailScreen, want) {
			t.Fatalf("expected %q on Deployment Detail screen, got:\n%s", want, detailScreen)
		}
	}

	// Deployment Detail is active on entry; pressing Enter opens the full YAML modal.
	sendAppKey(ts, app, tcell.KeyEnter, 0, tcell.ModNone)
	if !app.HasActiveModalPage() {
		t.Fatal("expected Enter on active Deployment Detail to open YAML modal")
	}
	yamlScreen := ts.Text()
	for _, want := range []string{"Deployment YAML: default/api", "apiVersion: apps/v1", "kind: Deployment"} {
		if !strings.Contains(yamlScreen, want) {
			t.Fatalf("expected %q in Deployment YAML modal, got:\n%s", want, yamlScreen)
		}
	}

	// ESC dismisses the YAML modal and returns focus to Deployment Detail.
	sendAppKey(ts, app, tcell.KeyEsc, 0, tcell.ModNone)
	if app.HasActiveModalPage() {
		t.Fatal("expected ESC to dismiss YAML modal")
	}

	// Tab moves focus from Deployment Detail -> Owned Pods; Enter opens the shared Pod Inspector modal.
	sendAppKey(ts, app, tcell.KeyTab, 0, tcell.ModNone)
	sendAppKey(ts, app, tcell.KeyEnter, 0, tcell.ModNone)
	if !app.HasActiveModalPage() {
		t.Fatal("expected Enter on owned pod to open floating Pod Inspector modal")
	}
	// Underlying Deployment Detail breadcrumb stays anchored on the TabFrame behind the modal.
	if got := application.FormatBreadcrumb("Workloads", app.GetNavigationStack().Breadcrumb()); got != "Workloads › deploy/default/api" {
		t.Fatalf("expected depth-1 Deployment breadcrumb preserved behind Pod modal, got %q", got)
	}
	podModalScreen := ts.Text()
	for _, want := range []string{"[1] Overview", "[2] Workloads", "Workloads › deploy/default/api", "Pod: default/api-abc-1", "app"} {
		if !strings.Contains(podModalScreen, want) {
			t.Fatalf("expected %q visible with floating Pod Inspector modal, got:\n%s", want, podModalScreen)
		}
	}

	// Pressing 'v' inside Pod modal switches inline to Pod YAML; ESC returns to Pod Detail.
	sendAppKey(ts, app, tcell.KeyRune, 'v', tcell.ModNone)
	podYAMLScreen := ts.Text()
	for _, want := range []string{"Pod YAML: default/api-abc-1", "kind: Pod", "name: api-abc-1"} {
		if !strings.Contains(podYAMLScreen, want) {
			t.Fatalf("expected %q in inline Pod YAML view, got:\n%s", want, podYAMLScreen)
		}
	}
	sendAppKey(ts, app, tcell.KeyEsc, 0, tcell.ModNone)
	if !app.HasActiveModalPage() {
		t.Fatal("expected ESC from Pod YAML to return to Pod Detail inside the same modal")
	}

	// Pressing Enter on container row transitions inline to Container Logs; ESC returns to Pod Detail.
	sendAppKey(ts, app, tcell.KeyEnter, 0, tcell.ModNone)
	if !app.HasActiveModalPage() {
		t.Fatal("expected Container Logs to stay inside the active Pod Inspector modal")
	}
	if !strings.Contains(ts.Text(), "Container Detail") {
		t.Fatalf("expected Container Logs view inside modal, got:\n%s", ts.Text())
	}
	sendAppKey(ts, app, tcell.KeyEsc, 0, tcell.ModNone)
	if !app.HasActiveModalPage() {
		t.Fatal("expected ESC from Container Logs to return to Pod Detail inside the modal")
	}
	if !strings.Contains(ts.Text(), "Pod: default/api-abc-1") {
		t.Fatalf("expected Pod Detail restored after ESC from Container Logs, got:\n%s", ts.Text())
	}

	// ESC from Pod Detail closes the Pod Inspector modal and returns directly to Deployment Detail.
	sendAppKey(ts, app, tcell.KeyEsc, 0, tcell.ModNone)
	if app.HasActiveModalPage() {
		t.Fatal("expected ESC from Pod Detail to close Pod Inspector modal")
	}
	if !strings.Contains(ts.Text(), "Workloads › deploy/default/api") {
		t.Fatalf("expected Deployment Detail visible after closing Pod modal, got:\n%s", ts.Text())
	}

	// Next ESC unwinds Deployment Detail back to Workloads root.
	sendAppKey(ts, app, tcell.KeyEsc, 0, tcell.ModNone)
	if app.IsInDetailView() {
		t.Fatal("expected ESC from Deployment Detail to return to Workloads root")
	}
	if !strings.Contains(ts.Text(), "Summary") {
		t.Fatalf("expected Workloads root after unwinding, got:\n%s", ts.Text())
	}
}
