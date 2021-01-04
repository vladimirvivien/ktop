package k8s

import (
	"context"

	coreV1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type NodeController struct {
	manager     ctrl.Manager
	listFunc ListFunc
}

func NewNodeController(ctx context.Context, mgr ctrl.Manager) (*NodeController, error) {
	nodeCtrl := &NodeController{
		manager:     mgr,
	}

	ctrl, err := controller.New("nodeCtrl", mgr, controller.Options{Reconciler: nodeCtrl})

	if err != nil {
		return nil, err
	}

	if err := ctrl.Watch(&source.Kind{Type: &coreV1.Node{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	return nodeCtrl, nil
}

func (c *NodeController) SetListFunc(fn ListFunc) {
	c.listFunc = fn
}

func (c *NodeController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	client := c.manager.GetClient()
	var nodes coreV1.NodeList
	if err := client.List(ctx, &nodes); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	if c.listFunc != nil{
		if err := c.listFunc(req.Namespace, &nodes); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
	}
	return ctrl.Result{Requeue: false}, nil
}