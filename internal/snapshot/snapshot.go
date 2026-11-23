package snapshot

import (
	"slices"
	"time"
)

// Snapshot is an immutable, point-in-time capture of cluster state and metrics.
// It contains only pure value types with no mutexes or channels, suitable for
// both synchronous TUI rendering and structured JSON serialization.
type Snapshot struct {
	Timestamp  time.Time       `json:"timestamp"`
	Cluster    ClusterInfo     `json:"cluster"`
	Summary    SummaryData     `json:"summary"`
	Nodes      []NodeData      `json:"nodes"`
	Pods       []PodData       `json:"pods"`
	Workloads  *WorkloadsData  `json:"workloads,omitempty"`
	Events     *EventsData     `json:"events,omitempty"`
	Governance *GovernanceData `json:"governance,omitempty"`
	SLIs       *SLIData        `json:"slis,omitempty"`
}

// GovernanceData captures kube-state-metrics HPA, PDB, ResourceQuota, and CronJob telemetry.
type GovernanceData struct {
	Available      bool                `json:"available"`
	Endpoint       string              `json:"endpoint,omitempty"`
	HPAs           []HPAData           `json:"hpas,omitempty"`
	PDBs           []PDBData           `json:"pdbs,omitempty"`
	ResourceQuotas []ResourceQuotaData `json:"resource_quotas,omitempty"`
	CronJobs       []CronJobData       `json:"cronjobs,omitempty"`
}

// HPAData captures HorizontalPodAutoscaler spec, live replica counts, and scaling flags.
type HPAData struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	TargetKind      string `json:"target_kind,omitempty"`
	TargetName      string `json:"target_name,omitempty"`
	MinReplicas     int32  `json:"min_replicas"`
	MaxReplicas     int32  `json:"max_replicas"`
	CurrentReplicas int32  `json:"current_replicas"`
	DesiredReplicas int32  `json:"desired_replicas"`
	ScalingLimited  bool   `json:"scaling_limited,omitempty"`
}

// PDBData captures PodDisruptionBudget health and allowed disruptions.
type PDBData struct {
	Namespace          string `json:"namespace"`
	Name               string `json:"name"`
	CurrentHealthy     int32  `json:"current_healthy"`
	DesiredHealthy     int32  `json:"desired_healthy"`
	ExpectedPods       int32  `json:"expected_pods"`
	DisruptionsAllowed int32  `json:"disruptions_allowed"`
}

// ResourceQuotaData captures hard limit vs. used utilization for one quota resource.
type ResourceQuotaData struct {
	Namespace string  `json:"namespace"`
	Name      string  `json:"name"`
	Resource  string  `json:"resource"`
	Hard      float64 `json:"hard"`
	Used      float64 `json:"used"`
	UsedPct   float64 `json:"used_pct"`
}

// CronJobData captures CronJob active/suspended status and schedule timestamps.
type CronJobData struct {
	Namespace        string    `json:"namespace"`
	Name             string    `json:"name"`
	Suspended        bool      `json:"suspended,omitempty"`
	Active           int32     `json:"active"`
	LastScheduleTime time.Time `json:"last_schedule_time,omitempty"`
	NextScheduleTime time.Time `json:"next_schedule_time,omitempty"`
}

// SLIData captures Kubernetes Component SLIs (/metrics/slis) and Kubelet probe status.
type SLIData struct {
	Available       bool              `json:"available"`
	ProbesAvailable bool              `json:"probes_available,omitempty"`
	Components      []ComponentSLIDat `json:"components,omitempty"`
}

// ComponentSLIDat captures /metrics/slis status for one component target.
type ComponentSLIDat struct {
	Component     string   `json:"component"`
	Target        string   `json:"target"`
	Healthy       bool     `json:"healthy"`
	TotalChecks   int      `json:"total_checks"`
	FailingChecks []string `json:"failing_checks,omitempty"`
}

// PodProbeData captures failing kubelet probe statistics for a container.
type PodProbeData struct {
	Container    string  `json:"container"`
	ProbeType    string  `json:"probe_type"`
	SuccessCount float64 `json:"success_count"`
	FailureCount float64 `json:"failure_count"`
}

// EventsData captures recent Kubernetes Events and summary counts.
type EventsData struct {
	Summary   EventsSummaryData `json:"summary"`
	Items     []EventData       `json:"items,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

// EventsSummaryData aggregates event totals and the top firing reason.
type EventsSummaryData struct {
	Total          int    `json:"total"`
	WarningCount   int    `json:"warning_count"`
	NormalCount    int    `json:"normal_count"`
	TopReason      string `json:"top_reason,omitempty"`
	TopReasonCount int    `json:"top_reason_count,omitempty"`
}

// EventData captures a single normalized Kubernetes Event.
type EventData struct {
	UID               string    `json:"uid,omitempty"`
	Namespace         string    `json:"namespace,omitempty"`
	Name              string    `json:"name"`
	Type              string    `json:"type"`
	Reason            string    `json:"reason"`
	Message           string    `json:"message"`
	InvolvedKind      string    `json:"involved_kind,omitempty"`
	InvolvedNamespace string    `json:"involved_namespace,omitempty"`
	InvolvedName      string    `json:"involved_name,omitempty"`
	Source            string    `json:"source,omitempty"`
	Count             int32     `json:"count"`
	FirstSeen         time.Time `json:"first_seen,omitempty"`
	LastSeen          time.Time `json:"last_seen,omitempty"`
}

// WorkloadsData captures cluster Deployments, StatefulSets, DaemonSets, and Jobs.
type WorkloadsData struct {
	HasMetrics   bool              `json:"has_metrics,omitempty"`
	Deployments  []DeploymentData  `json:"deployments,omitempty"`
	StatefulSets []StatefulSetData `json:"statefulsets,omitempty"`
	DaemonSets   []DaemonSetData   `json:"daemonsets,omitempty"`
	Jobs         []JobData         `json:"jobs,omitempty"`
}

// DeploymentData captures a Deployment's replicas, status, and aggregated pod usage.
type DeploymentData struct {
	Namespace      string    `json:"namespace"`
	Name           string    `json:"name"`
	Desired        int32     `json:"desired"`
	Ready          int32     `json:"ready"`
	Updated        int32     `json:"updated"`
	Available      int32     `json:"available"`
	Unavailable    int32     `json:"unavailable"`
	Age            string    `json:"age"`
	CreationTime   time.Time `json:"creation_time"`
	Status         string    `json:"status"`
	Strategy       string    `json:"strategy,omitempty"`
	UsageCPUMillis int64     `json:"usage_cpu_millis,omitempty"`
	UsageMemBytes  int64     `json:"usage_memory_bytes,omitempty"`
	HasUsageCPU    bool      `json:"has_usage_cpu,omitempty"`
	HasUsageMem    bool      `json:"has_usage_memory,omitempty"`
}

// StatefulSetData captures a StatefulSet's replicas, status, and aggregated pod usage.
type StatefulSetData struct {
	Namespace      string    `json:"namespace"`
	Name           string    `json:"name"`
	Desired        int32     `json:"desired"`
	Ready          int32     `json:"ready"`
	Current        int32     `json:"current"`
	Updated        int32     `json:"updated"`
	Age            string    `json:"age"`
	CreationTime   time.Time `json:"creation_time"`
	Status         string    `json:"status"`
	ServiceName    string    `json:"service_name,omitempty"`
	UsageCPUMillis int64     `json:"usage_cpu_millis,omitempty"`
	UsageMemBytes  int64     `json:"usage_memory_bytes,omitempty"`
	HasUsageCPU    bool      `json:"has_usage_cpu,omitempty"`
	HasUsageMem    bool      `json:"has_usage_memory,omitempty"`
}

// DaemonSetData captures a DaemonSet's pod scheduling counts, status, and aggregated usage.
type DaemonSetData struct {
	Namespace      string    `json:"namespace"`
	Name           string    `json:"name"`
	Desired        int32     `json:"desired"`
	Current        int32     `json:"current"`
	Ready          int32     `json:"ready"`
	UpToDate       int32     `json:"up_to_date"`
	Available      int32     `json:"available"`
	Misscheduled   int32     `json:"misscheduled"`
	Age            string    `json:"age"`
	CreationTime   time.Time `json:"creation_time"`
	Status         string    `json:"status"`
	UsageCPUMillis int64     `json:"usage_cpu_millis,omitempty"`
	UsageMemBytes  int64     `json:"usage_memory_bytes,omitempty"`
	HasUsageCPU    bool      `json:"has_usage_cpu,omitempty"`
	HasUsageMem    bool      `json:"has_usage_memory,omitempty"`
}

// JobData captures a batch/v1 Job's execution state and completion counts.
type JobData struct {
	Namespace          string    `json:"namespace"`
	Name               string    `json:"name"`
	CompletionsDesired int32     `json:"completions_desired"`
	HasCompletions     bool      `json:"has_completions,omitempty"`
	Succeeded          int32     `json:"succeeded"`
	Active             int32     `json:"active"`
	Failed             int32     `json:"failed"`
	Suspended          bool      `json:"suspended,omitempty"`
	Age                string    `json:"age"`
	CreationTime       time.Time `json:"creation_time"`
	Status             string    `json:"status"`
	Owner              string    `json:"owner,omitempty"`
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
	SLIAvailable           bool      `json:"sli_available,omitempty"`
	SLIUnhealthyComponents int       `json:"sli_unhealthy_components,omitempty"`
	SLIFailingChecks       []string  `json:"sli_failing_checks,omitempty"`
	ProbeFailingPods       int       `json:"probe_failing_pods,omitempty"`
	KSMAvailable           bool      `json:"ksm_available,omitempty"`
	KSMHPAsLimited         int       `json:"ksm_hpas_limited,omitempty"`
	KSMPDBsAtRisk          int       `json:"ksm_pdbs_at_risk,omitempty"`
	KSMQuotasNearLimit     int       `json:"ksm_quotas_near_limit,omitempty"`
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
	ResizeStatus           string          `json:"resize_status,omitempty"`
	DisruptionReason       string          `json:"disruption_reason,omitempty"`
	ResourceClaims         []string        `json:"resource_claims,omitempty"`
	FailingProbes          []PodProbeData  `json:"failing_probes,omitempty"`
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
	cp.Summary.SLIFailingChecks = slices.Clone(s.Summary.SLIFailingChecks)
	cp.Nodes = make([]NodeData, len(s.Nodes))
	for i, n := range s.Nodes {
		n.Roles = slices.Clone(n.Roles)
		n.Pressures = slices.Clone(n.Pressures)
		cp.Nodes[i] = n
	}
	cp.Pods = make([]PodData, len(s.Pods))
	for i, p := range s.Pods {
		p.Containers = slices.Clone(p.Containers)
		p.ResourceClaims = slices.Clone(p.ResourceClaims)
		p.FailingProbes = slices.Clone(p.FailingProbes)
		cp.Pods[i] = p
	}
	if s.Workloads != nil {
		w := &WorkloadsData{
			HasMetrics:   s.Workloads.HasMetrics,
			Deployments:  slices.Clone(s.Workloads.Deployments),
			StatefulSets: slices.Clone(s.Workloads.StatefulSets),
			DaemonSets:   slices.Clone(s.Workloads.DaemonSets),
			Jobs:         slices.Clone(s.Workloads.Jobs),
		}
		cp.Workloads = w
	}
	if s.Events != nil {
		cp.Events = &EventsData{
			Summary:   s.Events.Summary,
			Items:     slices.Clone(s.Events.Items),
			Truncated: s.Events.Truncated,
		}
	}
	if s.Governance != nil {
		cp.Governance = &GovernanceData{
			Available:      s.Governance.Available,
			Endpoint:       s.Governance.Endpoint,
			HPAs:           slices.Clone(s.Governance.HPAs),
			PDBs:           slices.Clone(s.Governance.PDBs),
			ResourceQuotas: slices.Clone(s.Governance.ResourceQuotas),
			CronJobs:       slices.Clone(s.Governance.CronJobs),
		}
	}
	if s.SLIs != nil {
		comps := make([]ComponentSLIDat, len(s.SLIs.Components))
		for i, c := range s.SLIs.Components {
			c.FailingChecks = slices.Clone(c.FailingChecks)
			comps[i] = c
		}
		cp.SLIs = &SLIData{
			Available:       s.SLIs.Available,
			ProbesAvailable: s.SLIs.ProbesAvailable,
			Components:      comps,
		}
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
