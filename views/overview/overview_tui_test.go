package overview

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/api/resource"
)

func testNodeFixtures() []model.NodeModel {
	return []model.NodeModel{
		{
			Name:                  "worker-node-1",
			Status:                "Ready",
			Role:                  "worker",
			InternalIP:            "10.0.0.11",
			PodsCount:             18,
			Restarts:              1,
			TaintCount:            0,
			VolumesInUse:          2,
			VolumesAttached:       2,
			AllocatableCpuQty:     resource.NewMilliQuantity(4000, resource.DecimalSI),
			AllocatableMemQty:     resource.NewQuantity(16*1024*1024*1024, resource.BinarySI),
			AllocatableStorageQty: resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
			RequestedPodCpuQty:    resource.NewMilliQuantity(1200, resource.DecimalSI),
			RequestedPodMemQty:    resource.NewQuantity(4*1024*1024*1024, resource.BinarySI),
			UsageCpuQty:           resource.NewMilliQuantity(900, resource.DecimalSI),
			UsageMemQty:           resource.NewQuantity(3*1024*1024*1024, resource.BinarySI),
			PSI:                   metrics.PSIMetrics{CPUStallPct: 4.2},
			PSIKernelSupported:    true,
		},
		{
			Name:                  "control-plane-1",
			Status:                "Ready",
			Controller:            true,
			InternalIP:            "10.0.0.10",
			PodsCount:             12,
			Restarts:              0,
			TaintCount:            1,
			VolumesInUse:          1,
			VolumesAttached:       1,
			AllocatableCpuQty:     resource.NewMilliQuantity(4000, resource.DecimalSI),
			AllocatableMemQty:     resource.NewQuantity(16*1024*1024*1024, resource.BinarySI),
			AllocatableStorageQty: resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
			RequestedPodCpuQty:    resource.NewMilliQuantity(2500, resource.DecimalSI),
			RequestedPodMemQty:    resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			UsageCpuQty:           resource.NewMilliQuantity(2400, resource.DecimalSI),
			UsageMemQty:           resource.NewQuantity(10*1024*1024*1024, resource.BinarySI),
			PSI:                   metrics.PSIMetrics{IOStallPct: 0.45},
			PSIKernelSupported:    true,
		},
	}
}

func testPodFixtures() []model.PodModel {
	return []model.PodModel{
		{
			Namespace:              "default",
			Name:                   "api-gateway-7f8b",
			Status:                 "Running",
			Node:                   "worker-node-1",
			IP:                     "10.244.1.15",
			TimeSince:              "3h",
			ReadyContainers:        2,
			TotalContainers:        2,
			Restarts:               0,
			Volumes:                2,
			VolMounts:              2,
			PodRequestedCpuQty:     resource.NewMilliQuantity(500, resource.DecimalSI),
			PodRequestedMemQty:     resource.NewQuantity(512*1024*1024, resource.BinarySI),
			PodUsageCpuQty:         resource.NewMilliQuantity(350, resource.DecimalSI),
			PodUsageMemQty:         resource.NewQuantity(400*1024*1024, resource.BinarySI),
			NodeAllocatableCpuQty:  resource.NewMilliQuantity(4000, resource.DecimalSI),
			NodeAllocatableMemQty:  resource.NewQuantity(16*1024*1024*1024, resource.BinarySI),
			PSI:                    metrics.PSIMetrics{CPUStallPct: 14.8},
			NodePSIKernelSupported: true,
		},
		{
			Namespace:              "kube-system",
			Name:                   "coredns-64897985d",
			Status:                 "Running",
			Node:                   "control-plane-1",
			IP:                     "10.244.0.4",
			TimeSince:              "5d",
			ReadyContainers:        1,
			TotalContainers:        1,
			Restarts:               2,
			Volumes:                1,
			VolMounts:              1,
			PodRequestedCpuQty:     resource.NewMilliQuantity(100, resource.DecimalSI),
			PodRequestedMemQty:     resource.NewQuantity(170*1024*1024, resource.BinarySI),
			PodUsageCpuQty:         resource.NewMilliQuantity(900, resource.DecimalSI),
			PodUsageMemQty:         resource.NewQuantity(120*1024*1024, resource.BinarySI),
			NodeAllocatableCpuQty:  resource.NewMilliQuantity(4000, resource.DecimalSI),
			NodeAllocatableMemQty:  resource.NewQuantity(16*1024*1024*1024, resource.BinarySI),
			PSI:                    metrics.PSIMetrics{},
			NodePSIKernelSupported: true,
		},
	}
}

func TestNodePanel_SortAndFilterTUI(t *testing.T) {
	ts, err := ui.NewTestScreen(168, 14)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	cols := []string{"NAME", "STATUS", "RST", "PODS", "TAINTS", "PRESSURE", "IP", "VOLS", "DISK", "CPU", "MEM", "STALL"}
	panel := NewNodePanel(nil, " Nodes ")
	panel.DrawHeader(cols)
	panel.DrawBody(testNodeFixtures())

	root := panel.GetRootView()
	ts.Draw(root)
	_ = ts.WriteSnapshotIfEnv("overview_nodes_default", "Overview — Nodes (Default Sort: NAME ▲, with PSI STALL)")

	screenText := ts.Text()
	if !strings.Contains(screenText, "control-plane-1") || !strings.Contains(screenText, "worker-node-1") {
		t.Fatalf("expected both nodes in initial render, got:\n%s", screenText)
	}
	if !strings.Contains(screenText, "CPU 4.2%") || !strings.Contains(screenText, "IO 0.45%") {
		t.Fatalf("expected PSI stall cells in rendered Nodes table, got:\n%s", screenText)
	}
	// Default sort is NAME ascending: control-plane-1 appears before worker-node-1
	if strings.Index(screenText, "control-plane-1") > strings.Index(screenText, "worker-node-1") {
		t.Fatalf("expected control-plane-1 before worker-node-1 under default NAME ▲ sort:\n%s", screenText)
	}

	// Sort by MEM ascending ('m'), then descending ('m')
	ts.SendRune(root, 'm')
	afterMemAsc := ts.Text()
	if strings.Index(afterMemAsc, "worker-node-1") > strings.Index(afterMemAsc, "control-plane-1") {
		t.Fatalf("expected worker-node-1 (3Gi) before control-plane-1 (10Gi) on MEM ▲:\n%s", afterMemAsc)
	}

	ts.SendRune(root, 'm')
	_ = ts.WriteSnapshotIfEnv("overview_nodes_mem_desc", "Overview — Nodes (Sorted by MEM ▼)")
	afterMemDesc := ts.Text()
	if strings.Index(afterMemDesc, "control-plane-1") > strings.Index(afterMemDesc, "worker-node-1") {
		t.Fatalf("expected control-plane-1 (10Gi) before worker-node-1 (3Gi) on MEM ▼:\n%s", afterMemDesc)
	}

	// Filter with '/' -> "worker" -> Enter
	ts.SendRune(root, '/')
	ts.SendText(root, "worker")
	ts.SendKey(root, tcell.KeyEnter, 0, tcell.ModNone)
	_ = ts.WriteSnapshotIfEnv("overview_nodes_filtered", "Overview — Nodes (Filtered: /worker)")

	filteredText := ts.Text()
	if !strings.Contains(filteredText, "worker-node-1") {
		t.Fatalf("expected worker-node-1 to remain visible when filtered, got:\n%s", filteredText)
	}
	if strings.Contains(filteredText, "control-plane-1") {
		t.Fatalf("expected control-plane-1 to be hidden when filtered by 'worker', got:\n%s", filteredText)
	}

	// Clear filter with Escape
	ts.SendKey(root, tcell.KeyEscape, 0, tcell.ModNone)
	clearedText := ts.Text()
	if !strings.Contains(clearedText, "control-plane-1") {
		t.Fatalf("expected control-plane-1 to reappear after clearing filter with Esc, got:\n%s", clearedText)
	}
}

func TestPodPanel_SortAndFilterTUI(t *testing.T) {
	ts, err := ui.NewTestScreen(168, 14)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	cols := []string{"NAMESPACE", "POD", "READY", "STATUS", "RST", "AGE", "VOLS", "IP", "NODE", "CPU", "MEMORY", "STALL"}
	panel := NewPodPanel(nil, " Pods ")
	panel.DrawHeader(cols)
	panel.DrawBody(testPodFixtures())

	root := panel.GetRootView()
	ts.Draw(root)
	_ = ts.WriteSnapshotIfEnv("overview_pods_default", "Overview — Pods (Default Sort: NAMESPACE ▲, with PSI STALL)")

	initial := ts.Text()
	if !strings.Contains(initial, "api-gateway-7f8b") || !strings.Contains(initial, "coredns-64897985d") {
		t.Fatalf("expected both pods in initial render, got:\n%s", initial)
	}
	if !strings.Contains(initial, "CPU 15%") {
		t.Fatalf("expected PSI stall cell 'CPU 15%%' in rendered Pods table, got:\n%s", initial)
	}

	// Sort by CPU ascending ('c') then descending ('c')
	ts.SendRune(root, 'c')
	ts.SendRune(root, 'c')
	_ = ts.WriteSnapshotIfEnv("overview_pods_cpu_desc", "Overview — Pods (Sorted by CPU ▼)")
	cpuDesc := ts.Text()
	if strings.Index(cpuDesc, "coredns-64897985d") > strings.Index(cpuDesc, "api-gateway-7f8b") {
		t.Fatalf("expected coredns (900m CPU) before api-gateway (350m CPU) under CPU ▼ sort:\n%s", cpuDesc)
	}

	// Sort by MEMORY ('m') ascending then descending ('m')
	ts.SendRune(root, 'm')
	ts.SendRune(root, 'm')
	memDesc := ts.Text()
	if strings.Index(memDesc, "api-gateway-7f8b") > strings.Index(memDesc, "coredns-64897985d") {
		t.Fatalf("expected api-gateway (400Mi) before coredns (120Mi) under MEMORY ▼ sort:\n%s", memDesc)
	}
}

func TestSparklineEvictionOnDrawBody(t *testing.T) {
	nodeCols := []string{"NAME", "STATUS", "CPU", "MEM"}
	np := NewNodePanel(nil, " Nodes ").(*nodePanel)
	np.DrawHeader(nodeCols)

	allNodes := testNodeFixtures()
	np.DrawBody(allNodes)
	if len(np.cpuSparklines) != len(allNodes) || len(np.memSparklines) != len(allNodes) {
		t.Fatalf("expected %d node sparklines after first DrawBody, got cpu=%d mem=%d", len(allNodes), len(np.cpuSparklines), len(np.memSparklines))
	}

	// Simulate node eviction (only 1 node remains)
	np.DrawBody(allNodes[:1])
	if len(np.cpuSparklines) != 1 || len(np.memSparklines) != 1 {
		t.Fatalf("expected 1 node sparkline after pruning removed nodes, got cpu=%d mem=%d", len(np.cpuSparklines), len(np.memSparklines))
	}

	podCols := []string{"NAMESPACE", "POD", "CPU", "MEMORY"}
	pp := NewPodPanel(nil, " Pods ").(*podPanel)
	pp.DrawHeader(podCols)

	allPods := testPodFixtures()
	pp.DrawBody(allPods)
	if len(pp.cpuSparklines) != len(allPods) || len(pp.memSparklines) != len(allPods) {
		t.Fatalf("expected %d pod sparklines after first DrawBody, got cpu=%d mem=%d", len(allPods), len(pp.cpuSparklines), len(pp.memSparklines))
	}

	// Simulate pod deletion (only 1 pod remains)
	pp.DrawBody(allPods[:1])
	if len(pp.cpuSparklines) != 1 || len(pp.memSparklines) != 1 {
		t.Fatalf("expected 1 pod sparkline after pruning deleted pods, got cpu=%d mem=%d", len(pp.cpuSparklines), len(pp.memSparklines))
	}
}

func TestPodPanelNamespaceFilterSparklinePreservation(t *testing.T) {
	podCols := []string{"NAMESPACE", "POD", "CPU", "MEMORY"}
	pp := NewPodPanel(nil, " Pods ").(*podPanel)
	pp.DrawHeader(podCols)

	allPods := testPodFixtures()
	if len(allPods) < 2 {
		t.Fatalf("test requires at least 2 pod fixtures")
	}

	// 1. Initial draw with all pods
	pp.DrawBody(allPods)
	if len(pp.cpuSparklines) != len(allPods) {
		t.Fatalf("expected %d sparklines initially, got %d", len(allPods), len(pp.cpuSparklines))
	}

	// 2. Filter to "kube-system" - only 1 pod displayed, but sparklines for all pods must NOT be pruned
	pp.SetNamespaceFilter("kube-system")
	pp.DrawBody(allPods)
	if pp.filter.MatchRows != 1 {
		t.Fatalf("expected 1 matched row with kube-system filter, got %d", pp.filter.MatchRows)
	}
	if len(pp.cpuSparklines) != len(allPods) {
		t.Fatalf("expected all %d sparklines preserved during namespace filter, got %d", len(allPods), len(pp.cpuSparklines))
	}

	// 3. Clear namespace filter - all pods displayed and sparklines still present
	pp.SetNamespaceFilter("")
	pp.DrawBody(allPods)
	if pp.filter.MatchRows != len(allPods) {
		t.Fatalf("expected %d matched rows after clearing filter, got %d", len(allPods), pp.filter.MatchRows)
	}
	if len(pp.cpuSparklines) != len(allPods) {
		t.Fatalf("expected %d sparklines retained after clearing filter, got %d", len(allPods), len(pp.cpuSparklines))
	}
}
