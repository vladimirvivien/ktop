package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// MetricsServerSource implements metrics.MetricsSource using the Kubernetes Metrics Server.
// This source provides basic CPU and memory metrics only.
type MetricsServerSource struct {
	metricsClient *metricsclient.Clientset
	healthy       bool
	mu            sync.RWMutex
	lastError     error
	errorCount    int
}

// NewMetricsServerSource creates a new MetricsServerSource wrapping the k8s.Controller.
func NewMetricsServerSource(controller *k8s.Controller) *MetricsServerSource {
	var metricsClient *metricsclient.Clientset
	if controller != nil {
		if client := controller.GetClient(); client != nil {
			metricsClient = client.GetMetricsClient()
		}
	}

	return &MetricsServerSource{
		metricsClient: metricsClient,
		healthy:       false, // Start unhealthy, will become healthy on first successful fetch
	}
}

// GetNodeMetrics retrieves metrics for a specific node from the Metrics Server.
func (m *MetricsServerSource) GetNodeMetrics(ctx context.Context, nodeName string) (*metrics.NodeMetrics, error) {
	// Call Metrics Server API directly to avoid circular dependency
	if m.metricsClient == nil {
		m.recordError(fmt.Errorf("metrics client not available"))
		return nil, fmt.Errorf("metrics client not available")
	}

	nodeMetrics, err := m.metricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		m.recordError(err)
		return nil, fmt.Errorf("metrics server: %w", err)
	}

	m.recordSuccess()
	return convertNodeMetrics(nodeMetrics), nil
}

// GetPodMetrics retrieves metrics for a specific pod by namespace and name.
func (m *MetricsServerSource) GetPodMetrics(ctx context.Context, namespace, podName string) (*metrics.PodMetrics, error) {
	// Call Metrics Server API directly
	if m.metricsClient == nil {
		m.recordError(fmt.Errorf("metrics client not available"))
		return nil, fmt.Errorf("metrics client not available")
	}

	podMetrics, err := m.metricsClient.MetricsV1beta1().PodMetricses(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		// Metrics Server unavailable - return error so caller can handle graceful degradation
		m.recordError(err)
		return nil, fmt.Errorf("metrics server unavailable: %w", err)
	}

	m.recordSuccess()
	return convertPodMetrics(podMetrics), nil
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
	// Call Metrics Server API directly
	if m.metricsClient == nil {
		m.recordError(fmt.Errorf("metrics client not available"))
		return nil, fmt.Errorf("metrics client not available")
	}

	podMetricsList, err := m.metricsClient.MetricsV1beta1().PodMetricses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		m.recordError(err)
		return nil, fmt.Errorf("metrics server: %w", err)
	}

	m.recordSuccess()

	result := make([]*metrics.PodMetrics, 0, len(podMetricsList.Items))
	for i := range podMetricsList.Items {
		result = append(result, convertPodMetrics(&podMetricsList.Items[i]))
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
