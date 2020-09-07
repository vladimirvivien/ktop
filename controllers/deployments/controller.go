package deployments

import (
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/k8s"
)

type DeploymentController struct {
	depInformer *k8s.InformerAdapter
	k8sClient   *k8s.Client
	app         *application.Application
}

func New(app *application.Application) *DeploymentController {
	k8sClient := app.GetK8sClient()
	informerFac := k8sClient.InformerFactory
	ctrl := &DeploymentController{
		depInformer: k8s.NewInformerAdapter(informerFac.ForResource(k8s.Resources[k8s.DeploymentsResource])),
		app:         app,
		k8sClient:   k8sClient,
	}

	return ctrl
}

func (c *DeploymentController) Run() {
	c.setupEventHandlers()
	c.setupViews()
}

func (c *DeploymentController) setupViews() {
	c.nodePanel = NewNodePanel(fmt.Sprintf(" %c Nodes ", ui.Icons.Factory))
	c.nodePanel.Layout()
	c.nodePanel.DrawHeader("NAME", "STATUS", "ROLE", "VERSION", "CPU", "MEMORY")

	page := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(c.podPanel.GetView(), 0, 1, true)

	c.app.AddPage("Overview", page)
}