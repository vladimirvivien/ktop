package k8s

import (
	"context"
	"io"
	"time"

	"github.com/vladimirvivien/ktop/views/model"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// LogOptions configures pod log retrieval
type LogOptions struct {
	Container  string
	Follow     bool
	Previous   bool
	Timestamps bool
	TailLines  int64
}

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

// GetPod returns a single pod by namespace and name
func (c *Controller) GetPod(ctx context.Context, namespace, podName string) (*coreV1.Pod, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	pod, err := c.podInformer.Lister().Pods(namespace).Get(podName)
	if err != nil {
		return nil, err
	}
	// Return a deep copy to avoid pointer to informer cache issues
	return pod.DeepCopy(), nil
}

// GetPodLogs returns a reader for streaming pod container logs
func (c *Controller) GetPodLogs(ctx context.Context, namespace, podName string, opts LogOptions) (io.ReadCloser, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	logOpts := &coreV1.PodLogOptions{
		Container:  opts.Container,
		Follow:     opts.Follow,
		Previous:   opts.Previous,
		Timestamps: opts.Timestamps,
	}
	if opts.TailLines > 0 {
		logOpts.TailLines = &opts.TailLines
	}

	req := c.client.kubeClient.CoreV1().Pods(namespace).GetLogs(podName, logOpts)
	return req.Stream(ctx)
}

func (c *Controller) GetPodModels(ctx context.Context) (models []model.PodModel, err error) {
	pods, err := c.GetPodList(ctx)
	if err != nil {
		return
	}
	nodeAllocResMap := make(map[string]coreV1.ResourceList)

	// NOTE: Pod metrics are now fetched in batch by the view layer (refreshPods)
	// to avoid N individual API calls. We pass empty metrics here and they'll be
	// populated later with actual metrics from GetAllPodMetrics().
	// This significantly improves performance with large pod counts.

	// NOTE: Node metrics are also not fetched here anymore. They were causing
	// slow timeouts (6s per node) when metrics-server is unavailable.
	// Node metrics are used for percentage calculations, which can be done in the view layer.
	emptyNodeMetrics := new(metricsV1beta1.NodeMetrics)

	for _, pod := range pods {
		podMetrics := new(metricsV1beta1.PodMetrics)

		model := model.NewPodModel(pod, podMetrics, emptyNodeMetrics)

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
	// Skip refresh if API is disconnected - don't update UI with stale cached data
	if c.healthTracker != nil && c.healthTracker.IsDisconnected() {
		return nil
	}

	models, err := c.GetPodModels(ctx)
	if err != nil {
		c.reportError(err)
		return err
	}
	c.reportSuccess()
	refreshFunc(ctx, models)
	return nil
}
