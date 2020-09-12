package overview

import (
	"fmt"

	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type MainPanel struct {
	k8sClient     *k8s.Client
	title         string
	refresh       func()
	view          *tview.Flex
	nodePanel     ui.Panel
	nodeStore     *model.Store
	podPanel      ui.Panel
	workloadPanel ui.Panel
}

func New(client *k8s.Client, title string, refreshFunc func()) *MainPanel {
	ctrl := &MainPanel{
		title:     title,
		k8sClient: client,
		refresh:   refreshFunc,
		nodeStore: model.NewStore(),
	}

	return ctrl
}

func (p *MainPanel) Layout(data interface{}) {
	p.nodePanel = NewNodePanel(fmt.Sprintf(" %c Nodes ", ui.Icons.Factory))
	p.nodePanel.Layout(nil)
	p.nodePanel.DrawHeader([]string{"NAME", "STATUS", "ROLE", "VERSION", "CPU", "MEMORY"})

	p.workloadPanel = NewWorkloadPanel(fmt.Sprintf(" %c Workload Health ", ui.Icons.Battery))
	p.workloadPanel.Layout(nil)
	p.workloadPanel.DrawHeader(nil)

	p.podPanel = NewPodPanel(fmt.Sprintf(" %c Pods ", ui.Icons.Package))
	p.podPanel.Layout(nil)
	p.podPanel.DrawHeader([]string{"NAME", "STATUS", "IP", "NODE", "CPU", "MEMORY"})

	view := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.nodePanel.GetView(), 7, 1, true).
		AddItem(p.workloadPanel.GetView(), 4, 1, true).
		AddItem(p.podPanel.GetView(), 0, 1, true)

	p.view = view
}

func (p *MainPanel) DrawHeader(data interface{})  {}
func (p *MainPanel) DrawBody(data interface{})    {}
func (p *MainPanel) DrawFooter(param interface{}) {}
func (p *MainPanel) Clear()                       {}

func (p *MainPanel) GetTitle() string {
	return p.title
}
func (p *MainPanel) GetView() tview.Primitive {
	return p.view
}

func (p *MainPanel) Run() error {
	p.Layout(nil)
	p.setupEventHandlers()
	return nil
}

func (p *MainPanel) setupEventHandlers() {
	p.k8sClient.AddNodeUpdateHandler(func(name string, obj *coreV1.Node) {
		p.refreshNodes(obj)
	})
	//p.nodeInformer.SetAddObjectFunc(func(obj interface{}) {
	//	p.refreshNodes(obj)
	//})
	//p.nodeInformer.SetUpdateObjectFunc(func(old, new interface{}) {
	//	p.refreshNodes(new)
	//})
	//
	//p.podInformer.SetAddObjectFunc(func(obj interface{}) {
	//	p.refreshPods(obj)
	//})
	//p.podInformer.SetUpdateObjectFunc(func(old, new interface{}) {
	//	p.refreshPods(new)
	//})
	//
	//p.depInformer.SetAddObjectFunc(func(obj interface{}) {
	//	p.refreshWorkload()
	//})
	//p.depInformer.SetUpdateObjectFunc(func(old, new interface{}) {
	//	p.refreshWorkload()
	//})
	//
	//p.dsInformer.SetAddObjectFunc(func(obj interface{}) {
	//	p.refreshWorkload()
	//})
	//p.dsInformer.SetUpdateObjectFunc(func(old, new interface{}) {
	//	p.refreshWorkload()
	//})
	//
	//p.rsInformer.SetAddObjectFunc(func(obj interface{}) {
	//	p.refreshWorkload()
	//})
	//p.rsInformer.SetUpdateObjectFunc(func(old, new interface{}) {
	//	p.refreshWorkload()
	//})
}

func (p *MainPanel) refreshNodes(node *coreV1.Node) error {
	metrics, err := p.k8sClient.GetNodeMetrics(node.Name)
	if err != nil {
		return err
	}

	conds := node.Status.Conditions
	availRes := node.Status.Allocatable
	row := model.NodeModel{
		UID:           string(node.GetUID()),
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

	p.nodeStore.Save(row.UID, row)

	p.nodePanel.DrawBody(p.nodeStore)
	if p.refresh != nil {
		p.refresh()
	}
	return nil
}

//func (c *MainPanel) refreshPods(obj interface{}) error {
//	podObjects, err := p.podInformer.Lister().List(labels.Everything())
//	if err != nil {
//		return err
//	}
//
//	podRows := make([]PodItem, len(podObjects))
//	for i, obj := range podObjects {
//		unstructPod := obj.(*unstructured.Unstructured)
//		pod := new(coreV1.Pod)
//		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructPod.Object, &pod); err != nil {
//			return err
//		}
//
//		podMetrics, err := p.k8sClient.GetMetricsByPod(pod.Name)
//		if err != nil {
//			return err
//		}
//
//		nodeMetrics, err := p.k8sClient.GetMetricsByNode(pod.Spep.NodeName)
//		if err != nil {
//			return err
//		}
//
//		totalCpu, totalMem := podMetricsTotals(podMetrics)
//		row := PodItem{
//			Name:         pod.Name,
//			Status:       string(pod.Status.Phase),
//			IP:           pod.Status.PodIP,
//			Node:         pod.Spep.NodeName,
//			Volumes:      len(pod.Spep.Volumes),
//			NodeCPUValue: nodeMetrics.Usage.Cpu().MilliValue(),
//			NodeMemValue: nodeMetrics.Usage.Memory().MilliValue(),
//			PodCPUValue:  totalCpu.MilliValue(),
//			PodMemValue:  totalMem.MilliValue(),
//		}
//		podRows[i] = row
//	}
//
//	p.podPanel.DrawBody(podRows)
//	p.app.Refresh()
//	return nil
//}
//
//func (c *MainPanel) refreshWorkload() error {
//	var summary WorkloadItem
//
//	deps, err := p.depInformer.Lister().List(labels.Everything())
//	if err != nil {
//		return err
//	}
//	summary.DeploymentsTotal, summary.DeploymentsReady, err = getDepsSummary(deps)
//	if err != nil {
//		return err
//	}
//
//	daemonSets, err := p.dsInformer.Lister().List(labels.Everything())
//	if err != nil {
//		return err
//	}
//	summary.DaemonSetsTotal, summary.DaemonSetsReady, err = getDSSummary(daemonSets)
//	if err != nil {
//		return err
//	}
//
//	reps, err := p.rsInformer.Lister().List(labels.Everything())
//	if err != nil {
//		return err
//	}
//	summary.ReplicaSetsTotal, summary.ReplicaSetsReady, err = getRSSummary(reps)
//	if err != nil {
//		return err
//	}
//
//	pods, err := p.podInformer.Lister().List(labels.Everything())
//	if err != nil {
//		return err
//	}
//	summary.PodsTotal, summary.PodsReady, err = getPodsSummary(pods)
//	if err != nil {
//		return err
//	}
//
//	p.workloadPanel.DrawBody(summary)
//	p.app.Refresh()
//
//	return nil
//}

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
