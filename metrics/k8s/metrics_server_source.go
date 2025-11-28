package k8s

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/prom"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// historyDataPoint is a simple struct for storing historical values
type historyDataPoint struct {
	timestamp int64   // Unix milliseconds
	value     float64 // Metric value
}

// MetricsServerSource implements metrics.MetricsSource using the Kubernetes Metrics Server.
// This source provides basic CPU and memory metrics only, but maintains local ring buffers
// for historical data to support sparklines and trends.
type MetricsServerSource struct {
	metricsClient *metricsclient.Clientset
	healthy       bool
	mu            sync.RWMutex
	lastError     error
	errorCount    int

	// History buffers for sparkline support
	// Key format: "node:{nodeName}:{resource}" or "pod:{namespace}/{podName}:{resource}"
	historyBuffers map[string]*prom.RingBuffer[historyDataPoint]
	historyMu      sync.RWMutex
	maxHistorySamples int
}

// DefaultMaxHistorySamples is the default number of historical samples to keep
const DefaultMaxHistorySamples = 120 // ~10 minutes at 5s scrape interval

// NewMetricsServerSource creates a new MetricsServerSource wrapping the k8s.Controller.
func NewMetricsServerSource(controller *k8s.Controller) *MetricsServerSource {
	var metricsClient *metricsclient.Clientset
	if controller != nil {
		if client := controller.GetClient(); client != nil {
			metricsClient = client.GetMetricsClient()
		}
	}

	return &MetricsServerSource{
		metricsClient:     metricsClient,
		healthy:           false, // Start unhealthy, will become healthy on first successful fetch
		historyBuffers:    make(map[string]*prom.RingBuffer[historyDataPoint]),
		maxHistorySamples: DefaultMaxHistorySamples,
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
	result := convertNodeMetrics(nodeMetrics)

	// Record history for CPU and memory
	now := time.Now().UnixMilli()
	if result.CPUUsage != nil {
		m.recordHistory(fmt.Sprintf("node:%s:cpu", nodeName), now, float64(result.CPUUsage.MilliValue()))
	}
	if result.MemoryUsage != nil {
		m.recordHistory(fmt.Sprintf("node:%s:memory", nodeName), now, float64(result.MemoryUsage.Value()))
	}

	return result, nil
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
	result := convertPodMetrics(podMetrics)

	// Record aggregated history for CPU and memory across all containers
	now := time.Now().UnixMilli()
	var totalCPU, totalMem int64
	for _, c := range result.Containers {
		if c.CPUUsage != nil {
			totalCPU += c.CPUUsage.MilliValue()
		}
		if c.MemoryUsage != nil {
			totalMem += c.MemoryUsage.Value()
		}
	}

	key := fmt.Sprintf("pod:%s/%s", namespace, podName)
	m.recordHistory(key+":cpu", now, float64(totalCPU))
	m.recordHistory(key+":memory", now, float64(totalMem))

	return result, nil
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

// recordHistory stores a data point in the history buffer for the given key
func (m *MetricsServerSource) recordHistory(key string, timestamp int64, value float64) {
	m.historyMu.Lock()
	defer m.historyMu.Unlock()

	buffer, exists := m.historyBuffers[key]
	if !exists {
		buffer = prom.NewRingBuffer[historyDataPoint](m.maxHistorySamples)
		m.historyBuffers[key] = buffer
	}

	buffer.Add(historyDataPoint{
		timestamp: timestamp,
		value:     value,
	})
}

// GetNodeHistory retrieves historical data for a specific resource on a node.
// For Metrics Server, this queries the local ring buffer maintained since startup.
func (m *MetricsServerSource) GetNodeHistory(ctx context.Context, nodeName string, query metrics.HistoryQuery) (*metrics.ResourceHistory, error) {
	var suffix string
	switch query.Resource {
	case metrics.ResourceCPU:
		suffix = "cpu"
	case metrics.ResourceMemory:
		suffix = "memory"
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", query.Resource)
	}

	key := fmt.Sprintf("node:%s:%s", nodeName, suffix)
	return m.getHistoryFromBuffer(key, query)
}

// GetPodHistory retrieves historical data for a specific resource on a pod.
// For Metrics Server, this queries the local ring buffer maintained since startup.
func (m *MetricsServerSource) GetPodHistory(ctx context.Context, namespace, podName string, query metrics.HistoryQuery) (*metrics.ResourceHistory, error) {
	var suffix string
	switch query.Resource {
	case metrics.ResourceCPU:
		suffix = "cpu"
	case metrics.ResourceMemory:
		suffix = "memory"
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", query.Resource)
	}

	key := fmt.Sprintf("pod:%s/%s:%s", namespace, podName, suffix)
	return m.getHistoryFromBuffer(key, query)
}

// getHistoryFromBuffer retrieves history data from the ring buffer
func (m *MetricsServerSource) getHistoryFromBuffer(key string, query metrics.HistoryQuery) (*metrics.ResourceHistory, error) {
	m.historyMu.RLock()
	defer m.historyMu.RUnlock()

	buffer, exists := m.historyBuffers[key]
	if !exists || buffer.IsEmpty() {
		return &metrics.ResourceHistory{
			Resource:   query.Resource,
			DataPoints: []metrics.HistoryDataPoint{},
			MinValue:   0,
			MaxValue:   0,
		}, nil
	}

	history := &metrics.ResourceHistory{
		Resource:   query.Resource,
		DataPoints: make([]metrics.HistoryDataPoint, 0, buffer.Len()),
		MinValue:   math.MaxFloat64,
		MaxValue:   -math.MaxFloat64,
	}

	// Filter by time range
	cutoffMs := time.Now().Add(-query.Duration).UnixMilli()

	buffer.Range(func(idx int, dp historyDataPoint) bool {
		if dp.timestamp >= cutoffMs {
			history.DataPoints = append(history.DataPoints, metrics.HistoryDataPoint{
				Timestamp: time.UnixMilli(dp.timestamp),
				Value:     dp.value,
			})

			if dp.value < history.MinValue {
				history.MinValue = dp.value
			}
			if dp.value > history.MaxValue {
				history.MaxValue = dp.value
			}
		}
		return true // continue iteration
	})

	// Apply MaxPoints limit if specified
	if query.MaxPoints > 0 && len(history.DataPoints) > query.MaxPoints {
		history.DataPoints = downsampleHistoryPoints(history.DataPoints, query.MaxPoints)
	}

	// Reset min/max if no data points
	if len(history.DataPoints) == 0 {
		history.MinValue = 0
		history.MaxValue = 0
	}

	return history, nil
}

// SupportsHistory returns true since we maintain local ring buffers
func (m *MetricsServerSource) SupportsHistory() bool {
	return true
}

// downsampleHistoryPoints reduces the number of data points by averaging
func downsampleHistoryPoints(points []metrics.HistoryDataPoint, maxPoints int) []metrics.HistoryDataPoint {
	if len(points) <= maxPoints {
		return points
	}

	result := make([]metrics.HistoryDataPoint, maxPoints)
	bucketSize := float64(len(points)) / float64(maxPoints)

	for i := 0; i < maxPoints; i++ {
		startIdx := int(float64(i) * bucketSize)
		endIdx := int(float64(i+1) * bucketSize)
		if endIdx > len(points) {
			endIdx = len(points)
		}

		var sum float64
		var count int
		var lastTimestamp time.Time
		for j := startIdx; j < endIdx; j++ {
			sum += points[j].Value
			lastTimestamp = points[j].Timestamp
			count++
		}

		if count > 0 {
			result[i] = metrics.HistoryDataPoint{
				Timestamp: lastTimestamp,
				Value:     sum / float64(count),
			}
		}
	}

	return result
}
