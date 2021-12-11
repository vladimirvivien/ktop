package k8s

import (
	"context"
	"time"

	"github.com/vladimirvivien/ktop/views/model"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func (c *Controller) GetPodList(ctx context.Context) (pods []coreV1.Pod, err error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.podInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		unstructPod, ok := item.(runtime.Unstructured)
		if !ok {
			panic("Controller: GetPodList: unexpected type encountered")
		}
		pod := new(coreV1.Pod)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructPod.UnstructuredContent(), pod); err != nil {
			continue
		}
		pods = append(pods, *pod)
	}
	return
}

func (c *Controller) GetPodModels(ctx context.Context) (models []model.PodModel, err error) {
	pods, err := c.GetPodList(ctx)
	if err != nil {
		return
	}
	nodeMetricsCache := make(map[string]*metricsV1beta1.NodeMetrics)
	for _, pod := range pods {
		// retrieve metrics for pod
		podMetrics, err := c.client.GetPodMetrics(ctx, pod.Name)
		if err != nil {
			podMetrics = new(metricsV1beta1.PodMetrics)
		}
		if metrics, ok := nodeMetricsCache[pod.Spec.NodeName]; !ok {
			metrics, err = c.client.GetNodeMetrics(ctx, pod.Spec.NodeName)
			if err != nil {
				metrics = new(metricsV1beta1.NodeMetrics)
			}
			nodeMetricsCache[pod.Spec.NodeName] = metrics
		}
		nodeMetrics := nodeMetricsCache[pod.Spec.NodeName]
		model := model.NewPodModel(&pod, podMetrics, nodeMetrics)
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
