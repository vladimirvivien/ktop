package metrics

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MetricsSource defines the interface for retrieving metrics from different sources.
// Implementations include Prometheus-based scraping and Kubernetes Metrics Server.
// This abstraction allows ktop to support multiple metrics backends with a unified API.
type MetricsSource interface {
	// GetNodeMetrics retrieves metrics for a specific node.
	// Returns NodeMetrics containing CPU, memory, and optionally network, load, and other metrics.
	GetNodeMetrics(ctx context.Context, nodeName string) (*NodeMetrics, error)

	// GetPodMetrics retrieves metrics for a specific pod by namespace and name.
	// Returns PodMetrics containing per-container resource usage.
	GetPodMetrics(ctx context.Context, namespace, podName string) (*PodMetrics, error)

	// GetMetricsForPod retrieves metrics for a specific pod object.
	// This method exists for compatibility with existing code that passes v1.Pod objects.
	// Implementations may delegate to GetPodMetrics(pod.GetNamespace(), pod.GetName()).
	GetMetricsForPod(ctx context.Context, pod metav1.Object) (*PodMetrics, error)

	// GetAllPodMetrics retrieves metrics for all pods across all namespaces.
	// Returns a slice of PodMetrics for all monitored pods.
	GetAllPodMetrics(ctx context.Context) ([]*PodMetrics, error)

	// GetAvailableMetrics returns a list of metric names available from this source.
	// For Metrics Server: ["cpu", "memory"]
	// For Prometheus: ["cpu", "memory", "network_rx", "network_tx", "load", "disk", ...]
	GetAvailableMetrics() []string

	// IsHealthy returns true if the metrics source is operational and returning data.
	// Used for health indicators in the UI.
	IsHealthy() bool

	// GetSourceInfo returns metadata about the metrics source.
	// Includes source type, version, last scrape time, and error counts.
	GetSourceInfo() SourceInfo

	// SetHealthCallback registers a callback for health state changes.
	// The callback is invoked whenever IsHealthy() would return a different value.
	// This enables event-driven health monitoring instead of polling.
	// Pass nil to unregister the callback.
	SetHealthCallback(callback func(healthy bool, info SourceInfo))

	// GetNodeHistory retrieves historical data for a specific resource on a node.
	// Returns ResourceHistory with data points spanning the requested duration.
	// For Prometheus: queries from stored time series data
	// For Metrics Server: returns data from local ring buffer (limited history)
	GetNodeHistory(ctx context.Context, nodeName string, query HistoryQuery) (*ResourceHistory, error)

	// GetPodHistory retrieves historical data for a specific resource on a pod.
	// Returns ResourceHistory with data points spanning the requested duration.
	// For Prometheus: queries from stored time series data
	// For Metrics Server: returns data from local ring buffer (limited history)
	GetPodHistory(ctx context.Context, namespace, podName string, query HistoryQuery) (*ResourceHistory, error)

	// SupportsHistory returns true if this source supports historical data queries.
	// Prometheus sources always return true.
	// Metrics Server sources return true only if local buffering is enabled.
	SupportsHistory() bool
}

// NodeMetrics represents resource usage metrics for a Kubernetes node.
// Contains both basic metrics (CPU, memory) available from all sources,
// and enhanced metrics (network, load, disk) available only from Prometheus.
type NodeMetrics struct {
	// NodeName is the name of the Kubernetes node
	NodeName string

	// Timestamp when these metrics were collected
	Timestamp time.Time

	// Basic metrics (available from all sources)

	// CPUUsage is the current CPU usage in cores (e.g., 0.5 = 500 millicores)
	CPUUsage *resource.Quantity

	// MemoryUsage is the current memory usage in bytes
	MemoryUsage *resource.Quantity

	// Enhanced metrics (Prometheus only, nil if unavailable)

	// NetworkRxBytes is cumulative network bytes received
	NetworkRxBytes *resource.Quantity

	// NetworkTxBytes is cumulative network bytes transmitted
	NetworkTxBytes *resource.Quantity

	// DiskUsage is the current disk usage in bytes
	DiskUsage *resource.Quantity

	// LoadAverage1m is the 1-minute load average
	LoadAverage1m float64

	// LoadAverage5m is the 5-minute load average
	LoadAverage5m float64

	// LoadAverage15m is the 15-minute load average
	LoadAverage15m float64

	// PodCount is the number of running pods on this node
	PodCount int

	// ContainerCount is the total number of containers running on this node
	ContainerCount int
}

// PodMetrics represents resource usage metrics for a Kubernetes pod.
// Contains per-container breakdowns when available.
type PodMetrics struct {
	// PodName is the name of the pod
	PodName string

	// Namespace is the namespace containing the pod
	Namespace string

	// Timestamp when these metrics were collected
	Timestamp time.Time

	// Containers contains metrics for each container in the pod
	Containers []ContainerMetrics
}

// ContainerMetrics represents resource usage metrics for a single container.
// Includes CPU and memory usage, plus enhanced metrics like throttling when available.
type ContainerMetrics struct {
	// Name is the container name
	Name string

	// CPUUsage is the current CPU usage in cores
	CPUUsage *resource.Quantity

	// MemoryUsage is the current memory usage in bytes
	MemoryUsage *resource.Quantity

	// Enhanced metrics (Prometheus only, zero/nil if unavailable)

	// CPUThrottled is the percentage of time CPU was throttled (0.0 - 100.0)
	CPUThrottled float64

	// CPULimit is the CPU limit configured for this container
	CPULimit *resource.Quantity

	// MemoryLimit is the memory limit configured for this container
	MemoryLimit *resource.Quantity

	// RestartCount is the number of times this container has restarted
	RestartCount int
}

// SourceInfo provides metadata about a metrics source.
// Used for debugging, health monitoring, and UI indicators.
type SourceInfo struct {
	// Type identifies the source type (e.g., "prometheus", "metrics-server")
	Type string

	// Version is the version of the metrics source (if available)
	Version string

	// LastScrape is the timestamp of the last successful metrics collection
	LastScrape time.Time

	// MetricsCount is the total number of metrics available from this source
	MetricsCount int

	// ErrorCount is the number of errors encountered since startup
	ErrorCount int

	// Healthy indicates if the source is currently operational
	Healthy bool
}

// SourceType constants for common metrics sources
const (
	SourceTypePrometheus    = "prometheus"
	SourceTypeMetricsServer = "metrics-server"
)

// ResourceType identifies the type of resource for history queries
type ResourceType string

const (
	ResourceCPU    ResourceType = "cpu"
	ResourceMemory ResourceType = "memory"
)

// HistoryDataPoint represents a single data point in a time series
type HistoryDataPoint struct {
	// Timestamp when this value was recorded
	Timestamp time.Time
	// Value is the metric value at this timestamp (CPU in millicores, memory in bytes)
	Value float64
}

// ResourceHistory contains historical data points for a specific resource
type ResourceHistory struct {
	// Resource identifies what metric this history is for
	Resource ResourceType
	// DataPoints contains the historical values, ordered from oldest to newest
	DataPoints []HistoryDataPoint
	// MinValue is the minimum value in the data points (for scaling)
	MinValue float64
	// MaxValue is the maximum value in the data points (for scaling)
	MaxValue float64
}

// HistoryQuery specifies parameters for querying resource history
type HistoryQuery struct {
	// Resource is the type of resource to query
	Resource ResourceType
	// Duration is how far back to look (e.g., 5*time.Minute)
	Duration time.Duration
	// MaxPoints limits the number of data points returned (0 = no limit)
	MaxPoints int
}
