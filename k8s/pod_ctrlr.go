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

type PodUpdateFunc func(name string, node *coreV1.Pod)
type PodDeleteFunc func(name string)

type PodController struct {
	ctx         context.Context
	manager     ctrl.Manager
	updateFuncs []PodUpdateFunc
	deleteFuncs []PodDeleteFunc
}

func NewPodController(ctx context.Context, mgr ctrl.Manager) (*PodController, error) {
	podCtrl := &PodController{
		ctx:         ctx,
		manager:     mgr,
		updateFuncs: make([]PodUpdateFunc, 0),
		deleteFuncs: make([]PodDeleteFunc, 0),
	}

	ctrl, err := controller.New("pod-runtimeCtrl", mgr, controller.Options{
		Reconciler: podCtrl,
	})

	if err != nil {
		return nil, err
	}

	if err := ctrl.Watch(&source.Kind{Type: &coreV1.Pod{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	return podCtrl, nil
}

func (c *PodController) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	client := c.manager.GetClient()
	var pod coreV1.Pod
	if err := client.Get(c.ctx, req.NamespacedName, &pod); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		// possibly a deletion
		c.processDelete(req.Name)
	}
	c.processUpdate(&pod)

	return ctrl.Result{Requeue: false}, nil
}

func (c *PodController) AddUpdateHandler(f PodUpdateFunc) {
	c.updateFuncs = append(c.updateFuncs, f)
}

func (c *PodController) AddDeleteHandler(f PodDeleteFunc) {
	c.deleteFuncs = append(c.deleteFuncs, f)
}

func (c *PodController) processUpdate(n *coreV1.Pod) {
	go func() {
		for _, f := range c.updateFuncs {
			if f != nil {
				f(n.GetName(), n.DeepCopy())
			}
		}
	}()
}

func (c *PodController) processDelete(name string) {
	go func() {
		for _, f := range c.deleteFuncs {
			if f != nil {
				f(name)
			}
		}
	}()
}

