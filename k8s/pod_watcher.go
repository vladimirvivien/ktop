package k8s

import (
	"context"

	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
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
		if err := c.listFunc(ctx, req.Namespace, &pods); err != nil {
			return ctrl.Result{Requeue: false}, err
		}
	}
	return ctrl.Result{Requeue: false}, nil
}

type PodWatcher struct {
	client    *Client
	listFuncs []ListFunc
}

func NewPodWatcher(client *Client) *PodWatcher {
	return &PodWatcher{client: client}
}

func (w *PodWatcher) AddPodListFunc(fn ListFunc) {
	w.listFuncs = append(w.listFuncs, fn)
}

func (w *PodWatcher) Start(ctx context.Context) error {
	watcher, err := w.client.NamespacedResourceInterface(PodsResource).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	podsCh := make(chan *coreV1.PodList, 1)

	go func() {
		for {
			select {
			case event := <-watcher.ResultChan():
				if event.Object == nil {
					continue
				}

				// react on Add, Delete, Modified events
				switch event.Type {
				case watch.Added, watch.Deleted, watch.Modified:
					listObj, err := w.client.NamespacedResourceInterface(PodsResource).List(ctx, metav1.ListOptions{})
					if err != nil {
						continue
					}

					podList := new(coreV1.PodList)
					if err := runtime.DefaultUnstructuredConverter.FromUnstructured(listObj.UnstructuredContent(), podList); err != nil {
						continue
					}

					// launch handler functions
					podsCh <- podList
				}
			case <-ctx.Done():
				watcher.Stop()
			}
		}
	}()

	// func processor
	go func() {
		for podList := range podsCh {
			if len(w.listFuncs) == 0 {
				continue
			}
			for _, fn := range w.listFuncs {
				if err := fn(ctx, w.client.namespace, podList); err != nil {
					continue
				}
			}
		}
	}()

	return nil
}


