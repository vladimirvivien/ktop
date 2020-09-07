package k8s

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	NodeMetricsResource = "nodemetrics"
	PodMetricsResource  = "podmetrics"
	DeploymentsResource = "deployments"
	NodesResource       = "nodes"
	PodsResource        = "pods"
	DaemonSetsResource  = "daemonsets"
	ReplicaSetsResource = "replicasets"

	Resources = map[string]schema.GroupVersionResource{
		NodesResource:       schema.GroupVersionResource{Group: "", Version: "v1", Resource: NodesResource},
		PodsResource:        schema.GroupVersionResource{Group: "", Version: "v1", Resource: PodsResource},
		DeploymentsResource: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: DeploymentsResource},
		NodeMetricsResource: schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"},
		PodMetricsResource:  schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"},
		DaemonSetsResource:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: DaemonSetsResource},
		ReplicaSetsResource: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: ReplicaSetsResource},
	}
)

type Interface interface {
	Start() error
	Namespace() string
	AssertMetricsAvailable() error
}

type k8sClient struct {
	ctx              context.Context
	namespace        string
	manager          ctrl.Manager
	discoClient      *discovery.DiscoveryClient
	metricsClient *metricsclient.Clientset
	metricsAvailable bool
}

func New(ctx context.Context, namespace string) (Interface, error) {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:    runtime.NewScheme(),
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

	client := &k8sClient{
		ctx:         ctx,
		namespace:   namespace,
		manager:     mgr,
		discoClient: disco,
		metricsClient: metrics,
	}

	if client.AssertMetricsAvailable()  != nil {
		return nil, fmt.Errorf("metrics not available: %s", err)
	}

	return client, nil
}

func (k8s *k8sClient) Start() error {
	return k8s.manager.Start(ctrl.SetupSignalHandler())
}

func (k8s *k8sClient) Namespace() string {
	return k8s.namespace
}

func (k8s *k8sClient) AssertMetricsAvailable() error {
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

	k8s.metricsAvailable =  avail
	if !avail {
		return fmt.Errorf("metrics api not available")
	}
	return nil
}

// GetMetricsByNode returns metrics for specified node
func (k8s *k8sClient) GetNodeMetrics(nodeName string) (*metricsV1beta1.NodeMetrics, error) {
	if !k8s.metricsAvailable {
		return nil, fmt.Errorf("metrics api not available")
	}

	metrics, err := k8s.metricsClient.MetricsV1beta1().NodeMetricses().Get(k8s.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return metrics, nil
}

// ********************************************************************************************************
type Client struct {
	Namespace       string
	DynamicClient   dynamic.Interface
	InformerFactory dynamicinformer.DynamicSharedInformerFactory
	Config          *restclient.Config

	MetricsAreAvailable bool
}

func NewClient(kubeconfig string, namespace string) (*Client, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	dynclient := dynamic.NewForConfigOrDie(config)
	discoClient := discovery.NewDiscoveryClientForConfigOrDie(config)
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynclient, time.Second*3, namespace, nil)
	k8sClient := &Client{
		Namespace:       namespace,
		DynamicClient:   dynclient,
		Config:          config,
		InformerFactory: factory,
	}
	k8sClient.MetricsAreAvailable = areMetricsAvail(discoClient)
	return k8sClient, nil
}

func (c *Client) Start(stopCh <-chan struct{}) {
	if c.InformerFactory == nil {
		panic("Failed to start Client, nil InformerFactory")
	}

	for name, res := range Resources {
		if synced := c.InformerFactory.WaitForCacheSync(stopCh); !synced[res] {
			panic(fmt.Sprintf("Informer for %s did not sync", name))
		}
	}
}

func areMetricsAvail(disco *discovery.DiscoveryClient) bool {
	groups, err := disco.ServerGroups()
	if err != nil {
		return false
	}

	for _, group := range groups.Groups {
		if group.Name == metricsapi.GroupName {
			return true
		}
	}
	return false
}

// GetMetricsByNode returns metrics for specified node
func (c *Client) GetMetricsByNode(nodeName string) (*metricsV1beta1.NodeMetrics, error) {
	// TODO unfortunately, nodemetric types are not watchable (without applying RBAC rules)
	// for now, the code just does a simple list every time metrics are needed

	if !c.MetricsAreAvailable {
		return new(metricsV1beta1.NodeMetrics), nil
	}

	objList, err := c.DynamicClient.Resource(Resources[NodeMetricsResource]).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, obj := range objList.Items {
		if obj.GetName() == nodeName {
			metrics := new(metricsV1beta1.NodeMetrics)
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &metrics); err != nil {
				return nil, err
			}
			return metrics, nil
		}
	}
	return new(metricsV1beta1.NodeMetrics), nil
}

// GetMetricsByPod returns metrics for specified pod
func (c *Client) GetMetricsByPod(podName string) (*metricsV1beta1.PodMetrics, error) {
	// TODO unfortunately, podmetric types are not watchable (without applying RBAC rules)
	// for now, the code just does a simple list every time metrics are needed

	if !c.MetricsAreAvailable {
		return new(metricsV1beta1.PodMetrics), nil
	}

	objList, err := c.DynamicClient.Resource(Resources[PodMetricsResource]).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, obj := range objList.Items {
		if obj.GetName() == podName {
			metrics := new(metricsV1beta1.PodMetrics)
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &metrics); err != nil {
				return nil, err
			}
			return metrics, nil
		}
	}
	return new(metricsV1beta1.PodMetrics), nil
}
