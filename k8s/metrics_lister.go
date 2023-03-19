package k8s

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type NodeMetricsLister struct {
	indexer cache.Indexer
}

func NewNodeMetricsLister(indexer cache.Indexer) *NodeMetricsLister {
	return &NodeMetricsLister{indexer: indexer}
}

func (s *NodeMetricsLister) List(selector labels.Selector) (ret []*metricsV1beta1.NodeMetrics, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*metricsV1beta1.NodeMetrics))
	})
	return ret, err
}

func (s *NodeMetricsLister) Get(name string) (*metricsV1beta1.NodeMetrics, error) {
	obj, exists, err := s.indexer.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("nodemetrics"), name)
	}
	return obj.(*metricsV1beta1.NodeMetrics), nil
}

type PodMetricsLister struct {
	indexer cache.Indexer
}

func NewPodMetricsLister(indexer cache.Indexer) *PodMetricsLister {
	return &PodMetricsLister{indexer: indexer}
}

func (s *PodMetricsLister) List(selector labels.Selector) (ret []*metricsV1beta1.PodMetrics, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*metricsV1beta1.PodMetrics))
	})
	return ret, err
}

func (s *PodMetricsLister) Get(name string) (*metricsV1beta1.PodMetrics, error) {
	obj, exists, err := s.indexer.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("podmetrics"), name)
	}
	return obj.(*metricsV1beta1.PodMetrics), nil
}
