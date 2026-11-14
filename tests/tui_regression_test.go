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

var _ metrics.PSIMetrics
