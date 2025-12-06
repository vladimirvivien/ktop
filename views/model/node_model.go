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
	TaintCount           int
	Unschedulable        bool
	Restarts             int

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
		TaintCount:           len(node.Spec.Taints),
		Unschedulable:        node.Spec.Unschedulable,

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
	SortNodeModelsBy(nodes, "NAME", true)
}

// SortNodeModelsBy sorts nodes by the specified column and direction
func SortNodeModelsBy(nodes []NodeModel, column string, ascending bool) {
	var sortFunc func(i, j int) bool

	switch column {
	case "NAME":
		sortFunc = func(i, j int) bool {
			return nodes[i].Name < nodes[j].Name
		}

	case "STATUS":
		sortFunc = func(i, j int) bool {
			// Ready before NotReady
			if nodes[i].Status == nodes[j].Status {
				return nodes[i].Name < nodes[j].Name
			}
			return nodes[i].Status > nodes[j].Status
		}

	case "IP":
		sortFunc = func(i, j int) bool {
			if nodes[i].InternalIP == nodes[j].InternalIP {
				return nodes[i].Name < nodes[j].Name
			}
			return nodes[i].InternalIP < nodes[j].InternalIP
		}

	case "PODS":
		sortFunc = func(i, j int) bool {
			if nodes[i].PodsCount == nodes[j].PodsCount {
				return nodes[i].Name < nodes[j].Name
			}
			return nodes[i].PodsCount < nodes[j].PodsCount
		}

	case "TAINTS":
		sortFunc = func(i, j int) bool {
			if nodes[i].TaintCount == nodes[j].TaintCount {
				return nodes[i].Name < nodes[j].Name
			}
			return nodes[i].TaintCount < nodes[j].TaintCount
		}

	case "PRESSURE":
		sortFunc = func(i, j int) bool {
			// Nodes with pressure sort after nodes without
			pressureI := len(nodes[i].Pressures)
			pressureJ := len(nodes[j].Pressures)
			if pressureI == pressureJ {
				return nodes[i].Name < nodes[j].Name
			}
			return pressureI < pressureJ
		}

	case "RST":
		sortFunc = func(i, j int) bool {
			if nodes[i].Restarts == nodes[j].Restarts {
				return nodes[i].Name < nodes[j].Name
			}
			return nodes[i].Restarts < nodes[j].Restarts
		}

	case "VOLS":
		sortFunc = func(i, j int) bool {
			if nodes[i].VolumesInUse == nodes[j].VolumesInUse {
				return nodes[i].Name < nodes[j].Name
			}
			return nodes[i].VolumesInUse < nodes[j].VolumesInUse
		}

	case "DISK":
		sortFunc = func(i, j int) bool {
			diskI := int64(0)
			diskJ := int64(0)

			if nodes[i].AllocatableStorageQty != nil {
				diskI = nodes[i].AllocatableStorageQty.Value()
			}
			if nodes[j].AllocatableStorageQty != nil {
				diskJ = nodes[j].AllocatableStorageQty.Value()
			}

			if diskI == diskJ {
				return nodes[i].Name < nodes[j].Name
			}
			return diskI < diskJ
		}

	case "CPU":
		sortFunc = func(i, j int) bool {
			cpuI := int64(0)
			cpuJ := int64(0)

			// Prefer usage metrics, fall back to requested if unavailable
			if nodes[i].UsageCpuQty != nil && nodes[i].UsageCpuQty.MilliValue() > 0 {
				cpuI = nodes[i].UsageCpuQty.MilliValue()
			} else if nodes[i].RequestedPodCpuQty != nil {
				cpuI = nodes[i].RequestedPodCpuQty.MilliValue()
			}

			if nodes[j].UsageCpuQty != nil && nodes[j].UsageCpuQty.MilliValue() > 0 {
				cpuJ = nodes[j].UsageCpuQty.MilliValue()
			} else if nodes[j].RequestedPodCpuQty != nil {
				cpuJ = nodes[j].RequestedPodCpuQty.MilliValue()
			}

			if cpuI == cpuJ {
				return nodes[i].Name < nodes[j].Name
			}
			return cpuI < cpuJ
		}

	case "MEM":
		sortFunc = func(i, j int) bool {
			memI := int64(0)
			memJ := int64(0)

			// Prefer usage metrics, fall back to requested if unavailable
			if nodes[i].UsageMemQty != nil && nodes[i].UsageMemQty.Value() > 0 {
				memI = nodes[i].UsageMemQty.Value()
			} else if nodes[i].RequestedPodMemQty != nil {
				memI = nodes[i].RequestedPodMemQty.Value()
			}

			if nodes[j].UsageMemQty != nil && nodes[j].UsageMemQty.Value() > 0 {
				memJ = nodes[j].UsageMemQty.Value()
			} else if nodes[j].RequestedPodMemQty != nil {
				memJ = nodes[j].RequestedPodMemQty.Value()
			}

			if memI == memJ {
				return nodes[i].Name < nodes[j].Name
			}
			return memI < memJ
		}

	default:
		// Default to NAME sorting
		sortFunc = func(i, j int) bool {
			return nodes[i].Name < nodes[j].Name
		}
	}

	// Apply sort
	if ascending {
		sort.Slice(nodes, sortFunc)
	} else {
		// Reverse for descending
		sort.Slice(nodes, func(i, j int) bool {
			return !sortFunc(i, j)
		})
	}
}
