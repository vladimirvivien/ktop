package k8s

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

// MetricsSource defines the interface for different metrics providers
type MetricsSource interface {
	GetNodeMetrics(ctx context.Context, nodeName string) (*NodeMetrics, error)
	GetPodMetrics(ctx context.Context, namespace, podName string) (*PodMetrics, error)
	GetAllPodMetrics(ctx context.Context) ([]*PodMetrics, error)
	GetAvailableMetrics() []string
	IsHealthy() bool
	GetSourceInfo() SourceInfo
}

// SourceInfo provides information about the metrics source
type SourceInfo struct {
	Type         string
	Version      string
	LastScrape   time.Time
	MetricsCount int
	Errors       int
	State        SourceState // Current state of the source
}

// SourceState represents the current state of a metrics source
type SourceState string

const (
	SourceStateInitializing SourceState = "initializing"
	SourceStateCollecting   SourceState = "collecting"
	SourceStateHealthy      SourceState = "healthy"
	SourceStateUnhealthy    SourceState = "unhealthy"
)

// NodeMetrics contains metrics for a Kubernetes node
type NodeMetrics struct {
	NodeName         string
	CPUUsage         *resource.Quantity
	MemoryUsage      *resource.Quantity
	NetworkRxBytes   *resource.Quantity
	NetworkTxBytes   *resource.Quantity
	DiskUsage        *resource.Quantity
	LoadAverage1m    float64
	LoadAverage5m    float64
	LoadAverage15m   float64
	PodCount         int
	ContainerCount   int
	Timestamp        time.Time
}

// PodMetrics contains metrics for a Kubernetes pod
type PodMetrics struct {
	PodName          string
	Namespace        string
	ContainerMetrics []ContainerMetrics
	Timestamp        time.Time
}

// ContainerMetrics contains metrics for a container
type ContainerMetrics struct {
	Name         string
	CPUUsage     *resource.Quantity
	MemoryUsage  *resource.Quantity
	CPUThrottled float64
	MemoryLimit  *resource.Quantity
	RestartCount int
}