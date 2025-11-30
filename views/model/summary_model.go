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
	ContainerCount int // Total running containers

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
	ContainerRestarts1h int     // Container restarts in last hour
	OOMKillCount        int     // OOM kills in last hour
	NodePressureCount   int     // Nodes with memory/disk/PID pressure
	CPUThrottledPercent float64 // Avg CPU throttling across containers

	// Load averages (cluster-wide average)
	LoadAverage1m  float64
	LoadAverage5m  float64
	LoadAverage15m float64
}
