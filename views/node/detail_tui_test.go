package node

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeDetailPanel_RenderTUI(t *testing.T) {
	ts, err := ui.NewTestScreen(140, 42)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	panel := NewDetailPanel()
	detailData := &model.NodeDetailData{
		NodeModel: &model.NodeModel{
			Name:                    "worker-node-1",
			Status:                  "Ready",
			Role:                    "worker",
			InternalIP:              "10.0.0.11",
			TimeSinceStart:          "14d",
			KubeletVersion:          "v1.35.0",
			ContainerRuntimeVersion: "containerd://2.2.0",
			OS:                      "linux",
			OSImage:                 "Debian GNU/Linux 12 (bookworm)",
			OSKernel:                "6.12.0-amd64",
			Architecture:            "amd64",
			PodsCount:               2,
			ContainerImagesCount:    18,
			AllocatableCpuQty:       resource.NewMilliQuantity(4000, resource.DecimalSI),
			AllocatableMemQty:       resource.NewQuantity(16*1024*1024*1024, resource.BinarySI),
			AllocatableStorageQty:   resource.NewQuantity(100*1024*1024*1024, resource.BinarySI),
			RequestedPodCpuQty:      resource.NewMilliQuantity(1200, resource.DecimalSI),
			RequestedPodMemQty:      resource.NewQuantity(4*1024*1024*1024, resource.BinarySI),
			UsageCpuQty:             resource.NewMilliQuantity(900, resource.DecimalSI),
			UsageMemQty:             resource.NewQuantity(3*1024*1024*1024, resource.BinarySI),
		},
		Node: &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-node-1",
				Labels: map[string]string{
					"kubernetes.io/hostname": "worker-node-1",
					"kubernetes.io/os":       "linux",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, Reason: "KubeletReady"},
					{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
				},
			},
		},
		PodsOnNode: []*model.PodModel{
			{
				Namespace:          "default",
				Name:               "api-gateway-7f8b",
				Status:             "Running",
				ReadyContainers:    2,
				TotalContainers:    2,
				Restarts:           0,
				TimeSince:          "3h",
				PodRequestedCpuQty: resource.NewMilliQuantity(500, resource.DecimalSI),
				PodRequestedMemQty: resource.NewQuantity(512*1024*1024, resource.BinarySI),
				PodUsageCpuQty:     resource.NewMilliQuantity(350, resource.DecimalSI),
				PodUsageMemQty:     resource.NewQuantity(400*1024*1024, resource.BinarySI),
			},
			{
				Namespace:          "kube-system",
				Name:               "kube-proxy-x92k",
				Status:             "Running",
				ReadyContainers:    1,
				TotalContainers:    1,
				Restarts:           1,
				TimeSince:          "14d",
				PodRequestedCpuQty: resource.NewMilliQuantity(100, resource.DecimalSI),
				PodRequestedMemQty: resource.NewQuantity(128*1024*1024, resource.BinarySI),
				PodUsageCpuQty:     resource.NewMilliQuantity(80, resource.DecimalSI),
				PodUsageMemQty:     resource.NewQuantity(90*1024*1024, resource.BinarySI),
			},
		},
		MetricsSourceType: "prometheus",
		NetworkRxRate:     128 * 1024,
		NetworkTxRate:     64 * 1024,
		DiskReadRate:      256 * 1024,
		DiskWriteRate:     512 * 1024,
	}

	panel.DrawBody(detailData)
	root := panel.GetRootView()
	ts.Draw(root)
	// NodeDetailPanel uses a two-phase layout sized on first Draw; draw a second frame to render sub-panels.
	ts.Draw(root)
	_ = ts.WriteSnapshotIfEnv("node_detail_default", "Node Detail — worker-node-1")

	for _, expected := range []string{
		"worker-node-1",
		"v1.35.0",
		"api-gateway-7f8b",
		"kube-proxy-x92k",
	} {
		if !ts.ContainsText(expected) {
			t.Fatalf("expected %q in rendered NodeDetailPanel, got:\n%s", expected, ts.Text())
		}
	}
}

func TestNodeDetailPanel_PodsTableSortAndFilter(t *testing.T) {
	ts, err := ui.NewTestScreen(140, 42)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	panel := NewDetailPanel()
	detailData := &model.NodeDetailData{
		NodeModel: &model.NodeModel{
			Name: "worker-node-1",
		},
		PodsOnNode: []*model.PodModel{
			{
				Namespace:          "default",
				Name:               "api-gateway",
				Status:             "Running",
				ReadyContainers:    2,
				TotalContainers:    2,
				Restarts:           1,
				TimeSince:          "3h",
				PodUsageCpuQty:     resource.NewMilliQuantity(350, resource.DecimalSI),
				PodUsageMemQty:     resource.NewQuantity(400*1024*1024, resource.BinarySI),
			},
			{
				Namespace:          "kube-system",
				Name:               "coredns",
				Status:             "Running",
				ReadyContainers:    1,
				TotalContainers:    1,
				Restarts:           2,
				TimeSince:          "5d",
				PodUsageCpuQty:     resource.NewMilliQuantity(900, resource.DecimalSI),
				PodUsageMemQty:     resource.NewQuantity(120*1024*1024, resource.BinarySI),
			},
			{
				Namespace:          "prod",
				Name:               "backend-worker",
				Status:             "Running",
				ReadyContainers:    1,
				TotalContainers:    1,
				Restarts:           0,
				TimeSince:          "1d",
				PodUsageCpuQty:     resource.NewMilliQuantity(150, resource.DecimalSI),
				PodUsageMemQty:     resource.NewQuantity(800*1024*1024, resource.BinarySI),
			},
		},
	}

	panel.DrawBody(detailData)
	root := panel.GetRootView()
	ts.Draw(root)
	ts.Draw(root)

	initial := ts.Text()
	for _, name := range []string{"api-gateway", "coredns", "backend-worker"} {
		if !strings.Contains(initial, name) {
			t.Fatalf("expected pod %q in initial render, got:\n%s", name, initial)
		}
	}

	// Default sort is POD ascending: api-gateway < backend-worker < coredns
	if !strings.Contains(initial, "POD ▲") {
		t.Fatalf("expected 'POD ▲' in initial render header, got:\n%s", initial)
	}
	posAPI := strings.Index(initial, "api-gateway")
	posBackend := strings.Index(initial, "backend-worker")
	posCore := strings.Index(initial, "coredns")
	if !(posAPI < posBackend && posBackend < posCore) {
		t.Fatalf("expected POD ascending (api-gateway < backend-worker < coredns), got positions %d, %d, %d in:\n%s", posAPI, posBackend, posCore, initial)
	}

	// 1. Sort by POD descending ('p') then ascending ('p')
	ts.SendRune(panel.podsTable, 'p')
	podDesc := ts.Text()
	posCore = strings.Index(podDesc, "coredns")
	posBackend = strings.Index(podDesc, "backend-worker")
	posAPI = strings.Index(podDesc, "api-gateway")
	if !(posCore < posBackend && posBackend < posAPI) {
		t.Fatalf("expected POD descending (coredns < backend-worker < api-gateway), got:\n%s", podDesc)
	}
	ts.SendRune(panel.podsTable, 'p')

	// 2. Sort by CPU ascending ('c') then descending ('c')
	ts.SendRune(panel.podsTable, 'c')
	ts.SendRune(panel.podsTable, 'c')
	cpuDesc := ts.Text()
	posCore = strings.Index(cpuDesc, "coredns")
	posAPI = strings.Index(cpuDesc, "api-gateway")
	posBackend = strings.Index(cpuDesc, "backend-worker")
	if !(posCore < posAPI && posAPI < posBackend) {
		t.Fatalf("expected CPU descending (coredns 900m < api-gateway 350m < backend-worker 150m), got:\n%s", cpuDesc)
	}

	// 2. Sort by MEMORY ('m') ascending then descending ('m')
	ts.SendRune(panel.podsTable, 'm')
	ts.SendRune(panel.podsTable, 'm')
	memDesc := ts.Text()
	posBackend = strings.Index(memDesc, "backend-worker")
	posAPI = strings.Index(memDesc, "api-gateway")
	posCore = strings.Index(memDesc, "coredns")
	if !(posBackend < posAPI && posAPI < posCore) {
		t.Fatalf("expected MEM descending (backend-worker 800Mi < api-gateway 400Mi < coredns 120Mi), got:\n%s", memDesc)
	}

	// 3. Test filtering with '/'
	ts.SendRune(panel.podsTable, '/')
	for _, ch := range "kube" {
		ts.SendRune(panel.podsTable, ch)
	}
	ts.SendKey(panel.podsTable, tcell.KeyEnter, 0, 0)

	filtered := ts.Text()
	if !strings.Contains(filtered, "coredns") {
		t.Fatalf("expected filtered view to contain coredns, got:\n%s", filtered)
	}
	if strings.Contains(filtered, "api-gateway") || strings.Contains(filtered, "backend-worker") {
		t.Fatalf("expected filtered view NOT to contain api-gateway or backend-worker, got:\n%s", filtered)
	}

	// 4. Test ESC clearing filter
	ts.SendKey(panel.podsTable, tcell.KeyEscape, 0, 0)
	cleared := ts.Text()
	for _, name := range []string{"api-gateway", "coredns", "backend-worker"} {
		if !strings.Contains(cleared, name) {
			t.Fatalf("expected pod %q restored after Esc clears filter, got:\n%s", name, cleared)
		}
	}
}
