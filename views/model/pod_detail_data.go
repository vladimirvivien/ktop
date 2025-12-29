package model

import (
	corev1 "k8s.io/api/core/v1"
)

// PodDetailData contains all data needed to display a pod detail view
type PodDetailData struct {
	// PodModel contains the basic pod metrics and info
	PodModel *PodModel

	// Pod is the full Kubernetes Pod object
	Pod *corev1.Pod

	// Events are recent Kubernetes events for this pod
	Events []corev1.Event

	// MetricsHistory contains historical CPU/memory samples for sparkline graphs
	// Key is container name
	MetricsHistory map[string][]MetricSample

	// ContainerMetrics contains current CPU/memory usage for each container
	// Key is container name, value is ContainerUsage
	ContainerMetrics map[string]ContainerUsage

	// MetricsSourceType indicates the active metrics source ("prometheus", "metrics-server", or "")
	// Used by detail panels to determine which metrics visualizations to show
	MetricsSourceType string
}

// ContainerUsage holds formatted CPU and memory usage strings for a container
type ContainerUsage struct {
	CPUUsage    string // e.g., "45m"
	MemoryUsage string // e.g., "128Mi"
}

// ContainerInfo provides structured container information
type ContainerInfo struct {
	Name         string
	Image        string
	State        string
	Ready        bool
	RestartCount int32
	Started      bool

	// Resource requests and limits
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string

	// Resource usage (from metrics)
	CPUUsage    string
	MemoryUsage string

	// Probes
	LivenessProbe  string
	ReadinessProbe string
	StartupProbe   string
}

// PodConditionInfo provides structured pod condition information
type PodConditionInfo struct {
	Type    string
	Status  string
	Reason  string
	Message string
	Healthy bool
}

// VolumeInfo provides structured volume information
type VolumeInfo struct {
	Name      string
	Type      string
	MountPath string
	ReadOnly  bool
}

// GetContainers returns structured container information
func (d *PodDetailData) GetContainers() []ContainerInfo {
	if d.Pod == nil {
		return nil
	}

	containers := make([]ContainerInfo, 0, len(d.Pod.Spec.Containers))

	// Build a map of container statuses by name
	statusMap := make(map[string]corev1.ContainerStatus)
	for _, status := range d.Pod.Status.ContainerStatuses {
		statusMap[status.Name] = status
	}

	for _, container := range d.Pod.Spec.Containers {
		info := ContainerInfo{
			Name:  container.Name,
			Image: container.Image,
		}

		// Get container status
		if status, ok := statusMap[container.Name]; ok {
			info.Ready = status.Ready
			info.RestartCount = status.RestartCount
			if status.Started != nil {
				info.Started = *status.Started
			}

			// Determine state
			if status.State.Running != nil {
				info.State = "Running"
			} else if status.State.Waiting != nil {
				info.State = status.State.Waiting.Reason
				if info.State == "" {
					info.State = "Waiting"
				}
			} else if status.State.Terminated != nil {
				info.State = status.State.Terminated.Reason
				if info.State == "" {
					info.State = "Terminated"
				}
			}
		}

		// Resource requests
		if cpu := container.Resources.Requests.Cpu(); cpu != nil && !cpu.IsZero() {
			info.CPURequest = cpu.String()
		}
		if mem := container.Resources.Requests.Memory(); mem != nil && !mem.IsZero() {
			info.MemoryRequest = mem.String()
		}

		// Resource limits
		if cpu := container.Resources.Limits.Cpu(); cpu != nil && !cpu.IsZero() {
			info.CPULimit = cpu.String()
		}
		if mem := container.Resources.Limits.Memory(); mem != nil && !mem.IsZero() {
			info.MemoryLimit = mem.String()
		}

		// Resource usage from metrics
		if d.ContainerMetrics != nil {
			// Try actual container name first
			if usage, ok := d.ContainerMetrics[container.Name]; ok {
				info.CPUUsage = usage.CPUUsage
				info.MemoryUsage = usage.MemoryUsage
			} else if usage, ok := d.ContainerMetrics["main"]; ok && len(d.Pod.Spec.Containers) == 1 {
				// Fallback to "main" for single-container pods (static pod aggregate)
				info.CPUUsage = usage.CPUUsage
				info.MemoryUsage = usage.MemoryUsage
			}
		}

		// Probes
		if container.LivenessProbe != nil {
			info.LivenessProbe = formatProbe(container.LivenessProbe)
		}
		if container.ReadinessProbe != nil {
			info.ReadinessProbe = formatProbe(container.ReadinessProbe)
		}
		if container.StartupProbe != nil {
			info.StartupProbe = formatProbe(container.StartupProbe)
		}

		containers = append(containers, info)
	}

	return containers
}

// formatProbe returns a string representation of a probe
func formatProbe(probe *corev1.Probe) string {
	if probe.HTTPGet != nil {
		return "http-get " + probe.HTTPGet.Path
	}
	if probe.TCPSocket != nil {
		return "tcp " + probe.TCPSocket.Port.String()
	}
	if probe.Exec != nil {
		if len(probe.Exec.Command) > 0 {
			return "exec " + probe.Exec.Command[0]
		}
		return "exec"
	}
	if probe.GRPC != nil {
		if probe.GRPC.Service != nil {
			return "grpc " + *probe.GRPC.Service
		}
		return "grpc"
	}
	return ""
}

// GetConditions returns structured pod condition information
func (d *PodDetailData) GetConditions() []PodConditionInfo {
	if d.Pod == nil {
		return nil
	}

	conditions := make([]PodConditionInfo, 0, len(d.Pod.Status.Conditions))
	for _, cond := range d.Pod.Status.Conditions {
		healthy := cond.Status == corev1.ConditionTrue

		conditions = append(conditions, PodConditionInfo{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
			Healthy: healthy,
		})
	}
	return conditions
}

// GetVolumes returns structured volume information
func (d *PodDetailData) GetVolumes() []VolumeInfo {
	if d.Pod == nil {
		return nil
	}

	// Build a map of volume name to volume for lookup
	volumeMap := make(map[string]corev1.Volume)
	for _, vol := range d.Pod.Spec.Volumes {
		volumeMap[vol.Name] = vol
	}

	// Build volume info from volume mounts
	var volumes []VolumeInfo
	seen := make(map[string]bool)

	for _, container := range d.Pod.Spec.Containers {
		for _, mount := range container.VolumeMounts {
			if seen[mount.Name] {
				continue
			}
			seen[mount.Name] = true

			info := VolumeInfo{
				Name:      mount.Name,
				MountPath: mount.MountPath,
				ReadOnly:  mount.ReadOnly,
			}

			// Determine volume type
			if vol, ok := volumeMap[mount.Name]; ok {
				info.Type = getVolumeType(vol)
			}

			volumes = append(volumes, info)
		}
	}

	return volumes
}

// getVolumeType returns a string describing the volume type
func getVolumeType(vol corev1.Volume) string {
	switch {
	case vol.ConfigMap != nil:
		return "ConfigMap"
	case vol.Secret != nil:
		return "Secret"
	case vol.PersistentVolumeClaim != nil:
		return "PVC"
	case vol.EmptyDir != nil:
		return "EmptyDir"
	case vol.HostPath != nil:
		return "HostPath"
	case vol.Projected != nil:
		return "Projected"
	case vol.DownwardAPI != nil:
		return "DownwardAPI"
	case vol.NFS != nil:
		return "NFS"
	case vol.CSI != nil:
		return "CSI"
	default:
		return "Unknown"
	}
}

// GetLabels returns pod labels
func (d *PodDetailData) GetLabels() map[string]string {
	if d.Pod == nil {
		return nil
	}
	return d.Pod.Labels
}

// GetAnnotations returns pod annotations
func (d *PodDetailData) GetAnnotations() map[string]string {
	if d.Pod == nil {
		return nil
	}
	return d.Pod.Annotations
}

// GetOwnerReferences returns owner reference information
func (d *PodDetailData) GetOwnerReferences() []OwnerReferenceInfo {
	if d.Pod == nil {
		return nil
	}

	refs := make([]OwnerReferenceInfo, 0, len(d.Pod.OwnerReferences))
	for _, ref := range d.Pod.OwnerReferences {
		refs = append(refs, OwnerReferenceInfo{
			Kind:       ref.Kind,
			Name:       ref.Name,
			Controller: ref.Controller != nil && *ref.Controller,
		})
	}
	return refs
}

// OwnerReferenceInfo provides structured owner reference information
type OwnerReferenceInfo struct {
	Kind       string
	Name       string
	Controller bool
}

// GetQOSClass returns the pod's QoS class
func (d *PodDetailData) GetQOSClass() string {
	if d.Pod == nil {
		return ""
	}
	return string(d.Pod.Status.QOSClass)
}

// GetServiceAccountName returns the pod's service account name
func (d *PodDetailData) GetServiceAccountName() string {
	if d.Pod == nil {
		return ""
	}
	return d.Pod.Spec.ServiceAccountName
}
