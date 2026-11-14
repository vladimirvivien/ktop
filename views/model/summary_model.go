package model

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ClusterSummary struct {
	Uptime                  metav1.Time // oldest running node
	NodesReady              int
	NodesCount              int
	Namespaces              int
	PodsRunning             int
	PodsAvailable           int
	Pressures               int
	ImagesCount             int
	VolumesAttached         int
	VolumesInUse            int
	JobsCount               int
	CronJobsCount           int
	StatefulSetsReady       int
	DeploymentsTotal        int
	DeploymentsReady        int
	DaemonSetsDesired       int
	DaemonSetsReady         int
	ReplicaSetsReady        int
	ReplicaSetsDesired      int
	AllocatableNodeCpuTotal *resource.Quantity
	AllocatableNodeMemTotal *resource.Quantity
	RequestedPodCpuTotal    *resource.Quantity
	RequestedPodMemTotal    *resource.Quantity
	UsageNodeCpuTotal       *resource.Quantity
	UsageNodeMemTotal       *resource.Quantity
	PVCount                 int
	PVsTotal                *resource.Quantity
	PVCCount                int
	PVCsTotal               *resource.Quantity

	// === Prometheus-Enhanced Fields ===

	// Metrics source type for dynamic layout
	MetricsSourceType string // "prometheus" or "metrics-server"

	// Container stats
	ContainerCount           int // Total running containers (pod status)
	KubeletRunningPods       int // Native kubelet_running_pods sum across nodes
	KubeletRunningContainers int // Native kubelet container_count sum across nodes

	// HealthScore is a composite 0-100 cluster health score.
	HealthScore int

	// Network I/O (aggregated across nodes)
	NetworkRxBytes *resource.Quantity
	NetworkTxBytes *resource.Quantity
	NetworkRxRate  float64 // Bytes/sec received
	NetworkTxRate  float64 // Bytes/sec transmitted

	// Disk I/O (aggregated across nodes)
	DiskReadBytes  *resource.Quantity
	DiskWriteBytes *resource.Quantity
	DiskReadRate   float64 // Bytes/sec read
	DiskWriteRate  float64 // Bytes/sec written

	// Health indicators
	ContainerRestarts   int     // Total container restarts (cumulative)
	FailedPods          int     // Pods in Failed state
	EvictedPods         int     // Pods that were evicted
	OOMKillCount        int     // OOM kills
	NodePressureCount   int     // Nodes with memory/disk/PID pressure
	CPUThrottledPercent float64 // Avg CPU throttling across containers

	// Load averages (cluster-wide average)
	LoadAverage1m  float64
	LoadAverage5m  float64
	LoadAverage15m float64

	// PSI stall percentages averaged across nodes — cluster-wide health.
	// The summary strip renders the dominant axis (max of the three averages).
	// Zero when the metrics source does not provide PSI.
	AvgNodeCPUStallPct float64
	AvgNodeMemStallPct float64
	AvgNodeIOStallPct  float64

	// PSIBeta is true when the cluster's Kubernetes version is below 1.36 (GA).
	// Surfaced by the UI to mark stall values as best-effort.
	PSIBeta bool
}

// CalculateHealthScore computes a composite 0-100 cluster health score from
// node readiness, pod failures/OOMs, PSI saturation, node pressure, and metrics
// availability.
func CalculateHealthScore(summary ClusterSummary, hasMetrics bool) int {
	score := 100

	if summary.NodesCount > 0 && summary.NodesReady < summary.NodesCount {
		unready := summary.NodesCount - summary.NodesReady
		score -= unready * 30
	}

	if summary.FailedPods > 0 || summary.OOMKillCount > 0 {
		score -= 15
	}

	maxPSI := summary.AvgNodeCPUStallPct
	if summary.AvgNodeMemStallPct > maxPSI {
		maxPSI = summary.AvgNodeMemStallPct
	}
	if summary.AvgNodeIOStallPct > maxPSI {
		maxPSI = summary.AvgNodeIOStallPct
	}
	if maxPSI > 20.0 {
		score -= 10
	}

	if summary.NodePressureCount > 0 {
		score -= summary.NodePressureCount * 5
	}

	if !hasMetrics {
		score -= 5
	}

	return max(0, min(100, score))
}
