package controllers

import (
	"fmt"
	"log"
	"time"

	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type NodeController struct {
	SyncFunc   func([]*coreV1.Node)
	factory    informers.SharedInformerFactory
	nodeLister corelisters.NodeLister
	nodeSynced cache.InformerSynced
}

func Nodes(clientset kubernetes.Interface, resync time.Duration) *NodeController {
	informerFactory := informers.NewSharedInformerFactory(clientset, resync)
	nodeInformer := informerFactory.Core().V1().Nodes()

	ctrl := &NodeController{
		factory: informerFactory,
	}

	// setup event handler for Node resources
	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newNode := new.(*coreV1.Node)
			oldNode := old.(*coreV1.Node)
			if newNode.ResourceVersion == oldNode.ResourceVersion {
				// only update when new is different from old.
				return
			}
			ctrl.handleObject(new)
		},
		DeleteFunc: ctrl.handleObject,
	})

	ctrl.nodeLister = nodeInformer.Lister()
	ctrl.nodeSynced = nodeInformer.Informer().HasSynced

	return ctrl
}

func (c *NodeController) Run(stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()

	c.factory.Start(stopCh)
	if ok := cache.WaitForCacheSync(stopCh, c.nodeSynced); !ok {
		log.Println("failed to wait for cache sync")
		return fmt.Errorf("failed to wait for caches to sync")
	}

	c.callSyncFunc()

	<-stopCh
	log.Println("Stopping controller")
	return nil
}

func (c *NodeController) handleObject(obj interface{}) {
	//fmt.Println(obj)
	c.callSyncFunc()
}

func (c *NodeController) callSyncFunc() {
	// get list
	nodes, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		log.Println(err)
		return
	}
	if c.SyncFunc != nil {
		c.SyncFunc(nodes)
	}
}
