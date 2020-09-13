package overview

import (
	"fmt"

	"github.com/rivo/tview"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

type MainPanel struct {
	k8sClient     *k8s.Client
	title         string
	refresh       func()
	view          *tview.Flex
	nodePanel     ui.Panel
	nodeStore     *model.Store
	podPanel      ui.Panel
	podStore      *model.Store
	workloadPanel ui.Panel
}

func New(client *k8s.Client, title string, refreshFunc func()) *MainPanel {
	ctrl := &MainPanel{
		title:     title,
		k8sClient: client,
		refresh:   refreshFunc,
		nodeStore: model.NewStore(),
		podStore:  model.NewStore(),
	}

	return ctrl
}

func (p *MainPanel) Layout(data interface{}) {
	p.nodePanel = NewNodePanel(fmt.Sprintf(" %c Nodes ", ui.Icons.Factory))
	p.nodePanel.Layout(nil)
	p.nodePanel.DrawHeader([]string{"NAME", "STATUS", "ROLE", "VERSION", "CPU", "MEMORY"})

	p.workloadPanel = NewWorkloadPanel(fmt.Sprintf(" %c Workload Health ", ui.Icons.Thermometer))
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
	p.k8sClient.AddNodeDeleteHandler(func(name string) {
		p.removeNode(name)
	})

	p.k8sClient.AddPodUpdateHandler(func(name string, obj *coreV1.Pod) {
		p.refreshPods(obj)
	})

}

func (p *MainPanel) refreshNodes(node *coreV1.Node) error {
	metrics, err := p.k8sClient.GetNodeMetrics(node.Name)
	if err != nil {
		// TODO log metrics error on screen, but continue with display
		metrics = new(metricsV1beta1.NodeMetrics)
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

func (p *MainPanel) removeNode(name string) error {
	// look for node and remove from store
	var node model.NodeModel
	for _, key := range p.nodeStore.Keys() {
		data, found := p.nodeStore.Get(key)
		if !found {
			return nil
		}
		node = data.(model.NodeModel)
		if node.Name == name {
			p.nodeStore.Remove(node.UID)
			return nil
		}
	}
	return nil
}

func (p *MainPanel) refreshPods(pod *coreV1.Pod) error {
	podMetrics, err := p.k8sClient.GetPodMetrics(pod.Name)
	if err != nil {
		// TODO log metrics error on screen, but continue with display
		podMetrics = new(metricsV1beta1.PodMetrics)
	}
	nodeMetrics, err := p.k8sClient.GetNodeMetrics(pod.Spec.NodeName)
	if err != nil {
		// TODO log metrics error on screen, but continue with display
		nodeMetrics = new(metricsV1beta1.NodeMetrics)
	}

	totalCpu, totalMem := podMetricsTotals(podMetrics)
	row := model.PodModel{
		UID: string(pod.GetUID()),
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
	p.podStore.Save(row.UID, row)
	p.podPanel.DrawBody(p.podStore)
	if p.refresh != nil {
		p.refresh()
	}
	return nil
}
func (p *MainPanel) removePod(name string) error {
	// look for node and remove from store
	var pod model.PodModel
	for _, key := range p.podStore.Keys() {
		data, found := p.podStore.Get(key)
		if !found {
			return nil
		}
		pod = data.(model.PodModel)
		if pod.Name == name {
			p.podStore.Remove(pod.UID)
			return nil
		}
	}
	return nil
}

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
