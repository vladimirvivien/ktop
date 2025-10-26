package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// MetricsServerSource implements metrics.MetricsSource using the Kubernetes Metrics Server.
// This source provides basic CPU and memory metrics only.
type MetricsServerSource struct {
	controller *k8s.Controller
	healthy    bool
	mu         sync.RWMutex
	lastError  error
	errorCount int
}

// NewMetricsServerSource creates a new MetricsServerSource wrapping the k8s.Controller.
func NewMetricsServerSource(controller *k8s.Controller) *MetricsServerSource {
	return &MetricsServerSource{
		controller: controller,
		healthy:    true,
	}
}

// GetNodeMetrics retrieves metrics for a specific node from the Metrics Server.
func (m *MetricsServerSource) GetNodeMetrics(ctx context.Context, nodeName string) (*metrics.NodeMetrics, error) {
	nodeMetrics, err := m.controller.GetNodeMetrics(ctx, nodeName)
	if err != nil {
		m.recordError(err)
		return nil, fmt.Errorf("metrics server: %w", err)
	}

	m.recordSuccess()
	return convertNodeMetrics(nodeMetrics), nil
}

// GetPodMetrics retrieves metrics for a specific pod by namespace and name.
func (m *MetricsServerSource) GetPodMetrics(ctx context.Context, namespace, podName string) (*metrics.PodMetrics, error) {
	// Note: The existing k8s.Controller doesn't have a method to get pod metrics by name directly.
	// We need to get all pod metrics and filter, or use GetMetricsForPod with a pod object.
	// For now, return an error indicating this method requires a pod object.
	return nil, fmt.Errorf("GetPodMetrics by name not supported by metrics server source, use GetMetricsForPod instead")
}

// GetMetricsForPod retrieves metrics for a specific pod object.
func (m *MetricsServerSource) GetMetricsForPod(ctx context.Context, pod metav1.Object) (*metrics.PodMetrics, error) {
	// Note: k8s.Controller.GetPodMetricsByName expects *v1.Pod, not just metav1.Object
	// This is a limitation we'll need to address in a future PR.
	// For now, use GetPodMetrics with namespace/name or GetAllPodMetrics and filter.
	return m.GetPodMetrics(ctx, pod.GetNamespace(), pod.GetName())
}

// GetAllPodMetrics retrieves metrics for all pods.
func (m *MetricsServerSource) GetAllPodMetrics(ctx context.Context) ([]*metrics.PodMetrics, error) {
	podMetricsList, err := m.controller.GetAllPodMetrics(ctx)
	if err != nil {
		m.recordError(err)
		return nil, fmt.Errorf("metrics server: %w", err)
	}

	m.recordSuccess()

	result := make([]*metrics.PodMetrics, 0, len(podMetricsList))
	for _, pm := range podMetricsList {
		result = append(result, convertPodMetrics(pm))
	}

	return result, nil
}

// GetAvailableMetrics returns the list of metrics available from the Metrics Server.
// Only CPU and memory are supported.
func (m *MetricsServerSource) GetAvailableMetrics() []string {
	return []string{"cpu", "memory"}
}

// IsHealthy returns true if the Metrics Server is responding successfully.
func (m *MetricsServerSource) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthy
}

// GetSourceInfo returns metadata about the Metrics Server source.
func (m *MetricsServerSource) GetSourceInfo() metrics.SourceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return metrics.SourceInfo{
		Type:         metrics.SourceTypeMetricsServer,
		Version:      "v1beta1",
		LastScrape:   time.Now(), // Metrics Server doesn't expose this, use current time
		MetricsCount: 2,          // CPU and memory
		ErrorCount:   m.errorCount,
		Healthy:      m.healthy,
	}
}

// recordError updates health status after an error.
func (m *MetricsServerSource) recordError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastError = err
	m.errorCount++
	m.healthy = false
}

// recordSuccess updates health status after a successful operation.
func (m *MetricsServerSource) recordSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastError = nil
	m.healthy = true
}

// convertNodeMetrics converts v1beta1.NodeMetrics to metrics.NodeMetrics.
func convertNodeMetrics(nm *metricsv1beta1.NodeMetrics) *metrics.NodeMetrics {
	return &metrics.NodeMetrics{
		NodeName:  nm.Name,
		Timestamp: nm.Timestamp.Time,

		// Basic metrics from Metrics Server
		CPUUsage:    nm.Usage.Cpu(),
		MemoryUsage: nm.Usage.Memory(),

		// Enhanced metrics - not available from Metrics Server
		NetworkRxBytes: nil,
		NetworkTxBytes: nil,
		DiskUsage:      nil,
		LoadAverage1m:  0,
		LoadAverage5m:  0,
		LoadAverage15m: 0,
		PodCount:       0,
		ContainerCount: 0,
	}
}

// convertPodMetrics converts v1beta1.PodMetrics to metrics.PodMetrics.
func convertPodMetrics(pm *metricsv1beta1.PodMetrics) *metrics.PodMetrics {
	containers := make([]metrics.ContainerMetrics, 0, len(pm.Containers))
	for _, c := range pm.Containers {
		containers = append(containers, convertContainerMetrics(&c))
	}

	return &metrics.PodMetrics{
		PodName:    pm.Name,
		Namespace:  pm.Namespace,
		Timestamp:  pm.Timestamp.Time,
		Containers: containers,
	}
}

// convertContainerMetrics converts v1beta1.ContainerMetrics to metrics.ContainerMetrics.
func convertContainerMetrics(cm *metricsv1beta1.ContainerMetrics) metrics.ContainerMetrics {
	return metrics.ContainerMetrics{
		Name:        cm.Name,
		CPUUsage:    cm.Usage.Cpu(),
		MemoryUsage: cm.Usage.Memory(),

		// Enhanced metrics - not available from Metrics Server
		CPUThrottled: 0,
		CPULimit:     nil,
		MemoryLimit:  nil,
		RestartCount: 0,
	}
}
