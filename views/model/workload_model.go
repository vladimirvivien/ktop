package model

import (
	"fmt"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadKind identifies a Kubernetes workload resource category.
type WorkloadKind string

const (
	WorkloadKindDeployment  WorkloadKind = "Deployment"
	WorkloadKindStatefulSet WorkloadKind = "StatefulSet"
	WorkloadKindDaemonSet   WorkloadKind = "DaemonSet"
	WorkloadKindJob         WorkloadKind = "Job"
)

// ShortPrefix returns the concise resource prefix used in breadcrumbs and IDs.
func (k WorkloadKind) ShortPrefix() string {
	switch k {
	case WorkloadKindDeployment:
		return "deploy"
	case WorkloadKindStatefulSet:
		return "sts"
	case WorkloadKindDaemonSet:
		return "ds"
	case WorkloadKindJob:
		return "job"
	default:
		return strings.ToLower(string(k))
	}
}

// PluralTitle returns the plural section heading for a workload kind.
func (k WorkloadKind) PluralTitle() string {
	switch k {
	case WorkloadKindDeployment:
		return "Deployments"
	case WorkloadKindStatefulSet:
		return "StatefulSets"
	case WorkloadKindDaemonSet:
		return "DaemonSets"
	case WorkloadKindJob:
		return "Jobs"
	default:
		return string(k)
	}
}

// ParseWorkloadKind normalizes a workload kind or short prefix string.
func ParseWorkloadKind(raw string) WorkloadKind {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "deployment", "deployments", "deploy":
		return WorkloadKindDeployment
	case "statefulset", "statefulsets", "sts":
		return WorkloadKindStatefulSet
	case "daemonset", "daemonsets", "ds":
		return WorkloadKindDaemonSet
	case "job", "jobs":
		return WorkloadKindJob
	default:
		return WorkloadKind(strings.TrimSpace(raw))
	}
}

// WorkloadEfficiency summarizes actual usage vs. requested resources across owned pods.
type WorkloadEfficiency struct {
	HasEfficiency    bool
	EffPct           float64
	HasCPUEfficiency bool
	CPUEffPct        float64
	HasMemEfficiency bool
	MemEffPct        float64
}

// WorkloadUsage summarizes aggregated live CPU and memory usage across active owned pods.
type WorkloadUsage struct {
	CPU    *resource.Quantity
	Mem    *resource.Quantity
	HasCPU bool
	HasMem bool
}

// WorkloadConditionInfo provides a normalized view of a workload status condition.
type WorkloadConditionInfo struct {
	Type    string
	Status  string
	Reason  string
	Message string
	Healthy bool
}

// WorkloadSummaryModel summarizes top-level workload health counts.
type WorkloadSummaryModel struct {
	DeploymentsReady  int
	DeploymentsTotal  int
	StatefulSetsReady int
	StatefulSetsTotal int
	DaemonSetsReady   int
	DaemonSetsTotal   int
	JobsRunning       int
	JobsCompleted     int
	JobsFailed        int
}

// DeploymentModel represents an apps/v1 Deployment in workload views.
type DeploymentModel struct {
	Namespace          string
	Name               string
	Desired            int32
	Ready              int32
	Updated            int32
	Available          int32
	Unavailable        int32
	Age                string
	CreationTime       metav1.Time
	Status             string
	Usage              WorkloadUsage
	Efficiency         WorkloadEfficiency
	Strategy           string
	Generation         int64
	ObservedGeneration int64
	Selector           map[string]string
	Labels             map[string]string
	Annotations        map[string]string
	Conditions         []WorkloadConditionInfo
}

// StatefulSetModel represents an apps/v1 StatefulSet in workload views.
type StatefulSetModel struct {
	Namespace          string
	Name               string
	Desired            int32
	Ready              int32
	Current            int32
	Updated            int32
	Age                string
	CreationTime       metav1.Time
	Status             string
	Usage              WorkloadUsage
	Efficiency         WorkloadEfficiency
	ServiceName        string
	UpdateStrategy     string
	Generation         int64
	ObservedGeneration int64
	Selector           map[string]string
	Labels             map[string]string
	Annotations        map[string]string
	Conditions         []WorkloadConditionInfo
}

// DaemonSetModel represents an apps/v1 DaemonSet in workload views.
type DaemonSetModel struct {
	Namespace          string
	Name               string
	Desired            int32
	Current            int32
	Ready              int32
	UpToDate           int32
	Available          int32
	Misscheduled       int32
	Age                string
	CreationTime       metav1.Time
	Status             string
	Usage              WorkloadUsage
	Efficiency         WorkloadEfficiency
	UpdateStrategy     string
	Generation         int64
	ObservedGeneration int64
	Selector           map[string]string
	Labels             map[string]string
	Annotations        map[string]string
	Conditions         []WorkloadConditionInfo
}

// JobModel represents a batch/v1 Job in workload views.
type JobModel struct {
	Namespace          string
	Name               string
	CompletionsDesired int32
	HasCompletions     bool
	Succeeded          int32
	Active             int32
	Failed             int32
	Suspended          bool
	Age                string
	CreationTime       metav1.Time
	Status             string
	Owner              string
	Usage              WorkloadUsage
	Efficiency         WorkloadEfficiency
	Selector           map[string]string
	Labels             map[string]string
	Annotations        map[string]string
	Conditions         []WorkloadConditionInfo
}

// WorkloadsSnapshot holds a point-in-time snapshot of all cluster workload lists.
type WorkloadsSnapshot struct {
	Deployments  []DeploymentModel
	StatefulSets []StatefulSetModel
	DaemonSets   []DaemonSetModel
	Jobs         []JobModel
	HasMetrics   bool
}

// WorkloadDetailData contains full detail state for any single workload instance.
type WorkloadDetailData struct {
	Kind               WorkloadKind
	Namespace          string
	Name               string
	Status             string
	ReadyText          string
	Desired            int32
	Ready              int32
	Updated            int32
	Available          int32
	Unavailable        int32
	Current            int32
	Misscheduled       int32
	CompletionsText    string
	Active             int32
	Succeeded          int32
	Failed             int32
	Paused             bool
	Strategy           string
	StrategyDetail     string
	Revision           string
	ActiveReplicaSet   string
	RevHistoryLimit    *int32
	ServiceName        string
	PodManagement      string
	VolumeClaims       []string
	NodeSelector       map[string]string
	Owner              string
	Parallelism        *int32
	BackoffLimit       *int32
	JobDuration        string
	Images             []string
	ServiceAccount     string
	Age                string
	Generation         int64
	ObservedGeneration int64
	Efficiency         WorkloadEfficiency
	Selector           map[string]string
	Labels             map[string]string
	Annotations        map[string]string
	Conditions         []WorkloadConditionInfo
	OwnedPods          []*PodModel
	Events             []corev1.Event
	TotalRequestedCPU  *resource.Quantity
	TotalRequestedMem  *resource.Quantity
	TotalLimitCPU      *resource.Quantity
	TotalLimitMem      *resource.Quantity
	TotalUsageCPU      *resource.Quantity
	TotalUsageMem      *resource.Quantity
	RawObject          any
}

// NewDeploymentModel constructs a DeploymentModel from a Kubernetes Deployment.
func NewDeploymentModel(d *appsv1.Deployment) DeploymentModel {
	var desired int32 = 1
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}

	conds := make([]WorkloadConditionInfo, 0, len(d.Status.Conditions))
	for _, c := range d.Status.Conditions {
		healthy := c.Status == corev1.ConditionTrue
		if c.Type == appsv1.DeploymentReplicaFailure {
			healthy = c.Status == corev1.ConditionFalse
		}
		conds = append(conds, WorkloadConditionInfo{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
			Healthy: healthy,
		})
	}

	var selector map[string]string
	if d.Spec.Selector != nil {
		selector = d.Spec.Selector.MatchLabels
	}

	return DeploymentModel{
		Namespace:          d.Namespace,
		Name:               d.Name,
		Desired:            desired,
		Ready:              d.Status.ReadyReplicas,
		Updated:            d.Status.UpdatedReplicas,
		Available:          d.Status.AvailableReplicas,
		Unavailable:        d.Status.UnavailableReplicas,
		Age:                strings.TrimSpace(timeSince(d.CreationTimestamp)),
		CreationTime:       d.CreationTimestamp,
		Status:             deriveDeploymentStatus(desired, d.Status),
		Strategy:           string(d.Spec.Strategy.Type),
		Generation:         d.Generation,
		ObservedGeneration: d.Status.ObservedGeneration,
		Selector:           selector,
		Labels:             d.Labels,
		Annotations:        d.Annotations,
		Conditions:         conds,
	}
}

func deriveDeploymentStatus(desired int32, st appsv1.DeploymentStatus) string {
	for _, c := range st.Conditions {
		if c.Type == appsv1.DeploymentReplicaFailure && c.Status == corev1.ConditionTrue {
			return "Degraded"
		}
	}
	if desired == 0 {
		if st.Replicas == 0 {
			return "Scaled 0"
		}
		return "Progressing"
	}
	if st.ReadyReplicas >= desired && st.UpdatedReplicas >= desired && st.AvailableReplicas >= desired && st.UnavailableReplicas == 0 {
		return "Ready"
	}
	for _, c := range st.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Status == corev1.ConditionFalse {
			return "Degraded"
		}
	}
	if st.UpdatedReplicas < desired || st.ReadyReplicas < desired || st.AvailableReplicas < desired {
		if st.UnavailableReplicas > 0 && st.AvailableReplicas == 0 {
			return "Degraded"
		}
		return "Progressing"
	}
	return "Ready"
}

// NewStatefulSetModel constructs a StatefulSetModel from a Kubernetes StatefulSet.
func NewStatefulSetModel(s *appsv1.StatefulSet) StatefulSetModel {
	var desired int32 = 1
	if s.Spec.Replicas != nil {
		desired = *s.Spec.Replicas
	}

	conds := make([]WorkloadConditionInfo, 0, len(s.Status.Conditions))
	for _, c := range s.Status.Conditions {
		conds = append(conds, WorkloadConditionInfo{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
			Healthy: c.Status == corev1.ConditionTrue,
		})
	}

	var selector map[string]string
	if s.Spec.Selector != nil {
		selector = s.Spec.Selector.MatchLabels
	}

	status := "Ready"
	if desired == 0 {
		if s.Status.Replicas == 0 {
			status = "Scaled 0"
		} else {
			status = "Progressing"
		}
	} else if s.Status.ReadyReplicas == 0 {
		status = "Degraded"
	} else if s.Status.ReadyReplicas < desired || s.Status.UpdatedReplicas < desired {
		status = "Progressing"
	}

	return StatefulSetModel{
		Namespace:          s.Namespace,
		Name:               s.Name,
		Desired:            desired,
		Ready:              s.Status.ReadyReplicas,
		Current:            s.Status.CurrentReplicas,
		Updated:            s.Status.UpdatedReplicas,
		Age:                strings.TrimSpace(timeSince(s.CreationTimestamp)),
		CreationTime:       s.CreationTimestamp,
		Status:             status,
		ServiceName:        s.Spec.ServiceName,
		UpdateStrategy:     string(s.Spec.UpdateStrategy.Type),
		Generation:         s.Generation,
		ObservedGeneration: s.Status.ObservedGeneration,
		Selector:           selector,
		Labels:             s.Labels,
		Annotations:        s.Annotations,
		Conditions:         conds,
	}
}

// NewDaemonSetModel constructs a DaemonSetModel from a Kubernetes DaemonSet.
func NewDaemonSetModel(ds *appsv1.DaemonSet) DaemonSetModel {
	conds := make([]WorkloadConditionInfo, 0, len(ds.Status.Conditions))
	for _, c := range ds.Status.Conditions {
		conds = append(conds, WorkloadConditionInfo{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
			Healthy: c.Status == corev1.ConditionTrue,
		})
	}

	var selector map[string]string
	if ds.Spec.Selector != nil {
		selector = ds.Spec.Selector.MatchLabels
	}

	status := "Ready"
	if ds.Status.DesiredNumberScheduled == 0 {
		status = "Scaled 0"
	} else if ds.Status.NumberUnavailable > 0 && ds.Status.NumberReady == 0 {
		status = "Degraded"
	} else if ds.Status.NumberReady < ds.Status.DesiredNumberScheduled ||
		ds.Status.UpdatedNumberScheduled < ds.Status.DesiredNumberScheduled ||
		ds.Status.NumberMisscheduled > 0 {
		status = "Progressing"
	}

	return DaemonSetModel{
		Namespace:          ds.Namespace,
		Name:               ds.Name,
		Desired:            ds.Status.DesiredNumberScheduled,
		Current:            ds.Status.CurrentNumberScheduled,
		Ready:              ds.Status.NumberReady,
		UpToDate:           ds.Status.UpdatedNumberScheduled,
		Available:          ds.Status.NumberAvailable,
		Misscheduled:       ds.Status.NumberMisscheduled,
		Age:                strings.TrimSpace(timeSince(ds.CreationTimestamp)),
		CreationTime:       ds.CreationTimestamp,
		Status:             status,
		UpdateStrategy:     string(ds.Spec.UpdateStrategy.Type),
		Generation:         ds.Generation,
		ObservedGeneration: ds.Status.ObservedGeneration,
		Selector:           selector,
		Labels:             ds.Labels,
		Annotations:        ds.Annotations,
		Conditions:         conds,
	}
}

// NewJobModel constructs a JobModel from a Kubernetes Job.
func NewJobModel(job *batchv1.Job) JobModel {
	var desired int32 = 1
	hasCompletions := false
	if job.Spec.Completions != nil {
		desired = *job.Spec.Completions
		hasCompletions = true
	}

	conds := make([]WorkloadConditionInfo, 0, len(job.Status.Conditions))
	for _, c := range job.Status.Conditions {
		healthy := c.Status == corev1.ConditionTrue
		if c.Type == batchv1.JobFailed || c.Type == batchv1.JobFailureTarget {
			healthy = c.Status != corev1.ConditionTrue
		}
		conds = append(conds, WorkloadConditionInfo{
			Type:    string(c.Type),
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
			Healthy: healthy,
		})
	}

	var selector map[string]string
	if job.Spec.Selector != nil {
		selector = job.Spec.Selector.MatchLabels
	}

	owner := ""
	for _, ref := range job.OwnerReferences {
		if ref.Kind != "" && ref.Name != "" {
			owner = ref.Kind + "/" + ref.Name
			break
		}
	}

	suspended := job.Spec.Suspend != nil && *job.Spec.Suspend
	status := deriveJobStatus(job, desired, suspended)

	return JobModel{
		Namespace:          job.Namespace,
		Name:               job.Name,
		CompletionsDesired: desired,
		HasCompletions:     hasCompletions,
		Succeeded:          job.Status.Succeeded,
		Active:             job.Status.Active,
		Failed:             job.Status.Failed,
		Suspended:          suspended,
		Age:                strings.TrimSpace(timeSince(job.CreationTimestamp)),
		CreationTime:       job.CreationTimestamp,
		Status:             status,
		Owner:              owner,
		Selector:           selector,
		Labels:             job.Labels,
		Annotations:        job.Annotations,
		Conditions:         conds,
	}
}

func deriveJobStatus(job *batchv1.Job, desired int32, suspended bool) string {
	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			return "Complete"
		}
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return "Failed"
		}
	}
	if suspended {
		return "Suspended"
	}
	if job.Status.Active > 0 {
		return "Running"
	}
	if job.Status.Succeeded >= desired && desired > 0 {
		return "Complete"
	}
	if job.Status.Failed > 0 {
		return "Failed"
	}
	return "Running"
}

// ComputePodsEfficiency calculates CPU, memory, and combined resource efficiency
// (actual usage / requested * 100) across a workload's active pods.
// EffPct uses the highest active dimension so either over-provisioning (<25% on both)
// or single-dimension saturation (>90% on CPU or memory) surfaces cleanly.
func ComputePodsEfficiency(pods []*PodModel, hasMetrics bool) WorkloadEfficiency {
	if !hasMetrics || len(pods) == 0 {
		return WorkloadEfficiency{}
	}

	var cpuSum, memSum float64
	var cpuCount, memCount int

	for _, pod := range pods {
		if pod == nil {
			continue
		}
		if pod.Status == "Completed" || pod.Status == "Succeeded" {
			continue
		}
		if pod.PodRequestedCpuQty != nil && pod.PodRequestedCpuQty.MilliValue() > 0 && pod.PodUsageCpuQty != nil {
			ratio := float64(pod.PodUsageCpuQty.MilliValue()) / float64(pod.PodRequestedCpuQty.MilliValue()) * 100.0
			cpuSum += ratio
			cpuCount++
		}
		if pod.PodRequestedMemQty != nil && pod.PodRequestedMemQty.Value() > 0 && pod.PodUsageMemQty != nil {
			ratio := float64(pod.PodUsageMemQty.Value()) / float64(pod.PodRequestedMemQty.Value()) * 100.0
			memSum += ratio
			memCount++
		}
	}

	var eff WorkloadEfficiency
	if cpuCount > 0 {
		eff.HasCPUEfficiency = true
		eff.CPUEffPct = cpuSum / float64(cpuCount)
	}
	if memCount > 0 {
		eff.HasMemEfficiency = true
		eff.MemEffPct = memSum / float64(memCount)
	}

	switch {
	case eff.HasCPUEfficiency && eff.HasMemEfficiency:
		eff.HasEfficiency = true
		eff.EffPct = max(eff.CPUEffPct, eff.MemEffPct)
	case eff.HasCPUEfficiency:
		eff.HasEfficiency = true
		eff.EffPct = eff.CPUEffPct
	case eff.HasMemEfficiency:
		eff.HasEfficiency = true
		eff.EffPct = eff.MemEffPct
	}

	return eff
}

// ComputePodsUsage sums live CPU and memory usage across a workload's active owned pods.
func ComputePodsUsage(pods []*PodModel, hasMetrics bool) WorkloadUsage {
	if !hasMetrics || len(pods) == 0 {
		return WorkloadUsage{}
	}

	var cpuMillis int64
	var memBytes int64
	var hasCPU, hasMem bool

	for _, pod := range pods {
		if pod == nil {
			continue
		}
		if pod.Status == "Completed" || pod.Status == "Succeeded" {
			continue
		}
		if pod.PodUsageCpuQty != nil {
			cpuMillis += pod.PodUsageCpuQty.MilliValue()
			hasCPU = true
		}
		if pod.PodUsageMemQty != nil {
			memBytes += pod.PodUsageMemQty.Value()
			hasMem = true
		}
	}

	var usage WorkloadUsage
	if hasCPU {
		usage.HasCPU = true
		usage.CPU = resource.NewMilliQuantity(cpuMillis, resource.DecimalSI)
	}
	if hasMem {
		usage.HasMem = true
		usage.Mem = resource.NewQuantity(memBytes, resource.BinarySI)
	}
	return usage
}

// ComputeWorkloadSummary computes top-level counts from snap, optionally filtered by namespace substring.
func ComputeWorkloadSummary(snap WorkloadsSnapshot, nsFilter string) WorkloadSummaryModel {
	var summary WorkloadSummaryModel
	filterLower := strings.ToLower(strings.TrimSpace(nsFilter))
	matchesNS := func(ns string) bool {
		if filterLower == "" {
			return true
		}
		return strings.Contains(strings.ToLower(ns), filterLower)
	}

	for _, d := range snap.Deployments {
		if !matchesNS(d.Namespace) {
			continue
		}
		summary.DeploymentsTotal++
		if d.Desired > 0 && d.Ready >= d.Desired && d.Available >= d.Desired {
			summary.DeploymentsReady++
		} else if d.Desired == 0 && d.Ready == 0 {
			summary.DeploymentsReady++
		}
	}

	for _, s := range snap.StatefulSets {
		if !matchesNS(s.Namespace) {
			continue
		}
		summary.StatefulSetsTotal++
		if s.Ready >= s.Desired {
			summary.StatefulSetsReady++
		}
	}

	for _, ds := range snap.DaemonSets {
		if !matchesNS(ds.Namespace) {
			continue
		}
		summary.DaemonSetsTotal++
		if ds.Ready >= ds.Desired && ds.Desired > 0 {
			summary.DaemonSetsReady++
		} else if ds.Desired == 0 && ds.Ready == 0 {
			summary.DaemonSetsReady++
		}
	}

	for _, j := range snap.Jobs {
		if !matchesNS(j.Namespace) {
			continue
		}
		switch j.Status {
		case "Complete":
			summary.JobsCompleted++
		case "Failed":
			summary.JobsFailed++
		default:
			summary.JobsRunning++
		}
	}

	return summary
}

// FormatJobCompletions formats a Job's completion progress string (e.g., "1/1").
func (j JobModel) FormatCompletions() string {
	return fmt.Sprintf("%d/%d", j.Succeeded, j.CompletionsDesired)
}

func sortSliceBy[T any](items []T, ascending bool, less func(a, b T) bool) {
	if len(items) <= 1 {
		return
	}
	slices.SortFunc(items, func(a, b T) int {
		if less(a, b) {
			if ascending {
				return -1
			}
			return 1
		}
		if less(b, a) {
			if ascending {
				return 1
			}
			return -1
		}
		return 0
	})
}

func compareNamespaced(nsA, nameA, nsB, nameB string) bool {
	if nsA == nsB {
		return nameA < nameB
	}
	return nsA < nsB
}

func compareEfficiency(a, b WorkloadEfficiency, nameA, nameB string) bool {
	if a.HasEfficiency != b.HasEfficiency {
		return a.HasEfficiency
	}
	if a.EffPct == b.EffPct {
		return nameA < nameB
	}
	return a.EffPct < b.EffPct
}

func compareUsageCPU(a, b WorkloadUsage, nameA, nameB string) bool {
	if a.HasCPU != b.HasCPU {
		return a.HasCPU
	}
	var va, vb int64
	if a.CPU != nil {
		va = a.CPU.MilliValue()
	}
	if b.CPU != nil {
		vb = b.CPU.MilliValue()
	}
	if va == vb {
		return nameA < nameB
	}
	return va < vb
}

func compareUsageMem(a, b WorkloadUsage, nameA, nameB string) bool {
	if a.HasMem != b.HasMem {
		return a.HasMem
	}
	var va, vb int64
	if a.Mem != nil {
		va = a.Mem.Value()
	}
	if b.Mem != nil {
		vb = b.Mem.Value()
	}
	if va == vb {
		return nameA < nameB
	}
	return va < vb
}

func workloadStatusPriority(status string) int {
	switch status {
	case "Degraded", "Failed":
		return 0
	case "Progressing", "Suspended", "Running":
		return 1
	case "Ready", "Complete", "Scaled 0":
		return 2
	default:
		return 3
	}
}

// SortDeploymentModelsBy sorts Deployment models in-place by column and direction.
func SortDeploymentModelsBy(items []DeploymentModel, column string, ascending bool) {
	sortSliceBy(items, ascending, func(a, b DeploymentModel) bool {
		switch column {
		case "NAME":
			if a.Name == b.Name {
				return a.Namespace < b.Namespace
			}
			return a.Name < b.Name
		case "READY":
			ra := float64(a.Ready) / float64(max(a.Desired, 1))
			rb := float64(b.Ready) / float64(max(b.Desired, 1))
			if ra == rb {
				return a.Name < b.Name
			}
			return ra < rb
		case "UP-TO-DATE", "UP":
			if a.Updated == b.Updated {
				return a.Name < b.Name
			}
			return a.Updated < b.Updated
		case "AVAIL":
			if a.Available == b.Available {
				return a.Name < b.Name
			}
			return a.Available < b.Available
		case "CPU":
			return compareUsageCPU(a.Usage, b.Usage, a.Name, b.Name)
		case "MEM":
			return compareUsageMem(a.Usage, b.Usage, a.Name, b.Name)
		case "AGE":
			if a.CreationTime.Equal(&b.CreationTime) {
				return a.Name < b.Name
			}
			return a.CreationTime.Before(&b.CreationTime)
		case "EFF":
			return compareEfficiency(a.Efficiency, b.Efficiency, a.Name, b.Name)
		case "STATUS":
			pa, pb := workloadStatusPriority(a.Status), workloadStatusPriority(b.Status)
			if pa == pb {
				return a.Name < b.Name
			}
			return pa < pb
		default:
			return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
		}
	})
}

// SortStatefulSetModelsBy sorts StatefulSet models in-place by column and direction.
func SortStatefulSetModelsBy(items []StatefulSetModel, column string, ascending bool) {
	sortSliceBy(items, ascending, func(a, b StatefulSetModel) bool {
		switch column {
		case "NAME":
			if a.Name == b.Name {
				return a.Namespace < b.Namespace
			}
			return a.Name < b.Name
		case "READY":
			ra := float64(a.Ready) / float64(max(a.Desired, 1))
			rb := float64(b.Ready) / float64(max(b.Desired, 1))
			if ra == rb {
				return a.Name < b.Name
			}
			return ra < rb
		case "CPU":
			return compareUsageCPU(a.Usage, b.Usage, a.Name, b.Name)
		case "MEM":
			return compareUsageMem(a.Usage, b.Usage, a.Name, b.Name)
		case "AGE":
			if a.CreationTime.Equal(&b.CreationTime) {
				return a.Name < b.Name
			}
			return a.CreationTime.Before(&b.CreationTime)
		case "EFF":
			return compareEfficiency(a.Efficiency, b.Efficiency, a.Name, b.Name)
		case "STATUS":
			pa, pb := workloadStatusPriority(a.Status), workloadStatusPriority(b.Status)
			if pa == pb {
				return a.Name < b.Name
			}
			return pa < pb
		default:
			return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
		}
	})
}

// SortDaemonSetModelsBy sorts DaemonSet models in-place by column and direction.
func SortDaemonSetModelsBy(items []DaemonSetModel, column string, ascending bool) {
	sortSliceBy(items, ascending, func(a, b DaemonSetModel) bool {
		switch column {
		case "NAME":
			if a.Name == b.Name {
				return a.Namespace < b.Namespace
			}
			return a.Name < b.Name
		case "DESIRED":
			if a.Desired == b.Desired {
				return a.Name < b.Name
			}
			return a.Desired < b.Desired
		case "CURRENT":
			if a.Current == b.Current {
				return a.Name < b.Name
			}
			return a.Current < b.Current
		case "READY":
			ra := float64(a.Ready) / float64(max(a.Desired, 1))
			rb := float64(b.Ready) / float64(max(b.Desired, 1))
			if ra == rb {
				return a.Name < b.Name
			}
			return ra < rb
		case "CPU":
			return compareUsageCPU(a.Usage, b.Usage, a.Name, b.Name)
		case "MEM":
			return compareUsageMem(a.Usage, b.Usage, a.Name, b.Name)
		case "AGE":
			if a.CreationTime.Equal(&b.CreationTime) {
				return a.Name < b.Name
			}
			return a.CreationTime.Before(&b.CreationTime)
		case "EFF":
			return compareEfficiency(a.Efficiency, b.Efficiency, a.Name, b.Name)
		case "STATUS":
			pa, pb := workloadStatusPriority(a.Status), workloadStatusPriority(b.Status)
			if pa == pb {
				return a.Name < b.Name
			}
			return pa < pb
		default:
			return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
		}
	})
}

// SortJobModelsBy sorts Job models in-place by column and direction.
func SortJobModelsBy(items []JobModel, column string, ascending bool) {
	sortSliceBy(items, ascending, func(a, b JobModel) bool {
		switch column {
		case "NAME":
			if a.Name == b.Name {
				return a.Namespace < b.Namespace
			}
			return a.Name < b.Name
		case "COMPLETIONS":
			ra := float64(a.Succeeded) / float64(max(a.CompletionsDesired, 1))
			rb := float64(b.Succeeded) / float64(max(b.CompletionsDesired, 1))
			if ra == rb {
				return a.Name < b.Name
			}
			return ra < rb
		case "AGE":
			if a.CreationTime.Equal(&b.CreationTime) {
				return a.Name < b.Name
			}
			return a.CreationTime.Before(&b.CreationTime)
		case "STATUS":
			pa, pb := workloadStatusPriority(a.Status), workloadStatusPriority(b.Status)
			if pa == pb {
				return a.Name < b.Name
			}
			return pa < pb
		default:
			return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
		}
	})
}
