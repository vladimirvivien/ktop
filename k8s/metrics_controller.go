package k8s

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// GetNodeMetrics returns metrics for specified node
func (c *Controller) GetNodeMetrics(ctx context.Context, nodeName string) (*metricsV1beta1.NodeMetrics, error) {
	// Check if we have a hybrid metrics controller
	if hybridController := c.client.GetMetricsController(); hybridController != nil {
		// Use the hybrid controller to get metrics
		nodeMetrics, err := hybridController.GetNodeMetrics(ctx, nodeName)
		if err != nil {
			return nil, err
		}
		
		// Convert our NodeMetrics to metricsV1beta1.NodeMetrics
		result := &metricsV1beta1.NodeMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
			},
			Timestamp: metav1.Time{Time: nodeMetrics.Timestamp},
		}
		
		// Only set usage if we have metrics
		usage := v1.ResourceList{}
		if nodeMetrics.CPUUsage != nil {
			usage[v1.ResourceCPU] = *nodeMetrics.CPUUsage
		}
		if nodeMetrics.MemoryUsage != nil {
			usage[v1.ResourceMemory] = *nodeMetrics.MemoryUsage
		}
		result.Usage = usage
		
		return result, nil
	}

	// Fallback to original implementation
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
func (c *Controller) GetPodMetricsByName(ctx context.Context, pod *v1.Pod) (*metricsV1beta1.PodMetrics, error) {
	// Check if we have a hybrid metrics controller
	if hybridController := c.client.GetMetricsController(); hybridController != nil {
		// Use the hybrid controller to get metrics
		podMetrics, err := hybridController.GetPodMetrics(ctx, pod.Namespace, pod.Name)
		if err != nil {
			return nil, err
		}
		
		// Convert our PodMetrics to metricsV1beta1.PodMetrics
		result := &metricsV1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
			Timestamp: metav1.Time{Time: podMetrics.Timestamp},
		}
		
		// Convert container metrics
		var containers []metricsV1beta1.ContainerMetrics
		for _, cm := range podMetrics.ContainerMetrics {
			container := metricsV1beta1.ContainerMetrics{
				Name: cm.Name,
			}
			usage := v1.ResourceList{}
			if cm.CPUUsage != nil {
				usage[v1.ResourceCPU] = *cm.CPUUsage
			}
			if cm.MemoryUsage != nil {
				usage[v1.ResourceMemory] = *cm.MemoryUsage
			}
			container.Usage = usage
			containers = append(containers, container)
		}
		result.Containers = containers
		
		return result, nil
	}

	// Fallback to original implementation
	if err := c.client.AssertMetricsAvailable(); err != nil {
		return nil, fmt.Errorf("pod metrics by name: %s", err)
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

	metricsList, err := c.podMetricsInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	return metricsList, nil
}
