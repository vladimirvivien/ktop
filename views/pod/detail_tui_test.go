package pod

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodDetailPanel_ContainersTableSortAndFilter(t *testing.T) {
	ts, err := ui.NewTestScreen(140, 42)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	panel := NewDetailPanel()
	podDetailData := &model.PodDetailData{
		PodModel: &model.PodModel{
			Namespace: "default",
			Name:      "api-server-xyz",
			Status:    "Running",
			Node:      "worker-1",
		},
		Pod: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "api-server-xyz",
			},
			Spec: corev1.PodSpec{
				NodeName: "worker-1",
				Containers: []corev1.Container{
					{Name: "sidecar", Image: "envoyproxy/envoy:v1.28"},
					{Name: "app", Image: "myorg/api:v1.0"},
					{Name: "proxy", Image: "traefik/traefik:v3.0"},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "sidecar", Ready: true, RestartCount: 2, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					{Name: "app", Ready: true, RestartCount: 0, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					{Name: "proxy", Ready: true, RestartCount: 5, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				},
			},
		},
		ContainerMetrics: map[string]model.ContainerUsage{
			"sidecar": {CPUUsage: "150m", MemoryUsage: "256Mi"},
			"app":     {CPUUsage: "500m", MemoryUsage: "1Gi"},
			"proxy":   {CPUUsage: "50m", MemoryUsage: "64Mi"},
		},
	}

	panel.DrawBody(podDetailData)
	root := panel.GetRootView()
	ts.Draw(root)
	ts.Draw(root)

	initial := ts.Text()
	for _, expected := range []string{"app", "sidecar", "proxy"} {
		if !strings.Contains(initial, expected) {
			t.Fatalf("expected container %q in rendered PodDetailPanel, got:\n%s", expected, initial)
		}
	}

	// Default sort is CONTAINER ascending: app < proxy < sidecar
	posApp := strings.Index(initial, "app")
	posProxy := strings.Index(initial, "proxy")
	posSidecar := strings.Index(initial, "sidecar")
	if !(posApp < posProxy && posProxy < posSidecar) {
		t.Fatalf("expected CONTAINER ascending (app < proxy < sidecar), got pos: app=%d, proxy=%d, sidecar=%d in:\n%s",
			posApp, posProxy, posSidecar, initial)
	}

	// 1. Sort by CONTAINER descending ('o')
	ts.SendRune(panel.containersTable, 'o')
	desc := ts.Text()
	posApp = strings.Index(desc, "app")
	posProxy = strings.Index(desc, "proxy")
	posSidecar = strings.Index(desc, "sidecar")
	if !(posSidecar < posProxy && posProxy < posApp) {
		t.Fatalf("expected CONTAINER descending (sidecar < proxy < app), got pos: sidecar=%d, proxy=%d, app=%d in:\n%s",
			posSidecar, posProxy, posApp, desc)
	}

	// Reset back to ascending
	ts.SendRune(panel.containersTable, 'o')

	// 2. Sort by CPU descending ('c')
	ts.SendRune(panel.containersTable, 'c')
	ts.SendRune(panel.containersTable, 'c')
	cpuDesc := ts.Text()
	posApp = strings.Index(cpuDesc, "app")         // 500m
	posSidecar = strings.Index(cpuDesc, "sidecar") // 150m
	posProxy = strings.Index(cpuDesc, "proxy")     // 50m
	if !(posApp < posSidecar && posSidecar < posProxy) {
		t.Fatalf("expected CPU descending (app 500m < sidecar 150m < proxy 50m), got pos: app=%d, sidecar=%d, proxy=%d in:\n%s",
			posApp, posSidecar, posProxy, cpuDesc)
	}

	// 3. Sort by MEMORY descending ('m')
	ts.SendRune(panel.containersTable, 'm')
	ts.SendRune(panel.containersTable, 'm')
	memDesc := ts.Text()
	posApp = strings.Index(memDesc, "app")         // 1Gi
	posSidecar = strings.Index(memDesc, "sidecar") // 256Mi
	posProxy = strings.Index(memDesc, "proxy")     // 64Mi
	if !(posApp < posSidecar && posSidecar < posProxy) {
		t.Fatalf("expected MEM descending (app 1Gi < sidecar 256Mi < proxy 64Mi), got pos: app=%d, sidecar=%d, proxy=%d in:\n%s",
			posApp, posSidecar, posProxy, memDesc)
	}

	// 4. Test filtering with '/'
	ts.SendRune(panel.containersTable, '/')
	for _, ch := range "side" {
		ts.SendRune(panel.containersTable, ch)
	}
	ts.SendKey(panel.containersTable, tcell.KeyEnter, 0, 0)

	filtered := ts.Text()
	if !strings.Contains(filtered, "sidecar") {
		t.Fatalf("expected 'sidecar' in filtered output, got:\n%s", filtered)
	}
	if strings.Contains(filtered, "traefik") {
		t.Fatalf("expected 'traefik' (proxy) to be excluded from filtered output, got:\n%s", filtered)
	}

	// 5. ESC to clear filter
	ts.SendKey(panel.containersTable, tcell.KeyEscape, 0, 0)
	restored := ts.Text()
	if !strings.Contains(restored, "traefik") {
		t.Fatalf("expected 'traefik' (proxy) to be restored after ESC, got:\n%s", restored)
	}
}

func TestPodDetailPanel_PSISparklines(t *testing.T) {
	ts, err := ui.NewTestScreen(140, 42)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	panel := NewDetailPanel()
	podDetailData := &model.PodDetailData{
		PodModel: &model.PodModel{
			Namespace:              "default",
			Name:                   "test-psi-pod",
			NodePSIKernelSupported: true,
			PSI: metrics.PSIMetrics{
				CPUStallPct: 1.8,
				MemStallPct: 6.4,
				IOStallPct:  12.0,
			},
		},
	}

	panel.DrawBody(podDetailData)
	root := panel.GetRootView()
	ts.Draw(root)
	panel.DrawBody(podDetailData)
	ts.Draw(root)

	text := ts.Text()
	for _, expected := range []string{
		"PSI:",
		"CPU 1.8%",
		"MEM 6.4%",
		"IO 12%",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected PSI stall text %q in PodDetailPanel Info box, got:\n%s", expected, text)
		}
	}

	// Test unsupported kernel shows warning N/A
	podDetailUnsupported := &model.PodDetailData{
		PodModel: &model.PodModel{
			Namespace:              "default",
			Name:                   "test-psi-pod-unsupported",
			NodePSIKernelSupported: false,
		},
	}
	panel.DrawBody(podDetailUnsupported)
	ts.Draw(root)

	unsupportedText := ts.Text()
	if !strings.Contains(unsupportedText, "PSI: ⚠ N/A") {
		t.Fatalf("expected 'PSI: ⚠ N/A' in unsupported kernel pod detail view Info box, got:\n%s", unsupportedText)
	}
}
