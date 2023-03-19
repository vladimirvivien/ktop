package k8s

import (
	"context"
	"time"

	"github.com/vladimirvivien/ktop/views/model"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func (c *Controller) GetPodList(ctx context.Context) ([]*coreV1.Pod, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.podInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Controller) GetPodModels(ctx context.Context) (models []model.PodModel, err error) {
	pods, err := c.GetPodList(ctx)
	if err != nil {
		return
	}
	nodeMetricsCache := make(map[string]*metricsV1beta1.NodeMetrics)
	nodeAllocResMap := make(map[string]coreV1.ResourceList)
	for _, pod := range pods {

		// retrieve metrics per pod
		podMetrics, err := c.GetPodMetricsByName(ctx, pod)
		if err != nil {
			podMetrics = new(metricsV1beta1.PodMetrics)
		}

		// retrieve and cache node metrics for related pod-node
		if metrics, ok := nodeMetricsCache[pod.Spec.NodeName]; !ok {
			metrics, err = c.GetNodeMetrics(ctx, pod.Spec.NodeName)
			if err != nil {
				metrics = new(metricsV1beta1.NodeMetrics)
			}
			nodeMetricsCache[pod.Spec.NodeName] = metrics
		}
		nodeMetrics := nodeMetricsCache[pod.Spec.NodeName]

		model := model.NewPodModel(pod, podMetrics, nodeMetrics)

		// retrieve pod's node allocatable resources
		if alloc, ok := nodeAllocResMap[pod.Spec.NodeName]; !ok {
			node, err := c.GetNode(ctx, pod.Spec.NodeName)
			if err != nil {
				alloc = coreV1.ResourceList{}
			} else {
				alloc = node.Status.Allocatable
			}
			nodeAllocResMap[pod.Spec.NodeName] = alloc
		}
		alloc := nodeAllocResMap[pod.Spec.NodeName]
		model.NodeAllocatableMemQty = alloc.Memory()
		model.NodeAllocatableCpuQty = alloc.Cpu()
		models = append(models, *model)
	}
	return
}

func (c *Controller) installPodsHandler(ctx context.Context, refreshFunc RefreshPodsFunc) {
	if refreshFunc == nil {
		return
	}
	go func() {
		c.refreshPods(ctx, refreshFunc) // initial refresh
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.refreshPods(ctx, refreshFunc); err != nil {
					continue
				}
			}
		}
	}()
}

func (c *Controller) refreshPods(ctx context.Context, refreshFunc RefreshPodsFunc) error {
	models, err := c.GetPodModels(ctx)
	if err != nil {
		return err
	}
	refreshFunc(ctx, models)
	return nil
}
