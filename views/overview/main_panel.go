package overview

import (
	"context"
	"fmt"
	"time"

	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

type MainPanel struct {
	app                 *application.Application
	title               string
	refresh             func()
	root                *tview.Flex
	children            []tview.Primitive
	selPanelIndex       int
	nodePanel           ui.Panel
	podPanel            ui.Panel
	clusterSummaryPanel ui.Panel
}

func New(app *application.Application, title string) *MainPanel {
	ctrl := &MainPanel{
		app:           app,
		title:         title,
		refresh:       app.Refresh,
		selPanelIndex: -1,
	}

	return ctrl
}

func (p *MainPanel) Layout(data interface{}) {
	p.nodePanel = NewNodePanel(p.app, fmt.Sprintf(" %c Nodes ", ui.Icons.Factory))
	p.nodePanel.DrawHeader([]string{"NAME", "STATUS", "AGE", "VERSION", "INT/EXT IPs", "OS/ARC", "PODS/IMGs", "DISK", "CPU", "MEM"})

	p.clusterSummaryPanel = NewClusterSummaryPanel(p.app, fmt.Sprintf(" %c Cluster Summary ", ui.Icons.Thermometer))
	p.clusterSummaryPanel.Layout(nil)
	p.clusterSummaryPanel.DrawHeader(nil)

	p.podPanel = NewPodPanel(p.app, fmt.Sprintf(" %c Pods ", ui.Icons.Package))
	p.podPanel.DrawHeader([]string{"NAMESPACE", "POD", "READY", "STATUS", "RESTARTS", "AGE", "VOLS", "IP", "NODE", "CPU", "MEMORY"})

	p.children = []tview.Primitive{
		p.clusterSummaryPanel.GetRootView(),
		p.nodePanel.GetRootView(),
		p.podPanel.GetRootView(),
	}

	view := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.clusterSummaryPanel.GetRootView(), 4, 1, true).
		AddItem(p.nodePanel.GetRootView(), 7, 1, true).
		AddItem(p.podPanel.GetRootView(), 0, 1, true)

	p.root = view

}

func (p *MainPanel) DrawHeader(_ interface{})  {}
func (p *MainPanel) DrawBody(_ interface{})    {}
func (p *MainPanel) DrawFooter(_ interface{}) {}
func (p *MainPanel) Clear()                       {}

func (p *MainPanel) GetTitle() string {
	return p.title
}
func (p *MainPanel) GetRootView() tview.Primitive {
	return p.root
}
func (p *MainPanel) GetChildrenViews() []tview.Primitive {
	return p.children
}

func (p *MainPanel) Run(ctx context.Context) error {
	p.Layout(nil)
	ctrl := p.app.GetK8sClient().Controller()
	ctrl.SetClusterSummaryRefreshFunc(p.refreshWorkloadSummary)
	ctrl.SetNodeRefreshFunc(p.refreshNodeView)
	ctrl.SetPodRefreshFunc(p.refreshPods)

	if err := ctrl.Start(ctx, time.Second*10); err != nil {
		panic(fmt.Sprintf("main panel: controller start: %s", err))
	}
	return nil
}

func (p *MainPanel) refreshNodeView(ctx context.Context, models []model.NodeModel) error {
	model.SortNodeModels(models)

	p.nodePanel.Clear()
	p.nodePanel.DrawBody(models)

	// required: always schedule screen refresh
	if p.refresh != nil {
		p.refresh()
	}

	return nil
}

func (p *MainPanel) refreshPods(ctx context.Context, models []model.PodModel) error {
	model.SortPodModels(models)

	// refresh pod list
	p.podPanel.Clear()
	p.podPanel.DrawBody(models)

	// required: always refresh screen
	if p.refresh != nil {
		p.refresh()
	}
	return nil
}

func (p *MainPanel) refreshWorkloadSummary(ctx context.Context, summary model.ClusterSummary) error {
	p.clusterSummaryPanel.Clear()
	p.clusterSummaryPanel.DrawBody(summary)
	if p.refresh != nil {
		p.refresh()
	}
	return nil
}
