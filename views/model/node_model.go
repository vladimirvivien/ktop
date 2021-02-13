package model

import (
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type NodeModel struct {
	Node coreV1.Node
	NodeStatus coreV1.NodeStatus
	AvailRes coreV1.ResourceList
	NodeMetrics *v1beta1.NodeMetrics
}

func (m *NodeModel) IsNodeMaster() bool {
	_, ok := m.Node.Labels["node-role.kubernetes.io/master"]
	return ok
}

func (m *NodeModel) NodeRole() string {
	if m.IsNodeMaster() {
		return "Master"
	}
	return "Node"
}

func (m *NodeModel) NodeState() string {
	conds := m.Node.Status.Conditions
	if conds == nil || len(conds) == 0 {
		return "Unknown"
	}

	for _, cond := range conds {
		if cond.Status == coreV1.ConditionTrue {
			return string(cond.Type)
		}
	}

	return "NotReady"
}

func (m *NodeModel) GetNodeInternalIp() string {
	for _, addr := range m.Node.Status.Addresses {
		if addr.Type == coreV1.NodeInternalIP {
			return addr.Address
		}
	}
	return "<none>"
}

func (m *NodeModel) GetNodeExternalIp() string {
	for _, addr := range m.Node.Status.Addresses {
		if addr.Type == coreV1.NodeExternalIP {
			return addr.Address
		}
	}
	return "<none>"
}

func (m *NodeModel) GetNodeHostName() string {
	for _, addr := range m.Node.Status.Addresses {
		if addr.Type == coreV1.NodeHostName {
			return addr.Address
		}
	}
	return "<none>"
}

func (m *NodeModel) CpuAvail() int64 {
	return m.AvailRes.Cpu().Value()
}

func (m *NodeModel) CpuAvailMillis() int64 {
	return m.AvailRes.Cpu().MilliValue()
}

func (m *NodeModel) CpuUsage() int64 {
	return m.NodeMetrics.Usage.Cpu().Value()
}

func (m *NodeModel) CpuUsageMillis() int64 {
	return m.NodeMetrics.Usage.Cpu().MilliValue()
}

func (m *NodeModel) MemAvail() int64 {
	return m.AvailRes.Memory().ScaledValue(resource.Mega)
}

func (m *NodeModel) MemAvailMillis() int64 {
	return m.AvailRes.Memory().MilliValue()
}

func (m *NodeModel) MemUsage() int64 {
	return m.NodeMetrics.Usage.Memory().ScaledValue(resource.Mega)
}

func (m *NodeModel) MemUsageMillis() int64 {
	return m.NodeMetrics.Usage.Memory().MilliValue()
}

func (m *NodeModel) EphStorageAvail() int64 {
	return m.AvailRes.StorageEphemeral().ScaledValue(resource.Giga)
}