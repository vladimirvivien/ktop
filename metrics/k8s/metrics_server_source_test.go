package k8s

import (
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func TestConvertNodeMetrics(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))

	nm := &metricsv1beta1.NodeMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Timestamp: timestamp,
		Usage: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("500m"),
			v1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}

	result := convertNodeMetrics(nm)

	if result.NodeName != "test-node" {
		t.Errorf("Expected NodeName 'test-node', got '%s'", result.NodeName)
	}

	if result.Timestamp != timestamp.Time {
		t.Errorf("Expected Timestamp %v, got %v", timestamp.Time, result.Timestamp)
	}

	if result.CPUUsage == nil {
		t.Fatal("Expected CPUUsage to be set")
	}
	if result.CPUUsage.MilliValue() != 500 {
		t.Errorf("Expected CPU 500m, got %v", result.CPUUsage.MilliValue())
	}

	if result.MemoryUsage == nil {
		t.Fatal("Expected MemoryUsage to be set")
	}
	expectedMem := int64(2 * 1024 * 1024 * 1024) // 2Gi in bytes
	if result.MemoryUsage.Value() != expectedMem {
		t.Errorf("Expected Memory %d, got %d", expectedMem, result.MemoryUsage.Value())
	}

	// Enhanced metrics should be nil/zero for Metrics Server
	if result.NetworkRxBytes != nil {
		t.Errorf("Expected NetworkRxBytes to be nil, got %v", result.NetworkRxBytes)
	}
	if result.NetworkTxBytes != nil {
		t.Errorf("Expected NetworkTxBytes to be nil, got %v", result.NetworkTxBytes)
	}
	if result.DiskUsage != nil {
		t.Errorf("Expected DiskUsage to be nil, got %v", result.DiskUsage)
	}
	if result.LoadAverage1m != 0 {
		t.Errorf("Expected LoadAverage1m to be 0, got %f", result.LoadAverage1m)
	}
	if result.PodCount != 0 {
		t.Errorf("Expected PodCount to be 0, got %d", result.PodCount)
	}
	if result.ContainerCount != 0 {
		t.Errorf("Expected ContainerCount to be 0, got %d", result.ContainerCount)
	}
}

func TestConvertPodMetrics(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))

	pm := &metricsv1beta1.PodMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
		Timestamp: timestamp,
		Containers: []metricsv1beta1.ContainerMetrics{
			{
				Name: "container-1",
				Usage: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("100m"),
					v1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
			{
				Name: "container-2",
				Usage: v1.ResourceList{
					v1.ResourceCPU:    resource.MustParse("200m"),
					v1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
		},
	}

	result := convertPodMetrics(pm)

	if result.PodName != "test-pod" {
		t.Errorf("Expected PodName 'test-pod', got '%s'", result.PodName)
	}

	if result.Namespace != "test-namespace" {
		t.Errorf("Expected Namespace 'test-namespace', got '%s'", result.Namespace)
	}

	if result.Timestamp != timestamp.Time {
		t.Errorf("Expected Timestamp %v, got %v", timestamp.Time, result.Timestamp)
	}

	if len(result.Containers) != 2 {
		t.Fatalf("Expected 2 containers, got %d", len(result.Containers))
	}

	// Check first container
	c1 := result.Containers[0]
	if c1.Name != "container-1" {
		t.Errorf("Expected container name 'container-1', got '%s'", c1.Name)
	}
	if c1.CPUUsage == nil || c1.CPUUsage.MilliValue() != 100 {
		t.Errorf("Expected CPU 100m for container-1")
	}
	if c1.MemoryUsage == nil {
		t.Fatal("Expected MemoryUsage to be set for container-1")
	}

	// Check second container
	c2 := result.Containers[1]
	if c2.Name != "container-2" {
		t.Errorf("Expected container name 'container-2', got '%s'", c2.Name)
	}
	if c2.CPUUsage == nil || c2.CPUUsage.MilliValue() != 200 {
		t.Errorf("Expected CPU 200m for container-2")
	}
}

func TestConvertContainerMetrics(t *testing.T) {
	cm := &metricsv1beta1.ContainerMetrics{
		Name: "test-container",
		Usage: v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("250m"),
			v1.ResourceMemory: resource.MustParse("768Mi"),
		},
	}

	result := convertContainerMetrics(cm)

	if result.Name != "test-container" {
		t.Errorf("Expected Name 'test-container', got '%s'", result.Name)
	}

	if result.CPUUsage == nil {
		t.Fatal("Expected CPUUsage to be set")
	}
	if result.CPUUsage.MilliValue() != 250 {
		t.Errorf("Expected CPU 250m, got %v", result.CPUUsage.MilliValue())
	}

	if result.MemoryUsage == nil {
		t.Fatal("Expected MemoryUsage to be set")
	}

	// Enhanced metrics should be nil/zero
	if result.CPUThrottled != 0 {
		t.Errorf("Expected CPUThrottled to be 0, got %f", result.CPUThrottled)
	}
	if result.CPULimit != nil {
		t.Errorf("Expected CPULimit to be nil, got %v", result.CPULimit)
	}
	if result.MemoryLimit != nil {
		t.Errorf("Expected MemoryLimit to be nil, got %v", result.MemoryLimit)
	}
	if result.RestartCount != 0 {
		t.Errorf("Expected RestartCount to be 0, got %d", result.RestartCount)
	}
}

func TestMetricsServerSource_GetAvailableMetrics(t *testing.T) {
	source := NewMetricsServerSource(nil)

	metrics := source.GetAvailableMetrics()

	if len(metrics) != 2 {
		t.Fatalf("Expected 2 metrics, got %d", len(metrics))
	}

	expected := map[string]bool{"cpu": true, "memory": true}
	for _, m := range metrics {
		if !expected[m] {
			t.Errorf("Unexpected metric: %s", m)
		}
		delete(expected, m)
	}

	if len(expected) > 0 {
		t.Errorf("Missing expected metrics: %v", expected)
	}
}

func TestMetricsServerSource_HealthTracking(t *testing.T) {
	source := NewMetricsServerSource(nil)

	// Initially healthy
	if !source.IsHealthy() {
		t.Error("Expected source to be initially healthy")
	}

	info := source.GetSourceInfo()
	if info.Type != "metrics-server" {
		t.Errorf("Expected Type 'metrics-server', got '%s'", info.Type)
	}
	if info.Version != "v1beta1" {
		t.Errorf("Expected Version 'v1beta1', got '%s'", info.Version)
	}
	if info.MetricsCount != 2 {
		t.Errorf("Expected MetricsCount 2, got %d", info.MetricsCount)
	}
	if info.ErrorCount != 0 {
		t.Errorf("Expected ErrorCount 0, got %d", info.ErrorCount)
	}
	if !info.Healthy {
		t.Error("Expected SourceInfo.Healthy to be true")
	}

	// Record an error
	source.recordError(nil)

	if source.IsHealthy() {
		t.Error("Expected source to be unhealthy after error")
	}

	info = source.GetSourceInfo()
	if info.ErrorCount != 1 {
		t.Errorf("Expected ErrorCount 1, got %d", info.ErrorCount)
	}
	if info.Healthy {
		t.Error("Expected SourceInfo.Healthy to be false after error")
	}

	// Record success
	source.recordSuccess()

	if !source.IsHealthy() {
		t.Error("Expected source to be healthy after success")
	}

	info = source.GetSourceInfo()
	if !info.Healthy {
		t.Error("Expected SourceInfo.Healthy to be true after success")
	}
}

func TestMetricsServerSource_New(t *testing.T) {
	source := NewMetricsServerSource(nil)

	if source == nil {
		t.Fatal("Expected NewMetricsServerSource to return non-nil")
	}

	if source.controller != nil {
		t.Error("Expected controller to be nil when passed nil")
	}

	if !source.healthy {
		t.Error("Expected new source to be healthy")
	}

	if source.errorCount != 0 {
		t.Errorf("Expected errorCount to be 0, got %d", source.errorCount)
	}
}
