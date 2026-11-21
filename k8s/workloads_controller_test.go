package k8s

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/views/model"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/fake"
)

type fakeWorkloadsMetricsSource struct {
	metricsMap map[string]*metrics.PodMetrics
	history    map[string]*metrics.ResourceHistory
}

func (f *fakeWorkloadsMetricsSource) GetNodeMetrics(context.Context, string) (*metrics.NodeMetrics, error) {
	return nil, nil
}
func (f *fakeWorkloadsMetricsSource) GetPodMetrics(_ context.Context, ns, name string) (*metrics.PodMetrics, error) {
	return f.metricsMap[ns+"/"+name], nil
}
func (f *fakeWorkloadsMetricsSource) GetPodNetworkDiskMetrics(context.Context, string, string) (float64, float64, float64, float64, error) {
	return 0, 0, 0, 0, nil
}
func (f *fakeWorkloadsMetricsSource) GetMetricsForPod(_ context.Context, pod metav1.Object) (*metrics.PodMetrics, error) {
	return f.metricsMap[pod.GetNamespace()+"/"+pod.GetName()], nil
}
func (f *fakeWorkloadsMetricsSource) GetAllPodMetrics(context.Context) ([]*metrics.PodMetrics, error) {
	out := make([]*metrics.PodMetrics, 0, len(f.metricsMap))
	for _, m := range f.metricsMap {
		out = append(out, m)
	}
	return out, nil
}
func (f *fakeWorkloadsMetricsSource) GetAvailableMetrics() []string { return []string{"cpu", "memory"} }
func (f *fakeWorkloadsMetricsSource) IsHealthy() bool               { return true }
func (f *fakeWorkloadsMetricsSource) GetSourceInfo() metrics.SourceInfo {
	return metrics.SourceInfo{Type: metrics.SourceTypePrometheus, Healthy: true}
}
func (f *fakeWorkloadsMetricsSource) SetHealthCallback(func(bool, metrics.SourceInfo)) {}
func (f *fakeWorkloadsMetricsSource) GetNodeHistory(context.Context, string, metrics.HistoryQuery) (*metrics.ResourceHistory, error) {
	return nil, nil
}
func (f *fakeWorkloadsMetricsSource) GetPodHistory(_ context.Context, ns, name string, q metrics.HistoryQuery) (*metrics.ResourceHistory, error) {
	if f.history == nil {
		return nil, nil
	}
	return f.history[ns+"/"+name+"/"+string(q.Resource)], nil
}
func (f *fakeWorkloadsMetricsSource) SupportsHistory() bool { return f.history != nil }

func TestWorkloadsController_SnapshotAndDetail(t *testing.T) {
	depUID := types.UID("dep-uid-1")
	rsUID := types.UID("rs-uid-1")
	stsUID := types.UID("sts-uid-1")
	dsUID := types.UID("ds-uid-1")
	jobUID := types.UID("job-uid-1")

	replicas := int32(2)
	completions := int32(1)
	now := metav1.Now()

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api", UID: depUID, Labels: map[string]string{"app": "api"}},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
		},
		Status: appsv1.DeploymentStatus{Replicas: 2, ReadyReplicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2},
	}
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default", Name: "api-abc", UID: rsUID,
			OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "api", UID: depUID}},
		},
	}
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "db", Name: "pg", UID: stsUID},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas, ServiceName: "pg-headless"},
		Status:     appsv1.StatefulSetStatus{Replicas: 2, ReadyReplicas: 1, UpdatedReplicas: 2},
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "proxy", UID: dsUID},
		Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 1, CurrentNumberScheduled: 1, NumberReady: 1, UpdatedNumberScheduled: 1},
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Namespace: "batch", Name: "nightly", UID: jobUID},
		Spec:       batchv1.JobSpec{Completions: &completions},
		Status:     batchv1.JobStatus{Active: 1},
	}

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default", Name: "api-abc-1", Labels: map[string]string{"app": "api"},
			OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "api-abc", UID: rsUID}},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{{
				Name: "app",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("100Mi"),
					},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "app",
				Ready: true,
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			}},
		},
	}
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	}
	evt := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Namespace: "default", Name: "api-scale"},
		InvolvedObject: corev1.ObjectReference{Kind: "Deployment", Name: "api", Namespace: "default"},
		Type:           "Normal",
		Reason:         "ScalingReplicaSet",
		Message:        "Scaled up replica set api-abc to 2",
		LastTimestamp:  now,
	}

	fakeKube := fake.NewSimpleClientset(dep, rs, sts, ds, job, pod1, node1, evt)
	client := NewWithClient(fakeKube, AllNamespaces, "test-ctx", &version.Info{GitVersion: "v1.32.0"})
	ctrl := client.Controller()

	ms := &fakeWorkloadsMetricsSource{
		metricsMap: map[string]*metrics.PodMetrics{
			"default/api-abc-1": {
				Namespace: "default",
				PodName:   "api-abc-1",
				Containers: []metrics.ContainerMetrics{{
					Name:        "app",
					CPUUsage:    resource.NewMilliQuantity(250, resource.DecimalSI),
					MemoryUsage: resource.NewQuantity(80*1024*1024, resource.DecimalSI),
				}},
			},
		},
		history: map[string]*metrics.ResourceHistory{
			"default/api-abc-1/cpu": {
				DataPoints: []metrics.HistoryDataPoint{{Timestamp: time.Now(), Value: 250}},
			},
			"default/api-abc-1/memory": {
				DataPoints: []metrics.HistoryDataPoint{{Timestamp: time.Now(), Value: float64(80 * 1024 * 1024)}},
			},
		},
	}
	ctrl.SetMetricsSource(ms)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ctrl.StartInformers(ctx, 10*time.Second, 5*time.Second); err != nil {
		t.Fatalf("StartInformers failed: %v", err)
	}

	snap, err := ctrl.GetWorkloadsSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetWorkloadsSnapshot failed: %v", err)
	}
	if len(snap.Deployments) != 1 || len(snap.StatefulSets) != 1 || len(snap.DaemonSets) != 1 || len(snap.Jobs) != 1 {
		t.Fatalf("unexpected workload counts: %+v", snap)
	}
	dm := snap.Deployments[0]
	if !dm.Efficiency.HasEfficiency || math.Abs(dm.Efficiency.EffPct-80.0) > 0.5 || math.Abs(dm.Efficiency.CPUEffPct-50.0) > 0.5 {
		t.Fatalf("unexpected Deployment efficiency: %+v", dm.Efficiency)
	}

	detail, err := ctrl.BuildWorkloadDetailData(ctx, model.WorkloadKindDeployment, "default", "api")
	if err != nil {
		t.Fatalf("BuildWorkloadDetailData failed: %v", err)
	}
	if len(detail.OwnedPods) != 1 || detail.OwnedPods[0].Name != "api-abc-1" {
		t.Fatalf("expected 1 owned pod api-abc-1, got %+v", detail.OwnedPods)
	}
	if len(detail.Events) != 1 || detail.Events[0].Reason != "ScalingReplicaSet" {
		t.Fatalf("expected Deployment event ScalingReplicaSet, got %+v", detail.Events)
	}
	if detail.Desired != 2 || detail.Ready != 2 || detail.Updated != 2 || detail.Available != 2 {
		t.Fatalf("unexpected Deployment replica breakdown: %+v", detail)
	}
	if detail.ActiveReplicaSet != "api-abc" {
		t.Fatalf("expected ActiveReplicaSet api-abc, got %q", detail.ActiveReplicaSet)
	}
	if detail.RawObject == nil {
		t.Fatal("expected RawObject populated for YAML inspection")
	}
}

func TestWorkloadsController_LateRefreshRegistrationStartsHandler(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "late-app"},
	}
	fakeKube := fake.NewSimpleClientset(dep)
	client := NewWithClient(fakeKube, AllNamespaces, "test-ctx", &version.Info{GitVersion: "v1.32.0"})
	ctrl := client.Controller()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start controller first (simulating Overview calling Start before Workloads registers).
	if err := ctrl.Start(ctx, 10*time.Second); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	gotCh := make(chan model.WorkloadsSnapshot, 4)
	ctrl.SetWorkloadsRefreshFunc(func(_ context.Context, snap model.WorkloadsSnapshot) error {
		gotCh <- snap
		return nil
	})

	deadline := time.After(5 * time.Second)
	for {
		select {
		case snap := <-gotCh:
			if len(snap.Deployments) == 1 && snap.Deployments[0].Name == "late-app" {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for late-registered workloads handler to deliver Deployment")
		}
	}
}
