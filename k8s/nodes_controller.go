package k8s

import (
	"context"
	"time"

	"github.com/vladimirvivien/ktop/views/model"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func (c *Controller) GetNode(ctx context.Context, nodeName string) (coreV1.Node, error) {
	if ctx.Err() != nil {
		return coreV1.Node{}, ctx.Err()
	}
	obj, err := c.nodeInformer.Lister().Get(nodeName)
	if err != nil {
		return coreV1.Node{}, err
	}
	unstruct, ok := obj.(runtime.Unstructured)
	if !ok {
		panic("Controller: GetNodeList: unexpected type encountered")
	}
	var node = new(coreV1.Node)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.UnstructuredContent(), node); err != nil {
		return coreV1.Node{}, err
	}
	return *node, nil
}

func (c *Controller) GetNodeList(ctx context.Context) (nodes []coreV1.Node, err error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	items, err := c.nodeInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		unstructNode, ok := item.(runtime.Unstructured)
		if !ok {
			panic("Controller: GetNodeList: unexpected type encountered")
		}
		node := new(coreV1.Node)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructNode.UnstructuredContent(), node); err != nil {
			continue
		}
		nodes = append(nodes, *node)
	}
	return
}

func (c *Controller) GetNodeModels(ctx context.Context) (models []model.NodeModel, err error) {
	nodes, err := c.GetNodeList(ctx)
	if err != nil {
		return
	}

	// map each node to their pods
	pods, err := c.GetPodList(ctx)
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		metrics, err := c.client.GetNodeMetrics(ctx, node.Name)
		if err != nil {
			metrics = new(metricsV1beta1.NodeMetrics)
		}
		nodePods := getPodNodes(node.Name, pods)
		podsCount := len(nodePods)
		nodeModel := model.NewNodeModel(&node, metrics)
		nodeModel.PodsCount = podsCount
		nodeModel.RequestedPodMemQty = resource.NewQuantity(0, resource.DecimalSI)
		nodeModel.RequestedPodCpuQty = resource.NewQuantity(0, resource.DecimalSI)
		for _, pod := range nodePods {
			summary := model.GetPodContainerSummary(&pod)
			nodeModel.RequestedPodMemQty.Add(*summary.RequestedMemQty)
			nodeModel.RequestedPodCpuQty.Add(*summary.RequestedCpuQty)
		}

		models = append(models, *nodeModel)
	}
	return
}

func getPodNodes(nodeName string, pods []coreV1.Pod) []coreV1.Pod {
	var result []coreV1.Pod

	for _, pod := range pods {
		if pod.Spec.NodeName == nodeName {
			result = append(result, pod)
		}
	}
	return result
}

func (c *Controller) setupNodeHandler(ctx context.Context, handlerFunc RefreshNodesFunc) {
	go func() {
		c.refreshNodes(ctx, handlerFunc) // initial refresh
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.refreshNodes(ctx, handlerFunc); err != nil {
					continue
				}
			}
		}
	}()
}

func (c *Controller) refreshNodes(ctx context.Context, handlerFunc RefreshNodesFunc) error {
	models, err := c.GetNodeModels(ctx)
	if err != nil {
		return err
	}
	handlerFunc(ctx, models)
	return nil
}
