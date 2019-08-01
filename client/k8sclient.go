package client

import (
	"fmt"
	"time"

	"k8s.io/client-go/dynamic/dynamicinformer"

	"k8s.io/client-go/dynamic"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
)

var (
	NodeMetricsResource = "nodemetrics"
	PodMetricsResource  = "podmetrics"
	DeploymentsResource = "deployments"
	NodesResource       = "nodes"

	Resources = map[string]schema.GroupVersionResource{
		NodesResource:       schema.GroupVersionResource{Group: "", Version: "v1", Resource: NodesResource},
		DeploymentsResource: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: DeploymentsResource},
		NodeMetricsResource: schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: NodeMetricsResource},
		PodMetricsResource:  schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: PodMetricsResource},
	}
)

type K8sClient struct {
	Namespace       string
	DynamicClient   dynamic.Interface
	InformerFactory dynamicinformer.DynamicSharedInformerFactory
	Config          *restclient.Config

	MetricsAreAvailable bool
}

func New(kubeconfig string, namespace string) (*K8sClient, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	dynclient := dynamic.NewForConfigOrDie(config)
	discoClient := discovery.NewDiscoveryClientForConfigOrDie(config)
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynclient, time.Second*3, namespace, nil)
	k8sClient := &K8sClient{
		Namespace:       namespace,
		DynamicClient:   dynclient,
		Config:          config,
		InformerFactory: factory,
	}
	k8sClient.MetricsAreAvailable = areMetricsAvail(discoClient)
	return k8sClient, nil
}

func (c *K8sClient) Start(stopCh <-chan struct{}) {
	if c.InformerFactory == nil {
		panic("Failed to start K8sClient, nil InformerFactory")
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
