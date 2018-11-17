package deployments

import (
	"fmt"

	"github.com/vladimirvivien/ktop/client"
	"github.com/vladimirvivien/ktop/ui"
	appsV1 "k8s.io/api/apps/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
)

type deploymentController struct {
	k8s *client.K8sClient
	app *ui.Application

	depLister appslisters.DeploymentLister
	depSynced cache.InformerSynced

	page *deploymentPage
}

func New(
	k8s *client.K8sClient,
	app *ui.Application,
	pgTitle string,
) *deploymentController {
	ctrl := &deploymentController{k8s: k8s, app: app}
	ctrl.page = newPage()
	ctrl.app.AddPage(pgTitle, ctrl.page.root)

	ctrl.depLister = k8s.DeploymentInformer.Lister()
	ctrl.depSynced = k8s.DeploymentInformer.Informer().HasSynced
	k8s.DeploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: ctrl.updateDeploymentList,
		UpdateFunc: func(old, new interface{}) {
			newPod := new.(*appsV1.Deployment)
			oldPod := old.(*appsV1.Deployment)
			if newPod.ResourceVersion == oldPod.ResourceVersion {
				return
			}
			ctrl.updateDeploymentList(new)
		},
		DeleteFunc: ctrl.updateDeploymentList,
	})
	return ctrl
}

func (c *deploymentController) updateDeploymentList(obj interface{}) {

}

func (c *deploymentController) Run(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if ok := cache.WaitForCacheSync(stopCh, c.depSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	<-stopCh
	return nil
}