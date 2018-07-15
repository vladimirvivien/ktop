package controllers

import (
	"fmt"

	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/vladimirvivien/ktop/client"
	"github.com/vladimirvivien/ktop/ui"
)

type Overview struct {
	k8s        *client.K8sClient
	nodeLister corelisters.NodeLister
	nodeSynced cache.InformerSynced
	ui         *ui.OverviewPage
}

func NewOverview(
	k8s *client.K8sClient,
	ui *ui.OverviewPage,
) *Overview {
	ctrl := &Overview{k8s: k8s, ui: ui}
	// setup node callbacks
	nodeInformer := k8s.InformerFactory.Core().V1().Nodes()
	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.addNode,
		UpdateFunc: func(old, new interface{}) {
			newNode := new.(*coreV1.Node)
			oldNode := old.(*coreV1.Node)
			if newNode.ResourceVersion == oldNode.ResourceVersion {
				return
			}
			ctrl.updateNode(new)
		},
		DeleteFunc: ctrl.deleteNode,
	})

	ctrl.nodeLister = nodeInformer.Lister()
	ctrl.nodeSynced = nodeInformer.Informer().HasSynced

	return ctrl
}

func (c *Overview) addNode(obj interface{}) {

}

func (c *Overview) updateNode(obj interface{}) {

}

func (c *Overview) deleteNode(obj interface{}) {

}

func (c *Overview) initList() error {
	// initial Node list
	nodes, err := c.k8s.Clientset.CoreV1().Nodes().List(metaV1.ListOptions{})
	if err != nil {
		return err
	}

	var nodeMetrics *metricsV1beta1.NodeMetricsList
	if c.k8s.MetricsAPIAvailable {
		nodeMetrics, err = c.k8s.MetricsClient.Metrics().NodeMetricses().List(metaV1.ListOptions{})
	}

	c.syncNodes(nodes.Items, nodeMetrics.Items)

	// initial Pod list
	pods, err := c.k8s.Clientset.CoreV1().Pods(c.k8s.Namespace).List(metaV1.ListOptions{})
	if err != nil {
		return err
	}

	var podMetrics *metricsV1beta1.PodMetricsList
	if c.k8s.MetricsAPIAvailable {
		podMetrics, err = c.k8s.MetricsClient.Metrics().PodMetricses(c.k8s.Namespace).List(metaV1.ListOptions{})
	}

	c.syncPods(pods.Items, podMetrics.Items)

	return nil
}

func (c *Overview) Run(stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	c.ui.DrawHeader(c.k8s.Config.Host, c.k8s.Namespace)
	if err := c.initList(); err != nil {
		return err
	}

	if ok := cache.WaitForCacheSync(stopCh, c.nodeSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	<-stopCh
	return nil
}

func (c *Overview) syncNodes(nodes []coreV1.Node, metrics []metricsV1beta1.NodeMetrics) {

	nodeListRows := make([]ui.NodeRow, len(nodes))
	for i, node := range nodes {
		conds := node.Status.Conditions
		nodeMetrics := getMetricsByNodeName(metrics, node.Name)
		row := ui.NodeRow{
			Name:     node.Name,
			Role:     nodeRole(node),
			Status:   string(conds[len(conds)-1].Type),
			Version:  node.Status.NodeInfo.KubeletVersion,
			CPUUsage: nodeMetrics.Usage.Cpu().String(),
			MemUsage: nodeMetrics.Usage.Memory().String(),
		}
		nodeListRows[i] = row
	}
	c.ui.DrawNodeList(nodeListRows)
}

func isNodeMaster(node coreV1.Node) bool {
	_, ok := node.Labels["node-role.kubernetes.io/master"]
	return ok
}

func nodeRole(node coreV1.Node) string {
	if isNodeMaster(node) {
		return "Master"
	}
	return "Node"
}

func getMetricsByNodeName(metrics []metricsV1beta1.NodeMetrics, nodeName string) metricsV1beta1.NodeMetrics {
	for _, metric := range metrics {
		if metric.Name == nodeName {
			return metric
		}
	}
	return metricsV1beta1.NodeMetrics{}
}

func (c *Overview) syncPods(pods []coreV1.Pod, metrics []metricsV1beta1.PodMetrics) {

	podListRows := make([]ui.PodRow, len(pods))
	for i, pod := range pods {
		conds := pod.Status.Conditions
		podMetrics := getMetricsByPodName(metrics, pod.Name)
		totalCpu, totalMem := getPodMetricsTotal(podMetrics)
		row := ui.PodRow{
			Name:     pod.Name,
			Status:   string(conds[len(conds)-1].Type),
			Ready:    "",
			CPUUsage: totalCpu.String(),
			MemUsage: totalMem.String(),
		}
		podListRows[i] = row
	}
	c.ui.DrawPodList(podListRows)
}

func getMetricsByPodName(metrics []metricsV1beta1.PodMetrics, podName string) metricsV1beta1.PodMetrics {
	for _, metric := range metrics {
		if metric.Name == podName {
			return metric
		}
	}
	return metricsV1beta1.PodMetrics{}
}

func getPodMetricsTotal(metrics metricsV1beta1.PodMetrics) (totalCpu, totalMem resource.Quantity) {
	containers := metrics.Containers
	for _, c := range containers {
		totalCpu.Add(*c.Usage.Cpu())
		totalMem.Add(*c.Usage.Memory())
	}
	return
}
