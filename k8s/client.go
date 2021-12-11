package k8s

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	restclient "k8s.io/client-go/rest"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

type Client struct {
	namespace        string
	config           *restclient.Config
	dynaClient       dynamic.Interface
	discoClient      *discovery.DiscoveryClient
	metricsClient    *metricsclient.Clientset
	metricsAvailable bool
	refreshTimeout   time.Duration
	controller *Controller
}

func New(ctx context.Context, kubeconfig, kubectx, namespace string) (*Client, error) {
	config, err := loadConfig(kubeconfig, kubectx)
	if err != nil {
		return nil, err
	}

	dyna, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	disco, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}

	metrics, err := metricsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	client := &Client{
		namespace:     namespace,
		config:        config,
		dynaClient:    dyna,
		discoClient:   disco,
		metricsClient: metrics,
	}
	client.controller = newController(client)
	return client, nil
}

func (k8s *Client) ResourceInterface(resource schema.GroupVersionResource) dynamic.ResourceInterface {
	return k8s.dynaClient.Resource(resource)
}

func (k8s *Client) NamespacedResourceInterface(resource schema.GroupVersionResource) dynamic.ResourceInterface {
	return k8s.dynaClient.Resource(resource).Namespace(k8s.namespace)
}

func (k8s *Client) Namespace() string {
	return k8s.namespace
}

func (k8s *Client) Config() *restclient.Config {
	return k8s.config
}

func (k8s *Client) AssertMetricsAvailable() error {
	groups, err := k8s.discoClient.ServerGroups()
	if err != nil {
		return err
	}

	avail := false
	for _, group := range groups.Groups {
		if group.Name == metricsapi.GroupName {
			avail = true
		}
	}

	k8s.metricsAvailable = avail
	if !avail {
		return fmt.Errorf("metrics api not available")
	}
	return nil
}

// GetNodeMetrics returns metrics for specified node
func (k8s *Client) GetNodeMetrics(ctx context.Context, nodeName string) (*metricsV1beta1.NodeMetrics, error) {
	if !k8s.metricsAvailable {
		return nil, fmt.Errorf("metrics api not available")
	}

	metrics, err := k8s.metricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

// GetPodMetrics returns metrics for specified pod
func (k8s *Client) GetPodMetrics(ctx context.Context, podName string) (*metricsV1beta1.PodMetrics, error) {
	if !k8s.metricsAvailable {
		return nil, fmt.Errorf("metrics api not available")
	}

	metrics, err := k8s.metricsClient.MetricsV1beta1().PodMetricses(k8s.namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

func (k8s *Client) Controller() *Controller {
	return k8s.controller
}