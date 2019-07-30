package overview

import (
	"fmt"
	"time"

	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/vladimirvivien/ktop/client"
	"github.com/vladimirvivien/ktop/ui"
)

// overviewController type represents a view controller for displaying
// overview screen.  The controller follows a simple approach of using
// internal informers and theirn event handlers to react and update screen
// elemements.
type overviewController struct {
	k8s *client.K8sClient
	app *ui.Application

	nodeLister corelisters.NodeLister
	nodeSynced cache.InformerSynced

	podLister corelisters.PodLister
	podSynced cache.InformerSynced

	depLister appslisters.DeploymentLister
	depSynced cache.InformerSynced

	dsLister appslisters.DaemonSetLister
	dsSynced cache.InformerSynced

	rsLister appslisters.ReplicaSetLister
	rsSynced cache.InformerSynced

	page *overviewPage
}

// New creates a new overviewController. It sets up informers and listers
// that are used to retrieve updated resource values and display them.
func New(
	k8s *client.K8sClient,
	app *ui.Application,
	pgTitle string,
) *overviewController {
	ctrl := &overviewController{k8s: k8s, app: app}
	ctrl.page = newPage()
	ctrl.app.AddPage(pgTitle, ctrl.page.root)

	// setup node informer and eventHandlers to update screen
	ctrl.nodeLister = k8s.NodeInformer.Lister()
	ctrl.nodeSynced = k8s.NodeInformer.Informer().HasSynced
	k8s.NodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
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

	// setup pod informer and eventHandlers to update screen
	ctrl.podLister = k8s.PodInformer.Lister()
	ctrl.podSynced = k8s.PodInformer.Informer().HasSynced
	k8s.PodInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.updatePodList,
		UpdateFunc: func(old, new interface{}) {
			newPod := new.(*coreV1.Pod)
			oldPod := old.(*coreV1.Pod)
			if newPod.ResourceVersion == oldPod.ResourceVersion {
				return
			}
			ctrl.updatePodList(new)
		},
		DeleteFunc: ctrl.updatePodList,
	})

	// setup deployment informer and eventHandlers to update screen
	ctrl.depLister = k8s.DeploymentInformer.Lister()
	ctrl.depSynced = k8s.DeploymentInformer.Informer().HasSynced
	k8s.DeploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
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

	// setup DaemonSet informer and eventHandlers to update screen
	ctrl.dsLister = k8s.DaemonSetInformer.Lister()
	ctrl.dsSynced = k8s.DaemonSetInformer.Informer().HasSynced
	k8s.DaemonSetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
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

	// setup ReplicaSet informer and eventHandlers to update screen
	ctrl.rsLister = k8s.ReplicaSetInformer.Lister()
	ctrl.rsSynced = k8s.ReplicaSetInformer.Informer().HasSynced
	k8s.ReplicaSetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
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

// Run starts the controller.  It initialize screen elements
// and waits for informers to sycn.
func (c *overviewController) Run(stopCh <-chan struct{}) error {
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

// runMetricsUpdate setups a ticker to intermittently update
// the metrics in the node and pod list.  This is necessary because
// the metric types do not have a watchers or informers.
// In an active cluster, this will be a bit noisy. However, the best
// TODO figure a way to create metrics informer (pull request or otherwise).
func (c *overviewController) runMetricsUpdates(done <-chan struct{}) error {
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

func (c *overviewController) updateNodeList(obj interface{}) {
	c.syncNodeList()
}

func (c *overviewController) updatePodList(obj interface{}) {
	c.syncPodList()
}

func (c *overviewController) updateDeps(obj interface{}) {
	c.syncWorkload()
}

func (c *overviewController) updateDaemonSets(obj interface{}) {
	c.syncWorkload()
}

func (c *overviewController) updateReplicaSets(obj interface{}) {
	c.syncWorkload()
}

// initScreen initializes screen elements
func (c *overviewController) initScreen() error {
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

// syncNodeList fetches data from node lister.  For each fetched item
// it then retrieves associated NodeMetrics values (if available).
// This method is called by node informer events (or metrics refresh event)
func (c *overviewController) syncNodeList() error {
	nodeList, err := c.nodeLister.List(labels.Everything())
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

	// collect node and metrics info in nodeRow type
	// used for display.
	nodeListRows := make([]nodeRow, len(nodes))
	for i, node := range nodes {
		conds := node.Status.Conditions
		availRes := node.Status.Allocatable
		metrics := getMetricsByNodeName(nodeMetrics, node.Name)
		row := nodeRow{
			name:          node.Name,
			role:          nodeRole(node),
			status:        string(conds[len(conds)-1].Type),
			version:       node.Status.NodeInfo.KubeletVersion,
			cpuAvail:      availRes.Cpu().String(),
			cpuAvailValue: availRes.Cpu().MilliValue(),
			cpuUsage:      metrics.Usage.Cpu().String(),
			cpuValue:      metrics.Usage.Cpu().MilliValue(),
			memAvail:      availRes.Memory().String(),
			memAvailValue: availRes.Memory().MilliValue(),
			memUsage:      metrics.Usage.Memory().String(),
			memValue:      metrics.Usage.Memory().MilliValue(),
		}
		nodeListRows[i] = row
	}
	c.page.drawNodeList(0, nodeListRows)
	c.app.Refresh()

	return nil
}

// syncPodList fetches data from pod lister.  For each fetched item
// it then retrieves associated NodeMetrics values (if available).
// This method is called by pod informer events (or metrics refresh event)
func (c *overviewController) syncPodList() error {
	// get pod list
	podList, err := c.podLister.List(labels.Everything())
	if err != nil {
		return err
	}
	pods := convertPodsPtr(podList)

	// get pod metrics and associated node metrics
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

	// transfer pod and metric data to type podRow for display
	podListRows := make([]podRow, len(pods))
	for i, pod := range pods {
		podMetrics := getMetricsByPodName(podMetricsItems, pod.Name)
		totalCpu, totalMem := getPodMetricsTotal(podMetrics)
		nodeMetrics := getMetricsByNodeName(nodeMetricsItems, pod.Spec.NodeName)
		row := podRow{
			name:         pod.Name,
			status:       string(pod.Status.Phase),
			ip:           pod.Status.PodIP,
			node:         pod.Spec.NodeName,
			volumes:      len(pod.Spec.Volumes),
			nodeCPUValue: nodeMetrics.Usage.Cpu().MilliValue(),
			nodeMemValue: nodeMetrics.Usage.Memory().MilliValue(),
			podCPUValue:  totalCpu.MilliValue(),
			podMemValue:  totalMem.MilliValue(),
		}
		podListRows[i] = row
	}
	c.page.drawPodList(0, podListRows)
	c.app.Refresh()

	return nil
}

// syncWorkload fetches summarial data from multiple sources including deployments,
// daemonsets, and replicasets.  This method is called by its respective eventhandlers
// when associated informers have data.
func (c *overviewController) syncWorkload() error {
	summary := workloadSummary{}

	deps, err := c.depLister.List(labels.Everything())
	if err != nil {
		return err
	}
	summary.deploymentsTotal, summary.deploymentsReady = getDeploymentSummary(deps)

	daemonSets, err := c.dsLister.List(labels.Everything())
	if err != nil {
		return err
	}
	summary.daemonSetsTotal, summary.daemonSetsReady = getDaemonSetSummary(daemonSets)

	reps, err := c.rsLister.List(labels.Everything())
	if err != nil {
		return err
	}
	summary.replicaSetsTotal, summary.replicaSetsReady = getReplicaSetSummary(reps)

	pods, err := c.podLister.List(labels.Everything())
	if err != nil {
		return err
	}
	summary.podsTotal, summary.podsReady = getPodSummary(pods)

	c.page.drawWorkloadGrid(summary)
	c.app.Refresh()

	return nil
}

func convertPodsPtr(nodes []*coreV1.Pod) (out []coreV1.Pod) {
	for _, ptr := range nodes {
		out = append(out, *ptr)
	}
	return
}

// func isNodeMaster(node coreV1.Node) bool {
// 	_, ok := node.Labels["node-role.kubernetes.io/master"]
// 	return ok
// }

// func nodeRole(node coreV1.Node) string {
// 	if isNodeMaster(node) {
// 		return "Master"
// 	}
// 	return "Node"
// }

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
