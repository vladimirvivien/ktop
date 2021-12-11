package k8s

import (
	"context"
	"time"

	"github.com/vladimirvivien/ktop/views/model"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

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

	for _, node := range nodes {
		metrics, err := c.client.GetNodeMetrics(ctx, node.Name)
		if err != nil {
			metrics = new(metricsV1beta1.NodeMetrics)
		}
		models = append(models, *model.NewNodeModel(&node, metrics))
	}
	return
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
