package overview

import (
	"fmt"

	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/client"
	"github.com/vladimirvivien/ktop/controllers"
	"github.com/vladimirvivien/ktop/ui"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type OverviewController struct {
	nodeMetricsInformer *controllers.InformerAdapter
	nodeInformer        *controllers.InformerAdapter
	k8sClient           *client.K8sClient
	app                 *application.Application
	nodePanel           ui.Panel
	podPanel            ui.Panel
}

func NewNodePanelCtrl(k8sClient *client.K8sClient, app *application.Application) *OverviewController {
	informerFac := k8sClient.InformerFactory
	ctrl := &OverviewController{
		nodeInformer: controllers.NewInformerAdapter(informerFac.ForResource(client.Resources[client.NodesResource])),
		app:          app,
		k8sClient:    k8sClient,
	}

	if k8sClient.MetricsAreAvailable {
		ctrl.nodeMetricsInformer = controllers.NewInformerAdapter(informerFac.ForResource(client.Resources[client.NodeMetricsResource]))
	}

	return ctrl
}

func (c *OverviewController) Run() {
	c.setupNodeEventHandlers()
	c.setupViews()
}

func (c *OverviewController) setupNodeEventHandlers() {
	c.nodeInformer.SetAddObjectFunc(func(obj interface{}) {
		c.refreshNodes(obj)
	})

	c.nodeInformer.SetUpdateObjectFunc(func(old, new interface{}) {
		c.refreshNodes(new)
	})

	if c.k8sClient.MetricsAreAvailable {
		c.nodeMetricsInformer.SetAddObjectFunc(func(obj interface{}) {
			c.refreshNodes(obj)
		})

		c.nodeMetricsInformer.SetUpdateObjectFunc(func(old, new interface{}) {
			c.refreshNodes(new)
		})
	}
}

func (c *OverviewController) refreshNodes(obj interface{}) error {
	nodeObjects, err := c.nodeInformer.Lister().List(labels.Everything())
	if err != nil {
		return err
	}

	// get all metrics for all nodes
	var nodeMetricsObjects []runtime.Object
	if c.k8sClient.MetricsAreAvailable {
		list, err := c.nodeMetricsInformer.Lister().List(labels.Everything())
		if err != nil {
			return err
		}
		nodeMetricsObjects = list
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
		metrics, err := getNodeMetricsByName(nodeMetricsObjects, node.Name)
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

func (c *OverviewController) setupViews() {
	c.nodePanel = NewNodePanel("Nodes")
	c.nodePanel.Layout()
	c.nodePanel.DrawHeader("NAME", "STATUS", "ROLE", "VERSION", "CPU", "MEMORY")

	c.podPanel = NewPodPanel("Pods")
	c.podPanel.Layout()
	c.podPanel.DrawHeader("NAME", "STATUS", "IP", "NODE", "CPU", "MEMORY")

	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(c.nodePanel.GetView(), 7, 1, true).
		AddItem(c.podPanel.GetView(), 4, 1, true)
		//AddItem(p.podList, 0, 1, true)

	c.app.AddPage("Overview", page)
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

func getNodeMetricsByName(metricsObjects []runtime.Object, nodeName string) (*metricsV1beta1.NodeMetrics, error) {
	for _, obj := range metricsObjects {
		metrics, ok := obj.(*unstructured.Unstructured)
		if !ok {
			return nil, fmt.Errorf("unexpected type for NodeMetrics")
		}
		if metrics.GetName() == nodeName {
			nodeMetrics := new(metricsV1beta1.NodeMetrics)
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(metrics.Object, &nodeMetrics); err != nil {
				return nil, err
			}
			return nodeMetrics, nil
		}
	}
	return new(metricsV1beta1.NodeMetrics), nil
}
