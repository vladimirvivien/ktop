package k8s

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// GetNodeMetrics returns metrics for specified node
// Now uses MetricsSource interface instead of metrics informers
func (c *Controller) GetNodeMetrics(ctx context.Context, nodeName string) (*metricsV1beta1.NodeMetrics, error) {
	// If no metrics source available, return error for graceful degradation
	if c.metricsSource == nil {
		return nil, fmt.Errorf("metrics source not available")
	}

	// Fetch metrics from the configured source (metrics-server or prometheus)
	nodeMetrics, err := c.metricsSource.GetNodeMetrics(ctx, nodeName)
	if err != nil {
		return nil, fmt.Errorf("node metrics: %w", err)
	}

	// Convert from metrics.NodeMetrics to v1beta1.NodeMetrics
	usage := v1.ResourceList{}
	if nodeMetrics.CPUUsage != nil {
		usage[v1.ResourceCPU] = *nodeMetrics.CPUUsage
	}
	if nodeMetrics.MemoryUsage != nil {
		usage[v1.ResourceMemory] = *nodeMetrics.MemoryUsage
	}

	return &metricsV1beta1.NodeMetrics{
		ObjectMeta: metav1.ObjectMeta{Name: nodeMetrics.NodeName},
		Timestamp:  metav1.NewTime(nodeMetrics.Timestamp),
		Window:     metav1.Duration{Duration: 0},
		Usage:      usage,
	}, nil
}

// GetPodMetricsByName returns pod metrics for specified pod
func (c *Controller) GetPodMetricsByName(ctx context.Context, pod *v1.Pod) (*metricsV1beta1.PodMetrics, error) {
	if err := c.client.AssertMetricsAvailable(); err != nil {
		return nil, fmt.Errorf("pod metrics by name: %s", err)
	}

	if c.podMetricsInformer == nil {
		return nil, fmt.Errorf("pod metrics informer not available")
	}

	metrics, err := c.podMetricsInformer.Lister().Get(pod)
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

	if c.podMetricsInformer == nil {
		return nil, fmt.Errorf("pod metrics informer not available")
	}

	metricsList, err := c.podMetricsInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	return metricsList, nil
}
