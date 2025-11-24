package overview

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type MainPanel struct {
	app                 *application.Application
	metricsSource       metrics.MetricsSource
	title               string
	refresh             func()
	root                *tview.Flex
	children            []tview.Primitive
	selPanelIndex       int
	nodePanel           ui.Panel
	podPanel            ui.Panel
	clusterSummaryPanel ui.Panel
	showAllColumns      bool
	nodeColumns         []string
	podColumns          []string
}

func New(app *application.Application, title string) *MainPanel {
	return NewWithColumnOptions(app, title, true, nil, nil)
}

func NewWithColumnOptions(app *application.Application, title string, showAllColumns bool, nodeColumns, podColumns []string) *MainPanel {
	ctrl := &MainPanel{
		app:            app,
		metricsSource:  app.GetMetricsSource(),
		title:          title,
		refresh:        app.Refresh,
		selPanelIndex:  -1,
		showAllColumns: showAllColumns,
		nodeColumns:    nodeColumns,
		podColumns:     podColumns,
	}

	return ctrl
}

func (p *MainPanel) Layout(data interface{}) {
	// Define the default columns
	allNodeColumns := []string{"NAME", "STATUS", "AGE", "VERSION", "INT/EXT IPs", "OS/ARC", "PODS/IMGs", "DISK", "CPU", "MEM"}
	allPodColumns := []string{"NAMESPACE", "POD", "READY", "STATUS", "RESTARTS", "AGE", "VOLS", "IP", "NODE", "CPU", "MEMORY"}

	// Use filtered columns if specified
	nodeColumnsToDisplay := allNodeColumns
	podColumnsToDisplay := allPodColumns

	if !p.showAllColumns {
		if len(p.nodeColumns) > 0 {
			// Filter node columns
			nodeColumnsToDisplay = filterColumns(allNodeColumns, p.nodeColumns)
		}

		if len(p.podColumns) > 0 {
			// Filter pod columns
			podColumnsToDisplay = filterColumns(allPodColumns, p.podColumns)
		}
	}

	p.nodePanel = NewNodePanel(p.app, fmt.Sprintf(" %c Nodes ", ui.Icons.Factory))
	p.nodePanel.DrawHeader(nodeColumnsToDisplay)

	p.clusterSummaryPanel = NewClusterSummaryPanel(p.app, fmt.Sprintf(" %c Cluster Summary ", ui.Icons.Thermometer))
	p.clusterSummaryPanel.Layout(nil)
	p.clusterSummaryPanel.DrawHeader(nil)

	p.podPanel = NewPodPanel(p.app, fmt.Sprintf(" %c Pods ", ui.Icons.Package))
	p.podPanel.DrawHeader(podColumnsToDisplay)

	p.children = []tview.Primitive{
		p.clusterSummaryPanel.GetRootView(),
		p.nodePanel.GetRootView(),
		p.podPanel.GetRootView(),
	}

	view := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.clusterSummaryPanel.GetRootView(), 4, 1, true).
		AddItem(p.nodePanel.GetRootView(), 15, 1, true).
		AddItem(p.podPanel.GetRootView(), 0, 1, true)

	p.root = view
}

func (p *MainPanel) DrawHeader(_ interface{}) {}
func (p *MainPanel) DrawBody(_ interface{})   {}
func (p *MainPanel) DrawFooter(_ interface{}) {}
func (p *MainPanel) Clear()                   {}

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
	ctrl.SetMetricsSource(p.metricsSource) // Provide metrics source to controller for cluster summary
	ctrl.SetClusterSummaryRefreshFunc(p.refreshWorkloadSummary)
	ctrl.SetNodeRefreshFunc(p.refreshNodeView)
	ctrl.SetPodRefreshFunc(p.refreshPods)

	if err := ctrl.Start(ctx, time.Second*10); err != nil {
		panic(fmt.Sprintf("main panel: controller start: %s", err))
	}
	return nil
}

func (p *MainPanel) refreshNodeView(ctx context.Context, models []model.NodeModel) error {
	// The controller passes us models, but we need to rebuild them with fresh metrics
	// from our MetricsSource. We'll extract the node objects from the models.

	// For now, use a simpler approach: update metrics in the existing models
	// This requires accessing the original node objects, which the controller has via informers.
	//
	// Since we don't have direct access to Get methods, we'll work with what we have:
	// The models already contain node information, we just need to update their metrics.

	nodeModels := make([]model.NodeModel, 0, len(models))
	for _, existingModel := range models {
		// Fetch fresh metrics from our MetricsSource (if available)
		if p.metricsSource != nil {
			nodeMetrics, err := p.metricsSource.GetNodeMetrics(ctx, existingModel.Name)
			if err == nil {
				// Update the model's metrics fields
				existingModel.UsageCpuQty = nodeMetrics.CPUUsage
				existingModel.UsageMemQty = nodeMetrics.MemoryUsage
			}
			// If err != nil, graceful degradation: keep existing model as-is
		}
		// If metricsSource is nil, keep existing model as-is (fallback mode)

		nodeModels = append(nodeModels, existingModel)
	}

	model.SortNodeModels(nodeModels)

	p.nodePanel.Clear()
	p.nodePanel.DrawBody(nodeModels)

	// required: always schedule screen refresh
	if p.refresh != nil {
		p.refresh()
	}

	return nil
}

func (p *MainPanel) refreshPods(ctx context.Context, models []model.PodModel) error {
	// The controller passes us models, but we need to update them with fresh metrics
	// from our MetricsSource.

	// OPTIMIZATION: Fetch ALL pod metrics in a single batch API call
	// instead of making 200 individual calls (one per pod)
	var allPodMetrics []*metrics.PodMetrics
	var err error

	if p.metricsSource != nil {
		allPodMetrics, err = p.metricsSource.GetAllPodMetrics(ctx)
	}

	if p.metricsSource == nil || err != nil {
		// Fallback: if metrics source is nil or batch fetch fails, use existing models without metrics updates
		model.SortPodModels(models)
		p.podPanel.Clear()
		p.podPanel.DrawBody(models)
		if p.refresh != nil {
			p.refresh()
		}
		return nil
	}

	// Build a map for fast lookup: namespace/name -> PodMetrics
	metricsMap := make(map[string]*metrics.PodMetrics, len(allPodMetrics))
	for _, pm := range allPodMetrics {
		key := pm.Namespace + "/" + pm.PodName
		metricsMap[key] = pm
	}

	// Update models with metrics from the map
	podModels := make([]model.PodModel, 0, len(models))
	for _, existingModel := range models {
		key := existingModel.Namespace + "/" + existingModel.Name
		if podMetrics, found := metricsMap[key]; found {
			// Convert metrics and sum up CPU/Memory for all containers
			v1PodMetrics := convertToV1Beta1PodMetrics(podMetrics)

			// Update the model's usage metrics only if we got actual metrics
			totalCpu, totalMem := podMetricsTotals(v1PodMetrics)

			// If metrics are available (non-zero), use them
			// Otherwise keep the existing model which may have requests/limits
			if totalCpu.Value() > 0 || totalMem.Value() > 0 {
				existingModel.PodUsageCpuQty = totalCpu
				existingModel.PodUsageMemQty = totalMem
			}
		}
		// If no metrics found in map, keep existing model's values (requests/limits from controller)

		podModels = append(podModels, existingModel)
	}

	model.SortPodModels(podModels)

	// refresh pod list
	p.podPanel.Clear()
	p.podPanel.DrawBody(podModels)

	// required: always refresh screen
	if p.refresh != nil {
		p.refresh()
	}
	return nil
}

// podMetricsTotals sums up CPU and memory usage across all containers in a pod
// This helper is copied from model/pod_model.go to avoid import cycles
func podMetricsTotals(podMetrics *metricsV1beta1.PodMetrics) (totalCpu, totalMem *resource.Quantity) {
	totalCpu = resource.NewQuantity(0, resource.DecimalSI)
	totalMem = resource.NewQuantity(0, resource.DecimalSI)

	if podMetrics == nil {
		return
	}

	for _, c := range podMetrics.Containers {
		if cpu := c.Usage.Cpu(); cpu != nil {
			totalCpu.Add(*cpu)
		}
		if mem := c.Usage.Memory(); mem != nil {
			totalMem.Add(*mem)
		}
	}

	return
}

func (p *MainPanel) refreshWorkloadSummary(ctx context.Context, summary model.ClusterSummary) error {
	p.clusterSummaryPanel.Clear()
	p.clusterSummaryPanel.DrawBody(summary)
	if p.refresh != nil {
		p.refresh()
	}
	return nil
}

// filterColumns filters the allColumns based on the user-provided filterCols
// It returns a slice of columns that match the case-insensitive filter
func filterColumns(allColumns []string, filterCols []string) []string {
	if len(filterCols) == 0 {
		return allColumns
	}

	result := []string{}
	for _, col := range allColumns {
		for _, filterCol := range filterCols {
			if strings.EqualFold(col, filterCol) {
				result = append(result, col)
				break
			}
		}
	}

	// If no matches found, return at least the first column (usually NAME)
	if len(result) == 0 && len(allColumns) > 0 {
		return []string{allColumns[0]}
	}

	return result
}

// convertToV1Beta1NodeMetrics converts metrics.NodeMetrics to v1beta1.NodeMetrics
// This allows us to use the new MetricsSource interface with existing NodeModel constructors
func convertToV1Beta1NodeMetrics(nm *metrics.NodeMetrics) *metricsV1beta1.NodeMetrics {
	if nm == nil {
		return &metricsV1beta1.NodeMetrics{}
	}

	usage := v1.ResourceList{}
	if nm.CPUUsage != nil {
		usage[v1.ResourceCPU] = *nm.CPUUsage
	}
	if nm.MemoryUsage != nil {
		usage[v1.ResourceMemory] = *nm.MemoryUsage
	}

	return &metricsV1beta1.NodeMetrics{
		ObjectMeta: metav1.ObjectMeta{Name: nm.NodeName},
		Timestamp:  metav1.NewTime(nm.Timestamp),
		Window:     metav1.Duration{Duration: 0},
		Usage:      usage,
	}
}

// convertToV1Beta1PodMetrics converts metrics.PodMetrics to v1beta1.PodMetrics
// This allows us to use the new MetricsSource interface with existing PodModel constructors
func convertToV1Beta1PodMetrics(pm *metrics.PodMetrics) *metricsV1beta1.PodMetrics {
	if pm == nil {
		return &metricsV1beta1.PodMetrics{}
	}

	containers := make([]metricsV1beta1.ContainerMetrics, 0, len(pm.Containers))
	for _, c := range pm.Containers {
		usage := v1.ResourceList{}
		if c.CPUUsage != nil {
			usage[v1.ResourceCPU] = *c.CPUUsage
		}
		if c.MemoryUsage != nil {
			usage[v1.ResourceMemory] = *c.MemoryUsage
		}

		containers = append(containers, metricsV1beta1.ContainerMetrics{
			Name:  c.Name,
			Usage: usage,
		})
	}

	return &metricsV1beta1.PodMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pm.PodName,
			Namespace: pm.Namespace,
		},
		Timestamp:  metav1.NewTime(pm.Timestamp),
		Window:     metav1.Duration{Duration: 0},
		Containers: containers,
	}
}
