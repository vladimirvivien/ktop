package k8s

import (
	"context"
	"fmt"

	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// GetNodeMetrics returns metrics for specified node
func (c *Controller) GetNodeMetrics(ctx context.Context, nodeName string) (*metricsV1beta1.NodeMetrics, error) {
	if err := c.client.AssertMetricsAvailable(); err != nil {
		return nil, fmt.Errorf("node metrics: %s", err)
	}

	metrics, err := c.nodeMetricsInformer.Lister().Get(nodeName)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

// GetPodMetricsByName returns pod metrics for specified pod
func (c *Controller) GetPodMetricsByName(ctx context.Context, pod *coreV1.Pod) (*metricsV1beta1.PodMetrics, error) {
	if err := c.client.AssertMetricsAvailable(); err != nil {
		return nil, fmt.Errorf("pod metrics by name: %s", err)
	}

	metrics, err := c.podMetricsInformer.Lister().Get(pod.Name)
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

// GetAllPodMetrics retrieve all available pod emtrics
func (c *Controller) GetAllPodMetrics(ctx context.Context) ([]*metricsV1beta1.PodMetrics, error) {
	if err := c.client.AssertMetricsAvailable(); err != nil {
		return nil, fmt.Errorf("all pod metrics: %s", err)
	}

	metricsList, err := c.podMetricsInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	return metricsList, nil
}
