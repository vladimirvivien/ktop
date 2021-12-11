package model

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type NodeSummary struct {
	CapacityCpuTotal resource.Quantity
	CapacityMemTotal resource.Quantity
	UsageCpuTotal    resource.Quantity
	UsageMemTotal    resource.Quantity
}

func NewNodeSummary(nodes []NodeModel) *NodeSummary {
	summary := NodeSummary{}
	for _, node := range nodes {
		summary.CapacityCpuTotal.Add(node.CapacityCPU)
		summary.CapacityMemTotal.Add(node.CapacityMem)
		summary.UsageCpuTotal.Add(node.UsageCPU)
		summary.UsageMemTotal.Add(node.UsageMem)
	}
	return &summary
}

type ClusterSummary struct {
	Uptime              metav1.Time // oldest running node
	NodesReady          int
	NodesCount          int
	Namespaces          int
	PodsRunning         int
	PodsAvailable       int
	Pressures           int
	ContainersRunning   int
	ImagesCount         int
	PVsAttached         int
	PVsInUse            int
	JobsCount           int
	CronJobsCount       int
	StatefulSetsReady   int
	DeploymentsDesired  int
	DeploymentsReady    int
	DaemonSetsDesired   int
	DaemonSetsReady     int
	ReplicaSetsReady    int
	ReplicaSetsDesired  int
	AllocatableCpuTotal *resource.Quantity
	AllocatableMemTotal *resource.Quantity
	RequestedCpuTotal   *resource.Quantity
	RequestedMemTotal   *resource.Quantity
}
