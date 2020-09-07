package overview

import (
	"fmt"

	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/ui"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type OverviewController struct {
	podInformer   *k8s.InformerAdapter
	nodeInformer  *k8s.InformerAdapter
	depInformer   *k8s.InformerAdapter
	dsInformer    *k8s.InformerAdapter
	rsInformer    *k8s.InformerAdapter
	k8sClient     *k8s.Client
	app           *application.Application
	nodePanel     ui.Panel
	podPanel      ui.Panel
	workloadPanel ui.Panel
}

func New(app *application.Application) *OverviewController {
	k8sClient := app.GetK8sClient()
	informerFac := k8sClient.InformerFactory
	ctrl := &OverviewController{
		nodeInformer: k8s.NewInformerAdapter(informerFac.ForResource(k8s.Resources[k8s.NodesResource])),
		podInformer:  k8s.NewInformerAdapter(informerFac.ForResource(k8s.Resources[k8s.PodsResource])),
		depInformer:  k8s.NewInformerAdapter(informerFac.ForResource(k8s.Resources[k8s.DeploymentsResource])),
		dsInformer:   k8s.NewInformerAdapter(informerFac.ForResource(k8s.Resources[k8s.DaemonSetsResource])),
		rsInformer:   k8s.NewInformerAdapter(informerFac.ForResource(k8s.Resources[k8s.ReplicaSetsResource])),
		app:          app,
		k8sClient:    k8sClient,
	}

	return ctrl
}

func (c *OverviewController) Run() {
	c.setupEventHandlers()
	c.setupViews()
}

func (c *OverviewController) setupViews() {
	c.nodePanel = NewNodePanel(fmt.Sprintf(" %c Nodes ", ui.Icons.Factory))
	c.nodePanel.Layout()
	c.nodePanel.DrawHeader("NAME", "STATUS", "ROLE", "VERSION", "CPU", "MEMORY")

	c.workloadPanel = NewWorkloadPanel(fmt.Sprintf(" %c Workload Health ", ui.Icons.Battery))
	c.workloadPanel.Layout()
	c.workloadPanel.DrawHeader()

	c.podPanel = NewPodPanel(fmt.Sprintf(" %c Pods ", ui.Icons.Package))
	c.podPanel.Layout()
	c.podPanel.DrawHeader("NAME", "STATUS", "IP", "NODE", "CPU", "MEMORY")

	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(c.nodePanel.GetView(), 7, 1, true).
		AddItem(c.workloadPanel.GetView(), 4, 1, true).
		AddItem(c.podPanel.GetView(), 0, 1, true)

	c.app.AddPage("Overview", page)
}

func (c *OverviewController) setupEventHandlers() {
	c.nodeInformer.SetAddObjectFunc(func(obj interface{}) {
		c.refreshNodes(obj)
	})
	c.nodeInformer.SetUpdateObjectFunc(func(old, new interface{}) {
		c.refreshNodes(new)
	})

	c.podInformer.SetAddObjectFunc(func(obj interface{}) {
		c.refreshPods(obj)
	})
	c.podInformer.SetUpdateObjectFunc(func(old, new interface{}) {
		c.refreshPods(new)
	})

	c.depInformer.SetAddObjectFunc(func(obj interface{}) {
		c.refreshWorkload()
	})
	c.depInformer.SetUpdateObjectFunc(func(old, new interface{}) {
		c.refreshWorkload()
	})

	c.dsInformer.SetAddObjectFunc(func(obj interface{}) {
		c.refreshWorkload()
	})
	c.dsInformer.SetUpdateObjectFunc(func(old, new interface{}) {
		c.refreshWorkload()
	})

	c.rsInformer.SetAddObjectFunc(func(obj interface{}) {
		c.refreshWorkload()
	})
	c.rsInformer.SetUpdateObjectFunc(func(old, new interface{}) {
		c.refreshWorkload()
	})
}

func (c *OverviewController) refreshNodes(obj interface{}) error {
	nodeObjects, err := c.nodeInformer.Lister().List(labels.Everything())
	if err != nil {
		return err
	}

	// collect node and metrics info in nodeRow type
	// used for display.
	nodeListRows := make([]NodeItem, len(nodeObjects))
	for i, obj := range nodeObjects {
		unstructNode := obj.(*unstructured.Unstructured)
		node := new(coreV1.Node)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructNode.Object, &node); err != nil {
			return err
		}
		metrics, err := c.k8sClient.GetMetricsByNode(node.Name)
		if err != nil {
			return err
		}

		conds := node.Status.Conditions
		availRes := node.Status.Allocatable
		row := NodeItem{
			Name:          node.Name,
			Role:          nodeRole(*node),
			Status:        string(conds[len(conds)-1].Type),
			Version:       node.Status.NodeInfo.KubeletVersion,
			CpuAvail:      availRes.Cpu().String(),
			CpuAvailValue: availRes.Cpu().MilliValue(),
			CpuUsage:      metrics.Usage.Cpu().String(),
			CpuValue:      metrics.Usage.Cpu().MilliValue(),
			MemAvail:      availRes.Memory().String(),
			MemAvailValue: availRes.Memory().MilliValue(),
			MemUsage:      metrics.Usage.Memory().String(),
			MemValue:      metrics.Usage.Memory().MilliValue(),
		}
		nodeListRows[i] = row
	}

	c.nodePanel.DrawBody(nodeListRows)
	c.app.Refresh()
	return nil
}

func (c *OverviewController) refreshPods(obj interface{}) error {
	podObjects, err := c.podInformer.Lister().List(labels.Everything())
	if err != nil {
		return err
	}

	podRows := make([]PodItem, len(podObjects))
	for i, obj := range podObjects {
		unstructPod := obj.(*unstructured.Unstructured)
		pod := new(coreV1.Pod)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructPod.Object, &pod); err != nil {
			return err
		}

		podMetrics, err := c.k8sClient.GetMetricsByPod(pod.Name)
		if err != nil {
			return err
		}

		nodeMetrics, err := c.k8sClient.GetMetricsByNode(pod.Spec.NodeName)
		if err != nil {
			return err
		}

		totalCpu, totalMem := podMetricsTotals(podMetrics)
		row := PodItem{
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
		podRows[i] = row
	}

	c.podPanel.DrawBody(podRows)
	c.app.Refresh()
	return nil
}

func (c *OverviewController) refreshWorkload() error {
	var summary WorkloadItem

	deps, err := c.depInformer.Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	summary.DeploymentsTotal, summary.DeploymentsReady, err = getDepsSummary(deps)
	if err != nil {
		return err
	}

	daemonSets, err := c.dsInformer.Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	summary.DaemonSetsTotal, summary.DaemonSetsReady, err = getDSSummary(daemonSets)
	if err != nil {
		return err
	}

	reps, err := c.rsInformer.Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	summary.ReplicaSetsTotal, summary.ReplicaSetsReady, err = getRSSummary(reps)
	if err != nil {
		return err
	}

	pods, err := c.podInformer.Lister().List(labels.Everything())
	if err != nil {
		return err
	}
	summary.PodsTotal, summary.PodsReady, err = getPodsSummary(pods)
	if err != nil {
		return err
	}

	c.workloadPanel.DrawBody(summary)
	c.app.Refresh()

	return nil
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

func getDepsSummary(depObjects []runtime.Object) (desired, ready int, err error) {
	for _, obj := range depObjects {
		unstructObj, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return 0, 0, fmt.Errorf("unexpected type %T", obj)
		}
		dep := new(appsV1.Deployment)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructObj.Object, &dep); err != nil {
			return 0, 0, err
		}
		desired += int(dep.Status.Replicas)
		ready += int(dep.Status.ReadyReplicas)
	}
	return
}

func getDSSummary(dsObjects []runtime.Object) (desired, ready int, err error) {
	for _, obj := range dsObjects {
		unstructObj, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return 0, 0, fmt.Errorf("unexpected type %T", obj)
		}
		ds := new(appsV1.DaemonSet)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructObj.Object, &ds); err != nil {
			return 0, 0, err
		}
		desired += int(ds.Status.DesiredNumberScheduled)
		ready += int(ds.Status.NumberReady)
	}
	return
}

func getRSSummary(rsObjects []runtime.Object) (desired, ready int, err error) {
	for _, obj := range rsObjects {
		unstructObj, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return 0, 0, fmt.Errorf("unexpected type %T", obj)
		}
		rs := new(appsV1.ReplicaSet)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructObj.Object, &rs); err != nil {
			return 0, 0, err
		}
		desired += int(rs.Status.Replicas)
		ready += int(rs.Status.ReadyReplicas)
	}
	return
}

func getPodsSummary(podObjects []runtime.Object) (desired, ready int, err error) {
	desired = len(podObjects)
	for _, obj := range podObjects {
		unstructObj, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return 0, 0, fmt.Errorf("unexpected type %T", obj)
		}
		pod := new(coreV1.Pod)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructObj.Object, &pod); err != nil {
			return 0, 0, err
		}
		if pod.Status.Phase == coreV1.PodRunning {
			ready++
		}
	}
	return
}

func podMetricsTotals(metrics *metricsV1beta1.PodMetrics) (totalCpu, totalMem resource.Quantity) {
	containers := metrics.Containers
	for _, c := range containers {
		totalCpu.Add(*c.Usage.Cpu())
		totalMem.Add(*c.Usage.Memory())
	}
	return
}
