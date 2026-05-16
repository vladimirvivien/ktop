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

	// GetPodNetworkDiskMetrics retrieves network and disk I/O rates for a specific pod.
	// Network metrics are at the pod level (shared network namespace).
	// Disk metrics are aggregated across containers in the pod.
	// Returns zero values if metrics are not available (e.g., for metrics-server source).
	GetPodNetworkDiskMetrics(ctx context.Context, namespace, podName string) (netRx, netTx, diskRead, diskWrite float64, err error)

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

// PSIMetrics holds Pressure Stall Information for a node, pod, or container.
// Values are percentages [0..100] of wall time spent stalled on the resource
// during the most recent scrape window.
//
// A zero value is indistinguishable at this layer between a genuinely idle
// cgroup and a host kernel that does not expose PSI (kernel <4.20, cgroup v1,
// or an unpatched kubelet hitting kubernetes/kubernetes#136333). Callers that
// need to interpret zeros should consult the node's kernel version and the
// cluster's server version separately.
type PSIMetrics struct {
	// "Waiting" axis — at least one task in the cgroup blocked on the
	// resource. This is the value rendered in the STALL column.
	CPUStallPct float64
	MemStallPct float64
	IOStallPct  float64

	// "Stalled" axis — every task blocked. Not every kernel emits the
	// CPU-stalled counter; absent values arrive here as zero.
	CPUStalledPct float64
	MemStalledPct float64
	IOStalledPct  float64
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

	// Network rates (bytes/sec) - calculated from counter deltas
	NetworkRxRate float64
	NetworkTxRate float64

	// Disk I/O rates (bytes/sec) - calculated from counter deltas
	DiskReadRate  float64
	DiskWriteRate float64

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

	// PSI captures resource pressure for the node's root cgroup.
	// Zero when the source does not provide PSI (e.g., metrics-server).
	PSI PSIMetrics
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

	// PSI captures the worst-affected axis across containers in the pod.
	// Computed as max(container.PSI.*) per axis — a pod with any heavily
	// stalled container surfaces that container's stall percentage.
	PSI PSIMetrics
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

	// PSI captures resource pressure for this container's cgroup.
	// Zero when the source does not provide PSI.
	PSI PSIMetrics
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
