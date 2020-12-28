package k8s

import (
	"context"

	coreV1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type NodeUpdateFunc func(name string, node *coreV1.Node)
type NodeDeleteFunc func(name string)

type NodeController struct {
	ctx         context.Context
	manager     ctrl.Manager
	updateChan  chan *coreV1.Node
	updateFuncs []NodeUpdateFunc
	deleteFuncs []NodeDeleteFunc
}

func NewNodeController(ctx context.Context, mgr ctrl.Manager) (*NodeController, error) {
	nodeCtrl := &NodeController{
		ctx:         ctx,
		manager:     mgr,
		updateChan:  make(chan *coreV1.Node, 1),
		updateFuncs: make([]NodeUpdateFunc, 0),
		deleteFuncs: make([]NodeDeleteFunc, 0),
	}

	ctrl, err := controller.New("node-runtimeCtrl", mgr, controller.Options{
		Reconciler: nodeCtrl,
	})

	if err != nil {
		return nil, err
	}

	if err := ctrl.Watch(&source.Kind{Type: &coreV1.Node{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	return nodeCtrl, nil
}

func (c *NodeController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	client := c.manager.GetClient()
	var node coreV1.Node
	if err := client.Get(c.ctx, req.NamespacedName, &node); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		// possibly a deletion
		c.processDelete(req.Name)
	}
	c.processUpdate(&node)

	return ctrl.Result{Requeue: false}, nil
}

func (c *NodeController) AddUpdateHandler(f NodeUpdateFunc) {
	c.updateFuncs = append(c.updateFuncs, f)
}

func (c *NodeController) AddDeleteHandler(f NodeDeleteFunc) {
	c.deleteFuncs = append(c.deleteFuncs, f)
}

func (c *NodeController) processUpdate(n *coreV1.Node) {
	go func() {
		for _, f := range c.updateFuncs {
			if f != nil {
				f(n.GetName(), n.DeepCopy())
			}
		}
	}()
}

func (c *NodeController) processDelete(name string) {
	go func() {
		for _, f := range c.deleteFuncs {
			if f != nil {
				f(name)
			}
		}
	}()
}
