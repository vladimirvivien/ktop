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
}
