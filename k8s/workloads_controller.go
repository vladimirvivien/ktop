package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/views/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func (c *Controller) setupWorkloadsHandler(ctx context.Context, handlerFunc RefreshWorkloadsFunc) {
	if handlerFunc == nil {
		return
	}
	go func() {
		_ = c.refreshWorkloads(ctx, handlerFunc)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-c.workloadsNotifyCh:
				_ = c.refreshWorkloads(ctx, handlerFunc)
			case <-ticker.C:
				_ = c.refreshWorkloads(ctx, handlerFunc)
			}
		}
	}()
}

func (c *Controller) refreshWorkloads(ctx context.Context, handlerFunc RefreshWorkloadsFunc) error {
	if c.healthTracker != nil && c.healthTracker.IsDisconnected() {
		return nil
	}
	snap, err := c.GetWorkloadsSnapshot(ctx)
	if err != nil {
		return err
	}
	return handlerFunc(ctx, snap)
}

// GetWorkloadsSnapshot builds a snapshot of Deployments, StatefulSets, DaemonSets,
// and Jobs along with per-workload efficiency scores computed from owned pods and metrics.
func (c *Controller) GetWorkloadsSnapshot(ctx context.Context) (model.WorkloadsSnapshot, error) {
	if ctx.Err() != nil {
		return model.WorkloadsSnapshot{}, ctx.Err()
	}

	var snap model.WorkloadsSnapshot
	podIndex, hasMetrics := c.buildWorkloadPodIndex(ctx)
	snap.HasMetrics = hasMetrics

	rsByDeployment := c.indexReplicaSetsByDeployment()

	if c.deploymentInformer != nil {
		if deps, err := c.deploymentInformer.Lister().List(labels.Everything()); err == nil {
			snap.Deployments = make([]model.DeploymentModel, 0, len(deps))
			for _, d := range deps {
				dm := model.NewDeploymentModel(d)
				owned := podIndex.matchPods(d.Namespace, "Deployment", d.UID, d.Name, d.Spec.Selector, rsByDeployment[d.UID])
				dm.Efficiency = model.ComputePodsEfficiency(owned, hasMetrics)
				snap.Deployments = append(snap.Deployments, dm)
			}
		}
	}

	if c.statefulSetInformer != nil {
		if stsList, err := c.statefulSetInformer.Lister().List(labels.Everything()); err == nil {
			snap.StatefulSets = make([]model.StatefulSetModel, 0, len(stsList))
			for _, s := range stsList {
				sm := model.NewStatefulSetModel(s)
				owned := podIndex.matchPods(s.Namespace, "StatefulSet", s.UID, s.Name, s.Spec.Selector, nil)
				sm.Efficiency = model.ComputePodsEfficiency(owned, hasMetrics)
				snap.StatefulSets = append(snap.StatefulSets, sm)
			}
		}
	}

	if c.daemonSetInformer != nil {
		if dsList, err := c.daemonSetInformer.Lister().List(labels.Everything()); err == nil {
			snap.DaemonSets = make([]model.DaemonSetModel, 0, len(dsList))
			for _, ds := range dsList {
				dsm := model.NewDaemonSetModel(ds)
				owned := podIndex.matchPods(ds.Namespace, "DaemonSet", ds.UID, ds.Name, ds.Spec.Selector, nil)
				dsm.Efficiency = model.ComputePodsEfficiency(owned, hasMetrics)
				snap.DaemonSets = append(snap.DaemonSets, dsm)
			}
		}
	}

	if c.jobInformer != nil {
		if jobs, err := c.jobInformer.Lister().List(labels.Everything()); err == nil {
			snap.Jobs = make([]model.JobModel, 0, len(jobs))
			for _, job := range jobs {
				jm := model.NewJobModel(job)
				owned := podIndex.matchPods(job.Namespace, "Job", job.UID, job.Name, job.Spec.Selector, nil)
				jm.Efficiency = model.ComputePodsEfficiency(owned, hasMetrics)
				snap.Jobs = append(snap.Jobs, jm)
			}
		}
	}

	return snap, nil
}

// BuildWorkloadDetailData builds live detail state, owned pods, events, and
// aggregate replica sparkline history for a single workload.
func (c *Controller) BuildWorkloadDetailData(ctx context.Context, kind model.WorkloadKind, namespace, name string) (*model.WorkloadDetailData, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	podIndex, hasMetrics := c.buildWorkloadPodIndex(ctx)
	rsByDeployment := c.indexReplicaSetsByDeployment()

	var detail *model.WorkloadDetailData
	var owned []*model.PodModel

	switch kind {
	case model.WorkloadKindDeployment:
		d, err := c.GetDeployment(ctx, namespace, name)
		if err != nil {
			return nil, err
		}
		dm := model.NewDeploymentModel(d)
		owned = podIndex.matchPods(d.Namespace, "Deployment", d.UID, d.Name, d.Spec.Selector, rsByDeployment[d.UID])
		eff := model.ComputePodsEfficiency(owned, hasMetrics)
		detail = &model.WorkloadDetailData{
			Kind:               kind,
			Namespace:          dm.Namespace,
			Name:               dm.Name,
			Status:             dm.Status,
			ReadyText:          fmt.Sprintf("%d/%d ready (updated %d, available %d)", dm.Ready, dm.Desired, dm.Updated, dm.Available),
			Strategy:           dm.Strategy,
			Age:                dm.Age,
			Generation:         dm.Generation,
			ObservedGeneration: dm.ObservedGeneration,
			Efficiency:         eff,
			Selector:           dm.Selector,
			Labels:             dm.Labels,
			Annotations:        dm.Annotations,
			Conditions:         dm.Conditions,
		}

	case model.WorkloadKindStatefulSet:
		s, err := c.GetStatefulSet(ctx, namespace, name)
		if err != nil {
			return nil, err
		}
		sm := model.NewStatefulSetModel(s)
		owned = podIndex.matchPods(s.Namespace, "StatefulSet", s.UID, s.Name, s.Spec.Selector, nil)
		eff := model.ComputePodsEfficiency(owned, hasMetrics)
		detail = &model.WorkloadDetailData{
			Kind:               kind,
			Namespace:          sm.Namespace,
			Name:               sm.Name,
			Status:             sm.Status,
			ReadyText:          fmt.Sprintf("%d/%d ready (current %d, updated %d)", sm.Ready, sm.Desired, sm.Current, sm.Updated),
			Strategy:           sm.UpdateStrategy,
			ServiceName:        sm.ServiceName,
			Age:                sm.Age,
			Generation:         sm.Generation,
			ObservedGeneration: sm.ObservedGeneration,
			Efficiency:         eff,
			Selector:           sm.Selector,
			Labels:             sm.Labels,
			Annotations:        sm.Annotations,
			Conditions:         sm.Conditions,
		}

	case model.WorkloadKindDaemonSet:
		ds, err := c.GetDaemonSet(ctx, namespace, name)
		if err != nil {
			return nil, err
		}
		dsm := model.NewDaemonSetModel(ds)
		owned = podIndex.matchPods(ds.Namespace, "DaemonSet", ds.UID, ds.Name, ds.Spec.Selector, nil)
		eff := model.ComputePodsEfficiency(owned, hasMetrics)
		detail = &model.WorkloadDetailData{
			Kind:               kind,
			Namespace:          dsm.Namespace,
			Name:               dsm.Name,
			Status:             dsm.Status,
			ReadyText:          fmt.Sprintf("%d/%d ready (current %d, up-to-date %d)", dsm.Ready, dsm.Desired, dsm.Current, dsm.UpToDate),
			Strategy:           dsm.UpdateStrategy,
			Age:                dsm.Age,
			Generation:         dsm.Generation,
			ObservedGeneration: dsm.ObservedGeneration,
			Efficiency:         eff,
			Selector:           dsm.Selector,
			Labels:             dsm.Labels,
			Annotations:        dsm.Annotations,
			Conditions:         dsm.Conditions,
		}

	case model.WorkloadKindJob:
		job, err := c.GetJob(ctx, namespace, name)
		if err != nil {
			return nil, err
		}
		jm := model.NewJobModel(job)
		owned = podIndex.matchPods(job.Namespace, "Job", job.UID, job.Name, job.Spec.Selector, nil)
		eff := model.ComputePodsEfficiency(owned, hasMetrics)
		detail = &model.WorkloadDetailData{
			Kind:        kind,
			Namespace:   jm.Namespace,
			Name:        jm.Name,
			Status:      jm.Status,
			ReadyText:   fmt.Sprintf("%s completions (active %d, failed %d)", jm.FormatCompletions(), jm.Active, jm.Failed),
			Owner:       jm.Owner,
			Age:         jm.Age,
			Efficiency:  eff,
			Selector:    jm.Selector,
			Labels:      jm.Labels,
			Annotations: jm.Annotations,
			Conditions:  jm.Conditions,
		}

	default:
		return nil, fmt.Errorf("unsupported workload kind %q", kind)
	}

	detail.OwnedPods = owned
	detail.TotalRequestedCPU, detail.TotalRequestedMem, detail.TotalUsageCPU, detail.TotalUsageMem = sumOwnedPodResources(owned, hasMetrics)

	if events, err := c.GetEventsForObject(ctx, namespace, string(kind), name); err == nil {
		detail.Events = events
	}

	if c.metricsSource != nil {
		detail.MetricsSourceType = c.metricsSource.GetSourceInfo().Type
		if c.metricsSource.SupportsHistory() && len(owned) > 0 {
			detail.MetricsHistory = c.aggregatePodHistory(ctx, owned, detail.TotalRequestedCPU, detail.TotalRequestedMem)
		}
	}

	return detail, nil
}

func sumOwnedPodResources(pods []*model.PodModel, hasMetrics bool) (reqCPU, reqMem, usageCPU, usageMem *resource.Quantity) {
	reqCPU = resource.NewQuantity(0, resource.DecimalSI)
	reqMem = resource.NewQuantity(0, resource.DecimalSI)
	if hasMetrics {
		usageCPU = resource.NewQuantity(0, resource.DecimalSI)
		usageMem = resource.NewQuantity(0, resource.DecimalSI)
	}
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		if pod.PodRequestedCpuQty != nil {
			reqCPU.Add(*pod.PodRequestedCpuQty)
		}
		if pod.PodRequestedMemQty != nil {
			reqMem.Add(*pod.PodRequestedMemQty)
		}
		if hasMetrics && pod.PodUsageCpuQty != nil {
			usageCPU.Add(*pod.PodUsageCpuQty)
		}
		if hasMetrics && pod.PodUsageMemQty != nil {
			usageMem.Add(*pod.PodUsageMemQty)
		}
	}
	return
}

func (c *Controller) aggregatePodHistory(ctx context.Context, pods []*model.PodModel, totalReqCPU, totalReqMem *resource.Quantity) []model.MetricSample {
	const (
		historyDuration = 5 * time.Minute
		maxPoints       = 15
	)
	cpuSums := make([]float64, maxPoints)
	memSums := make([]float64, maxPoints)
	timestamps := make([]int64, maxPoints)
	validLen := 0

	for _, pod := range pods {
		if pod == nil {
			continue
		}
		cpuHist, errCPU := c.metricsSource.GetPodHistory(ctx, pod.Namespace, pod.Name, metrics.HistoryQuery{
			Resource:  metrics.ResourceCPU,
			Duration:  historyDuration,
			MaxPoints: maxPoints,
		})
		memHist, errMem := c.metricsSource.GetPodHistory(ctx, pod.Namespace, pod.Name, metrics.HistoryQuery{
			Resource:  metrics.ResourceMemory,
			Duration:  historyDuration,
			MaxPoints: maxPoints,
		})
		if errCPU != nil || cpuHist == nil || errMem != nil || memHist == nil {
			continue
		}

		n := min(len(cpuHist.DataPoints), len(memHist.DataPoints), maxPoints)
		if n == 0 {
			continue
		}
		if n > validLen {
			validLen = n
		}
		for i := 0; i < n; i++ {
			// Align tail-most samples so shorter pod histories contribute to recent buckets.
			cpuIdx := len(cpuHist.DataPoints) - n + i
			memIdx := len(memHist.DataPoints) - n + i
			bucket := maxPoints - n + i
			cpuSums[bucket] += cpuHist.DataPoints[cpuIdx].Value
			memSums[bucket] += memHist.DataPoints[memIdx].Value
			if timestamps[bucket] == 0 {
				timestamps[bucket] = cpuHist.DataPoints[cpuIdx].Timestamp.UnixMilli()
			}
		}
	}

	if validLen == 0 {
		return nil
	}

	denomCPU := float64(1000)
	if totalReqCPU != nil && totalReqCPU.MilliValue() > 0 {
		denomCPU = float64(totalReqCPU.MilliValue())
	} else {
		// Without requests, normalize against max observed sample so trends stay legible.
		peak := 0.0
		for _, v := range cpuSums {
			if v > peak {
				peak = v
			}
		}
		if peak > 0 {
			denomCPU = peak
		}
	}

	denomMem := float64(1 << 30)
	if totalReqMem != nil && totalReqMem.Value() > 0 {
		denomMem = float64(totalReqMem.Value())
	} else {
		peak := 0.0
		for _, v := range memSums {
			if v > peak {
				peak = v
			}
		}
		if peak > 0 {
			denomMem = peak
		}
	}

	start := maxPoints - validLen
	samples := make([]model.MetricSample, 0, validLen)
	for i := start; i < maxPoints; i++ {
		samples = append(samples, model.MetricSample{
			Timestamp: timestamps[i],
			CPURatio:  min(cpuSums[i]/denomCPU, 1.0),
			MemRatio:  min(memSums[i]/denomMem, 1.0),
		})
	}
	return samples
}

type workloadPodEntry struct {
	pod   *corev1.Pod
	model *model.PodModel
}

type workloadPodIndex struct {
	byNamespace map[string][]workloadPodEntry
}

func (c *Controller) buildWorkloadPodIndex(ctx context.Context) (*workloadPodIndex, bool) {
	idx := &workloadPodIndex{
		byNamespace: make(map[string][]workloadPodEntry),
	}
	if c.podInformer == nil {
		return idx, false
	}

	pods, err := c.podInformer.Lister().List(labels.Everything())
	if err != nil {
		return idx, false
	}

	var metricsMap map[string]*metrics.PodMetrics
	hasMetrics := false
	if c.metricsSource != nil && c.metricsSource.IsHealthy() {
		if allMetrics, err := c.metricsSource.GetAllPodMetrics(ctx); err == nil {
			hasMetrics = true
			metricsMap = make(map[string]*metrics.PodMetrics, len(allMetrics))
			for _, pm := range allMetrics {
				if pm != nil {
					metricsMap[pm.Namespace+"/"+pm.PodName] = pm
				}
			}
		}
	}

	emptyPodMetrics := new(metricsV1beta1.PodMetrics)
	emptyNodeMetrics := new(metricsV1beta1.NodeMetrics)
	nodeAllocCache := make(map[string]corev1.ResourceList)
	for _, pod := range pods {
		if pod == nil {
			continue
		}
		pm := model.NewPodModel(pod, emptyPodMetrics, emptyNodeMetrics)
		if alloc, ok := nodeAllocCache[pod.Spec.NodeName]; ok {
			pm.NodeAllocatableCpuQty = alloc.Cpu()
			pm.NodeAllocatableMemQty = alloc.Memory()
		} else if c.nodeInformer != nil && pod.Spec.NodeName != "" {
			if node, err := c.nodeInformer.Lister().Get(pod.Spec.NodeName); err == nil && node != nil {
				nodeAllocCache[pod.Spec.NodeName] = node.Status.Allocatable
				pm.NodeAllocatableCpuQty = node.Status.Allocatable.Cpu()
				pm.NodeAllocatableMemQty = node.Status.Allocatable.Memory()
			}
		}
		if hasMetrics {
			if m, found := metricsMap[pod.Namespace+"/"+pod.Name]; found {
				cpuTotal, memTotal := sumMetricsPodTotals(m)
				pm.PodUsageCpuQty = cpuTotal
				pm.PodUsageMemQty = memTotal
				pm.PSI = m.PSI
				pm.CPUThrottledPct = m.CPUThrottledPct
			}
		}
		idx.byNamespace[pod.Namespace] = append(idx.byNamespace[pod.Namespace], workloadPodEntry{
			pod:   pod,
			model: pm,
		})
	}
	return idx, hasMetrics
}

func sumMetricsPodTotals(podMetrics *metrics.PodMetrics) (totalCPU, totalMem *resource.Quantity) {
	totalCPU = resource.NewQuantity(0, resource.DecimalSI)
	totalMem = resource.NewQuantity(0, resource.DecimalSI)
	if podMetrics == nil {
		return
	}
	for _, cm := range podMetrics.Containers {
		if cm.CPUUsage != nil {
			totalCPU.Add(*cm.CPUUsage)
		}
		if cm.MemoryUsage != nil {
			totalMem.Add(*cm.MemoryUsage)
		}
	}
	return
}

func (c *Controller) indexReplicaSetsByDeployment() map[types.UID]map[types.UID]struct{} {
	result := make(map[types.UID]map[types.UID]struct{})
	if c.replicaSetInformer == nil {
		return result
	}
	rsList, err := c.replicaSetInformer.Lister().List(labels.Everything())
	if err != nil {
		return result
	}
	for _, rs := range rsList {
		if rs == nil {
			continue
		}
		for _, ref := range rs.OwnerReferences {
			if ref.Kind == "Deployment" {
				set := result[ref.UID]
				if set == nil {
					set = make(map[types.UID]struct{})
					result[ref.UID] = set
				}
				set[rs.UID] = struct{}{}
			}
		}
	}
	return result
}

func (idx *workloadPodIndex) matchPods(
	namespace string,
	ownerKind string,
	ownerUID types.UID,
	ownerName string,
	selector *metav1.LabelSelector,
	ownedRSUIDs map[types.UID]struct{},
) []*model.PodModel {
	entries := idx.byNamespace[namespace]
	if len(entries) == 0 {
		return nil
	}

	var compiledSelector labels.Selector
	if selector != nil {
		if s, err := metav1.LabelSelectorAsSelector(selector); err == nil {
			compiledSelector = s
		}
	}

	var matched []*model.PodModel
	for _, entry := range entries {
		if podOwnedByWorkload(entry.pod, ownerKind, ownerUID, ownerName, ownedRSUIDs) {
			matched = append(matched, entry.model)
			continue
		}
		// Label-selector fallback handles tests or objects missing explicit OwnerReferences.
		if compiledSelector != nil && !compiledSelector.Empty() && compiledSelector.Matches(labels.Set(entry.pod.Labels)) {
			matched = append(matched, entry.model)
		}
	}
	return matched
}

func podOwnedByWorkload(pod *corev1.Pod, ownerKind string, ownerUID types.UID, ownerName string, ownedRSUIDs map[types.UID]struct{}) bool {
	if pod == nil {
		return false
	}
	for _, ref := range pod.OwnerReferences {
		if ownerKind == "Deployment" && ref.Kind == "ReplicaSet" {
			if _, ok := ownedRSUIDs[ref.UID]; ok {
				return true
			}
			// Fallback when synthetic fixtures omit UIDs or ReplicaSet uses "<deploy>-<hash>".
			if ownerUID == "" && strings.HasPrefix(ref.Name, ownerName+"-") {
				return true
			}
		}
		if ref.Kind == ownerKind {
			if ownerUID != "" && ref.UID != "" {
				if ref.UID == ownerUID {
					return true
				}
			} else if ref.Name == ownerName {
				return true
			}
		}
	}
	return false
}
