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

	// Network/Disk I/O rates for this pod (Prometheus only)
	NetworkRxRate float64
	NetworkTxRate float64
	DiskReadRate  float64
	DiskWriteRate float64

	// Kubelet /metrics/probes results for this pod (when available)
	Probes []PodProbeInfo

	// kube-state-metrics enrichments for this pod (when KSM is available)
	DisruptionReason string
	ResourceClaims   []string

	// Mounted PVC volume stats and network error rates (Prometheus only, when available)
	VolumeUsages         []PodVolumeUsage
	NetworkRxDroppedRate float64
	NetworkTxDroppedRate float64
	NetworkRxErrorRate   float64
	NetworkTxErrorRate   float64
}

// PodVolumeUsage captures live fill metrics for a PVC mounted by the pod.
type PodVolumeUsage struct {
	PVCName       string
	UsedBytes     int64
	CapacityBytes int64
	FillPct       float64
	HasUsage      bool
}

// PodProbeInfo captures cumulative kubelet probe results for one container probe.
type PodProbeInfo struct {
	Container    string
	ProbeType    string
	SuccessCount float64
	FailureCount float64
	Failing      bool
}

// ContainerUsage holds formatted CPU and memory usage strings for a container
type ContainerUsage struct {
	CPUUsage        string  // e.g., "45m"
	MemoryUsage     string  // e.g., "128Mi"
	CPUThrottledPct float64 // CFS CPU throttling percentage (0.0-100.0)
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
	CPUUsage        string
	MemoryUsage     string
	CPUThrottledPct float64

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

		// Resource requests & limits (prefer live In-Place Pod Vertical Scaling
		// status.allocatedResources / status.resources when present).
		cpuReq := container.Resources.Requests.Cpu()
		memReq := container.Resources.Requests.Memory()
		cpuLim := container.Resources.Limits.Cpu()
		memLim := container.Resources.Limits.Memory()
		if status, ok := statusMap[container.Name]; ok {
			if q := status.AllocatedResources.Cpu(); q != nil && !q.IsZero() {
				cpuReq = q
			} else if status.Resources != nil {
				if q := status.Resources.Requests.Cpu(); q != nil && !q.IsZero() {
					cpuReq = q
				}
			}
			if q := status.AllocatedResources.Memory(); q != nil && !q.IsZero() {
				memReq = q
			} else if status.Resources != nil {
				if q := status.Resources.Requests.Memory(); q != nil && !q.IsZero() {
					memReq = q
				}
			}
			if status.Resources != nil {
				if q := status.Resources.Limits.Cpu(); q != nil && !q.IsZero() {
					cpuLim = q
				}
				if q := status.Resources.Limits.Memory(); q != nil && !q.IsZero() {
					memLim = q
				}
			}
		}

		if cpuReq != nil && !cpuReq.IsZero() {
			info.CPURequest = cpuReq.String()
		}
		if memReq != nil && !memReq.IsZero() {
			info.MemoryRequest = memReq.String()
		}
		if cpuLim != nil && !cpuLim.IsZero() {
			info.CPULimit = cpuLim.String()
		}
		if memLim != nil && !memLim.IsZero() {
			info.MemoryLimit = memLim.String()
		}

		// Resource usage from metrics
		if d.ContainerMetrics != nil {
			// Try actual container name first
			if usage, ok := d.ContainerMetrics[container.Name]; ok {
				info.CPUUsage = usage.CPUUsage
				info.MemoryUsage = usage.MemoryUsage
				info.CPUThrottledPct = usage.CPUThrottledPct
			}
			if mainUsage, ok := d.ContainerMetrics["main"]; ok && len(d.Pod.Spec.Containers) == 1 {
				if info.CPUUsage == "" {
					info.CPUUsage = mainUsage.CPUUsage
				}
				if info.MemoryUsage == "" {
					info.MemoryUsage = mainUsage.MemoryUsage
				}
				if info.CPUThrottledPct == 0 {
					info.CPUThrottledPct = mainUsage.CPUThrottledPct
				}
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

// PodEffectiveResources aggregates effective CPU/Memory requests and limits
// accounting for PodLevelResources (v1.34+) and In-Place Pod Vertical Scaling (v1.35+).
type PodEffectiveResources struct {
	ReqCPUMillis int64
	ReqMemBytes  int64
	LimCPUMillis int64
	LimMemBytes  int64
	PodLevel     bool
	ResizeStatus string
}

// GetEffectiveResources computes effective pod-wide requests and limits.
func (d *PodDetailData) GetEffectiveResources() PodEffectiveResources {
	var out PodEffectiveResources
	if d == nil || d.Pod == nil {
		return out
	}
	pod := d.Pod
	out.ResizeStatus = GetPodResizeStatus(pod)

	statusMap := make(map[string]corev1.ContainerStatus, len(pod.Status.ContainerStatuses))
	for _, cs := range pod.Status.ContainerStatuses {
		statusMap[cs.Name] = cs
	}

	for _, c := range pod.Spec.Containers {
		cReqCPU := c.Resources.Requests.Cpu()
		cReqMem := c.Resources.Requests.Memory()
		cLimCPU := c.Resources.Limits.Cpu()
		cLimMem := c.Resources.Limits.Memory()
		if cs, ok := statusMap[c.Name]; ok {
			if q := cs.AllocatedResources.Cpu(); q != nil && !q.IsZero() {
				cReqCPU = q
			} else if cs.Resources != nil {
				if q := cs.Resources.Requests.Cpu(); q != nil && !q.IsZero() {
					cReqCPU = q
				}
			}
			if q := cs.AllocatedResources.Memory(); q != nil && !q.IsZero() {
				cReqMem = q
			} else if cs.Resources != nil {
				if q := cs.Resources.Requests.Memory(); q != nil && !q.IsZero() {
					cReqMem = q
				}
			}
			if cs.Resources != nil {
				if q := cs.Resources.Limits.Cpu(); q != nil && !q.IsZero() {
					cLimCPU = q
				}
				if q := cs.Resources.Limits.Memory(); q != nil && !q.IsZero() {
					cLimMem = q
				}
			}
		}
		if cReqCPU != nil {
			out.ReqCPUMillis += cReqCPU.MilliValue()
		}
		if cReqMem != nil {
			out.ReqMemBytes += cReqMem.Value()
		}
		if cLimCPU != nil {
			out.LimCPUMillis += cLimCPU.MilliValue()
		}
		if cLimMem != nil {
			out.LimMemBytes += cLimMem.Value()
		}
	}

	if pod.Status.AllocatedResources != nil {
		if q := pod.Status.AllocatedResources.Cpu(); q != nil && !q.IsZero() {
			out.ReqCPUMillis = q.MilliValue()
		}
		if q := pod.Status.AllocatedResources.Memory(); q != nil && !q.IsZero() {
			out.ReqMemBytes = q.Value()
		}
	}
	if pod.Status.Resources != nil {
		out.PodLevel = true
		if q := pod.Status.Resources.Requests.Cpu(); q != nil && !q.IsZero() {
			out.ReqCPUMillis = q.MilliValue()
		}
		if q := pod.Status.Resources.Requests.Memory(); q != nil && !q.IsZero() {
			out.ReqMemBytes = q.Value()
		}
		if q := pod.Status.Resources.Limits.Cpu(); q != nil && !q.IsZero() {
			out.LimCPUMillis = q.MilliValue()
		}
		if q := pod.Status.Resources.Limits.Memory(); q != nil && !q.IsZero() {
			out.LimMemBytes = q.Value()
		}
	} else if pod.Spec.Resources != nil {
		out.PodLevel = true
		if q := pod.Spec.Resources.Requests.Cpu(); q != nil && !q.IsZero() {
			out.ReqCPUMillis = q.MilliValue()
		}
		if q := pod.Spec.Resources.Requests.Memory(); q != nil && !q.IsZero() {
			out.ReqMemBytes = q.Value()
		}
		if q := pod.Spec.Resources.Limits.Cpu(); q != nil && !q.IsZero() {
			out.LimCPUMillis = q.MilliValue()
		}
		if q := pod.Spec.Resources.Limits.Memory(); q != nil && !q.IsZero() {
			out.LimMemBytes = q.Value()
		}
	}

	return out
}

// GetConditions returns structured pod condition information
func (d *PodDetailData) GetConditions() []PodConditionInfo {
	if d.Pod == nil {
		return nil
	}

	conditions := make([]PodConditionInfo, 0, len(d.Pod.Status.Conditions))
	for _, cond := range d.Pod.Status.Conditions {
		healthy := cond.Status == corev1.ConditionTrue
		switch cond.Type {
		case corev1.PodResizePending:
			healthy = cond.Status != corev1.ConditionTrue
		case corev1.PodResizeInProgress:
			healthy = cond.Status != corev1.ConditionTrue || cond.Reason != string(corev1.PodReasonError)
		}

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
