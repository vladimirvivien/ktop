package model

import (
	corev1 "k8s.io/api/core/v1"
)

// NodeDetailData contains all data needed to display a node detail view
type NodeDetailData struct {
	// NodeModel contains the basic node metrics and info
	NodeModel *NodeModel

	// Node is the full Kubernetes Node object for accessing labels, conditions, etc.
	Node *corev1.Node

	// PodsOnNode is the list of pods running on this node
	PodsOnNode []*PodModel

	// Events are recent Kubernetes events for this node
	Events []corev1.Event

	// MetricsHistory contains historical CPU/memory samples for sparkline graphs
	MetricsHistory []MetricSample

	// MetricsSourceType indicates the active metrics source ("prometheus", "metrics-server", or "")
	// Used by detail panels to determine which metrics visualizations to show
	MetricsSourceType string
}

// MetricSample represents a single point in time for metrics history
type MetricSample struct {
	Timestamp int64   // Unix timestamp
	CPURatio  float64 // CPU usage as ratio 0-1
	MemRatio  float64 // Memory usage as ratio 0-1
}

// NodeConditionInfo provides a structured view of a node condition
type NodeConditionInfo struct {
	Type    string
	Status  string
	Reason  string
	Message string
	Healthy bool
}

// GetConditions returns structured condition information from the node
func (d *NodeDetailData) GetConditions() []NodeConditionInfo {
	if d.Node == nil {
		return nil
	}

	conditions := make([]NodeConditionInfo, 0, len(d.Node.Status.Conditions))
	for _, cond := range d.Node.Status.Conditions {
		// Determine if condition is healthy
		// Ready=True is healthy, all pressure conditions True are unhealthy
		healthy := false
		switch cond.Type {
		case corev1.NodeReady:
			healthy = cond.Status == corev1.ConditionTrue
		case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure:
			healthy = cond.Status == corev1.ConditionFalse
		default:
			healthy = cond.Status == corev1.ConditionFalse
		}

		conditions = append(conditions, NodeConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
			Healthy: healthy,
		})
	}
	return conditions
}

// GetLabels returns node labels as a map
func (d *NodeDetailData) GetLabels() map[string]string {
	if d.Node == nil {
		return nil
	}
	return d.Node.Labels
}

// GetAnnotations returns node annotations as a map
func (d *NodeDetailData) GetAnnotations() map[string]string {
	if d.Node == nil {
		return nil
	}
	return d.Node.Annotations
}

// GetTaints returns node taints
func (d *NodeDetailData) GetTaints() []corev1.Taint {
	if d.Node == nil {
		return nil
	}
	return d.Node.Spec.Taints
}

// GetCapacity returns node capacity resources
func (d *NodeDetailData) GetCapacity() corev1.ResourceList {
	if d.Node == nil {
		return nil
	}
	return d.Node.Status.Capacity
}

// GetAllocatable returns node allocatable resources
func (d *NodeDetailData) GetAllocatable() corev1.ResourceList {
	if d.Node == nil {
		return nil
	}
	return d.Node.Status.Allocatable
}
