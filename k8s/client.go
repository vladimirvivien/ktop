package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	restclient "k8s.io/client-go/rest"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
)

type ListFunc func(namespace string, list runtime.Object) error

type Client struct {
	ctx              context.Context
	namespace        string
	manager          ctrl.Manager
	discoClient      *discovery.DiscoveryClient
	metricsClient    *metricsclient.Clientset
	metricsAvailable bool
	nodeCtrl         *NodeController
	podCtrl *PodController
}

func New(ctx context.Context, namespace string) (*Client, error) {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Namespace: namespace,
	})

	if err != nil {
		return nil, err
	}

	config := mgr.GetConfig()

	disco, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}

	metrics, err := metricsclient.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}

	nodeCtrl, err := NewNodeController(ctx, mgr)
	if err != nil {
		return nil, err
	}

	podCtrl, err := NewPodController(mgr)
	if err != nil {
		return nil, err
	}

	client := &Client{
		ctx:           ctx,
		namespace:     namespace,
		manager:       mgr,
		discoClient:   disco,
		metricsClient: metrics,
		nodeCtrl:      nodeCtrl,
		podCtrl: podCtrl,
	}

	//if err := client.AssertMetricsAvailable(); err != nil {
	//	return nil, fmt.Errorf("client: unable to create: %s", err)
	//}

	return client, nil
}

func (k8s *Client) Namespace() string {
	return k8s.namespace
}

func (k8s *Client) Config() *restclient.Config {
	return k8s.manager.GetConfig()
}

func (k8s *Client) Start() error {
	errCh := make(chan error)

	go func() {
		errCh <- k8s.manager.Start(ctrl.SetupSignalHandler())
	}()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
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
func (k8s *Client) GetNodeMetrics(nodeName string) (*metricsV1beta1.NodeMetrics, error) {
	if !k8s.metricsAvailable {
		return nil, fmt.Errorf("metrics api not available")
	}

	metrics, err := k8s.metricsClient.MetricsV1beta1().NodeMetricses().Get(k8s.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

// GetPodMetrics returns metrics for specified pod
func (k8s *Client) GetPodMetrics(podName string) (*metricsV1beta1.PodMetrics, error) {
	if !k8s.metricsAvailable {
		return nil, fmt.Errorf("metrics api not available")
	}

	metrics, err := k8s.metricsClient.MetricsV1beta1().PodMetricses(k8s.namespace).Get(k8s.ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

func (k8s *Client) SetNodeListFunc(f ListFunc) {
	k8s.nodeCtrl.SetListFunc(f)
}

func (k8s *Client) SetPodListFunc(f ListFunc) {
	k8s.podCtrl.SetListFunc(f)
}
