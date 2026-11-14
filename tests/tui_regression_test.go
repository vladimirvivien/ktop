package tests

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/internal/snapshot"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	nodedetail "github.com/vladimirvivien/ktop/views/node"
	"github.com/vladimirvivien/ktop/views/overview"
	poddetail "github.com/vladimirvivien/ktop/views/pod"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func assertScreensIdentical(t *testing.T, name string, a, b *ui.TestScreen) {
	t.Helper()
	linesA := a.Lines()
	linesB := b.Lines()
	if !slices.Equal(linesA, linesB) {
		t.Fatalf("%s: rendered lines diverged between legacy and snapshot paths\nlegacy:\n%s\nsnapshot:\n%s",
			name, strings.Join(linesA, "\n"), strings.Join(linesB, "\n"))
	}

	cellsA, wA, hA := a.Screen().GetContents()
	cellsB, wB, hB := b.Screen().GetContents()
	if wA != wB || hA != hB || len(cellsA) != len(cellsB) {
		t.Fatalf("%s: screen dimensions mismatch (%dx%d vs %dx%d)", name, wA, hA, wB, hB)
	}
	for i := range cellsA {
		if !slices.Equal(cellsA[i].Runes, cellsB[i].Runes) || cellsA[i].Style != cellsB[i].Style {
			x, y := i%wA, i/wA
			t.Fatalf("%s: cell (%d,%d) diverged: runes %v->%v style %v->%v",
				name, x, y, cellsA[i].Runes, cellsB[i].Runes, cellsA[i].Style, cellsB[i].Style)
		}
	}
}

func TestTUISnapshotRegressionSweep(t *testing.T) {
	created := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	summary := model.ClusterSummary{
		HealthScore:             95,
		Uptime:                  created,
		NodesReady:              1,
		NodesCount:              1,
		Namespaces:              2,
		PodsRunning:             1,
		PodsAvailable:           1,
		DeploymentsTotal:        1,
		DeploymentsReady:        1,
		AllocatableNodeCpuTotal: resource.NewMilliQuantity(4000, resource.DecimalSI),
		AllocatableNodeMemTotal: resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
		RequestedPodCpuTotal:    resource.NewMilliQuantity(250, resource.DecimalSI),
		RequestedPodMemTotal:    resource.NewQuantity(128*1024*1024, resource.BinarySI),
		UsageNodeCpuTotal:       resource.NewMilliQuantity(500, resource.DecimalSI),
		UsageNodeMemTotal:       resource.NewQuantity(512*1024*1024, resource.BinarySI),
		PVsTotal:                resource.NewQuantity(0, resource.DecimalSI),
		PVCsTotal:               resource.NewQuantity(0, resource.DecimalSI),
	}
	nodes := []model.NodeModel{
		{
			Name:               "node-1",
			Status:             "Ready",
			CreationTime:       created,
			TimeSinceStart:     "2h",
			InternalIP:         "10.0.0.10",
			PodsCount:          1,
			AllocatableCpuQty:  resource.NewMilliQuantity(4000, resource.DecimalSI),
			AllocatableMemQty:  resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			UsageCpuQty:        resource.NewMilliQuantity(500, resource.DecimalSI),
			UsageMemQty:        resource.NewQuantity(512*1024*1024, resource.BinarySI),
			PSIKernelSupported: true,
		},
	}
	pods := []model.PodModel{
		{
			Namespace:              "default",
			Name:                   "api-0",
			Status:                 "Running",
			Node:                   "node-1",
			IP:                     "10.244.0.5",
			TimeSince:              "2h",
			CreationTime:           created,
			ReadyContainers:        1,
			TotalContainers:        1,
			PodRequestedCpuQty:     resource.NewMilliQuantity(250, resource.DecimalSI),
			PodRequestedMemQty:     resource.NewQuantity(128*1024*1024, resource.BinarySI),
			PodUsageCpuQty:         resource.NewMilliQuantity(120, resource.DecimalSI),
			PodUsageMemQty:         resource.NewQuantity(64*1024*1024, resource.BinarySI),
			NodeAllocatableCpuQty:  resource.NewMilliQuantity(4000, resource.DecimalSI),
			NodeAllocatableMemQty:  resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			NodePSIKernelSupported: true,
			CPUThrottledPct:        4.5,
		},
	}

	snap, err := snapshot.NewBuilder().Build(context.Background(), snapshot.BuildInput{
		Cluster: snapshot.ClusterInfo{Ready: true},
		Summary: summary,
		Nodes:   nodes,
		Pods:    pods,
	})
	if err != nil {
		t.Fatalf("SnapshotBuilder.Build failed: %v", err)
	}

	t.Run("ClusterSummaryPanel", func(t *testing.T) {
		sA, _ := ui.NewTestScreen(140, 12)
		defer sA.Close()
		sB, _ := ui.NewTestScreen(140, 12)
		defer sB.Close()

		pA := overview.NewClusterSummaryPanel(nil, "Cluster Summary")
		pA.DrawBody(summary)
		sA.Draw(pA.GetRootView())

		pB := overview.NewClusterSummaryPanel(nil, "Cluster Summary")
		pB.DrawBody(snap)
		sB.Draw(pB.GetRootView())

		assertScreensIdentical(t, "ClusterSummaryPanel", sA, sB)
		if !strings.Contains(strings.Join(sB.Lines(), "\n"), "Health: 95%") {
			t.Errorf("expected Health: 95%% in summary panel output")
		}
	})

	t.Run("NodePanel", func(t *testing.T) {
		sA, _ := ui.NewTestScreen(140, 16)
		defer sA.Close()
		sB, _ := ui.NewTestScreen(140, 16)
		defer sB.Close()

		cols := []string{"NAME", "STATUS", "RST", "PODS", "CPU", "MEM", "STALL"}
		pA := overview.NewNodePanel(nil, "Nodes")
		pA.DrawHeader(cols)
		pA.DrawBody(nodes)
		sA.Draw(pA.GetRootView())

		pB := overview.NewNodePanel(nil, "Nodes")
		pB.DrawHeader(cols)
		pB.DrawBody(snap)
		sB.Draw(pB.GetRootView())

		assertScreensIdentical(t, "NodePanel", sA, sB)
	})

	t.Run("PodPanel", func(t *testing.T) {
		sA, _ := ui.NewTestScreen(140, 16)
		defer sA.Close()
		sB, _ := ui.NewTestScreen(140, 16)
		defer sB.Close()

		cols := []string{"NAMESPACE", "POD", "READY", "STATUS", "CPU", "MEMORY", "STALL", "THROTTLE"}
		pA := overview.NewPodPanel(nil, "Pods")
		pA.DrawHeader(cols)
		pA.DrawBody(pods)
		sA.Draw(pA.GetRootView())

		pB := overview.NewPodPanel(nil, "Pods")
		pB.DrawHeader(cols)
		pB.DrawBody(snap)
		sB.Draw(pB.GetRootView())

		assertScreensIdentical(t, "PodPanel", sA, sB)
		if !strings.Contains(strings.Join(sB.Lines(), "\n"), "4.5%") {
			t.Errorf("expected 4.5%% THROTTLE cell in pod panel output")
		}
	})

	t.Run("NodeDetailPanel", func(t *testing.T) {
		sA, _ := ui.NewTestScreen(140, 40)
		defer sA.Close()
		sB, _ := ui.NewTestScreen(140, 40)
		defer sB.Close()

		podPtr := &pods[0]
		pA := nodedetail.NewDetailPanel()
		pA.DrawBody(&model.NodeDetailData{
			NodeModel:  &nodes[0],
			PodsOnNode: []*model.PodModel{podPtr},
		})
		sA.Draw(pA.GetRootView())

		pB := nodedetail.NewDetailPanel()
		pB.DrawBody(&snapshot.NodeDetailProjection{
			Snapshot: snap,
			NodeName: "node-1",
		})
		sB.Draw(pB.GetRootView())

		assertScreensIdentical(t, "NodeDetailPanel", sA, sB)
	})

	t.Run("PodDetailPanel", func(t *testing.T) {
		sA, _ := ui.NewTestScreen(180, 40)
		defer sA.Close()
		sB, _ := ui.NewTestScreen(180, 40)
		defer sB.Close()

		pA := poddetail.NewDetailPanel()
		pA.GetRootView().SetRect(0, 0, 180, 40)
		pA.DrawBody(&model.PodDetailData{
			PodModel: &pods[0],
		})
		sA.Draw(pA.GetRootView())

		pB := poddetail.NewDetailPanel()
		pB.GetRootView().SetRect(0, 0, 180, 40)
		pB.DrawBody(&snapshot.PodDetailProjection{
			Snapshot:  snap,
			Namespace: "default",
			PodName:   "api-0",
		})
		sB.Draw(pB.GetRootView())

		assertScreensIdentical(t, "PodDetailPanel", sA, sB)
		if !strings.Contains(strings.Join(sB.Lines(), "\n"), "CFS: 4.5%") {
			t.Errorf("expected CFS: 4.5%% in pod detail info header")
		}
	})
}

func TestNodeCPUMetricsOverviewDetailParity(t *testing.T) {
	created := metav1.NewTime(time.Now().Add(-9 * 24 * time.Hour))
	cols := []string{"NAME", "STATUS", "RST", "PODS", "DISK", "CPU", "MEM"}

	t.Run("PartialMetrics_MemoryUsageOnly_CPUFallsBackToRequested", func(t *testing.T) {
		summary := model.ClusterSummary{
			HealthScore:             100,
			Uptime:                  created,
			NodesReady:              1,
			NodesCount:              1,
			MetricsSourceType:       metrics.SourceTypePrometheus,
			AllocatableNodeCpuTotal: resource.NewMilliQuantity(4000, resource.DecimalSI),
			AllocatableNodeMemTotal: resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			RequestedPodCpuTotal:    resource.NewMilliQuantity(3193, resource.DecimalSI),
			RequestedPodMemTotal:    resource.NewQuantity(1*1024*1024*1024, resource.BinarySI),
			UsageNodeCpuTotal:       resource.NewMilliQuantity(0, resource.DecimalSI),
			UsageNodeMemTotal:       resource.NewQuantity(2*1024*1024*1024, resource.BinarySI),
			HasUsageCPU:             false,
			HasUsageMem:             true,
			PVsTotal:                resource.NewQuantity(0, resource.DecimalSI),
			PVCsTotal:               resource.NewQuantity(0, resource.DecimalSI),
		}
		nodes := []model.NodeModel{
			{
				Name:                  "node-1",
				Status:                "Ready",
				CreationTime:          created,
				TimeSinceStart:        "9d",
				AllocatableCpuQty:     resource.NewMilliQuantity(4000, resource.DecimalSI),
				AllocatableMemQty:     resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
				AllocatableStorageQty: resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
				RequestedPodCpuQty:    resource.NewMilliQuantity(3193, resource.DecimalSI),
				RequestedPodMemQty:    resource.NewQuantity(1*1024*1024*1024, resource.BinarySI),
				UsageCpuQty:           nil,
				UsageMemQty:           resource.NewQuantity(2*1024*1024*1024, resource.BinarySI),
			},
		}

		snap, err := snapshot.NewBuilder().Build(context.Background(), snapshot.BuildInput{
			Cluster: snapshot.ClusterInfo{Ready: true, MetricsSource: metrics.SourceTypePrometheus},
			Summary: summary,
			Nodes:   nodes,
		})
		if err != nil {
			t.Fatalf("SnapshotBuilder.Build failed: %v", err)
		}

		// Overview Cluster Summary: CPU must render requested fallback, MEM must render used.
		sA, _ := ui.NewTestScreen(140, 14)
		defer sA.Close()
		sB, _ := ui.NewTestScreen(140, 14)
		defer sB.Close()
		pSumA := overview.NewClusterSummaryPanel(nil, "Cluster Summary")
		pSumA.DrawBody(summary)
		sA.Draw(pSumA.GetRootView())
		pSumB := overview.NewClusterSummaryPanel(nil, "Cluster Summary")
		pSumB.DrawBody(snap)
		sB.Draw(pSumB.GetRootView())
		assertScreensIdentical(t, "SummaryPanel_PartialUsage", sA, sB)
		sumText := sB.Text()
		if !strings.Contains(sumText, "3193m/4000m") || !strings.Contains(sumText, "requested") {
			t.Fatalf("expected ClusterSummary CPU requested fallback 3193m/4000m (requested), got:\n%s", sumText)
		}
		if strings.Contains(sumText, "0m/4000m") {
			t.Fatalf("unexpected 0m/4000m CPU in ClusterSummary when CPU usage is unavailable:\n%s", sumText)
		}

		// Overview Node table: CPU must render 3193m requests (79.8%) independently of UsageMemQty.
		nA, _ := ui.NewTestScreen(140, 12)
		defer nA.Close()
		nB, _ := ui.NewTestScreen(140, 12)
		defer nB.Close()
		pNodeA := overview.NewNodePanel(nil, "Nodes")
		pNodeA.DrawHeader(cols)
		pNodeA.DrawBody(nodes)
		nA.Draw(pNodeA.GetRootView())
		pNodeB := overview.NewNodePanel(nil, "Nodes")
		pNodeB.DrawHeader(cols)
		pNodeB.DrawBody(snap)
		nB.Draw(pNodeB.GetRootView())
		assertScreensIdentical(t, "NodePanel_PartialUsage", nA, nB)
		nodeText := nB.Text()
		if !strings.Contains(nodeText, "3193m") || !strings.Contains(nodeText, "79.8%") {
			t.Fatalf("expected NodePanel CPU requested fallback 3193m 79.8%%, got:\n%s", nodeText)
		}

		// Node Detail: CPU fallback must be labeled "requests", while MEM is labeled "used".
		dA, _ := ui.NewTestScreen(160, 40)
		defer dA.Close()
		dB, _ := ui.NewTestScreen(160, 40)
		defer dB.Close()
		pDetA := nodedetail.NewDetailPanel()
		pDetA.DrawBody(&model.NodeDetailData{
			NodeModel:         &nodes[0],
			MetricsSourceType: metrics.SourceTypePrometheus,
		})
		dA.Draw(pDetA.GetRootView())
		dA.Draw(pDetA.GetRootView())

		pDetB := nodedetail.NewDetailPanel()
		pDetB.DrawBody(&snapshot.NodeDetailProjection{
			Snapshot: snap,
			NodeName: "node-1",
		})
		dB.Draw(pDetB.GetRootView())
		dB.Draw(pDetB.GetRootView())
		assertScreensIdentical(t, "NodeDetailPanel_PartialUsage", dA, dB)

		detText := dB.Text()
		if !strings.Contains(detText, "CPU 3193m/4000m (79.8% requests)") {
			t.Fatalf("expected NodeDetail CPU requests label instead of mislabeled 'used', got:\n%s", detText)
		}
		if !strings.Contains(detText, "MEM") || !strings.Contains(detText, "used)") {
			t.Fatalf("expected NodeDetail MEM used label, got:\n%s", detText)
		}
	})

	t.Run("ActiveCPUUsage_ConsistentAcrossOverviewAndDetail", func(t *testing.T) {
		summary := model.ClusterSummary{
			HealthScore:             100,
			Uptime:                  created,
			NodesReady:              1,
			NodesCount:              1,
			MetricsSourceType:       metrics.SourceTypePrometheus,
			AllocatableNodeCpuTotal: resource.NewMilliQuantity(4000, resource.DecimalSI),
			AllocatableNodeMemTotal: resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			RequestedPodCpuTotal:    resource.NewMilliQuantity(3193, resource.DecimalSI),
			RequestedPodMemTotal:    resource.NewQuantity(1*1024*1024*1024, resource.BinarySI),
			UsageNodeCpuTotal:       resource.NewMilliQuantity(267, resource.DecimalSI),
			UsageNodeMemTotal:       resource.NewQuantity(2*1024*1024*1024, resource.BinarySI),
			HasUsageCPU:             true,
			HasUsageMem:             true,
			PVsTotal:                resource.NewQuantity(0, resource.DecimalSI),
			PVCsTotal:               resource.NewQuantity(0, resource.DecimalSI),
		}
		nodes := []model.NodeModel{
			{
				Name:                  "node-1",
				Status:                "Ready",
				CreationTime:          created,
				TimeSinceStart:        "9d",
				AllocatableCpuQty:     resource.NewMilliQuantity(4000, resource.DecimalSI),
				AllocatableMemQty:     resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
				AllocatableStorageQty: resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
				RequestedPodCpuQty:    resource.NewMilliQuantity(3193, resource.DecimalSI),
				RequestedPodMemQty:    resource.NewQuantity(1*1024*1024*1024, resource.BinarySI),
				UsageCpuQty:           resource.NewMilliQuantity(267, resource.DecimalSI),
				UsageMemQty:           resource.NewQuantity(2*1024*1024*1024, resource.BinarySI),
			},
		}

		snap, err := snapshot.NewBuilder().Build(context.Background(), snapshot.BuildInput{
			Cluster: snapshot.ClusterInfo{Ready: true, MetricsSource: metrics.SourceTypePrometheus},
			Summary: summary,
			Nodes:   nodes,
		})
		if err != nil {
			t.Fatalf("SnapshotBuilder.Build failed: %v", err)
		}

		sA, _ := ui.NewTestScreen(140, 14)
		defer sA.Close()
		sB, _ := ui.NewTestScreen(140, 14)
		defer sB.Close()
		pSumA := overview.NewClusterSummaryPanel(nil, "Cluster Summary")
		pSumA.DrawBody(summary)
		sA.Draw(pSumA.GetRootView())
		pSumB := overview.NewClusterSummaryPanel(nil, "Cluster Summary")
		pSumB.DrawBody(snap)
		sB.Draw(pSumB.GetRootView())
		assertScreensIdentical(t, "SummaryPanel_ActiveUsage", sA, sB)
		if !strings.Contains(sB.Text(), "CPU 267m/4000m (6.7% used)") {
			t.Fatalf("expected ClusterSummary CPU 267m/4000m (6.7%% used), got:\n%s", sB.Text())
		}

		dA, _ := ui.NewTestScreen(140, 40)
		defer dA.Close()
		dB, _ := ui.NewTestScreen(140, 40)
		defer dB.Close()
		pDetA := nodedetail.NewDetailPanel()
		pDetA.DrawBody(&model.NodeDetailData{
			NodeModel:         &nodes[0],
			MetricsSourceType: metrics.SourceTypePrometheus,
		})
		dA.Draw(pDetA.GetRootView())
		dA.Draw(pDetA.GetRootView())
		pDetB := nodedetail.NewDetailPanel()
		pDetB.DrawBody(&snapshot.NodeDetailProjection{
			Snapshot: snap,
			NodeName: "node-1",
		})
		dB.Draw(pDetB.GetRootView())
		dB.Draw(pDetB.GetRootView())
		assertScreensIdentical(t, "NodeDetailPanel_ActiveUsage", dA, dB)
		if !strings.Contains(dB.Text(), "CPU 267m/4000m (6.7% used)") {
			t.Fatalf("expected NodeDetail CPU 267m/4000m (6.7%% used), got:\n%s", dB.Text())
		}
	})
}

var _ metrics.PSIMetrics
