package k8s

import (
	"context"
	"time"

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
	summary.AllocatableMemTotal = new(resource.Quantity)
	summary.AllocatableCpuTotal = new(resource.Quantity)

	for _, node := range nodes {
		if model.GetNodeReadyStatus(&node) == string(coreV1.NodeReady) {
			summary.NodesReady++
		}
		if node.CreationTimestamp.Before(&summary.Uptime) {
			summary.Uptime = node.CreationTimestamp
		}

		summary.Pressures += len(model.GetNodePressures(&node))
		summary.ImagesCount += len(node.Status.Images)
		summary.PVsInUse += len(node.Status.VolumesInUse)

		summary.AllocatableMemTotal.Add(*node.Status.Allocatable.Memory())
		summary.AllocatableCpuTotal.Add(*node.Status.Allocatable.Cpu())
	}

	// extract pods summary
	pods, err := c.GetPodList(ctx)
	if err != nil {
		return err
	}
	summary.PodsAvailable = len(pods)
	summary.RequestedMemTotal = resource.NewQuantity(0, resource.DecimalSI)
	summary.RequestedCpuTotal = resource.NewQuantity(0, resource.DecimalSI)
	for _, pod := range pods {
		if pod.Status.Phase == coreV1.PodRunning {
			summary.PodsRunning++
		}
		for _, container := range pod.Status.ContainerStatuses {
			if container.State.Running != nil {
				summary.ContainersRunning++
			}
		}
		podCpus, podMems := model.GetPodResourceRequests(&pod)
		summary.RequestedMemTotal.Add(*podMems)
		summary.RequestedCpuTotal.Add(*podCpus)
	}

	// deployments count
	deps, err := c.GetDeploymentList(ctx)
	if err != nil {
		return err
	}
	for _, dep := range deps {
		summary.DeploymentsDesired += int(dep.Status.Replicas)
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

	handlerFunc(ctx, summary)
	return nil
}
