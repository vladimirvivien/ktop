package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	restclient "k8s.io/client-go/rest"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Client struct {
	ctx              context.Context
	namespace        string
	manager          ctrl.Manager
	discoClient      *discovery.DiscoveryClient
	metricsClient    *metricsclient.Clientset
	metricsAvailable bool
	nodeCtrl         *NodeController
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

	client := &Client{
		ctx:           ctx,
		namespace:     namespace,
		manager:       mgr,
		discoClient:   disco,
		metricsClient: metrics,
		nodeCtrl:      nodeCtrl,
	}

	if err := client.AssertMetricsAvailable(); err != nil {
		return nil, fmt.Errorf("failed to create client: %s", err)
	}

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

func (k8s *Client) AddNodeUpdateHandler(f NodeUpdateFunc) {
	k8s.nodeCtrl.AddUpdateHandler(f)
}

func (k8s *Client) AddNodeDeleteHandler(f NodeDeleteFunc) {
	k8s.nodeCtrl.AddDeleteHandler(f)
}

//// ********************************************************************************************************
//type Client struct {
//	Namespace       string
//	DynamicClient   dynamic.Interface
//	InformerFactory dynamicinformer.DynamicSharedInformerFactory
//	Config          *restclient.Config
//
//	MetricsAreAvailable bool
//}
//
//func NewClient(kubeconfig string, namespace string) (*Client, error) {
//	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
//	if err != nil {
//		return nil, err
//	}
//
//	dynclient := dynamic.NewForConfigOrDie(config)
//	discoClient := discovery.NewDiscoveryClientForConfigOrDie(config)
//	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynclient, time.Second*3, namespace, nil)
//	Client := &Client{
//		Namespace:       namespace,
//		DynamicClient:   dynclient,
//		Config:          config,
//		InformerFactory: factory,
//	}
//	Client.MetricsAreAvailable = areMetricsAvail(discoClient)
//	return Client, nil
//}
//
//func (c *Client) Start(stopCh <-chan struct{}) {
//	if c.InformerFactory == nil {
//		panic("Failed to start Client, nil InformerFactory")
//	}
//
//	for name, res := range Resources {
//		if synced := c.InformerFactory.WaitForCacheSync(stopCh); !synced[res] {
//			panic(fmt.Sprintf("Informer for %s did not sync", name))
//		}
//	}
//}
//
//func areMetricsAvail(disco *discovery.DiscoveryClient) bool {
//	groups, err := disco.ServerGroups()
//	if err != nil {
//		return false
//	}
//
//	for _, group := range groups.Groups {
//		if group.Name == metricsapi.GroupName {
//			return true
//		}
//	}
//	return false
//}
//
//// GetMetricsByNode returns metrics for specified node
//func (c *Client) GetMetricsByNode(nodeName string) (*metricsV1beta1.NodeMetrics, error) {
//	// TODO unfortunately, nodemetric types are not watchable (without applying RBAC rules)
//	// for now, the code just does a simple list every time metrics are needed
//
//	if !c.MetricsAreAvailable {
//		return new(metricsV1beta1.NodeMetrics), nil
//	}
//
//	objList, err := c.DynamicClient.Resource(Resources[NodeMetricsResource]).List(context.Background(), metav1.ListOptions{})
//	if err != nil {
//		return nil, err
//	}
//
//	for _, obj := range objList.Items {
//		if obj.GetName() == nodeName {
//			metrics := new(metricsV1beta1.NodeMetrics)
//			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &metrics); err != nil {
//				return nil, err
//			}
//			return metrics, nil
//		}
//	}
//	return new(metricsV1beta1.NodeMetrics), nil
//}
//
//// GetMetricsByPod returns metrics for specified pod
//func (c *Client) GetMetricsByPod(podName string) (*metricsV1beta1.PodMetrics, error) {
//	// TODO unfortunately, podmetric types are not watchable (without applying RBAC rules)
//	// for now, the code just does a simple list every time metrics are needed
//
//	if !c.MetricsAreAvailable {
//		return new(metricsV1beta1.PodMetrics), nil
//	}
//
//	objList, err := c.DynamicClient.Resource(Resources[PodMetricsResource]).List(context.Background(), metav1.ListOptions{})
//	if err != nil {
//		return nil, err
//	}
//
//	for _, obj := range objList.Items {
//		if obj.GetName() == podName {
//			metrics := new(metricsV1beta1.PodMetrics)
//			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &metrics); err != nil {
//				return nil, err
//			}
//			return metrics, nil
//		}
//	}
//	return new(metricsV1beta1.PodMetrics), nil
//}
