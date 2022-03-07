package k8s

import (
	"context"
	"fmt"
	"time"

	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	restclient "k8s.io/client-go/rest"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

const (
	AllNamespaces = "*"
)

type Client struct {
	namespace         string
	config            *restclient.Config
	kubeClient        kubernetes.Interface
	dynaClient        dynamic.Interface
	discoClient       discovery.CachedDiscoveryInterface
	metricsClient     *metricsclient.Clientset
	metricsAvailCount int
	refreshTimeout    time.Duration
	controller        *Controller
}

func New(flags *genericclioptions.ConfigFlags) (*Client, error) {
	if flags == nil {
		return nil, fmt.Errorf("configuration flagset is nil")
	}

	config, err := flags.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	dyna, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	disco, err := flags.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	metrics, err := metricsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	var namespace = *flags.Namespace
	if namespace == "" || namespace == "*" {
		namespace = AllNamespaces
	}

	client := &Client{
		namespace:     namespace,
		config:        config,
		kubeClient: kubeClient,
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

func (k8s *Client) GetServerVersion() (string, error){
	version, err := k8s.discoClient.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to connect to server: %s", err)
	}
	return version.String(), nil
}

// AssertMetricsAvailable checks for available metrics server every 10th invocation.
// Otherwise, it returns the last known registration state of metrics server.
func (k8s *Client) AssertMetricsAvailable() error {
	if k8s.metricsAvailCount != 0 {
		if k8s.metricsAvailCount%10 != 0 {
			k8s.metricsAvailCount++
		} else {
			k8s.metricsAvailCount = 0
		}
		return nil
	}

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

	if !avail {
		return fmt.Errorf("metrics api not available")
	}
	return nil
}

// GetNodeMetrics returns metrics for specified node
func (k8s *Client) GetNodeMetrics(ctx context.Context, nodeName string) (*metricsV1beta1.NodeMetrics, error) {
	if err := k8s.AssertMetricsAvailable(); err != nil {
		return nil, fmt.Errorf("node metrics: %s", err)
	}

	metrics, err := k8s.metricsClient.MetricsV1beta1().NodeMetricses().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

// GetPodMetricsByName returns metrics for specified pod
func (k8s *Client) GetPodMetricsByName(ctx context.Context, pod *coreV1.Pod) (*metricsV1beta1.PodMetrics, error) {
	if err := k8s.AssertMetricsAvailable(); err != nil {
		return nil, fmt.Errorf("pod metrics by name: %s", err)
	}

	metrics, err := k8s.metricsClient.MetricsV1beta1().PodMetricses(pod.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

func (k8s *Client) GetAllPodMetrics(ctx context.Context) ([]metricsV1beta1.PodMetrics, error) {
	if err := k8s.AssertMetricsAvailable(); err != nil {
		return nil, fmt.Errorf("pod metrics: %s", err)
	}

	metricsList, err := k8s.metricsClient.MetricsV1beta1().PodMetricses(k8s.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return metricsList.Items, nil
}

func (k8s *Client) Controller() *Controller {
	return k8s.controller
}
