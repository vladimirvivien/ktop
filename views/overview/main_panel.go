package overview

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"

	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

type MainPanel struct {
	app           *application.Application
	title         string
	refresh       func()
	root          *tview.Flex
	children      []tview.Primitive
	selPanelIndex int
	nodePanel     ui.Panel
	podPanel      ui.Panel
	workloadPanel ui.Panel
}

func New(app *application.Application, title string) *MainPanel {
	ctrl := &MainPanel{
		app:           app,
		title:         title,
		refresh:       app.Refresh,
		selPanelIndex: -1,
	}

	return ctrl
}

func (p *MainPanel) Layout(data interface{}) {
	p.nodePanel = NewNodePanel(p.app, fmt.Sprintf(" %c Nodes ", ui.Icons.Factory))
	p.nodePanel.DrawHeader([]string{"NAME", "STATUS", "VERSION", "INT/EXT IPs", "OS", "CPU/MEM", "EPH.STORE", "CPU USAGE", "MEM USAGE"})
	p.children = append(p.children, p.nodePanel.GetRootView())

	p.workloadPanel = NewWorkloadPanel(fmt.Sprintf(" %c Workload Health ", ui.Icons.Thermometer))
	p.workloadPanel.Layout(nil)
	p.workloadPanel.DrawHeader(nil)
	p.children = append(p.children, p.workloadPanel.GetRootView())

	p.podPanel = NewPodPanel(fmt.Sprintf(" %c Pods ", ui.Icons.Package))
	p.podPanel.DrawHeader([]string{"NAME", "STATUS", "IP", "NODE", "CPU", "MEMORY"})
	p.children = append(p.children, p.podPanel.GetRootView())

	view := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.nodePanel.GetRootView(), 7, 1, true).
		AddItem(p.workloadPanel.GetRootView(), 4, 1, true).
		AddItem(p.podPanel.GetRootView(), 0, 1, true)

	p.root = view

}

func (p *MainPanel) DrawHeader(data interface{})  {}
func (p *MainPanel) DrawBody(data interface{})    {}
func (p *MainPanel) DrawFooter(param interface{}) {}
func (p *MainPanel) Clear()                       {}

func (p *MainPanel) GetTitle() string {
	return p.title
}
func (p *MainPanel) GetRootView() tview.Primitive {
	return p.root
}
func (p *MainPanel) GetChildrenViews() []tview.Primitive {
	return p.children
}

func (p *MainPanel) Run() error {
	p.Layout(nil)
	p.setupEventHandlers()
	return nil
}

func (p *MainPanel) setupEventHandlers() {
	// setup ui event handlers
	p.root.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTAB {
			//p.selPanelIndex++
			//hasFoc := p.panels[p.selPanelIndex].GetRootView().HasFocus()
			//fmt.Println("has focus:", "true")
		}

		return event
	})

	// set up k8s event handlers
	p.app.GetK8sClient().AddNodeListFunc(p.refreshNodes)
	p.app.GetK8sClient().AddPodListFunc(p.refreshPods)
}

func (p *MainPanel) refreshNodes(ctx context.Context, namespace string, nodes runtime.Object) error {
	if nodes == nil {
		return fmt.Errorf("main panel: nodes nil")
	}
	nodeList, ok := nodes.(*coreV1.NodeList)
	if !ok {
		return fmt.Errorf("main panel: NodeList type mismatched")
	}

	p.nodePanel.DrawBody(nodeList)
	// required: always schedule screen refresh
	if p.refresh != nil {
		p.refresh()
	}

	return nil
}

func (p *MainPanel) refreshPods(ctx context.Context, namespace string, pods runtime.Object) error {
	if pods == nil {
		return fmt.Errorf("overview panel: pods nil")
	}
	podList, ok := pods.(*coreV1.PodList)
	if !ok {
		return fmt.Errorf("overview panel: PodList type mismatched")
	}

	rows := make([]model.PodModel, len(podList.Items))
	for i, pod := range podList.Items {
		podMetrics, err := p.app.GetK8sClient().GetPodMetrics(ctx, pod.Name)
		if err != nil {
			// TODO log metrics error on screen, but continue with display
			podMetrics = new(metricsV1beta1.PodMetrics)
		}
		nodeMetrics, err := p.app.GetK8sClient().GetNodeMetrics(ctx, pod.Spec.NodeName)
		if err != nil {
			// TODO log metrics error on screen, but continue with display
			nodeMetrics = new(metricsV1beta1.NodeMetrics)
		}

		totalCpu, totalMem := podMetricsTotals(podMetrics)
		row := model.PodModel{
			UID:          model.GetNamespacedKey(namespace, pod.GetName()),
			Namespace:    namespace,
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
		rows[i] = row
	}
	p.podPanel.Clear()
	p.podPanel.DrawBody(rows)
	// required: always refresh screen
	if p.refresh != nil {
		p.refresh()
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

func nodeStatus(node coreV1.Node) string {
	conds := node.Status.Conditions
	if conds == nil || len(conds) == 0 {
		return "Unknown"
	}

	for _, cond := range conds {
		if cond.Status == coreV1.ConditionTrue {
			return string(cond.Type)
		}
	}

	return "NotReady"
}

func getNodeInternalIp(node coreV1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == coreV1.NodeInternalIP {
			return addr.Address
		}
	}
	return "<none>"
}

func getNodeExternalIp(node coreV1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == coreV1.NodeExternalIP {
			return addr.Address
		}
	}
	return "<none>"
}

func getNodeHostName(node coreV1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == coreV1.NodeHostName {
			return addr.Address
		}
	}
	return "<none>"
}

func gigaScale(qty *resource.Quantity) string {
	if qty.RoundUp(resource.Giga) {
		return qty.String()
	}
	return qty.String()
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
