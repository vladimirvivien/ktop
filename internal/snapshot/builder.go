package snapshot

import (
	"context"
	"fmt"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildInput supplies in-memory cached state and metrics to SnapshotBuilder.
type BuildInput struct {
	Cluster    ClusterInfo
	Summary    model.ClusterSummary
	Nodes      []model.NodeModel
	Pods       []model.PodModel
	PodMetrics []*metrics.PodMetrics
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
		pods = append(pods, pd)
	}

	cluster := in.Cluster
	if cluster.MetricsSource == "" {
		cluster.MetricsSource = in.Summary.MetricsSourceType
	}
	cluster.PSIBeta = in.Summary.PSIBeta

	return &Snapshot{
		Timestamp: time.Now(),
		Cluster:   cluster,
		Summary:   summaryDataFromModel(in.Summary),
		Nodes:     nodes,
		Pods:      pods,
	}, nil
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
		NetworkRxRateBytesPerS: s.NetworkRxRate,
		NetworkTxRateBytesPerS: s.NetworkTxRate,
		DiskReadRateBytesPerS:  s.DiskReadRate,
		DiskWriteRateBytesPerS: s.DiskWriteRate,
		AvgNodeCPUStallPct:     s.AvgNodeCPUStallPct,
		AvgNodeMemStallPct:     s.AvgNodeMemStallPct,
		AvgNodeIOStallPct:      s.AvgNodeIOStallPct,
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
		NetworkRxRate:           s.NetworkRxRateBytesPerS,
		NetworkTxRate:           s.NetworkTxRateBytesPerS,
		DiskReadRate:            s.DiskReadRateBytesPerS,
		DiskWriteRate:           s.DiskWriteRateBytesPerS,
		AvgNodeCPUStallPct:      s.AvgNodeCPUStallPct,
		AvgNodeMemStallPct:      s.AvgNodeMemStallPct,
		AvgNodeIOStallPct:       s.AvgNodeIOStallPct,
		PSIBeta:                 c.PSIBeta,
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
