package controllers

import (
	"fmt"
	"log"
	"time"

	appsV1 "k8s.io/api/apps/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1beta1"
	"k8s.io/client-go/tools/cache"
)

type DeploymentController struct {
	SyncFunc func([]*appsV1.Deployment)
	factory  informers.SharedInformerFactory
	lister   appslisters.DeploymentLister
	synced   cache.InformerSynced
}

func Deployments(clientset kubernetes.Interface, resync time.Duration) *DeploymentController {
	informerFactory := informers.NewSharedInformerFactory(clientset, resync)
	informer := informerFactory.Apps().V1beta1().Deployments()

	ctrl := &DeploymentController{
		factory: informerFactory,
	}

	// setup event handler for Node resources
	informer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newOne := new.(*appsV1.Deployment)
			oldOne := old.(*appsV1.Deployment)
			if newOne.ResourceVersion == oldOne.ResourceVersion {
				// only update when new is different from old.
				return
			}
			ctrl.handleObject(new)
		},
		DeleteFunc: ctrl.handleObject,
	})

	ctrl.lister = informer.Lister()
	ctrl.synced = informer.Informer().HasSynced

	return ctrl
}

func (c *DeploymentController) Run(stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()

	c.factory.Start(stopCh)
	if ok := cache.WaitForCacheSync(stopCh, c.synced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	c.callSyncFunc()

	<-stopCh
	log.Println("Stopping controller")
	return nil
}

func (c *DeploymentController) handleObject(obj interface{}) {
	//fmt.Println(obj)
	c.callSyncFunc()
}

func (c *DeploymentController) callSyncFunc() {
	// get list
	deps, err := c.lister.List(labels.Everything())
	if err != nil {
		log.Println(err)
		return
	}
	if c.SyncFunc != nil {
		c.SyncFunc(deps)
	}
}
