package overview

import (
	"context"

	"github.com/vladimirvivien/ktop/client"
	topctx "github.com/vladimirvivien/ktop/context"
	"github.com/vladimirvivien/ktop/controllers"
	"github.com/vladimirvivien/ktop/ui"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/kubernetes"
)

type OverviewController struct {
	nodeInformer   *controllers.InformerAdapter
	k8s            kubernetes.Interface
	isMetricsAvail bool
	ns             string
	app            *ui.Application
	nodePanel      ui.Panel
}

func NewNodePanelCtrl(ctx context.Context, informerFac dynamicinformer.DynamicSharedInformerFactory, app *ui.Application) *OverviewController {
	k8s, _ := topctx.K8sInterface(ctx)
	ns, _ := topctx.Namespace(ctx)
	isMetrics, _ := topctx.IsMetricsAvailable(ctx)

	ctrl := &OverviewController{
		nodeInformer:   controllers.NewInformerAdapter(informerFac.ForResource(client.NodesResource)),
		k8s:            k8s,
		ns:             ns,
		isMetricsAvail: isMetrics,
		app:            app,
	}
	return ctrl
}

func (c *OverviewController) Start() {
	c.setupEventHandlers()
	c.setupViews()
}

func (c *OverviewController) setupEventHandlers() {
	c.nodeInformer.SetAddObjectFunc(func(obj interface{}) {
		c.refreshNode(obj)
	})

	c.nodeInformer.SetUpdateObjectFunc(func(old, new interface{}) {
		c.refreshNode(new)
	})
}

func (c *OverviewController) refreshNode(obj interface{}) error {
	unstructList, err := c.nodeInformer.Lister().List(labels.Everything())
	if err != nil {
		return err
	}

	// for i, obj := range unstructList {
	// 	unstructNode := obj.(*unstructured.Unstructured)
	// 	node := new(coreV1.Node)
	// 	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructNode.Object, &node); err != nil {
	// 		return err
	// 	}

	// 	name := node.GetName()
	// 	role := nodeRole(node)
	// }

	// var nodeMetrics []metricsV1beta1.NodeMetrics
	// if c.isMetricsAvail {
	// 	nodeMetricsList, err := c.k8s.MetricsClient.Metrics().NodeMetricses().List(metaV1.ListOptions{})
	// 	if err != nil {
	// 		return err
	// 	}
	// 	nodeMetrics = nodeMetricsList.Items
	// }

	// collect node and metrics info in nodeRow type
	// used for display.
	nodeListRows := make([]NodeItem, len(unstructList))
	for i, obj := range unstructList {
		unstructNode := obj.(*unstructured.Unstructured)
		node := new(coreV1.Node)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructNode.Object, &node); err != nil {
			return err
		}
		conds := node.Status.Conditions
		//availRes := node.Status.Allocatable
		//metrics := getMetricsByNodeName(nodeMetrics, node.Name)
		row := NodeItem{
			Name:    node.Name,
			Role:    nodeRole(*node),
			Status:  string(conds[len(conds)-1].Type),
			Version: node.Status.NodeInfo.KubeletVersion,
			// 		cpuAvail:      availRes.Cpu().String(),
			// 		cpuAvailValue: availRes.Cpu().MilliValue(),
			// 		cpuUsage:      metrics.Usage.Cpu().String(),
			// 		cpuValue:      metrics.Usage.Cpu().MilliValue(),
			// 		memAvail:      availRes.Memory().String(),
			// 		memAvailValue: availRes.Memory().MilliValue(),
			// 		memUsage:      metrics.Usage.Memory().String(),
			// 		memValue:      metrics.Usage.Memory().MilliValue(),
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
	c.app.AddPage("Overview", c.nodePanel.GetView())
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
