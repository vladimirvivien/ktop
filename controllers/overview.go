package controllers

import (
	"fmt"
	"time"

	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/vladimirvivien/ktop/client"
	"github.com/vladimirvivien/ktop/ui"
)

type Overview struct {
	k8s *client.K8sClient

	nodeInformer coreinformers.NodeInformer
	nodeLister   corelisters.NodeLister
	nodeSynced   cache.InformerSynced

	podInformer coreinformers.PodInformer
	podLister   corelisters.PodLister
	podSynced   cache.InformerSynced

	depInformer appsinformers.DeploymentInformer
	depLister   appslisters.DeploymentLister
	depSynced   cache.InformerSynced

	dsInformer appsinformers.DaemonSetInformer
	dsLister   appslisters.DaemonSetLister
	dsSynced   cache.InformerSynced

	rsInformer appsinformers.ReplicaSetInformer
	rsLister   appslisters.ReplicaSetLister
	rsSynced   cache.InformerSynced

	ui *ui.OverviewPage
}

func NewOverview(
	k8s *client.K8sClient,
	ui *ui.OverviewPage,
) *Overview {
	ctrl := &Overview{k8s: k8s, ui: ui}

	// setup node informer
	ctrl.nodeInformer = k8s.InformerFactory.Core().V1().Nodes()
	ctrl.nodeLister = ctrl.nodeInformer.Lister()
	ctrl.nodeSynced = ctrl.nodeInformer.Informer().HasSynced

	ctrl.nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.updateNodeList,
		UpdateFunc: func(old, new interface{}) {
			newNode := new.(*coreV1.Node)
			oldNode := old.(*coreV1.Node)
			if newNode.ResourceVersion == oldNode.ResourceVersion {
				return
			}
			ctrl.updateNodeList(new)
		},
		DeleteFunc: ctrl.updateNodeList,
	})

	// setup pod informer
	ctrl.podInformer = k8s.InformerFactory.Core().V1().Pods()
	ctrl.podLister = ctrl.podInformer.Lister()
	ctrl.podSynced = ctrl.podInformer.Informer().HasSynced

	ctrl.podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.updatePodList,
		UpdateFunc: func(old, new interface{}) {
			newPod := new.(*coreV1.Pod)
			oldPod := old.(*coreV1.Pod)
			if newPod.ResourceVersion == oldPod.ResourceVersion {
				return
			}
			ctrl.updatePodList(new)
		},
		DeleteFunc: ctrl.updateNodeList,
	})

	// setup deployment informer
	ctrl.depInformer = k8s.InformerFactory.Apps().V1().Deployments()
	ctrl.depLister = ctrl.depInformer.Lister()
	ctrl.depSynced = ctrl.depInformer.Informer().HasSynced

	ctrl.depInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.updateDeps,
		UpdateFunc: func(old, new interface{}) {
			newPod := new.(*appsV1.Deployment)
			oldPod := old.(*appsV1.Deployment)
			if newPod.ResourceVersion == oldPod.ResourceVersion {
				return
			}
			ctrl.updateDeps(new)
		},
		DeleteFunc: ctrl.updateDeps,
	})

	ctrl.dsInformer = k8s.InformerFactory.Apps().V1().DaemonSets()
	ctrl.dsLister = ctrl.dsInformer.Lister()
	ctrl.dsSynced = ctrl.dsInformer.Informer().HasSynced

	ctrl.dsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.updateDaemonSets,
		UpdateFunc: func(old, new interface{}) {
			newPod := new.(*appsV1.DaemonSet)
			oldPod := old.(*appsV1.DaemonSet)
			if newPod.ResourceVersion == oldPod.ResourceVersion {
				return
			}
			ctrl.updateDaemonSets(new)
		},
		DeleteFunc: ctrl.updateDaemonSets,
	})

	ctrl.rsInformer = k8s.InformerFactory.Apps().V1().ReplicaSets()
	ctrl.rsLister = ctrl.rsInformer.Lister()
	ctrl.rsSynced = ctrl.rsInformer.Informer().HasSynced

	ctrl.rsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.updateReplicaSets,
		UpdateFunc: func(old, new interface{}) {
			newPod := new.(*appsV1.ReplicaSet)
			oldPod := old.(*appsV1.ReplicaSet)
			if newPod.ResourceVersion == oldPod.ResourceVersion {
				return
			}
			ctrl.updateReplicaSets(new)
		},
		DeleteFunc: ctrl.updateReplicaSets,
	})

	return ctrl
}

func (c *Overview) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if err := c.initScreen(); err != nil {
		return err
	}

	if ok := cache.WaitForCacheSync(stopCh, c.nodeSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	go c.runMetricsUpdates(stopCh)

	<-stopCh
	return nil
}

func (c *Overview) runMetricsUpdates(done <-chan struct{}) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return nil
		case <-ticker.C:
			if err := c.syncNodeList(); err != nil {
				return err
			}
			if err := c.syncPodList(); err != nil {
				return err
			}
		}
	}
}

func (c *Overview) updateNodeList(obj interface{}) {
	c.syncNodeList()
}

func (c *Overview) updatePodList(obj interface{}) {
	c.syncPodList()
}

func (c *Overview) updateDeps(obj interface{}) {
	c.syncWorkload()
}

func (c *Overview) updateDaemonSets(obj interface{}) {
	c.syncWorkload()
}

func (c *Overview) updateReplicaSets(obj interface{}) {
	c.syncWorkload()
}

func (c *Overview) initScreen() error {
	c.ui.DrawHeader(c.k8s.Config.Host, c.k8s.Namespace)

	if err := c.syncNodeList(); err != nil {
		return err
	}

	if err := c.syncPodList(); err != nil {
		return err
	}

	return nil
}

func convertNodesPtr(nodes []*coreV1.Node) (out []coreV1.Node) {
	for _, ptr := range nodes {
		out = append(out, *ptr)
	}
	return
}

func (c *Overview) syncNodeList() error {
	nodeList, err := c.nodeInformer.Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	nodes := convertNodesPtr(nodeList)

	var nodeMetrics []metricsV1beta1.NodeMetrics
	if c.k8s.MetricsAPIAvailable {
		nodeMetricsList, err := c.k8s.MetricsClient.Metrics().NodeMetricses().List(metaV1.ListOptions{})
		if err != nil {
			return err
		}
		nodeMetrics = nodeMetricsList.Items
	}

	nodeListRows := make([]ui.NodeRow, len(nodes))
	for i, node := range nodes {
		conds := node.Status.Conditions
		availRes := node.Status.Allocatable
		metrics := getMetricsByNodeName(nodeMetrics, node.Name)
		row := ui.NodeRow{
			Name:          node.Name,
			Role:          nodeRole(node),
			Status:        string(conds[len(conds)-1].Type),
			Version:       node.Status.NodeInfo.KubeletVersion,
			CPUAvail:      availRes.Cpu().String(),
			CPUAvailValue: availRes.Cpu().MilliValue(),
			CPUUsage:      metrics.Usage.Cpu().String(),
			CPUValue:      metrics.Usage.Cpu().MilliValue(),
			MemAvail:      availRes.Memory().String(),
			MemAvailValue: availRes.Memory().MilliValue(),
			MemUsage:      metrics.Usage.Memory().String(),
			MemValue:      metrics.Usage.Memory().MilliValue(),
		}
		nodeListRows[i] = row
	}
	c.ui.DrawNodeList(0, nodeListRows)

	return nil
}

func (c *Overview) syncPodList() error {
	// get pod list
	podList, err := c.podInformer.Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	pods := convertPodsPtr(podList)

	// get pod metrics
	var podMetricsItems []metricsV1beta1.PodMetrics
	var nodeMetricsItems []metricsV1beta1.NodeMetrics
	if c.k8s.MetricsAPIAvailable {
		podMetrics, err := c.k8s.MetricsClient.Metrics().PodMetricses(c.k8s.Namespace).List(metaV1.ListOptions{})
		if err != nil {
			return err
		}
		podMetricsItems = podMetrics.Items

		nodeMetrics, err := c.k8s.MetricsClient.Metrics().NodeMetricses().List(metaV1.ListOptions{})
		if err != nil {
			return err
		}
		nodeMetricsItems = nodeMetrics.Items

	}

	podListRows := make([]ui.PodRow, len(pods))
	for i, pod := range pods {
		podMetrics := getMetricsByPodName(podMetricsItems, pod.Name)
		totalCpu, totalMem := getPodMetricsTotal(podMetrics)
		nodeMetrics := getMetricsByNodeName(nodeMetricsItems, pod.Spec.NodeName)
		row := ui.PodRow{
			Name:         pod.Name,
			Status:       string(pod.Status.Phase),
			IP:           pod.Status.PodIP,
			Node:         pod.Spec.NodeName,
			Volumes:      len(pod.Spec.Volumes),
			NodeCPUValue: nodeMetrics.Usage.Cpu().MilliValue(),
			NodeMemValue: nodeMetrics.Usage.Memory().MilliValue(),
			PodCPUValue:  totalCpu.MilliValue(),
			PodMemValue:  totalMem.MilliValue(),
		}
		podListRows[i] = row
	}
	c.ui.DrawPodList(0, podListRows)

	return nil
}

func (c *Overview) syncWorkload() error {
	summary := ui.WorkloadSummary{}

	deps, err := c.depLister.List(labels.Everything())
	if err != nil {
		return err
	}
	summary.DeploymentsTotal, summary.DeploymentsReady = getDeploymentSummary(deps)

	daemonSets, err := c.dsLister.List(labels.Everything())
	if err != nil {
		return err
	}
	summary.DaemonSetsTotal, summary.DaemonSetsReady = getDaemonSetSummary(daemonSets)

	reps, err := c.rsLister.List(labels.Everything())
	if err != nil {
		return err
	}
	summary.ReplicaSetsTotal, summary.ReplicaSetsReady = getReplicaSetSummary(reps)

	pods, err := c.podLister.List(labels.Everything())
	if err != nil {
		return err
	}
	summary.PodsTotal, summary.PodsReady = getPodSummary(pods)

	c.ui.DrawWorkloadGrid(summary)

	return nil
}

func convertPodsPtr(nodes []*coreV1.Pod) (out []coreV1.Pod) {
	for _, ptr := range nodes {
		out = append(out, *ptr)
	}
	return
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

func isPodRunning(pod coreV1.Pod) bool {
	return pod.Status.Phase == coreV1.PodRunning
}

func getMetricsByNodeName(metrics []metricsV1beta1.NodeMetrics, nodeName string) metricsV1beta1.NodeMetrics {
	for _, metric := range metrics {
		if metric.Name == nodeName {
			return metric
		}
	}
	return metricsV1beta1.NodeMetrics{}
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

func getDeploymentSummary(deployments []*appsV1.Deployment) (desired, ready int) {
	for _, deploy := range deployments {
		desired += int(deploy.Status.Replicas)
		ready += int(deploy.Status.ReadyReplicas)
	}
	return
}

func getDaemonSetSummary(dmnSets []*appsV1.DaemonSet) (desired, ready int) {
	for _, ds := range dmnSets {
		desired += int(ds.Status.DesiredNumberScheduled)
		ready += int(ds.Status.NumberReady)
	}
	return
}

func getReplicaSetSummary(repSets []*appsV1.ReplicaSet) (desired, ready int) {
	for _, rs := range repSets {
		desired += int(rs.Status.Replicas)
		ready += int(rs.Status.ReadyReplicas)
	}
	return
}

func getPodSummary(pods []*coreV1.Pod) (desired, ready int) {
	desired = len(pods)
	for _, pod := range pods {
		if isPodRunning(*pod) {
			ready++
		}
	}
	return
}
