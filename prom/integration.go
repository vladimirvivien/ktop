package prom

import (
	"context"
	"time"

	"k8s.io/client-go/rest"
)

// Integration provides a simplified interface for ktop to use Prometheus metrics
type Integration struct {
	controller *CollectorController
	enabled    bool
}

// NewIntegration creates a new Prometheus integration for ktop
func NewIntegration(kubeConfig *rest.Config, enabled bool) *Integration {
	if !enabled {
		return &Integration{enabled: false}
	}
	
	config := DefaultScrapeConfig()
	// Use shorter intervals for responsive UI
	config.Interval = 15 * time.Second
	config.RetentionTime = 30 * time.Minute
	config.MaxSamples = 500
	
	controller := NewCollectorController(kubeConfig, config)
	
	return &Integration{
		controller: controller,
		enabled:    true,
	}
}

// IsEnabled returns whether Prometheus integration is enabled
func (i *Integration) IsEnabled() bool {
	return i.enabled
}

// Start initializes and starts the Prometheus metrics collection
func (i *Integration) Start(ctx context.Context) error {
	if !i.enabled {
		return nil
	}
	
	return i.controller.Start(ctx)
}

// Stop stops the Prometheus metrics collection
func (i *Integration) Stop() error {
	if !i.enabled {
		return nil
	}
	
	return i.controller.Stop()
}

// NodeMetrics represents enhanced metrics for a node
type NodeMetrics struct {
	// CPU metrics
	CPUUsagePercent    float64
	CPUUsageByCore     map[string]float64
	CPUThrottlePercent float64
	CPULoadAverage     [3]float64
	
	// Memory metrics
	MemoryUsageBytes     int64
	MemoryUsagePercent   float64
	MemoryAvailableBytes int64
	MemoryCachedBytes    int64
	MemoryBuffersBytes   int64
	
	// Network metrics
	NetworkRxBytesTotal float64
	NetworkTxBytesTotal float64
	NetworkRxErrors     float64
	NetworkTxErrors     float64
	
	// Disk metrics
	DiskReadBytesTotal  float64
	DiskWriteBytesTotal float64
	DiskIOUtilization   float64
	
	// System metrics
	ProcessCount    int64
	ProcessRunning  int64
	ProcessBlocked  int64
	UptimeSeconds   float64
}

// PodMetrics represents enhanced metrics for a pod
type PodMetrics struct {
	// Container-level CPU metrics
	CPUUsagePercent   float64
	CPUThrottleCount  float64
	CPUThrottlePercent float64
	
	// Container-level Memory metrics
	MemoryUsageBytes    int64
	MemoryRSSBytes      int64
	MemoryWorkingSetBytes int64
	MemoryLimitBytes    int64
	
	// Network metrics (if available)
	NetworkRxBytes float64
	NetworkTxBytes float64
	
	// Filesystem metrics
	FilesystemUsageBytes     int64
	FilesystemAvailableBytes int64
	FilesystemCapacityBytes  int64
	
	// Container state
	RestartCount     int64
	OOMKillCount     int64
	LastSeenRunning  time.Time
}

// GetNodeMetrics retrieves enhanced metrics for a specific node
func (i *Integration) GetNodeMetrics(nodeName string) (*NodeMetrics, error) {
	if !i.enabled || !i.controller.IsRunning() {
		return nil, nil
	}
	
	metrics := &NodeMetrics{}
	
	// CPU Usage - from cAdvisor or kubelet
	if cpuUsage, err := i.controller.QueryMetric("node_cpu_usage_seconds_total", 
		map[string]string{"instance": "*" + nodeName + "*"}); err == nil {
		metrics.CPUUsagePercent = cpuUsage
	}
	
	// Memory Usage - from node_exporter or kubelet
	if memUsage, err := i.controller.QueryMetric("node_memory_MemAvailable_bytes",
		map[string]string{"instance": "*" + nodeName + "*"}); err == nil {
		metrics.MemoryAvailableBytes = int64(memUsage)
	}
	
	// Network metrics
	if netRx, err := i.controller.QueryMetric("node_network_receive_bytes_total",
		map[string]string{"instance": "*" + nodeName + "*"}); err == nil {
		metrics.NetworkRxBytesTotal = netRx
	}
	
	if netTx, err := i.controller.QueryMetric("node_network_transmit_bytes_total",
		map[string]string{"instance": "*" + nodeName + "*"}); err == nil {
		metrics.NetworkTxBytesTotal = netTx
	}
	
	// Disk I/O metrics
	if diskRead, err := i.controller.QueryMetric("node_disk_read_bytes_total",
		map[string]string{"instance": "*" + nodeName + "*"}); err == nil {
		metrics.DiskReadBytesTotal = diskRead
	}
	
	if diskWrite, err := i.controller.QueryMetric("node_disk_written_bytes_total",
		map[string]string{"instance": "*" + nodeName + "*"}); err == nil {
		metrics.DiskWriteBytesTotal = diskWrite
	}
	
	return metrics, nil
}

// GetPodMetrics retrieves enhanced metrics for a specific pod
func (i *Integration) GetPodMetrics(namespace, podName string) (*PodMetrics, error) {
	if !i.enabled || !i.controller.IsRunning() {
		return nil, nil
	}
	
	metrics := &PodMetrics{}
	
	// CPU metrics from cAdvisor
	if cpuUsage, err := i.controller.QueryMetric("container_cpu_usage_seconds_total",
		map[string]string{"namespace": namespace, "pod": podName}); err == nil {
		metrics.CPUUsagePercent = cpuUsage
	}
	
	// Memory metrics from cAdvisor
	if memUsage, err := i.controller.QueryMetric("container_memory_usage_bytes",
		map[string]string{"namespace": namespace, "pod": podName}); err == nil {
		metrics.MemoryUsageBytes = int64(memUsage)
	}
	
	if memWorkingSet, err := i.controller.QueryMetric("container_memory_working_set_bytes",
		map[string]string{"namespace": namespace, "pod": podName}); err == nil {
		metrics.MemoryWorkingSetBytes = int64(memWorkingSet)
	}
	
	// Network metrics
	if netRx, err := i.controller.QueryMetric("container_network_receive_bytes_total",
		map[string]string{"namespace": namespace, "pod": podName}); err == nil {
		metrics.NetworkRxBytes = netRx
	}
	
	if netTx, err := i.controller.QueryMetric("container_network_transmit_bytes_total",
		map[string]string{"namespace": namespace, "pod": podName}); err == nil {
		metrics.NetworkTxBytes = netTx
	}
	
	// Filesystem metrics
	if fsUsage, err := i.controller.QueryMetric("container_fs_usage_bytes",
		map[string]string{"namespace": namespace, "pod": podName}); err == nil {
		metrics.FilesystemUsageBytes = int64(fsUsage)
	}
	
	// Restart count
	if restarts, err := i.controller.QueryMetric("kube_pod_container_status_restarts_total",
		map[string]string{"namespace": namespace, "pod": podName}); err == nil {
		metrics.RestartCount = int64(restarts)
	}
	
	return metrics, nil
}

// GetClusterSummary provides cluster-wide metrics summary
func (i *Integration) GetClusterSummary() map[string]interface{} {
	if !i.enabled || !i.controller.IsRunning() {
		return map[string]interface{}{
			"prometheus_enabled": false,
		}
	}
	
	summary := map[string]interface{}{
		"prometheus_enabled": true,
	}
	
	// Add controller stats
	if stats := i.controller.GetStats(); stats != nil {
		summary["controller_stats"] = stats
	}
	
	// Add available metrics count
	if store := i.controller.GetStore(); store != nil {
		metricNames := store.GetMetricNames()
		summary["total_metrics"] = len(metricNames)
		summary["sample_metric_names"] = metricNames[:minInt(10, len(metricNames))]
	}
	
	return summary
}

// GetAvailableComponents returns the list of available Prometheus-scrapable components
func (i *Integration) GetAvailableComponents() []string {
	if !i.enabled {
		return []string{}
	}
	
	components := i.controller.GetAvailableComponents()
	result := make([]string, len(components))
	for i, comp := range components {
		result[i] = string(comp)
	}
	
	return result
}

// GetTrends returns historical trends for key metrics
func (i *Integration) GetNodeTrends(nodeName string, duration time.Duration) map[string][]*MetricSample {
	if !i.enabled || !i.controller.IsRunning() {
		return nil
	}
	
	trends := make(map[string][]*MetricSample)
	
	// CPU trend
	if samples, err := i.controller.QueryMetricRange("node_cpu_usage_seconds_total",
		map[string]string{"instance": "*" + nodeName + "*"}, duration); err == nil {
		trends["cpu"] = samples
	}
	
	// Memory trend
	if samples, err := i.controller.QueryMetricRange("node_memory_MemAvailable_bytes",
		map[string]string{"instance": "*" + nodeName + "*"}, duration); err == nil {
		trends["memory"] = samples
	}
	
	return trends
}

// IsHealthy returns whether the Prometheus integration is healthy
func (i *Integration) IsHealthy() bool {
	if !i.enabled {
		return true // Not enabled is considered "healthy"
	}
	
	return i.controller.IsRunning() && i.controller.GetLastError() == nil
}

// GetLastError returns the last error from the Prometheus integration
func (i *Integration) GetLastError() error {
	if !i.enabled {
		return nil
	}
	
	return i.controller.GetLastError()
}

// helper function
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}