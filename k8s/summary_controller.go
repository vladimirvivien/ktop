package k8s

import (
	"context"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/views/model"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Controller) setupSummaryHandler(ctx context.Context, handlerFunc RefreshSummaryFunc) {
	if handlerFunc == nil {
		return
	}
	go func() {
		c.refreshSummary(ctx, handlerFunc)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.refreshSummary(ctx, handlerFunc); err != nil {
					continue
				}
			}
		}
	}()
}

func (c *Controller) refreshSummary(ctx context.Context, handlerFunc RefreshSummaryFunc) error {
	// Skip refresh if API is disconnected - don't update UI with stale cached data
	if c.healthTracker != nil && c.healthTracker.IsDisconnected() {
		return nil
	}

	summary, err := c.BuildClusterSummary(ctx)
	if err != nil {
		return err
	}

	if handlerFunc != nil {
		handlerFunc(ctx, summary)
	}
	return nil
}

// BuildClusterSummary aggregates cluster-wide summary counts, resource totals,
// metrics, and composite health score from synced informers and the configured
// metrics source.
func (c *Controller) BuildClusterSummary(ctx context.Context) (model.ClusterSummary, error) {
	var summary model.ClusterSummary

	// Set metrics source type for dynamic UI layout
	if c.metricsSource != nil {
		info := c.metricsSource.GetSourceInfo()
		summary.MetricsSourceType = info.Type
	}

	// extract namespace summary
	namespaces, err := c.GetNamespaceList(ctx)
	if err != nil {
		c.reportError(err)
		return summary, err
	}
	summary.Namespaces = len(namespaces)

	nodes, err := c.GetNodeList(ctx)
	if err != nil {
		c.reportError(err)
		return summary, err
	}
	summary.Uptime = metav1.NewTime(time.Now())
	summary.NodesCount = len(nodes)
	summary.AllocatableNodeMemTotal = resource.NewQuantity(0, resource.DecimalSI)
	summary.AllocatableNodeCpuTotal = resource.NewQuantity(0, resource.DecimalSI)
	summary.UsageNodeMemTotal = resource.NewQuantity(0, resource.DecimalSI)
	summary.UsageNodeCpuTotal = resource.NewQuantity(0, resource.DecimalSI)
	nodeMetricsByNode := make(map[string]*metrics.NodeMetrics, len(nodes))
	for _, node := range nodes {
		if model.GetNodeReadyStatus(node) == string(coreV1.NodeReady) {
			summary.NodesReady++
		}
		if node.CreationTimestamp.Before(&summary.Uptime) {
			summary.Uptime = node.CreationTimestamp
		}

		pressures := model.GetNodePressures(node)
		summary.Pressures += len(pressures)
		if len(pressures) > 0 {
			summary.NodePressureCount++
		}
		summary.ImagesCount += len(node.Status.Images)
		summary.VolumesInUse += len(node.Status.VolumesInUse)

		summary.AllocatableNodeMemTotal.Add(*node.Status.Allocatable.Memory())
		summary.AllocatableNodeCpuTotal.Add(*node.Status.Allocatable.Cpu())

		if c.metricsSource == nil {
			continue
		}
		nodeMetrics, err := c.metricsSource.GetNodeMetrics(ctx, node.Name)
		if err != nil || nodeMetrics == nil {
			// Metrics not available - skip adding to usage totals (graceful degradation)
			continue
		}
		nodeMetricsByNode[node.Name] = nodeMetrics
		if nodeMetrics.MemoryUsage != nil {
			summary.UsageNodeMemTotal.Add(*nodeMetrics.MemoryUsage)
			summary.HasUsageMem = true
		}
		if nodeMetrics.CPUUsage != nil {
			summary.UsageNodeCpuTotal.Add(*nodeMetrics.CPUUsage)
			summary.HasUsageCPU = true
		}
	}

	// extract pods summary
	pods, err := c.GetPodList(ctx)
	if err != nil {
		c.reportError(err)
		return summary, err
	}
	summary.PodsAvailable = len(pods)
	summary.RequestedPodMemTotal = resource.NewQuantity(0, resource.DecimalSI)
	summary.RequestedPodCpuTotal = resource.NewQuantity(0, resource.DecimalSI)
	for _, pod := range pods {
		switch pod.Status.Phase {
		case coreV1.PodRunning:
			summary.PodsRunning++
		case coreV1.PodFailed:
			summary.FailedPods++
			if pod.Status.Reason == "Evicted" {
				summary.EvictedPods++
			}
		}

		var hasCrashLoop bool
		for _, cs := range pod.Status.InitContainerStatuses {
			summary.ContainerRestarts += int(cs.RestartCount)
			if isContainerOOMKilled(cs) {
				summary.OOMKillCount++
			}
			if isContainerCrashLoop(cs) {
				hasCrashLoop = true
			}
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if pod.Status.Phase == coreV1.PodRunning && cs.State.Running != nil {
				summary.ContainerCount++
			}
			summary.ContainerRestarts += int(cs.RestartCount)
			if isContainerOOMKilled(cs) {
				summary.OOMKillCount++
			}
			if isContainerCrashLoop(cs) {
				hasCrashLoop = true
			}
		}
		if hasCrashLoop {
			summary.CrashLoopPods++
		}

		containerSummary := model.GetPodContainerSummary(pod)
		summary.RequestedPodMemTotal.Add(*containerSummary.RequestedMemQty)
		summary.RequestedPodCpuTotal.Add(*containerSummary.RequestedCpuQty)
	}

	// deployments count
	deps, err := c.GetDeploymentList(ctx)
	if err != nil {
		c.reportError(err)
		return summary, err
	}
	for _, dep := range deps {
		summary.DeploymentsTotal += int(dep.Status.Replicas)
		summary.DeploymentsReady += int(dep.Status.ReadyReplicas)
	}

	// deamonset count
	daemonsets, err := c.GetDaemonSetList(ctx)
	if err != nil {
		c.reportError(err)
		return summary, err
	}
	for _, set := range daemonsets {
		summary.DaemonSetsDesired += int(set.Status.DesiredNumberScheduled)
		summary.DaemonSetsReady += int(set.Status.NumberReady)
	}

	// replicasets count
	replicasets, err := c.GetReplicaSetList(ctx)
	if err != nil {
		c.reportError(err)
		return summary, err
	}
	for _, replica := range replicasets {
		summary.ReplicaSetsDesired += int(replica.Status.Replicas)
		summary.ReplicaSetsReady += int(replica.Status.ReadyReplicas)
	}

	// statefulsets count
	statefulsets, err := c.GetStatefulSetList(ctx)
	if err != nil {
		c.reportError(err)
		return summary, err
	}
	for _, stateful := range statefulsets {
		summary.StatefulSetsReady += int(stateful.Status.ReadyReplicas)
	}

	// extract jobs summary
	jobs, err := c.GetJobList(ctx)
	if err != nil {
		c.reportError(err)
		return summary, err
	}
	summary.JobsCount = len(jobs)
	cronjobs, err := c.GetCronJobList(ctx)
	if err != nil {
		c.reportError(err)
		return summary, err
	}
	summary.CronJobsCount = len(cronjobs)

	pvs, err := c.GetPVList(ctx)
	if err != nil {
		c.reportError(err)
		return summary, err
	}
	summary.PVCount = len(pvs)
	summary.PVsTotal = resource.NewQuantity(0, resource.DecimalSI)
	for _, pv := range pvs {
		if storage := pv.Spec.Capacity.Storage(); storage != nil {
			summary.PVsTotal.Add(*storage)
		}
	}

	pvcs, err := c.GetPVCList(ctx)
	if err != nil {
		c.reportError(err)
		return summary, err
	}
	summary.PVCCount = len(pvcs)
	summary.PVCsTotal = resource.NewQuantity(0, resource.DecimalSI)
	for _, pvc := range pvcs {
		if storage := pvc.Spec.Resources.Requests.Storage(); storage != nil {
			summary.PVCsTotal.Add(*storage)
		}
	}

	// === Prometheus-Enhanced Metrics Collection ===
	// When using Prometheus source, aggregate additional cluster-wide metrics
	// from the already-fetched nodeMetricsByNode map without re-querying.
	if c.metricsSource != nil && summary.MetricsSourceType == metrics.SourceTypePrometheus {
		c.collectPrometheusEnhancedMetrics(&summary, nodes, nodeMetricsByNode)
	}

	hasMetrics := c.metricsSource != nil && (len(nodeMetricsByNode) > 0 || (len(nodes) == 0 && c.metricsSource.IsHealthy()))
	summary.HealthScore = model.CalculateHealthScore(summary, hasMetrics)

	// Report success after all data is collected
	c.reportSuccess()
	return summary, nil
}

func isContainerOOMKilled(cs coreV1.ContainerStatus) bool {
	return (cs.State.Terminated != nil && cs.State.Terminated.Reason == "OOMKilled") ||
		(cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled")
}

func isContainerCrashLoop(cs coreV1.ContainerStatus) bool {
	return cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff"
}

// collectPrometheusEnhancedMetrics populates Prometheus-specific cluster summary fields
func (c *Controller) collectPrometheusEnhancedMetrics(summary *model.ClusterSummary, nodes []*coreV1.Node, nodeMetricsByNode map[string]*metrics.NodeMetrics) {
	for _, node := range nodes {
		nodeMetrics, ok := nodeMetricsByNode[node.Name]
		if !ok || nodeMetrics == nil {
			continue
		}

		// Network I/O rates (sum across nodes)
		summary.NetworkRxRate += nodeMetrics.NetworkRxRate
		summary.NetworkTxRate += nodeMetrics.NetworkTxRate

		// Disk I/O rates (sum across nodes)
		summary.DiskReadRate += nodeMetrics.DiskReadRate
		summary.DiskWriteRate += nodeMetrics.DiskWriteRate

		// Native Kubelet pod and container inventory counts
		summary.KubeletRunningPods += nodeMetrics.PodCount
		summary.KubeletRunningContainers += nodeMetrics.ContainerCount

		// Load averages (compute cluster average)
		// Note: Load averages are not exposed by kubelet/cAdvisor (require node_exporter)
		summary.LoadAverage1m += nodeMetrics.LoadAverage1m
		summary.LoadAverage5m += nodeMetrics.LoadAverage5m
		summary.LoadAverage15m += nodeMetrics.LoadAverage15m

		// PSI: sum per-axis, divide after the loop for cluster-wide averages.
		// Per-node values are surfaced separately in the Nodes table.
		summary.AvgNodeCPUStallPct += nodeMetrics.PSI.CPUStallPct
		summary.AvgNodeMemStallPct += nodeMetrics.PSI.MemStallPct
		summary.AvgNodeIOStallPct += nodeMetrics.PSI.IOStallPct
	}

	// Average the PSI sums across all nodes.
	if len(nodes) > 0 {
		summary.AvgNodeCPUStallPct /= float64(len(nodes))
		summary.AvgNodeMemStallPct /= float64(len(nodes))
		summary.AvgNodeIOStallPct /= float64(len(nodes))
	}

	// PSIBeta is a cluster-wide capability flag; set once from the client.
	summary.PSIBeta = !c.client.PSIVersionOK()

	// Average the load across nodes (will be 0 without node_exporter)
	if len(nodes) > 0 {
		summary.LoadAverage1m /= float64(len(nodes))
		summary.LoadAverage5m /= float64(len(nodes))
		summary.LoadAverage15m /= float64(len(nodes))
	}
}
