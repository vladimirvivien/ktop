package model

import (
	"slices"

	"github.com/vladimirvivien/ktop/internal/psi"
	"github.com/vladimirvivien/ktop/metrics"
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

	// PSI carries pressure stall percentages for the node's root cgroup.
	// Zero when the metrics source does not provide PSI (e.g., metrics-server).
	PSI metrics.PSIMetrics

	// PSIKernelSupported is true when the node's Linux kernel is >= 4.20.
	// PSI values from a node with this set to false are not reliable.
	PSIKernelSupported bool
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

		PSIKernelSupported: psi.KernelVersionOK(node.Status.NodeInfo.KernelVersion),
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

func nodeLess(a, b NodeModel, column string) bool {
	switch column {
	case "NAME":
		return a.Name < b.Name

	case "STATUS":
		// Ready before NotReady
		if a.Status == b.Status {
			return a.Name < b.Name
		}
		return a.Status > b.Status

	case "IP":
		if a.InternalIP == b.InternalIP {
			return a.Name < b.Name
		}
		return a.InternalIP < b.InternalIP

	case "PODS":
		if a.PodsCount == b.PodsCount {
			return a.Name < b.Name
		}
		return a.PodsCount < b.PodsCount

	case "TAINTS":
		if a.TaintCount == b.TaintCount {
			return a.Name < b.Name
		}
		return a.TaintCount < b.TaintCount

	case "PRESSURE":
		// Nodes with pressure sort after nodes without
		pressureA := len(a.Pressures)
		pressureB := len(b.Pressures)
		if pressureA == pressureB {
			return a.Name < b.Name
		}
		return pressureA < pressureB

	case "RST":
		if a.Restarts == b.Restarts {
			return a.Name < b.Name
		}
		return a.Restarts < b.Restarts

	case "VOLS":
		if a.VolumesInUse == b.VolumesInUse {
			return a.Name < b.Name
		}
		return a.VolumesInUse < b.VolumesInUse

	case "DISK":
		diskA := int64(0)
		diskB := int64(0)

		if a.AllocatableStorageQty != nil {
			diskA = a.AllocatableStorageQty.Value()
		}
		if b.AllocatableStorageQty != nil {
			diskB = b.AllocatableStorageQty.Value()
		}

		if diskA == diskB {
			return a.Name < b.Name
		}
		return diskA < diskB

	case "CPU":
		cpuA := int64(0)
		cpuB := int64(0)

		// Prefer usage metrics, fall back to requested if unavailable
		if a.UsageCpuQty != nil && a.UsageCpuQty.MilliValue() > 0 {
			cpuA = a.UsageCpuQty.MilliValue()
		} else if a.RequestedPodCpuQty != nil {
			cpuA = a.RequestedPodCpuQty.MilliValue()
		}

		if b.UsageCpuQty != nil && b.UsageCpuQty.MilliValue() > 0 {
			cpuB = b.UsageCpuQty.MilliValue()
		} else if b.RequestedPodCpuQty != nil {
			cpuB = b.RequestedPodCpuQty.MilliValue()
		}

		if cpuA == cpuB {
			return a.Name < b.Name
		}
		return cpuA < cpuB

	case "MEM":
		memA := int64(0)
		memB := int64(0)

		// Prefer usage metrics, fall back to requested if unavailable
		if a.UsageMemQty != nil && a.UsageMemQty.Value() > 0 {
			memA = a.UsageMemQty.Value()
		} else if a.RequestedPodMemQty != nil {
			memA = a.RequestedPodMemQty.Value()
		}

		if b.UsageMemQty != nil && b.UsageMemQty.Value() > 0 {
			memB = b.UsageMemQty.Value()
		} else if b.RequestedPodMemQty != nil {
			memB = b.RequestedPodMemQty.Value()
		}

		if memA == memB {
			return a.Name < b.Name
		}
		return memA < memB

	case "STALL":
		// Sort by the dominant stall axis (max of CPU/MEM/IO waiting %).
		dom := func(n NodeModel) float64 {
			m := n.PSI.CPUStallPct
			if n.PSI.MemStallPct > m {
				m = n.PSI.MemStallPct
			}
			if n.PSI.IOStallPct > m {
				m = n.PSI.IOStallPct
			}
			return m
		}
		sa, sb := dom(a), dom(b)
		if sa == sb {
			return a.Name < b.Name
		}
		return sa < sb

	default:
		// Default to NAME sorting
		return a.Name < b.Name
	}
}

// SortNodeModelsBy sorts nodes by the specified column and direction
func SortNodeModelsBy(nodes []NodeModel, column string, ascending bool) {
	if len(nodes) <= 1 {
		return
	}
	slices.SortFunc(nodes, func(a, b NodeModel) int {
		if nodeLess(a, b, column) {
			if ascending {
				return -1
			}
			return 1
		}
		if nodeLess(b, a, column) {
			if ascending {
				return 1
			}
			return -1
		}
		return 0
	})
}
