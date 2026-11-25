package snapshot

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/metrics/ksm"
	"github.com/vladimirvivien/ktop/metrics/sli"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildInput supplies in-memory cached state and metrics to SnapshotBuilder.
type BuildInput struct {
	Cluster          ClusterInfo
	Summary          model.ClusterSummary
	Nodes            []model.NodeModel
	Pods             []model.PodModel
	PodMetrics       []*metrics.PodMetrics
	Workloads        *model.WorkloadsSnapshot
	Events           *model.EventsSnapshot
	Storage          *model.StorageSnapshot
	Network          *model.NetworkSnapshot
	PodVolumeUsages  map[string][]PodVolumeUsageData
	PodNetworkErrors map[string]metrics.NetworkErrorRates
	KSM              *ksm.Snapshot
	SLI              *sli.Snapshot
}

// Builder constructs immutable Snapshots via lock-free in-memory joins.
type Builder struct{}

// NewBuilder returns a new SnapshotBuilder.
func NewBuilder() *Builder {
	return &Builder{}
}

// Build joins the provided models and metrics into a point-in-time Snapshot
// without performing network I/O or acquiring locks.
func (b *Builder) Build(ctx context.Context, in BuildInput) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	nodeKernelSupport := make(map[string]bool, len(in.Nodes))
	nodes := make([]NodeData, 0, len(in.Nodes))
	for _, n := range in.Nodes {
		nodeKernelSupport[n.Name] = n.PSIKernelSupported
		nodes = append(nodes, nodeDataFromModel(n))
	}

	podMetricsByKey := make(map[string]*metrics.PodMetrics, len(in.PodMetrics))
	for _, pm := range in.PodMetrics {
		if pm != nil {
			podMetricsByKey[pm.Namespace+"/"+pm.PodName] = pm
		}
	}

	pods := make([]PodData, 0, len(in.Pods))
	for _, p := range in.Pods {
		pd := podDataFromModel(p)
		if ks, ok := nodeKernelSupport[p.Node]; ok {
			pd.NodePSIKernelSupported = ks
		}
		if pm, ok := podMetricsByKey[p.Namespace+"/"+p.Name]; ok {
			var totalCPU, totalMem int64
			containers := make([]ContainerData, 0, len(pm.Containers))
			for _, cm := range pm.Containers {
				var cCPU, cMem int64
				if cm.CPUUsage != nil {
					cCPU = cm.CPUUsage.MilliValue()
					totalCPU += cCPU
				}
				if cm.MemoryUsage != nil {
					cMem = cm.MemoryUsage.Value()
					totalMem += cMem
				}
				containers = append(containers, ContainerData{
					Name:            cm.Name,
					UsageCPUMillis:  cCPU,
					UsageMemBytes:   cMem,
					CPUThrottledPct: cm.CPUThrottled,
					RestartCount:    cm.RestartCount,
					PSI:             psiDataFromMetrics(cm.PSI),
				})
			}
			if totalCPU > 0 || totalMem > 0 {
				pd.UsageCPUMillis = totalCPU
				pd.UsageMemoryBytes = totalMem
			}
			pd.PSI = psiDataFromMetrics(pm.PSI)
			pd.CPUThrottledPct = pm.CPUThrottledPct
			pd.Containers = containers
		}
		podKey := p.Namespace + "/" + p.Name
		if in.KSM != nil && in.KSM.Available {
			if reason := in.KSM.PodDisruptions[podKey]; reason != "" {
				pd.DisruptionReason = reason
			}
			if claims := in.KSM.PodResourceClaims[podKey]; len(claims) > 0 {
				claimNames := make([]string, 0, len(claims))
				for _, cl := range claims {
					name := cl.ResourceClaimName
					if name == "" {
						name = cl.Name
					}
					if name != "" {
						claimNames = append(claimNames, name)
					}
				}
				pd.ResourceClaims = claimNames
			}
		}
		if in.SLI != nil && in.SLI.ProbesAvailable {
			for _, pr := range in.SLI.ProbesForPod(p.Namespace, p.Name) {
				if pr.Failing {
					pd.FailingProbes = append(pd.FailingProbes, PodProbeData{
						Container:    pr.Container,
						ProbeType:    pr.ProbeType,
						SuccessCount: pr.SuccessCount,
						FailureCount: pr.FailureCount,
					})
				}
			}
		}
		if vols := in.PodVolumeUsages[podKey]; len(vols) > 0 {
			pd.VolumeUsages = slices.Clone(vols)
		}
		if rates, ok := in.PodNetworkErrors[podKey]; ok {
			pd.NetworkDropsPerSec = rates.RxDroppedPerSec + rates.TxDroppedPerSec
			pd.NetworkErrorsPerSec = rates.RxErrorsPerSec + rates.TxErrorsPerSec
		}
		pods = append(pods, pd)
	}

	cluster := in.Cluster
	if cluster.MetricsSource == "" {
		cluster.MetricsSource = in.Summary.MetricsSourceType
	}
	cluster.PSIBeta = in.Summary.PSIBeta

	return &Snapshot{
		Timestamp:  time.Now(),
		Cluster:    cluster,
		Summary:    summaryDataFromModel(in.Summary),
		Nodes:      nodes,
		Pods:       pods,
		Workloads:  workloadsDataFromModel(in.Workloads),
		Events:     eventsDataFromModel(in.Events),
		Storage:    storageDataFromModel(in.Storage),
		Network:    networkDataFromModel(in.Network),
		Governance: governanceDataFromKSM(in.KSM),
		SLIs:       sliDataFromSnapshot(in.SLI),
	}, nil
}

func governanceDataFromKSM(k *ksm.Snapshot) *GovernanceData {
	if k == nil || !k.Available {
		return nil
	}
	out := &GovernanceData{
		Available:      true,
		Endpoint:       k.Endpoint,
		HPAs:           make([]HPAData, 0, len(k.HPAs)),
		PDBs:           make([]PDBData, 0, len(k.PDBs)),
		ResourceQuotas: make([]ResourceQuotaData, 0, len(k.ResourceQuotas)),
		CronJobs:       make([]CronJobData, 0, len(k.CronJobs)),
	}
	for _, h := range k.HPAs {
		out.HPAs = append(out.HPAs, HPAData{
			Namespace:       h.Namespace,
			Name:            h.Name,
			TargetKind:      h.TargetKind,
			TargetName:      h.TargetName,
			MinReplicas:     h.MinReplicas,
			MaxReplicas:     h.MaxReplicas,
			CurrentReplicas: h.CurrentReplicas,
			DesiredReplicas: h.DesiredReplicas,
			ScalingLimited:  h.ScalingLimited != nil && *h.ScalingLimited,
		})
	}
	for _, p := range k.PDBs {
		out.PDBs = append(out.PDBs, PDBData{
			Namespace:          p.Namespace,
			Name:               p.Name,
			CurrentHealthy:     p.CurrentHealthy,
			DesiredHealthy:     p.DesiredHealthy,
			ExpectedPods:       p.ExpectedPods,
			DisruptionsAllowed: p.DisruptionsAllowed,
		})
	}
	for _, q := range k.ResourceQuotas {
		out.ResourceQuotas = append(out.ResourceQuotas, ResourceQuotaData{
			Namespace: q.Namespace,
			Name:      q.Name,
			Resource:  q.Resource,
			Hard:      q.Hard,
			Used:      q.Used,
			UsedPct:   q.UsedPct,
		})
	}
	for _, cj := range k.CronJobs {
		out.CronJobs = append(out.CronJobs, CronJobData{
			Namespace:        cj.Namespace,
			Name:             cj.Name,
			Suspended:        cj.Suspended,
			Active:           cj.Active,
			LastScheduleTime: cj.LastScheduleTime,
			NextScheduleTime: cj.NextScheduleTime,
		})
	}
	return out
}

func sliDataFromSnapshot(s *sli.Snapshot) *SLIData {
	if s == nil || (!s.Available && !s.ProbesAvailable) {
		return nil
	}
	out := &SLIData{
		Available:       s.Available,
		ProbesAvailable: s.ProbesAvailable,
		Components:      make([]ComponentSLIDat, 0, len(s.Components)),
	}
	for _, c := range s.Components {
		failing := make([]string, 0, len(c.FailingChecks))
		for _, chk := range c.FailingChecks {
			failing = append(failing, chk.Name)
		}
		out.Components = append(out.Components, ComponentSLIDat{
			Component:     c.Component,
			Target:        c.Target,
			Healthy:       c.Healthy,
			TotalChecks:   c.TotalChecks,
			FailingChecks: failing,
		})
	}
	return out
}

func eventsDataFromModel(ev *model.EventsSnapshot) *EventsData {
	if ev == nil {
		return nil
	}
	out := &EventsData{
		Summary: EventsSummaryData{
			Total:          ev.Summary.Total,
			WarningCount:   ev.Summary.WarningCount,
			NormalCount:    ev.Summary.NormalCount,
			TopReason:      ev.Summary.TopReason,
			TopReasonCount: ev.Summary.TopReasonCount,
		},
		Items:     make([]EventData, 0, len(ev.Events)),
		Truncated: ev.Truncated,
	}
	for _, e := range ev.Events {
		out.Items = append(out.Items, EventData{
			UID:               e.UID,
			Namespace:         e.Namespace,
			Name:              e.Name,
			Type:              e.Type,
			Reason:            e.Reason,
			Message:           e.Message,
			InvolvedKind:      e.InvolvedKind,
			InvolvedNamespace: e.InvolvedNamespace,
			InvolvedName:      e.InvolvedName,
			Source:            e.Source,
			Count:             e.Count,
			FirstSeen:         e.FirstSeen,
			LastSeen:          e.LastSeen,
		})
	}
	return out
}

// ToEventsSnapshot converts EventsData back to model.EventsSnapshot for views.
func (e *EventsData) ToEventsSnapshot() model.EventsSnapshot {
	if e == nil {
		return model.EventsSnapshot{}
	}
	out := model.EventsSnapshot{
		Summary: model.EventsSummary{
			Total:          e.Summary.Total,
			WarningCount:   e.Summary.WarningCount,
			NormalCount:    e.Summary.NormalCount,
			TopReason:      e.Summary.TopReason,
			TopReasonCount: e.Summary.TopReasonCount,
		},
		Events:    make([]model.EventModel, 0, len(e.Items)),
		Truncated: e.Truncated,
	}
	for _, item := range e.Items {
		out.Events = append(out.Events, model.EventModel{
			UID:               item.UID,
			Namespace:         item.Namespace,
			Name:              item.Name,
			Type:              item.Type,
			Reason:            item.Reason,
			Message:           item.Message,
			InvolvedKind:      item.InvolvedKind,
			InvolvedNamespace: item.InvolvedNamespace,
			InvolvedName:      item.InvolvedName,
			Source:            item.Source,
			Count:             item.Count,
			FirstSeen:         item.FirstSeen,
			LastSeen:          item.LastSeen,
		})
	}
	return out
}

func workloadsDataFromModel(w *model.WorkloadsSnapshot) *WorkloadsData {
	if w == nil {
		return nil
	}
	out := &WorkloadsData{
		HasMetrics:   w.HasMetrics,
		Deployments:  make([]DeploymentData, 0, len(w.Deployments)),
		StatefulSets: make([]StatefulSetData, 0, len(w.StatefulSets)),
		DaemonSets:   make([]DaemonSetData, 0, len(w.DaemonSets)),
		Jobs:         make([]JobData, 0, len(w.Jobs)),
	}
	for _, d := range w.Deployments {
		out.Deployments = append(out.Deployments, DeploymentData{
			Namespace:      d.Namespace,
			Name:           d.Name,
			Desired:        d.Desired,
			Ready:          d.Ready,
			Updated:        d.Updated,
			Available:      d.Available,
			Unavailable:    d.Unavailable,
			Age:            d.Age,
			CreationTime:   d.CreationTime.Time,
			Status:         d.Status,
			Strategy:       d.Strategy,
			UsageCPUMillis: qtyMillis(d.Usage.CPU),
			UsageMemBytes:  qtyBytes(d.Usage.Mem),
			HasUsageCPU:    d.Usage.HasCPU,
			HasUsageMem:    d.Usage.HasMem,
		})
	}
	for _, s := range w.StatefulSets {
		out.StatefulSets = append(out.StatefulSets, StatefulSetData{
			Namespace:      s.Namespace,
			Name:           s.Name,
			Desired:        s.Desired,
			Ready:          s.Ready,
			Current:        s.Current,
			Updated:        s.Updated,
			Age:            s.Age,
			CreationTime:   s.CreationTime.Time,
			Status:         s.Status,
			ServiceName:    s.ServiceName,
			UsageCPUMillis: qtyMillis(s.Usage.CPU),
			UsageMemBytes:  qtyBytes(s.Usage.Mem),
			HasUsageCPU:    s.Usage.HasCPU,
			HasUsageMem:    s.Usage.HasMem,
		})
	}
	for _, ds := range w.DaemonSets {
		out.DaemonSets = append(out.DaemonSets, DaemonSetData{
			Namespace:      ds.Namespace,
			Name:           ds.Name,
			Desired:        ds.Desired,
			Current:        ds.Current,
			Ready:          ds.Ready,
			UpToDate:       ds.UpToDate,
			Available:      ds.Available,
			Misscheduled:   ds.Misscheduled,
			Age:            ds.Age,
			CreationTime:   ds.CreationTime.Time,
			Status:         ds.Status,
			UsageCPUMillis: qtyMillis(ds.Usage.CPU),
			UsageMemBytes:  qtyBytes(ds.Usage.Mem),
			HasUsageCPU:    ds.Usage.HasCPU,
			HasUsageMem:    ds.Usage.HasMem,
		})
	}
	for _, j := range w.Jobs {
		out.Jobs = append(out.Jobs, JobData{
			Namespace:          j.Namespace,
			Name:               j.Name,
			CompletionsDesired: j.CompletionsDesired,
			HasCompletions:     j.HasCompletions,
			Succeeded:          j.Succeeded,
			Active:             j.Active,
			Failed:             j.Failed,
			Suspended:          j.Suspended,
			Age:                j.Age,
			CreationTime:       j.CreationTime.Time,
			Status:             j.Status,
			Owner:              j.Owner,
		})
	}
	return out
}

// ToWorkloadsSnapshot converts WorkloadsData back to model.WorkloadsSnapshot for views.
func (w *WorkloadsData) ToWorkloadsSnapshot() model.WorkloadsSnapshot {
	if w == nil {
		return model.WorkloadsSnapshot{}
	}
	out := model.WorkloadsSnapshot{
		HasMetrics:   w.HasMetrics,
		Deployments:  make([]model.DeploymentModel, 0, len(w.Deployments)),
		StatefulSets: make([]model.StatefulSetModel, 0, len(w.StatefulSets)),
		DaemonSets:   make([]model.DaemonSetModel, 0, len(w.DaemonSets)),
		Jobs:         make([]model.JobModel, 0, len(w.Jobs)),
	}
	usageFrom := func(cpuMillis, memBytes int64, hasCPU, hasMem bool) model.WorkloadUsage {
		u := model.WorkloadUsage{HasCPU: hasCPU, HasMem: hasMem}
		if hasCPU {
			u.CPU = resource.NewMilliQuantity(cpuMillis, resource.DecimalSI)
		}
		if hasMem {
			u.Mem = resource.NewQuantity(memBytes, resource.DecimalSI)
		}
		return u
	}
	for _, d := range w.Deployments {
		out.Deployments = append(out.Deployments, model.DeploymentModel{
			Namespace:    d.Namespace,
			Name:         d.Name,
			Desired:      d.Desired,
			Ready:        d.Ready,
			Updated:      d.Updated,
			Available:    d.Available,
			Unavailable:  d.Unavailable,
			Age:          d.Age,
			CreationTime: metav1.NewTime(d.CreationTime),
			Status:       d.Status,
			Strategy:     d.Strategy,
			Usage:        usageFrom(d.UsageCPUMillis, d.UsageMemBytes, d.HasUsageCPU, d.HasUsageMem),
		})
	}
	for _, s := range w.StatefulSets {
		out.StatefulSets = append(out.StatefulSets, model.StatefulSetModel{
			Namespace:    s.Namespace,
			Name:         s.Name,
			Desired:      s.Desired,
			Ready:        s.Ready,
			Current:      s.Current,
			Updated:      s.Updated,
			Age:          s.Age,
			CreationTime: metav1.NewTime(s.CreationTime),
			Status:       s.Status,
			ServiceName:  s.ServiceName,
			Usage:        usageFrom(s.UsageCPUMillis, s.UsageMemBytes, s.HasUsageCPU, s.HasUsageMem),
		})
	}
	for _, ds := range w.DaemonSets {
		out.DaemonSets = append(out.DaemonSets, model.DaemonSetModel{
			Namespace:    ds.Namespace,
			Name:         ds.Name,
			Desired:      ds.Desired,
			Current:      ds.Current,
			Ready:        ds.Ready,
			UpToDate:     ds.UpToDate,
			Available:    ds.Available,
			Misscheduled: ds.Misscheduled,
			Age:          ds.Age,
			CreationTime: metav1.NewTime(ds.CreationTime),
			Status:       ds.Status,
			Usage:        usageFrom(ds.UsageCPUMillis, ds.UsageMemBytes, ds.HasUsageCPU, ds.HasUsageMem),
		})
	}
	for _, j := range w.Jobs {
		out.Jobs = append(out.Jobs, model.JobModel{
			Namespace:          j.Namespace,
			Name:               j.Name,
			CompletionsDesired: j.CompletionsDesired,
			HasCompletions:     j.HasCompletions,
			Succeeded:          j.Succeeded,
			Active:             j.Active,
			Failed:             j.Failed,
			Suspended:          j.Suspended,
			Age:                j.Age,
			CreationTime:       metav1.NewTime(j.CreationTime),
			Status:             j.Status,
			Owner:              j.Owner,
		})
	}
	return out
}

func psiDataFromMetrics(p metrics.PSIMetrics) PSIData {
	return PSIData{
		CPUStallPct:   p.CPUStallPct,
		CPUStalledPct: p.CPUStalledPct,
		MemStallPct:   p.MemStallPct,
		MemStalledPct: p.MemStalledPct,
		IOStallPct:    p.IOStallPct,
		IOStalledPct:  p.IOStalledPct,
	}
}

func (p PSIData) toMetrics() metrics.PSIMetrics {
	return metrics.PSIMetrics{
		CPUStallPct:   p.CPUStallPct,
		CPUStalledPct: p.CPUStalledPct,
		MemStallPct:   p.MemStallPct,
		MemStalledPct: p.MemStalledPct,
		IOStallPct:    p.IOStallPct,
		IOStalledPct:  p.IOStalledPct,
	}
}

func qtyMillis(q *resource.Quantity) int64 {
	if q == nil {
		return 0
	}
	return q.MilliValue()
}

func qtyBytes(q *resource.Quantity) int64 {
	if q == nil {
		return 0
	}
	return q.Value()
}

func summaryDataFromModel(s model.ClusterSummary) SummaryData {
	return SummaryData{
		HealthScore:            s.HealthScore,
		Uptime:                 s.Uptime.Time,
		NodesReady:             s.NodesReady,
		NodesCount:             s.NodesCount,
		Namespaces:             s.Namespaces,
		PodsRunning:            s.PodsRunning,
		PodsAvailable:          s.PodsAvailable,
		Pressures:              s.Pressures,
		NodePressureCount:      s.NodePressureCount,
		ImagesCount:            s.ImagesCount,
		VolumesInUse:           s.VolumesInUse,
		DeploymentsTotal:       s.DeploymentsTotal,
		DeploymentsReady:       s.DeploymentsReady,
		DaemonSetsDesired:      s.DaemonSetsDesired,
		DaemonSetsReady:        s.DaemonSetsReady,
		ReplicaSetsDesired:     s.ReplicaSetsDesired,
		ReplicaSetsReady:       s.ReplicaSetsReady,
		StatefulSetsReady:      s.StatefulSetsReady,
		JobsCount:              s.JobsCount,
		CronJobsCount:          s.CronJobsCount,
		PVCount:                s.PVCount,
		PVsTotalBytes:          qtyBytes(s.PVsTotal),
		PVCCount:               s.PVCCount,
		PVCsTotalBytes:         qtyBytes(s.PVCsTotal),
		AllocatableCPUMillis:   qtyMillis(s.AllocatableNodeCpuTotal),
		AllocatableMemoryBytes: qtyBytes(s.AllocatableNodeMemTotal),
		RequestedCPUMillis:     qtyMillis(s.RequestedPodCpuTotal),
		RequestedMemoryBytes:   qtyBytes(s.RequestedPodMemTotal),
		UsageCPUMillis:         qtyMillis(s.UsageNodeCpuTotal),
		UsageMemoryBytes:       qtyBytes(s.UsageNodeMemTotal),
		HasUsageCPU:            s.HasUsageCPU,
		HasUsageMemory:         s.HasUsageMem,
		ContainerCount:         s.ContainerCount,
		ContainerRestarts:      s.ContainerRestarts,
		FailedPods:             s.FailedPods,
		EvictedPods:            s.EvictedPods,
		OOMKillCount:           s.OOMKillCount,
		CrashLoopPods:          s.CrashLoopPods,
		NetworkRxRateBytesPerS: s.NetworkRxRate,
		NetworkTxRateBytesPerS: s.NetworkTxRate,
		DiskReadRateBytesPerS:  s.DiskReadRate,
		DiskWriteRateBytesPerS: s.DiskWriteRate,
		AvgNodeCPUStallPct:     s.AvgNodeCPUStallPct,
		AvgNodeMemStallPct:     s.AvgNodeMemStallPct,
		AvgNodeIOStallPct:      s.AvgNodeIOStallPct,
		SLIAvailable:           s.SLIAvailable,
		SLIUnhealthyComponents: s.SLIUnhealthyComponents,
		SLIFailingChecks:       append([]string(nil), s.SLIFailingChecks...),
		ProbeFailingPods:       s.ProbeFailingPods,
		KSMAvailable:           s.KSMAvailable,
		KSMHPAsLimited:         s.KSMHPAsLimited,
		KSMPDBsAtRisk:          s.KSMPDBsAtRisk,
		KSMQuotasNearLimit:     s.KSMQuotasNearLimit,
	}
}

// ToClusterSummary converts SummaryData back to model.ClusterSummary for view rendering.
func (s SummaryData) ToClusterSummary(c ClusterInfo) model.ClusterSummary {
	return model.ClusterSummary{
		HealthScore:             s.HealthScore,
		Uptime:                  metav1.NewTime(s.Uptime),
		NodesReady:              s.NodesReady,
		NodesCount:              s.NodesCount,
		Namespaces:              s.Namespaces,
		PodsRunning:             s.PodsRunning,
		PodsAvailable:           s.PodsAvailable,
		Pressures:               s.Pressures,
		NodePressureCount:       s.NodePressureCount,
		ImagesCount:             s.ImagesCount,
		VolumesInUse:            s.VolumesInUse,
		DeploymentsTotal:        s.DeploymentsTotal,
		DeploymentsReady:        s.DeploymentsReady,
		DaemonSetsDesired:       s.DaemonSetsDesired,
		DaemonSetsReady:         s.DaemonSetsReady,
		ReplicaSetsDesired:      s.ReplicaSetsDesired,
		ReplicaSetsReady:        s.ReplicaSetsReady,
		StatefulSetsReady:       s.StatefulSetsReady,
		JobsCount:               s.JobsCount,
		CronJobsCount:           s.CronJobsCount,
		PVCount:                 s.PVCount,
		PVsTotal:                resource.NewQuantity(s.PVsTotalBytes, resource.DecimalSI),
		PVCCount:                s.PVCCount,
		PVCsTotal:               resource.NewQuantity(s.PVCsTotalBytes, resource.DecimalSI),
		AllocatableNodeCpuTotal: resource.NewMilliQuantity(s.AllocatableCPUMillis, resource.DecimalSI),
		AllocatableNodeMemTotal: resource.NewQuantity(s.AllocatableMemoryBytes, resource.DecimalSI),
		RequestedPodCpuTotal:    resource.NewMilliQuantity(s.RequestedCPUMillis, resource.DecimalSI),
		RequestedPodMemTotal:    resource.NewQuantity(s.RequestedMemoryBytes, resource.DecimalSI),
		UsageNodeCpuTotal:       resource.NewMilliQuantity(s.UsageCPUMillis, resource.DecimalSI),
		UsageNodeMemTotal:       resource.NewQuantity(s.UsageMemoryBytes, resource.DecimalSI),
		HasUsageCPU:             s.HasUsageCPU,
		HasUsageMem:             s.HasUsageMemory,
		MetricsSourceType:       c.MetricsSource,
		ContainerCount:          s.ContainerCount,
		ContainerRestarts:       s.ContainerRestarts,
		FailedPods:              s.FailedPods,
		EvictedPods:             s.EvictedPods,
		OOMKillCount:            s.OOMKillCount,
		CrashLoopPods:           s.CrashLoopPods,
		NetworkRxRate:           s.NetworkRxRateBytesPerS,
		NetworkTxRate:           s.NetworkTxRateBytesPerS,
		DiskReadRate:            s.DiskReadRateBytesPerS,
		DiskWriteRate:           s.DiskWriteRateBytesPerS,
		AvgNodeCPUStallPct:      s.AvgNodeCPUStallPct,
		AvgNodeMemStallPct:      s.AvgNodeMemStallPct,
		AvgNodeIOStallPct:       s.AvgNodeIOStallPct,
		PSIBeta:                 c.PSIBeta,
		SLIAvailable:            s.SLIAvailable,
		SLIUnhealthyComponents:  s.SLIUnhealthyComponents,
		SLIFailingChecks:        append([]string(nil), s.SLIFailingChecks...),
		ProbeFailingPods:        s.ProbeFailingPods,
		KSMAvailable:            s.KSMAvailable,
		KSMHPAsLimited:          s.KSMHPAsLimited,
		KSMPDBsAtRisk:           s.KSMPDBsAtRisk,
		KSMQuotasNearLimit:      s.KSMQuotasNearLimit,
	}
}

func nodeDataFromModel(n model.NodeModel) NodeData {
	return NodeData{
		Name:                    n.Name,
		Roles:                   append([]string(nil), n.Roles...),
		Controller:              n.Controller,
		Hostname:                n.Hostname,
		Status:                  n.Status,
		Pressures:               append([]string(nil), n.Pressures...),
		CreationTime:            n.CreationTime.Time,
		TimeSinceStart:          n.TimeSinceStart,
		InternalIP:              n.InternalIP,
		ExternalIP:              n.ExternalIP,
		PodsCount:               n.PodsCount,
		ContainerImagesCount:    n.ContainerImagesCount,
		VolumesInUse:            n.VolumesInUse,
		VolumesAttached:         n.VolumesAttached,
		TaintCount:              n.TaintCount,
		Unschedulable:           n.Unschedulable,
		Restarts:                n.Restarts,
		KubeletVersion:          n.KubeletVersion,
		OS:                      n.OS,
		OSImage:                 n.OSImage,
		OSKernel:                n.OSKernel,
		Architecture:            n.Architecture,
		ContainerRuntimeVersion: n.ContainerRuntimeVersion,
		RequestedCPUMillis:      qtyMillis(n.RequestedPodCpuQty),
		RequestedMemoryBytes:    qtyBytes(n.RequestedPodMemQty),
		AllocatableCPUMillis:    qtyMillis(n.AllocatableCpuQty),
		AllocatableMemoryBytes:  qtyBytes(n.AllocatableMemQty),
		AllocatableStorageBytes: qtyBytes(n.AllocatableStorageQty),
		UsageCPUMillis:          qtyMillis(n.UsageCpuQty),
		UsageMemoryBytes:        qtyBytes(n.UsageMemQty),
		HasUsageCPU:             n.UsageCpuQty != nil,
		HasUsageMemory:          n.UsageMemQty != nil,
		PSI:                     psiDataFromMetrics(n.PSI),
		PSIKernelSupported:      n.PSIKernelSupported,
	}
}

// ToNodeModel converts NodeData back to model.NodeModel for view rendering.
func (n NodeData) ToNodeModel() model.NodeModel {
	var usageCPU, usageMem *resource.Quantity
	if n.HasUsageCPU || n.UsageCPUMillis > 0 {
		usageCPU = resource.NewMilliQuantity(n.UsageCPUMillis, resource.DecimalSI)
	}
	if n.HasUsageMemory || n.UsageMemoryBytes > 0 {
		usageMem = resource.NewQuantity(n.UsageMemoryBytes, resource.DecimalSI)
	}
	return model.NodeModel{
		Name:                    n.Name,
		Roles:                   append([]string(nil), n.Roles...),
		Controller:              n.Controller,
		Hostname:                n.Hostname,
		Status:                  n.Status,
		Pressures:               append([]string(nil), n.Pressures...),
		CreationTime:            metav1.NewTime(n.CreationTime),
		TimeSinceStart:          n.TimeSinceStart,
		InternalIP:              n.InternalIP,
		ExternalIP:              n.ExternalIP,
		PodsCount:               n.PodsCount,
		ContainerImagesCount:    n.ContainerImagesCount,
		VolumesInUse:            n.VolumesInUse,
		VolumesAttached:         n.VolumesAttached,
		TaintCount:              n.TaintCount,
		Unschedulable:           n.Unschedulable,
		Restarts:                n.Restarts,
		KubeletVersion:          n.KubeletVersion,
		OS:                      n.OS,
		OSImage:                 n.OSImage,
		OSKernel:                n.OSKernel,
		Architecture:            n.Architecture,
		ContainerRuntimeVersion: n.ContainerRuntimeVersion,
		RequestedPodCpuQty:      resource.NewMilliQuantity(n.RequestedCPUMillis, resource.DecimalSI),
		RequestedPodMemQty:      resource.NewQuantity(n.RequestedMemoryBytes, resource.DecimalSI),
		AllocatableCpuQty:       resource.NewMilliQuantity(n.AllocatableCPUMillis, resource.DecimalSI),
		AllocatableMemQty:       resource.NewQuantity(n.AllocatableMemoryBytes, resource.DecimalSI),
		AllocatableStorageQty:   resource.NewQuantity(n.AllocatableStorageBytes, resource.DecimalSI),
		UsageCpuQty:             usageCPU,
		UsageMemQty:             usageMem,
		PSI:                     n.PSI.toMetrics(),
		PSIKernelSupported:      n.PSIKernelSupported,
	}
}

func podDataFromModel(p model.PodModel) PodData {
	return PodData{
		Namespace:              p.Namespace,
		Name:                   p.Name,
		Status:                 p.Status,
		Node:                   p.Node,
		IP:                     p.IP,
		TimeSince:              p.TimeSince,
		CreationTime:           p.CreationTime.Time,
		RequestedCPUMillis:     qtyMillis(p.PodRequestedCpuQty),
		RequestedMemoryBytes:   qtyBytes(p.PodRequestedMemQty),
		UsageCPUMillis:         qtyMillis(p.PodUsageCpuQty),
		UsageMemoryBytes:       qtyBytes(p.PodUsageMemQty),
		NodeAllocCPUMillis:     qtyMillis(p.NodeAllocatableCpuQty),
		NodeAllocMemoryBytes:   qtyBytes(p.NodeAllocatableMemQty),
		ReadyContainers:        p.ReadyContainers,
		TotalContainers:        p.TotalContainers,
		Restarts:               p.Restarts,
		Volumes:                p.Volumes,
		VolMounts:              p.VolMounts,
		PSI:                    psiDataFromMetrics(p.PSI),
		NodePSIKernelSupported: p.NodePSIKernelSupported,
		CPUThrottledPct:        p.CPUThrottledPct,
		ResizeStatus:           p.ResizeStatus,
	}
}

// ToPodModel converts PodData back to model.PodModel for view rendering.
func (p PodData) ToPodModel() model.PodModel {
	return model.PodModel{
		Namespace:              p.Namespace,
		Name:                   p.Name,
		Status:                 p.Status,
		Node:                   p.Node,
		IP:                     p.IP,
		TimeSince:              p.TimeSince,
		CreationTime:           metav1.NewTime(p.CreationTime),
		PodRequestedCpuQty:     resource.NewMilliQuantity(p.RequestedCPUMillis, resource.DecimalSI),
		PodRequestedMemQty:     resource.NewQuantity(p.RequestedMemoryBytes, resource.DecimalSI),
		PodUsageCpuQty:         resource.NewMilliQuantity(p.UsageCPUMillis, resource.DecimalSI),
		PodUsageMemQty:         resource.NewQuantity(p.UsageMemoryBytes, resource.DecimalSI),
		NodeAllocatableCpuQty:  resource.NewMilliQuantity(p.NodeAllocCPUMillis, resource.DecimalSI),
		NodeAllocatableMemQty:  resource.NewQuantity(p.NodeAllocMemoryBytes, resource.DecimalSI),
		ReadyContainers:        p.ReadyContainers,
		TotalContainers:        p.TotalContainers,
		Restarts:               p.Restarts,
		Volumes:                p.Volumes,
		VolMounts:              p.VolMounts,
		PSI:                    p.PSI.toMetrics(),
		NodePSIKernelSupported: p.NodePSIKernelSupported,
		CPUThrottledPct:        p.CPUThrottledPct,
		ResizeStatus:           p.ResizeStatus,
	}
}

// NodeDetailProjection bundles a Snapshot with the target node name for
// snapshot-driven Node Detail rendering.
type NodeDetailProjection struct {
	Snapshot *Snapshot
	NodeName string
}

// ToNodeDetailData projects the named node and its scheduled pods into *model.NodeDetailData.
func (p *NodeDetailProjection) ToNodeDetailData() *model.NodeDetailData {
	if p == nil || p.Snapshot == nil || p.NodeName == "" {
		return nil
	}
	nd, ok := p.Snapshot.NodeByName(p.NodeName)
	if !ok {
		return nil
	}
	nm := nd.ToNodeModel()
	snapPods := p.Snapshot.PodsOnNode(p.NodeName)
	pods := make([]*model.PodModel, 0, len(snapPods))
	for _, sp := range snapPods {
		pm := sp.ToPodModel()
		pods = append(pods, &pm)
	}
	return &model.NodeDetailData{
		NodeModel:         &nm,
		PodsOnNode:        pods,
		MetricsSourceType: p.Snapshot.Cluster.MetricsSource,
	}
}

// PodDetailProjection bundles a Snapshot with the target pod key for
// snapshot-driven Pod Detail rendering.
type PodDetailProjection struct {
	Snapshot  *Snapshot
	Namespace string
	PodName   string
}

// ToPodDetailData projects the named pod and its container metrics into *model.PodDetailData.
func (p *PodDetailProjection) ToPodDetailData() *model.PodDetailData {
	if p == nil || p.Snapshot == nil || p.Namespace == "" || p.PodName == "" {
		return nil
	}
	pd, ok := p.Snapshot.PodByKey(p.Namespace, p.PodName)
	if !ok {
		return nil
	}
	pm := pd.ToPodModel()
	cmMap := make(map[string]model.ContainerUsage, len(pd.Containers))
	for _, c := range pd.Containers {
		usage := model.ContainerUsage{
			CPUThrottledPct: c.CPUThrottledPct,
		}
		if c.UsageCPUMillis > 0 {
			usage.CPUUsage = fmt.Sprintf("%dm", c.UsageCPUMillis)
		}
		if c.UsageMemBytes > 0 {
			usage.MemoryUsage = fmt.Sprintf("%dMi", c.UsageMemBytes/(1024*1024))
		}
		cmMap[c.Name] = usage
	}
	return &model.PodDetailData{
		PodModel:          &pm,
		ContainerMetrics:  cmMap,
		MetricsSourceType: p.Snapshot.Cluster.MetricsSource,
	}
}

// ContainerUsageFromSnapshot extracts formatted CPU and memory usage for a
// container from a Snapshot.
func ContainerUsageFromSnapshot(snap *Snapshot, namespace, podName, containerName string) (cpuUsage, memUsage string, ok bool) {
	if snap == nil {
		return "", "", false
	}
	pd, found := snap.PodByKey(namespace, podName)
	if !found {
		return "", "", false
	}
	for _, c := range pd.Containers {
		if c.Name == containerName {
			return fmt.Sprintf("%dm", c.UsageCPUMillis), fmt.Sprintf("%dMi", c.UsageMemBytes/(1024*1024)), true
		}
	}
	return "", "", false
}

func storageDataFromModel(st *model.StorageSnapshot) *StorageData {
	if st == nil {
		return nil
	}
	out := &StorageData{
		Summary: StorageSummaryData{
			PVCsBound:              st.Summary.PVCsBound,
			PVCsPending:            st.Summary.PVCsPending,
			PVCsTotal:              st.Summary.PVCsTotal,
			PVsBound:               st.Summary.PVsBound,
			PVsAvailable:           st.Summary.PVsAvailable,
			PVsReleased:            st.Summary.PVsReleased,
			PVsTotal:               st.Summary.PVsTotal,
			StorageClasses:         st.Summary.StorageClasses,
			HighFillCount:          st.Summary.HighFillCount,
			TotalCapacityBytes:     st.Summary.TotalCapacity,
			TotalUsedBytes:         st.Summary.TotalUsed,
			DiskReadRateBytesPerS:  st.Summary.DiskReadRateBps,
			DiskWriteRateBytesPerS: st.Summary.DiskWriteRateBps,
		},
		PVCs:           make([]PVCData, 0, len(st.PVCs)),
		PVs:            make([]PVData, 0, len(st.PVs)),
		StorageClasses: make([]StorageClassData, 0, len(st.StorageClasses)),
	}
	for _, pvc := range st.PVCs {
		out.PVCs = append(out.PVCs, PVCData{
			Namespace:      pvc.Namespace,
			Name:           pvc.Name,
			Status:         pvc.Status,
			StorageClass:   pvc.StorageClass,
			VolumeName:     pvc.VolumeName,
			AccessModes:    slices.Clone(pvc.AccessModes),
			CapacityBytes:  pvc.CapacityBytes,
			UsedBytes:      pvc.UsedBytes,
			AvailableBytes: pvc.AvailableBytes,
			FillPct:        pvc.FillPct,
			InodesTotal:    pvc.InodesTotal,
			InodesUsed:     pvc.InodesUsed,
			InodesUsedPct:  pvc.InodesUsedPct,
			HasUsage:       pvc.HasUsage,
			HasInodes:      pvc.HasInodes,
			CreationTime:   pvc.CreationTime.Time,
			Age:            pvc.Age,
		})
	}
	for _, pv := range st.PVs {
		out.PVs = append(out.PVs, PVData{
			Name:          pv.Name,
			Status:        pv.Status,
			CapacityBytes: pv.CapacityBytes,
			AccessModes:   slices.Clone(pv.AccessModes),
			ReclaimPolicy: pv.ReclaimPolicy,
			StorageClass:  pv.StorageClass,
			ClaimRef:      pv.ClaimRef,
			CreationTime:  pv.CreationTime.Time,
			Age:           pv.Age,
		})
	}
	for _, sc := range st.StorageClasses {
		out.StorageClasses = append(out.StorageClasses, StorageClassData{
			Name:          sc.Name,
			Provisioner:   sc.Provisioner,
			ReclaimPolicy: sc.ReclaimPol,
			BindingMode:   sc.BindingMode,
			IsDefault:     sc.IsDefault,
		})
	}
	return out
}

func networkDataFromModel(netSnap *model.NetworkSnapshot) *NetworkData {
	if netSnap == nil {
		return nil
	}
	out := &NetworkData{
		Summary: NetworkSummaryData{
			ServicesTotal:          netSnap.Summary.ServicesTotal,
			ZeroEndpointServices:   netSnap.Summary.ZeroEndpointServices,
			IngressesTotal:         netSnap.Summary.IngressesTotal,
			ClusterRxRateBytesPerS: netSnap.Summary.ClusterRxRateBps,
			ClusterTxRateBytesPerS: netSnap.Summary.ClusterTxRateBps,
			DropsPerSec:            netSnap.Summary.DropsPerSec,
			ErrorsPerSec:           netSnap.Summary.ErrorsPerSec,
			HasMetrics:             netSnap.Summary.HasMetrics,
		},
		Services:    make([]ServiceData, 0, len(netSnap.Services)),
		Ingresses:   make([]IngressData, 0, len(netSnap.Ingresses)),
		NodeNetwork: make([]NodeNetworkData, 0, len(netSnap.NodeNetwork)),
	}
	for _, svc := range netSnap.Services {
		out.Services = append(out.Services, ServiceData{
			Namespace:        svc.Namespace,
			Name:             svc.Name,
			Type:             svc.Type,
			ClusterIP:        svc.ClusterIP,
			ExternalIP:       svc.ExternalIP,
			Ports:            svc.Ports,
			ReadyEndpoints:   svc.ReadyEndpoints,
			TotalEndpoints:   svc.TotalEndpoints,
			HasEndpoints:     svc.HasEndpoints,
			Selectorless:     svc.Selectorless,
			HasEndpointAlert: svc.HasEndpointAlert(),
			CreationTime:     svc.CreationTime.Time,
			Age:              svc.Age,
		})
	}
	for _, ing := range netSnap.Ingresses {
		out.Ingresses = append(out.Ingresses, IngressData{
			Namespace:    ing.Namespace,
			Name:         ing.Name,
			ClassName:    ing.ClassName,
			Hosts:        slices.Clone(ing.Hosts),
			Address:      ing.Address,
			Ports:        ing.Ports,
			CreationTime: ing.CreationTime.Time,
			Age:          ing.Age,
		})
	}
	for _, n := range netSnap.NodeNetwork {
		out.NodeNetwork = append(out.NodeNetwork, NodeNetworkData{
			NodeName:        n.NodeName,
			RxRateBytesPerS: n.RxRateBps,
			TxRateBytesPerS: n.TxRateBps,
			RxDroppedPerSec: n.RxDroppedSec,
			TxDroppedPerSec: n.TxDroppedSec,
			RxErrorsPerSec:  n.RxErrorsSec,
			TxErrorsPerSec:  n.TxErrorsSec,
		})
	}
	return out
}
