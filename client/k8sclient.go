package client

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/informers"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	metricsapi "k8s.io/metrics/pkg/apis/metrics"
	metricsclientset "k8s.io/metrics/pkg/client/clientset/versioned"
)

var (
	DeploymentsResource = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	NodesResource       = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
)

type K8sClient struct {
	Namespace     string
	Clientset     kubernetes.Interface
	Config        *restclient.Config
	ServerVersion *version.Info

	InformerFactory    informers.SharedInformerFactory
	NodeInformer       coreinformers.NodeInformer
	PodInformer        coreinformers.PodInformer
	DeploymentInformer appsinformers.DeploymentInformer
	DaemonSetInformer  appsinformers.DaemonSetInformer
	ReplicaSetInformer appsinformers.ReplicaSetInformer

	MetricsAPIAvailable bool
	MetricsClient       metricsclientset.Interface
}

func New(kubeconfig, namespace string, resyncPeriod time.Duration) (*K8sClient, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	svrver, err := clientset.ServerVersion()
	if err != nil {
		return nil, err
	}

	factory := informers.NewFilteredSharedInformerFactory(clientset, time.Second*3, namespace, nil)

	client := &K8sClient{
		Namespace:           namespace,
		Clientset:           clientset,
		Config:              config,
		ServerVersion:       svrver,
		InformerFactory:     factory,
		NodeInformer:        factory.Core().V1().Nodes(),
		PodInformer:         factory.Core().V1().Pods(),
		DeploymentInformer:  factory.Apps().V1().Deployments(),
		DaemonSetInformer:   factory.Apps().V1().DaemonSets(),
		ReplicaSetInformer:  factory.Apps().V1().ReplicaSets(),
		MetricsAPIAvailable: isMetricAPIAvail(clientset.Discovery()),
	}

	if client.MetricsAPIAvailable {
		client.MetricsClient, err = metricsclientset.NewForConfig(config)
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}

func isMetricAPIAvail(disco discovery.DiscoveryInterface) bool {
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
