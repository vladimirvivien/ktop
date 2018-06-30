package controllers

import (
	"fmt"

	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/runtime"
	informers "k8s.io/client-go/informers"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/rivo/tview"
)

type OverviewController struct {
	nodeLister corelisters.NodeLister
	nodeSynced cache.InformerSynced
	nodeUI     *tview.TextView
	rootUI     *tview.Flex
}

func Overview(factory informers.SharedInformerFactory) *OverviewController {
	ctrl := new(OverviewController)

	// setup node callbacks
	nodeInformer := factory.Core().V1().Nodes()
	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.addNode,
		UpdateFunc: func(old, new interface{}) {
			newNode := new.(*coreV1.Node)
			oldNode := old.(*coreV1.Node)
			if newNode.ResourceVersion == oldNode.ResourceVersion {
				return
			}
			ctrl.updateNode(new)
		},
		DeleteFunc: ctrl.deleteNode,
	})

	ctrl.nodeLister = nodeInformer.Lister()
	ctrl.nodeSynced = nodeInformer.Informer().HasSynced

	return ctrl
}

func (c *OverviewController) RootUI() *tview.Flex {
	return c.rootUI
}

func (c *OverviewController) addNode(obj interface{}) {

}

func (c *OverviewController) updateNode(obj interface{}) {

}

func (c *OverviewController) deleteNode(obj interface{}) {

}

func (c *OverviewController) syncui() error {
	nodes, err := c.nodeLister.List(labels.Everything())
	if err != nil {
		return err
	}
	c.syncNodes(nodes)
	return nil
}

func (c *OverviewController) Run(stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()

	if ok := cache.WaitForCacheSync(stopCh, c.nodeSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	<-stopCh
	return nil
}

func (c *OverviewController) syncNodes(nodes []*coreV1.Node) {

}
