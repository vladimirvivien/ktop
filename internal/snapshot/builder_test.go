package snapshot

import (
	"context"
	"testing"

	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestSnapshotBuilder_BuildAndRoundTrip(t *testing.T) {
	builder := NewBuilder()
	in := BuildInput{
		Cluster: ClusterInfo{
			Context: "kind-dev",
			Ready:   true,
		},
		Summary: model.ClusterSummary{
			HealthScore:       95,
			NodesReady:        1,
			NodesCount:        1,
			MetricsSourceType: "prometheus",
			PSIBeta:           true,
		},
		Nodes: []model.NodeModel{
			{
				Name:               "node-a",
				Status:             "Ready",
				PSIKernelSupported: true,
				AllocatableCpuQty:  resource.NewMilliQuantity(4000, resource.DecimalSI),
				UsageCpuQty:        resource.NewMilliQuantity(500, resource.DecimalSI),
			},
		},
		Pods: []model.PodModel{
			{
				Namespace: "default",
				Name:      "web-0",
				Node:      "node-a",
				Status:    "Running",
			},
		},
		PodMetrics: []*metrics.PodMetrics{
			{
				Namespace:       "default",
				PodName:         "web-0",
				CPUThrottledPct: 12.5,
				PSI:             metrics.PSIMetrics{CPUStallPct: 1.5},
				Containers: []metrics.ContainerMetrics{
					{
						Name:         "web",
						CPUUsage:     resource.NewMilliQuantity(250, resource.DecimalSI),
						MemoryUsage:  resource.NewQuantity(64*1024*1024, resource.BinarySI),
						CPUThrottled: 12.5,
					},
				},
			},
		},
	}

	snap, err := builder.Build(context.Background(), in)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if snap.Cluster.MetricsSource != "prometheus" || !snap.Cluster.PSIBeta {
		t.Errorf("unexpected cluster metadata: %+v", snap.Cluster)
	}
	if len(snap.Pods) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(snap.Pods))
	}
	pod := snap.Pods[0]
	if !pod.NodePSIKernelSupported {
		t.Errorf("expected NodePSIKernelSupported=true from node join")
	}
	if pod.UsageCPUMillis != 250 || pod.CPUThrottledPct != 12.5 || pod.PSI.CPUStallPct != 1.5 {
		t.Errorf("unexpected pod metrics join: %+v", pod)
	}
	if len(pod.Containers) != 1 || pod.Containers[0].Name != "web" {
		t.Errorf("unexpected containers: %+v", pod.Containers)
	}

	// Verify round-trip back to view models
	pm := pod.ToPodModel()
	if pm.PodUsageCpuQty.MilliValue() != 250 || pm.CPUThrottledPct != 12.5 {
		t.Errorf("ToPodModel round-trip mismatch: %+v", pm)
	}
	sm := snap.Summary.ToClusterSummary(snap.Cluster)
	if sm.HealthScore != 95 || !sm.PSIBeta {
		t.Errorf("ToClusterSummary round-trip mismatch: %+v", sm)
	}
}

func TestSnapshotBuilder_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewBuilder().Build(ctx, BuildInput{}); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
