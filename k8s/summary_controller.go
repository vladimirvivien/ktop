package k8s

import (
	"context"
	"time"

	"github.com/vladimirvivien/ktop/views/model"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
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
	var summary model.ClusterSummary

	// extract namespace summary
	namespaces, err := c.GetNamespaceList(ctx)
	if err != nil {
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
			metrics = new(metricsV1beta1.NodeMetrics)
		}
		summary.UsageNodeMemTotal.Add(*metrics.Usage.Memory())
		summary.UsageNodeCpuTotal.Add(*metrics.Usage.Cpu())
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
		if pod.Status.Phase == coreV1.PodRunning {
			summary.PodsRunning++
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
		return err
	}
	summary.PVCCount = len(pvcs)
	summary.PVCsTotal = resource.NewQuantity(0, resource.DecimalSI)
	for _, pvc := range pvcs {
		if pvc.Status.Phase == coreV1.ClaimBound {
			summary.PVCsTotal.Add(*pvc.Spec.Resources.Requests.Storage())
		}
	}

	handlerFunc(ctx, summary)
	return nil
}
