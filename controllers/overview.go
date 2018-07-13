package controllers

import (
	"fmt"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/vladimirvivien/ktop/client"
	"github.com/vladimirvivien/ktop/ui"
)

type Overview struct {
	k8s        *client.K8sClient
	nodeLister corelisters.NodeLister
	nodeSynced cache.InformerSynced
	ui         *ui.OverviewPage
}

func NewOverview(
	k8s *client.K8sClient,
	ui *ui.OverviewPage,
) *Overview {
	ctrl := &Overview{k8s: k8s, ui: ui}
	// setup node callbacks
	nodeInformer := k8s.InformerFactory.Core().V1().Nodes()
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

func (c *Overview) addNode(obj interface{}) {

}

func (c *Overview) updateNode(obj interface{}) {

}

func (c *Overview) deleteNode(obj interface{}) {

}

func (c *Overview) initList() error {
	nodes, err := c.k8s.Clientset.CoreV1().Nodes().List(metaV1.ListOptions{})
	if err != nil {
		return err
	}

	c.syncNodes(nodes.Items)
	return nil
}

func (c *Overview) Run(stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	c.ui.DrawHeader(c.k8s.Config.Host, c.k8s.Namespace)
	if err := c.initList(); err != nil {
		return err
	}

	if ok := cache.WaitForCacheSync(stopCh, c.nodeSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	<-stopCh
	return nil
}

func (c *Overview) syncNodes(nodes []coreV1.Node) {
	nodeListRows := make([]ui.NodeRow, len(nodes))
	for i, node := range nodes {
		conds := node.Status.Conditions
		row := ui.NodeRow{
			Name:    node.Name,
			Role:    nodeRole(node),
			Status:  string(conds[len(conds)-1].Type),
			Version: node.Status.NodeInfo.KubeletVersion,
		}
		nodeListRows[i] = row
	}
	c.ui.DrawNodeList(nodeListRows)
}

func isNodeMaster(node coreV1.Node) bool {
	_, ok := node.Labels["node-role.kubernetes.io/master"]
	return ok
}

func nodeRole(node coreV1.Node) string {
	if isNodeMaster(node) {
		return "Master"
	}
	return "Node"
}
