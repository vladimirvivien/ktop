package model

import (
	"math"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func int32Ptr(v int32) *int32 { return &v }
func boolPtr(v bool) *bool    { return &v }

func TestNewDeploymentModelStatuses(t *testing.T) {
	now := metav1.NewTime(time.Now().Add(-2 * time.Hour))

	tests := []struct {
		name   string
		dep    *appsv1.Deployment
		expect string
	}{
		{
			name: "healthy ready deployment",
			dep: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "web", CreationTimestamp: now},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
				Status:     appsv1.DeploymentStatus{Replicas: 3, ReadyReplicas: 3, UpdatedReplicas: 3, AvailableReplicas: 3},
			},
			expect: "Ready",
		},
		{
			name: "degraded replica failure",
			dep: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api", CreationTimestamp: now},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
				Status: appsv1.DeploymentStatus{
					Replicas: 3, ReadyReplicas: 1, UpdatedReplicas: 2, AvailableReplicas: 1, UnavailableReplicas: 2,
					Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentReplicaFailure, Status: corev1.ConditionTrue}},
				},
			},
			expect: "Degraded",
		},
		{
			name: "progressing rollout",
			dep: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "worker", CreationTimestamp: now},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(4)},
				Status:     appsv1.DeploymentStatus{Replicas: 4, ReadyReplicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2, UnavailableReplicas: 2},
			},
			expect: "Progressing",
		},
		{
			name: "scaled to zero",
			dep: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Namespace: "app", Name: "idle", CreationTimestamp: now},
				Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(0)},
				Status:     appsv1.DeploymentStatus{Replicas: 0},
			},
			expect: "Scaled 0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewDeploymentModel(tc.dep)
			if m.Status != tc.expect {
				t.Fatalf("Status = %q, want %q", m.Status, tc.expect)
			}
		})
	}
}

func TestComputePodsEfficiency(t *testing.T) {
	pods := []*PodModel{
		{
			Name:               "p1",
			Status:             "Running",
			PodRequestedCpuQty: resource.NewMilliQuantity(1000, resource.DecimalSI),
			PodUsageCpuQty:     resource.NewMilliQuantity(500, resource.DecimalSI), // 50%
			PodRequestedMemQty: resource.NewQuantity(1024, resource.DecimalSI),
			PodUsageMemQty:     resource.NewQuantity(256, resource.DecimalSI), // 25%
		},
		{
			Name:               "p2",
			Status:             "Running",
			PodRequestedCpuQty: resource.NewMilliQuantity(1000, resource.DecimalSI),
			PodUsageCpuQty:     resource.NewMilliQuantity(700, resource.DecimalSI), // 70%
			PodRequestedMemQty: resource.NewQuantity(1024, resource.DecimalSI),
			PodUsageMemQty:     resource.NewQuantity(512, resource.DecimalSI), // 50%
		},
		{
			Name:               "p-completed",
			Status:             "Completed",
			PodRequestedCpuQty: resource.NewMilliQuantity(1000, resource.DecimalSI),
			PodUsageCpuQty:     resource.NewMilliQuantity(0, resource.DecimalSI),
		},
	}

	if got := ComputePodsEfficiency(pods, false); got.HasEfficiency {
		t.Fatalf("expected HasEfficiency=false when metrics unavailable, got %+v", got)
	}

	eff := ComputePodsEfficiency(pods, true)
	if !eff.HasEfficiency {
		t.Fatal("expected HasEfficiency=true")
	}
	if math.Abs(eff.CPUEffPct-60.0) > 0.01 {
		t.Fatalf("CPUEffPct = %.2f, want 60.0", eff.CPUEffPct)
	}
	if math.Abs(eff.MemEffPct-37.5) > 0.01 {
		t.Fatalf("MemEffPct = %.2f, want 37.5", eff.MemEffPct)
	}
	if math.Abs(eff.EffPct-60.0) > 0.01 {
		t.Fatalf("EffPct = %.2f, want 60.0", eff.EffPct)
	}

	if got := ComputePodsUsage(pods, false); got.HasCPU || got.HasMem {
		t.Fatalf("expected zero usage when metrics unavailable, got %+v", got)
	}
	usage := ComputePodsUsage(pods, true)
	if !usage.HasCPU || !usage.HasMem {
		t.Fatalf("expected HasCPU and HasMem true, got %+v", usage)
	}
	if usage.CPU.MilliValue() != 1200 {
		t.Fatalf("CPU = %dm, want 1200m", usage.CPU.MilliValue())
	}
	if usage.Mem.Value() != 768 {
		t.Fatalf("Mem = %d, want 768", usage.Mem.Value())
	}
}

func TestComputeWorkloadSummaryWithNamespaceFilter(t *testing.T) {
	snap := WorkloadsSnapshot{
		Deployments: []DeploymentModel{
			{Namespace: "default", Name: "web", Desired: 2, Ready: 2, Available: 2},
			{Namespace: "prod", Name: "api", Desired: 3, Ready: 1, Available: 1},
		},
		StatefulSets: []StatefulSetModel{
			{Namespace: "prod", Name: "pg", Desired: 3, Ready: 3},
		},
		DaemonSets: []DaemonSetModel{
			{Namespace: "kube-system", Name: "proxy", Desired: 2, Ready: 2},
		},
		Jobs: []JobModel{
			{Namespace: "default", Name: "j1", Status: "Running"},
			{Namespace: "prod", Name: "j2", Status: "Complete"},
			{Namespace: "prod", Name: "j3", Status: "Failed"},
		},
	}

	all := ComputeWorkloadSummary(snap, "")
	if all.DeploymentsReady != 1 || all.DeploymentsTotal != 2 ||
		all.StatefulSetsReady != 1 || all.StatefulSetsTotal != 1 ||
		all.DaemonSetsReady != 1 || all.DaemonSetsTotal != 1 ||
		all.JobsRunning != 1 || all.JobsCompleted != 1 || all.JobsFailed != 1 {
		t.Fatalf("unexpected all-namespace summary: %+v", all)
	}

	prod := ComputeWorkloadSummary(snap, "prod")
	if prod.DeploymentsReady != 0 || prod.DeploymentsTotal != 1 ||
		prod.StatefulSetsTotal != 1 || prod.DaemonSetsTotal != 0 ||
		prod.JobsRunning != 0 || prod.JobsCompleted != 1 || prod.JobsFailed != 1 {
		t.Fatalf("unexpected prod-filtered summary: %+v", prod)
	}
}

func TestNewJobModelAndSorting(t *testing.T) {
	jRun := NewJobModel(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "batch", Name: "etl",
			OwnerReferences: []metav1.OwnerReference{{Kind: "CronJob", Name: "nightly"}},
		},
		Spec:   batchv1.JobSpec{Completions: int32Ptr(2)},
		Status: batchv1.JobStatus{Active: 1, Succeeded: 1},
	})
	if jRun.Status != "Running" || jRun.FormatCompletions() != "1/2" || jRun.Owner != "CronJob/nightly" {
		t.Fatalf("unexpected running JobModel: %+v", jRun)
	}

	jSusp := NewJobModel(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Namespace: "batch", Name: "paused"},
		Spec:       batchv1.JobSpec{Suspend: boolPtr(true)},
	})
	if jSusp.Status != "Suspended" {
		t.Fatalf("expected Suspended, got %q", jSusp.Status)
	}

	deps := []DeploymentModel{
		{
			Namespace: "default", Name: "z", Desired: 1, Ready: 1, Status: "Ready",
			Usage:      WorkloadUsage{HasCPU: true, CPU: resource.NewMilliQuantity(400, resource.DecimalSI), HasMem: true, Mem: resource.NewQuantity(128*1024*1024, resource.BinarySI)},
			Efficiency: WorkloadEfficiency{HasEfficiency: true, EffPct: 80},
		},
		{
			Namespace: "default", Name: "a", Desired: 2, Ready: 0, Status: "Degraded",
			Usage:      WorkloadUsage{HasCPU: true, CPU: resource.NewMilliQuantity(100, resource.DecimalSI), HasMem: true, Mem: resource.NewQuantity(512*1024*1024, resource.BinarySI)},
			Efficiency: WorkloadEfficiency{HasEfficiency: true, EffPct: 10},
		},
	}
	SortDeploymentModelsBy(deps, "STATUS", true)
	if deps[0].Name != "a" {
		t.Fatalf("expected Degraded deployment sorted first, got %s", deps[0].Name)
	}
	SortDeploymentModelsBy(deps, "CPU", false)
	if deps[0].Name != "z" {
		t.Fatalf("expected highest CPU descending first, got %s", deps[0].Name)
	}
	SortDeploymentModelsBy(deps, "MEM", false)
	if deps[0].Name != "a" {
		t.Fatalf("expected highest MEM descending first, got %s", deps[0].Name)
	}
	SortDeploymentModelsBy(deps, "EFF", false)
	if deps[0].Name != "z" {
		t.Fatalf("expected highest EFF descending first, got %s", deps[0].Name)
	}
}
