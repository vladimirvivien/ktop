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
		return err
	}
	summary.Namespaces = len(namespaces)

	nodes, err := c.GetNodeList(ctx)
	if err != nil {
		return err
	}
	summary.Uptime = metav1.NewTime(time.Now())
	summary.NodesCount = len(nodes)
	summary.AllocatableNodeMemTotal = resource.NewQuantity(0, resource.DecimalSI)
	summary.AllocatableNodeCpuTotal = resource.NewQuantity(0, resource.DecimalSI)
	summary.UsageNodeMemTotal = resource.NewQuantity(0, resource.DecimalSI)
	summary.UsageNodeCpuTotal = resource.NewQuantity(0, resource.DecimalSI)
	for _, node := range nodes {
		if model.GetNodeReadyStatus(node) == string(coreV1.NodeReady) {
			summary.NodesReady++
		}
		if node.CreationTimestamp.Before(&summary.Uptime) {
			summary.Uptime = node.CreationTimestamp
		}

		summary.Pressures += len(model.GetNodePressures(node))
		summary.ImagesCount += len(node.Status.Images)
		summary.VolumesInUse += len(node.Status.VolumesInUse)

		summary.AllocatableNodeMemTotal.Add(*node.Status.Allocatable.Memory())
		summary.AllocatableNodeCpuTotal.Add(*node.Status.Allocatable.Cpu())

		metrics, err := c.GetNodeMetrics(ctx, node.Name)
		if err != nil {
			// Metrics not available - skip adding to usage totals (graceful degradation)
			continue
		}
		// Only add if metrics.Usage is not nil
		if metrics.Usage.Memory() != nil {
			summary.UsageNodeMemTotal.Add(*metrics.Usage.Memory())
		}
		if metrics.Usage.Cpu() != nil {
			summary.UsageNodeCpuTotal.Add(*metrics.Usage.Cpu())
		}
	}

	// extract pods summary
	pods, err := c.GetPodList(ctx)
	if err != nil {
		return err
	}
	summary.PodsAvailable = len(pods)
	summary.RequestedPodMemTotal = resource.NewQuantity(0, resource.DecimalSI)
	summary.RequestedPodCpuTotal = resource.NewQuantity(0, resource.DecimalSI)
	for _, pod := range pods {
		switch pod.Status.Phase {
		case coreV1.PodRunning:
			summary.PodsRunning++
			// Count running containers and restarts
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Running != nil {
					summary.ContainerCount++
				}
				summary.ContainerRestarts += int(cs.RestartCount)
			}
		case coreV1.PodFailed:
			summary.FailedPods++
			// Check if evicted
			if pod.Status.Reason == "Evicted" {
				summary.EvictedPods++
			}
		}
		containerSummary := model.GetPodContainerSummary(pod)
		summary.RequestedPodMemTotal.Add(*containerSummary.RequestedMemQty)
		summary.RequestedPodCpuTotal.Add(*containerSummary.RequestedCpuQty)
	}

	// deployments count
	deps, err := c.GetDeploymentList(ctx)
	if err != nil {
		return err
	}
	for _, dep := range deps {
		summary.DeploymentsTotal += int(dep.Status.Replicas)
		summary.DeploymentsReady += int(dep.Status.ReadyReplicas)
	}

	// deamonset count
	daemonsets, err := c.GetDaemonSetList(ctx)
	if err != nil {
		return err
	}
	for _, set := range daemonsets {
		summary.DaemonSetsDesired += int(set.Status.DesiredNumberScheduled)
		summary.DaemonSetsReady += int(set.Status.NumberReady)
	}

	// replicasets count
	replicasets, err := c.GetReplicaSetList(ctx)
	if err != nil {
		return err
	}
	for _, replica := range replicasets {
		summary.ReplicaSetsDesired += int(replica.Status.Replicas)
		summary.ReplicaSetsReady += int(replica.Status.ReadyReplicas)
	}

	// statefulsets count
	statefulsets, err := c.GetStatefulSetList(ctx)
	if err != nil {
		return err
	}
	for _, stateful := range statefulsets {
		summary.StatefulSetsReady += int(stateful.Status.ReadyReplicas)
	}

	// extract jobs summary
	jobs, err := c.GetJobList(ctx)
	if err != nil {
		return err
	}
	summary.JobsCount = len(jobs)
	cronjobs, err := c.GetCronJobList(ctx)
	if err != nil {
		return err
	}
	summary.CronJobsCount = len(cronjobs)

	pvs, err := c.GetPVList(ctx)
	if err != nil {
		return err
	}
	summary.PVCount = len(pvs)
	summary.PVsTotal = resource.NewQuantity(0, resource.DecimalSI)
	for _, pv := range pvs {
		if pv.Status.Phase == coreV1.VolumeBound {
			summary.PVsTotal.Add(*pv.Spec.Capacity.Storage())
		}
	}

	pvcs, err := c.GetPVCList(ctx)
	if err != nil {
		c.reportError(err)
		return err
	}
	summary.PVCCount = len(pvcs)
	summary.PVCsTotal = resource.NewQuantity(0, resource.DecimalSI)
	for _, pvc := range pvcs {
		if pvc.Status.Phase == coreV1.ClaimBound {
			summary.PVCsTotal.Add(*pvc.Spec.Resources.Requests.Storage())
		}
	}

	// === Prometheus-Enhanced Metrics Collection ===
	// When using Prometheus source, collect additional cluster-wide metrics
	if c.metricsSource != nil && summary.MetricsSourceType == metrics.SourceTypePrometheus {
		c.collectPrometheusEnhancedMetrics(ctx, &summary, nodes, pods)
	}

	// Report success after all data is collected
	c.reportSuccess()
	handlerFunc(ctx, summary)
	return nil
}

// collectPrometheusEnhancedMetrics populates Prometheus-specific cluster summary fields
func (c *Controller) collectPrometheusEnhancedMetrics(ctx context.Context, summary *model.ClusterSummary, nodes []*coreV1.Node, pods []*coreV1.Pod) {
	// Aggregate enhanced metrics from all nodes
	// These come from the NodeMetrics returned by GetNodeMetrics when using Prometheus source

	for _, node := range nodes {
		nodeMetrics, err := c.metricsSource.GetNodeMetrics(ctx, node.Name)
		if err != nil {
			continue
		}

		// Network I/O rates (sum across nodes)
		summary.NetworkRxRate += nodeMetrics.NetworkRxRate
		summary.NetworkTxRate += nodeMetrics.NetworkTxRate

		// Disk I/O rates (sum across nodes)
		summary.DiskReadRate += nodeMetrics.DiskReadRate
		summary.DiskWriteRate += nodeMetrics.DiskWriteRate

		// Load averages (compute cluster average)
		// Note: Load averages are not exposed by kubelet/cAdvisor (require node_exporter)
		summary.LoadAverage1m += nodeMetrics.LoadAverage1m
		summary.LoadAverage5m += nodeMetrics.LoadAverage5m
		summary.LoadAverage15m += nodeMetrics.LoadAverage15m
	}

	// Average the load across nodes (will be 0 without node_exporter)
	if len(nodes) > 0 {
		summary.LoadAverage1m /= float64(len(nodes))
		summary.LoadAverage5m /= float64(len(nodes))
		summary.LoadAverage15m /= float64(len(nodes))
	}

	// Count node pressures
	for _, node := range nodes {
		pressures := model.GetNodePressures(node)
		if len(pressures) > 0 {
			summary.NodePressureCount++
		}
	}
}
