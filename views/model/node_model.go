package model

import (
	"sort"

	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

const (
	ControlPlaneLabel = "node-role.kubernetes.io/control-plane"
	MasterNodeLabel   = "node-role.kubernetes.io/master"
)

type NodeModel struct {
	Name                 string
	Roles                []string
	Controller           bool
	Hostname             string
	Role                 string
	Status               string
	Pressures            []string
	CreationTime         metav1.Time
	TimeSinceStart       string
	InternalIP           string
	ExternalIP           string
	PodsCount            int
	ContainerImagesCount int
	VolumesInUse         int
	VolumesAttached      int

	KubeletVersion          string
	OS                      string
	OSImage                 string
	OSKernel                string
	Architecture            string
	ContainerRuntimeVersion string

	RequestedPodCpuQty *resource.Quantity
	RequestedPodMemQty *resource.Quantity

	AllocatableCpuQty     *resource.Quantity
	AllocatableMemQty     *resource.Quantity
	AllocatableStorageQty *resource.Quantity

	UsageCpuQty *resource.Quantity
	UsageMemQty *resource.Quantity
}

func NewNodeModel(node *coreV1.Node, metrics *v1beta1.NodeMetrics) *NodeModel {
	roles := GetNodeControlRoles(node)
	return &NodeModel{
		Name:           node.Name,
		Roles:          roles,
		Controller:     IsNodeController(roles),
		Hostname:       GetNodeHostName(node),
		Status:         GetNodeReadyStatus(node),
		Pressures:      GetNodePressures(node),
		TimeSinceStart: timeSince(node.CreationTimestamp),
		CreationTime:   node.CreationTimestamp,
		InternalIP:     GetNodeIp(node, coreV1.NodeInternalIP),
		ExternalIP:     GetNodeIp(node, coreV1.NodeExternalIP),

		ContainerImagesCount: len(node.Status.Images),
		VolumesAttached:      len(node.Status.VolumesAttached),
		VolumesInUse:         len(node.Status.VolumesInUse),

		KubeletVersion:          node.Status.NodeInfo.KubeletVersion,
		ContainerRuntimeVersion: node.Status.NodeInfo.ContainerRuntimeVersion,
		OS:                      node.Status.NodeInfo.OperatingSystem,
		OSImage:                 node.Status.NodeInfo.OSImage,
		OSKernel:                node.Status.NodeInfo.KernelVersion,
		Architecture:            node.Status.NodeInfo.Architecture,

		AllocatableCpuQty:     node.Status.Allocatable.Cpu(),
		AllocatableMemQty:     node.Status.Allocatable.Memory(),
		AllocatableStorageQty: node.Status.Allocatable.StorageEphemeral(),

		UsageCpuQty: metrics.Usage.Cpu(),
		UsageMemQty: metrics.Usage.Memory(),
	}
}

func GetNodeControlRoles(node *coreV1.Node) []string {
	roles := []string{}
	for key, _ := range node.Labels {
		if key == ControlPlaneLabel {
			roles = append(roles, "control-plane")
		}
		if key == MasterNodeLabel {
			roles = append(roles, "master")
		}
	}
	return roles
}

func IsNodeController(roles []string) bool {
	for _, role := range roles {
		if role == "control-plane" || role == "master" {
			return true
		}
	}
	return false
}

func GetNodeHostName(node *coreV1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == coreV1.NodeHostName {
			return addr.Address
		}
	}
	return "<none>"
}

func GetNodeReadyStatus(node *coreV1.Node) string {
	for _, cond := range node.Status.Conditions {
		if cond.Type == coreV1.NodeReady && cond.Status == coreV1.ConditionTrue {
			return string(cond.Type)
		}
	}
	return "NotReady"
}

func GetNodePressures(node *coreV1.Node) []string {
	var pressures []string
	for _, cond := range node.Status.Conditions {
		switch {
		case cond.Type == coreV1.NodeMemoryPressure && cond.Status == coreV1.ConditionTrue:
			pressures = append(pressures, "mem")
		case cond.Type == coreV1.NodeDiskPressure && cond.Status == coreV1.ConditionTrue:
			pressures = append(pressures, "disk")
		case cond.Type == coreV1.NodePIDPressure && cond.Status == coreV1.ConditionTrue:
			pressures = append(pressures, "pid")
		}
	}
	return pressures
}

func GetNodeIp(node *coreV1.Node, addrType coreV1.NodeAddressType) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == addrType {
			return addr.Address
		}
	}
	return "<none>"
}

func SortNodeModels(nodes []NodeModel) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})
}
