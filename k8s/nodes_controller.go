package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/vladimirvivien/ktop/views/model"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func (c *Controller) GetNode(ctx context.Context, nodeName string) (*coreV1.Node, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	node, err := c.nodeInformer.Lister().Get(nodeName)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (c *Controller) GetNodeList(ctx context.Context) ([]*coreV1.Node, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if err := c.assertNodeAuthz(ctx); err != nil {
		return nil, err
	}

	items, err := c.nodeInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	return items, nil
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
		metrics, err := c.GetNodeMetrics(ctx, node.Name)
		if err != nil {
			metrics = new(metricsV1beta1.NodeMetrics)
		}
		nodePods := getPodNodes(node.Name, pods)
		podsCount := len(nodePods)
		nodeModel := model.NewNodeModel(node, metrics)
		nodeModel.PodsCount = podsCount
		nodeModel.RequestedPodMemQty = resource.NewQuantity(0, resource.DecimalSI)
		nodeModel.RequestedPodCpuQty = resource.NewQuantity(0, resource.DecimalSI)
		for _, pod := range nodePods {
			summary := model.GetPodContainerSummary(pod)
			nodeModel.RequestedPodMemQty.Add(*summary.RequestedMemQty)
			nodeModel.RequestedPodCpuQty.Add(*summary.RequestedCpuQty)
		}

		models = append(models, *nodeModel)
	}
	return
}

func (c *Controller) assertNodeAuthz(ctx context.Context) error {
	authzd, err := c.client.IsAuthz(ctx, "nodes", []string{"get", "list"})
	if err != nil {
		return fmt.Errorf("failed to check node authorization: %w", err)
	}
	if !authzd {
		return fmt.Errorf("node get, list not authorized")
	}
	return nil
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

func getPodNodes(nodeName string, pods []*coreV1.Pod) []*coreV1.Pod {
	var result []*coreV1.Pod

	for _, pod := range pods {
		if pod.Spec.NodeName == nodeName {
			result = append(result, pod)
		}
	}
	return result
}
