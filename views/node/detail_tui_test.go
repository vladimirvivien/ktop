package node

import (
	"testing"

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
