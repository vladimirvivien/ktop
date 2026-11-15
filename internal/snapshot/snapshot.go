package snapshot

import (
	"slices"
	"time"
)

// Snapshot is an immutable, point-in-time capture of cluster state and metrics.
// It contains only pure value types with no mutexes or channels, suitable for
// both synchronous TUI rendering and structured JSON serialization.
type Snapshot struct {
	Timestamp time.Time   `json:"timestamp"`
	Cluster   ClusterInfo `json:"cluster"`
	Summary   SummaryData `json:"summary"`
	Nodes     []NodeData  `json:"nodes"`
	Pods      []PodData   `json:"pods"`
}

// ClusterInfo describes cluster-wide connectivity and capability metadata.
type ClusterInfo struct {
	Context           string `json:"context,omitempty"`
	KubernetesVersion string `json:"kubernetes_version,omitempty"`
	MetricsSource     string `json:"metrics_source"`
	Ready             bool   `json:"ready"`
	PSIBeta           bool   `json:"psi_beta"`
}

// SummaryData holds cluster-wide aggregate counts, resource totals, and health.
type SummaryData struct {
	HealthScore            int       `json:"health_score"`
	Uptime                 time.Time `json:"uptime"`
	NodesReady             int       `json:"nodes_ready"`
	NodesCount             int       `json:"nodes_count"`
	Namespaces             int       `json:"namespaces"`
	PodsRunning            int       `json:"pods_running"`
	PodsAvailable          int       `json:"pods_available"`
	Pressures              int       `json:"pressures"`
	NodePressureCount      int       `json:"node_pressure_count"`
	ImagesCount            int       `json:"images_count"`
	VolumesInUse           int       `json:"volumes_in_use"`
	DeploymentsTotal       int       `json:"deployments_total"`
	DeploymentsReady       int       `json:"deployments_ready"`
	DaemonSetsDesired      int       `json:"daemonsets_desired"`
	DaemonSetsReady        int       `json:"daemonsets_ready"`
	ReplicaSetsDesired     int       `json:"replicasets_desired"`
	ReplicaSetsReady       int       `json:"replicasets_ready"`
	StatefulSetsReady      int       `json:"statefulsets_ready"`
	JobsCount              int       `json:"jobs_count"`
	CronJobsCount          int       `json:"cronjobs_count"`
	PVCount                int       `json:"pv_count"`
	PVsTotalBytes          int64     `json:"pvs_total_bytes"`
	PVCCount               int       `json:"pvc_count"`
	PVCsTotalBytes         int64     `json:"pvcs_total_bytes"`
	AllocatableCPUMillis   int64     `json:"allocatable_cpu_millis"`
	AllocatableMemoryBytes int64     `json:"allocatable_memory_bytes"`
	RequestedCPUMillis     int64     `json:"requested_cpu_millis"`
	RequestedMemoryBytes   int64     `json:"requested_memory_bytes"`
	UsageCPUMillis         int64     `json:"usage_cpu_millis"`
	UsageMemoryBytes       int64     `json:"usage_memory_bytes"`
	HasUsageCPU            bool      `json:"has_usage_cpu,omitempty"`
	HasUsageMemory         bool      `json:"has_usage_memory,omitempty"`
	ContainerCount         int       `json:"container_count"`
	ContainerRestarts      int       `json:"container_restarts"`
	FailedPods             int       `json:"failed_pods"`
	EvictedPods            int       `json:"evicted_pods"`
	OOMKillCount           int       `json:"oom_kill_count"`
	CrashLoopPods          int       `json:"crashloop_pods"`
	NetworkRxRateBytesPerS float64   `json:"network_rx_rate_bytes_per_sec"`
	NetworkTxRateBytesPerS float64   `json:"network_tx_rate_bytes_per_sec"`
	DiskReadRateBytesPerS  float64   `json:"disk_read_rate_bytes_per_sec"`
	DiskWriteRateBytesPerS float64   `json:"disk_write_rate_bytes_per_sec"`
	AvgNodeCPUStallPct     float64   `json:"avg_node_cpu_stall_pct"`
	AvgNodeMemStallPct     float64   `json:"avg_node_mem_stall_pct"`
	AvgNodeIOStallPct      float64   `json:"avg_node_io_stall_pct"`
}

// PSIData holds Pressure Stall Information percentages (0.0-100.0).
type PSIData struct {
	CPUStallPct   float64 `json:"cpu_stall_pct"`
	CPUStalledPct float64 `json:"cpu_stalled_pct"`
	MemStallPct   float64 `json:"mem_stall_pct"`
	MemStalledPct float64 `json:"mem_stalled_pct"`
	IOStallPct    float64 `json:"io_stall_pct"`
	IOStalledPct  float64 `json:"io_stalled_pct"`
}

// NodeData captures a single node's metadata, capacity, and usage metrics.
type NodeData struct {
	Name                    string    `json:"name"`
	Roles                   []string  `json:"roles,omitempty"`
	Controller              bool      `json:"controller"`
	Hostname                string    `json:"hostname,omitempty"`
	Status                  string    `json:"status"`
	Pressures               []string  `json:"pressures,omitempty"`
	CreationTime            time.Time `json:"creation_time"`
	TimeSinceStart          string    `json:"time_since_start"`
	InternalIP              string    `json:"internal_ip,omitempty"`
	ExternalIP              string    `json:"external_ip,omitempty"`
	PodsCount               int       `json:"pods_count"`
	ContainerImagesCount    int       `json:"container_images_count"`
	VolumesInUse            int       `json:"volumes_in_use"`
	VolumesAttached         int       `json:"volumes_attached"`
	TaintCount              int       `json:"taint_count"`
	Unschedulable           bool      `json:"unschedulable"`
	Restarts                int       `json:"restarts"`
	KubeletVersion          string    `json:"kubelet_version,omitempty"`
	OS                      string    `json:"os,omitempty"`
	OSImage                 string    `json:"os_image,omitempty"`
	OSKernel                string    `json:"os_kernel,omitempty"`
	Architecture            string    `json:"architecture,omitempty"`
	ContainerRuntimeVersion string    `json:"container_runtime_version,omitempty"`
	RequestedCPUMillis      int64     `json:"requested_cpu_millis"`
	RequestedMemoryBytes    int64     `json:"requested_memory_bytes"`
	AllocatableCPUMillis    int64     `json:"allocatable_cpu_millis"`
	AllocatableMemoryBytes  int64     `json:"allocatable_memory_bytes"`
	AllocatableStorageBytes int64     `json:"allocatable_storage_bytes"`
	UsageCPUMillis          int64     `json:"usage_cpu_millis"`
	UsageMemoryBytes        int64     `json:"usage_memory_bytes"`
	HasUsageCPU             bool      `json:"has_usage_cpu,omitempty"`
	HasUsageMemory          bool      `json:"has_usage_memory,omitempty"`
	PSI                     PSIData   `json:"psi"`
	PSIKernelSupported      bool      `json:"psi_kernel_supported"`
}

// PodData captures a single pod's metadata, status, and resource metrics.
type PodData struct {
	Namespace              string          `json:"namespace"`
	Name                   string          `json:"name"`
	Status                 string          `json:"status"`
	Node                   string          `json:"node,omitempty"`
	IP                     string          `json:"ip,omitempty"`
	TimeSince              string          `json:"time_since"`
	CreationTime           time.Time       `json:"creation_time"`
	RequestedCPUMillis     int64           `json:"requested_cpu_millis"`
	RequestedMemoryBytes   int64           `json:"requested_memory_bytes"`
	UsageCPUMillis         int64           `json:"usage_cpu_millis"`
	UsageMemoryBytes       int64           `json:"usage_memory_bytes"`
	NodeAllocCPUMillis     int64           `json:"node_allocatable_cpu_millis"`
	NodeAllocMemoryBytes   int64           `json:"node_allocatable_memory_bytes"`
	ReadyContainers        int             `json:"ready_containers"`
	TotalContainers        int             `json:"total_containers"`
	Restarts               int             `json:"restarts"`
	Volumes                int             `json:"volumes"`
	VolMounts              int             `json:"vol_mounts"`
	PSI                    PSIData         `json:"psi"`
	NodePSIKernelSupported bool            `json:"node_psi_kernel_supported"`
	CPUThrottledPct        float64         `json:"cpu_throttled_pct"`
	Containers             []ContainerData `json:"containers,omitempty"`
}

// ContainerData captures per-container metrics within a pod.
type ContainerData struct {
	Name            string  `json:"name"`
	UsageCPUMillis  int64   `json:"usage_cpu_millis"`
	UsageMemBytes   int64   `json:"usage_memory_bytes"`
	CPUThrottledPct float64 `json:"cpu_throttled_pct"`
	RestartCount    int     `json:"restart_count"`
	PSI             PSIData `json:"psi"`
}

// Clone returns a deep copy of the Snapshot so callers can inspect or pass it
// safely without aliasing underlying slices.
func (s *Snapshot) Clone() *Snapshot {
	if s == nil {
		return nil
	}
	cp := *s
	cp.Nodes = make([]NodeData, len(s.Nodes))
	for i, n := range s.Nodes {
		n.Roles = slices.Clone(n.Roles)
		n.Pressures = slices.Clone(n.Pressures)
		cp.Nodes[i] = n
	}
	cp.Pods = make([]PodData, len(s.Pods))
	for i, p := range s.Pods {
		p.Containers = slices.Clone(p.Containers)
		cp.Pods[i] = p
	}
	return &cp
}

// NodeByName returns the NodeData for the named node, if present.
func (s *Snapshot) NodeByName(name string) (NodeData, bool) {
	if s == nil {
		return NodeData{}, false
	}
	for _, n := range s.Nodes {
		if n.Name == name {
			return n, true
		}
	}
	return NodeData{}, false
}

// PodByKey returns the PodData for the given namespace and pod name, if present.
func (s *Snapshot) PodByKey(namespace, name string) (PodData, bool) {
	if s == nil {
		return PodData{}, false
	}
	for _, p := range s.Pods {
		if p.Namespace == namespace && p.Name == name {
			return p, true
		}
	}
	return PodData{}, false
}

// PodsOnNode returns all pods scheduled onto nodeName.
func (s *Snapshot) PodsOnNode(nodeName string) []PodData {
	if s == nil {
		return nil
	}
	var out []PodData
	for _, p := range s.Pods {
		if p.Node == nodeName {
			out = append(out, p)
		}
	}
	return out
}
