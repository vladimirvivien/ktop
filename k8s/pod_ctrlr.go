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



type PodController struct {
	ctx         context.Context
	manager     ctrl.Manager
	listFunc ListFunc
}

func NewPodController(mgr ctrl.Manager) (*PodController, error) {
	podCtrl := &PodController{
		manager:     mgr,
	}

	ctrl, err := controller.New("podCtrl", mgr, controller.Options{Reconciler: podCtrl})

	if err != nil {
		return nil, err
	}

	if err := ctrl.Watch(&source.Kind{Type: &coreV1.Pod{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	//if err := ctrl.NewControllerManagedBy(mgr).For(&coreV1.Pod{}).Complete(podCtrl); err != nil {
	//	return nil, err
	//}

	return podCtrl, nil
}

func (c *PodController) SetListFunc(fn ListFunc) {
	c.listFunc = fn
}

func (c *PodController) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	client := c.manager.GetClient()
	var pods coreV1.PodList
	if err := client.List(ctx, &pods); err != nil {
		return ctrl.Result{Requeue: false}, err
	}
	if c.listFunc != nil{
		if err := c.listFunc(req.Namespace, &pods); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
	}
	return ctrl.Result{Requeue: false}, nil
}

