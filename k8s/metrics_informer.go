package k8s

import (
	"context"
	time "time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

type NodeMetricsInformer struct {
	client   metricsclient.Interface
	informer cache.SharedIndexInformer
	lister   *NodeMetricsLister
}

func NewNodeMetricsInformer(client metricsclient.Interface, resyncPeriod time.Duration) *NodeMetricsInformer {
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.MetricsV1beta1().NodeMetricses().List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {

				return client.MetricsV1beta1().NodeMetricses().Watch(context.TODO(), options)
			},
		},
		&metricsV1beta1.NodeMetrics{},
		resyncPeriod,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	return &NodeMetricsInformer{client: client, informer: informer}
}

func (f *NodeMetricsInformer) Informer() cache.SharedIndexInformer {
	return f.informer
}

func (f *NodeMetricsInformer) Lister() *NodeMetricsLister {
	if f.lister != nil {
		return f.lister
	}
	f.lister = NewNodeMetricsLister(f.informer.GetIndexer())
	return f.lister
}

type PodMetricsInformer struct {
	client   metricsclient.Interface
	informer cache.SharedIndexInformer
	lister   *PodMetricsLister
}

func NewPodMetricsInformer(client metricsclient.Interface, resyncPeriod time.Duration, namespace string) *PodMetricsInformer {
	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return client.MetricsV1beta1().PodMetricses(namespace).List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {

				return client.MetricsV1beta1().PodMetricses(namespace).Watch(context.TODO(), options)
			},
		},
		&metricsV1beta1.PodMetrics{},
		resyncPeriod,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)
	return &PodMetricsInformer{client: client, informer: informer}
}

func (f *PodMetricsInformer) Informer() cache.SharedIndexInformer {
	return f.informer
}

func (f *PodMetricsInformer) Lister() *PodMetricsLister {
	if f.lister != nil {
		return f.lister
	}
	f.lister = NewPodMetricsLister(f.informer.GetIndexer())
	return f.lister
}
