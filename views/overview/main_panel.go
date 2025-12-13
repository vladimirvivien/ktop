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
	nodedetail "github.com/vladimirvivien/ktop/views/node"
	poddetail "github.com/vladimirvivien/ktop/views/pod"
	v1 "k8s.io/api/core/v1"
	// metrics package imported for SourceTypePrometheus constant
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
	childPanels         []ui.Panel // Ordered list of child panels for focus management
	selPanelIndex       int
	nodePanel           ui.Panel
	podPanel            ui.Panel
	clusterSummaryPanel ui.Panel
	showAllColumns      bool
	nodeColumns         []string
	podColumns          []string
	namespaceFilter     string            // Current namespace filter
	cachedPodModels     []model.PodModel  // Cached pod models for immediate re-filtering
	cachedNodeModels    []model.NodeModel // Cached node models for detail view

	// Detail panels
	nodeDetailPanel *nodedetail.DetailPanel
	podDetailPanel  *poddetail.DetailPanel

	// Track currently displayed detail view for live updates
	currentDetailNodeName string // Non-empty when node detail is displayed
	currentDetailPodKey   string // "namespace/name" when pod detail is displayed
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
	allNodeColumns := []string{"NAME", "STATUS", "RST", "PODS", "TAINTS", "PRESSURE", "IP", "VOLS", "DISK", "CPU", "MEM"}
	allPodColumns := []string{"NAMESPACE", "POD", "READY", "STATUS", "RST", "AGE", "VOLS", "IP", "NODE", "CPU", "MEMORY"}

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

	p.nodePanel = NewNodePanel(p.app, fmt.Sprintf(" %s Nodes ", ui.Icons.Factory))
	p.nodePanel.DrawHeader(nodeColumnsToDisplay)

	// Set up node selection callback for navigation
	if np, ok := p.nodePanel.(*nodePanel); ok {
		np.SetOnNodeSelected(func(nodeName string) {
			p.app.NavigateToNodeDetail(nodeName)
		})
	}

	p.clusterSummaryPanel = NewClusterSummaryPanel(p.app, fmt.Sprintf(" %s Cluster Summary ", ui.Icons.Thermometer))
	p.clusterSummaryPanel.Layout(nil)
	p.clusterSummaryPanel.DrawHeader(nil)

	p.podPanel = NewPodPanel(p.app, fmt.Sprintf(" %s Pods ", ui.Icons.Package))
	p.podPanel.DrawHeader(podColumnsToDisplay)

	// Set up pod selection callback for navigation
	if pp, ok := p.podPanel.(*podPanel); ok {
		pp.SetOnPodSelected(func(namespace, podName string) {
			p.app.NavigateToPodDetail(namespace, podName)
		})
	}

	p.children = []tview.Primitive{
		p.clusterSummaryPanel.GetRootView(),
		p.nodePanel.GetRootView(),
		p.podPanel.GetRootView(),
	}

	// Store panels in order for focus management
	p.childPanels = []ui.Panel{
		p.clusterSummaryPanel,
		p.nodePanel,
		p.podPanel,
	}

	// Determine summary panel height based on metrics source
	summaryHeight := 12 // Default for Metrics Server
	if p.metricsSource != nil {
		info := p.metricsSource.GetSourceInfo()
		if info.Type == metrics.SourceTypePrometheus {
			summaryHeight = 14 // Prometheus: stats + 4 sparklines + enhanced stats
		}
	}

	view := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.clusterSummaryPanel.GetRootView(), summaryHeight, 0, false).
		AddItem(p.nodePanel.GetRootView(), 0, 3, true). // 30% of remaining
		AddItem(p.podPanel.GetRootView(), 0, 7, true)   // 70% of remaining

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

// GetChildPanel returns the panel at the given index (for focus management)
func (p *MainPanel) GetChildPanel(index int) ui.Panel {
	if index < 0 || index >= len(p.childPanels) {
		return nil
	}
	return p.childPanels[index]
}

// HasEscapableState implements ui.EscapablePanel by checking child panels
func (p *MainPanel) HasEscapableState() bool {
	// Check node panel
	if escapable, ok := p.nodePanel.(ui.EscapablePanel); ok {
		if escapable.HasEscapableState() {
			return true
		}
	}
	// Check pod panel
	if escapable, ok := p.podPanel.(ui.EscapablePanel); ok {
		if escapable.HasEscapableState() {
			return true
		}
	}
	return false
}

// HandleEscape implements ui.EscapablePanel by delegating to child panels
func (p *MainPanel) HandleEscape() bool {
	// Try node panel first
	if escapable, ok := p.nodePanel.(ui.EscapablePanel); ok {
		if escapable.HandleEscape() {
			return true
		}
	}
	// Try pod panel
	if escapable, ok := p.podPanel.(ui.EscapablePanel); ok {
		if escapable.HandleEscape() {
			return true
		}
	}
	return false
}

func (p *MainPanel) Run(ctx context.Context) error {
	p.Layout(nil)
	ctrl := p.app.GetK8sClient().Controller()
	ctrl.SetMetricsSource(p.metricsSource) // Provide metrics source to controller for cluster summary
	ctrl.SetClusterSummaryRefreshFunc(p.refreshWorkloadSummary)
	ctrl.SetNodeRefreshFunc(p.refreshNodeView)
	ctrl.SetPodRefreshFunc(p.refreshPods)

	// Set up namespace filter callback to update filtering and immediately refresh pods
	p.app.SetNamespaceFilterCallback(func(namespace string) {
		p.namespaceFilter = namespace
		// Immediately re-filter and display pods with the new filter
		p.displayFilteredPods()
	})

	// Set up navigation callbacks on the app (detail panels created lazily on first use)
	p.app.SetNodeDetailCallback(p.showNodeDetail)
	p.app.SetPodDetailCallback(p.showPodDetail)

	if err := ctrl.Start(ctx, time.Second*10); err != nil {
		panic(fmt.Sprintf("main panel: controller start: %s", err))
	}
	return nil
}

// ensureNodeDetailPanel creates the node detail panel if not already created
func (p *MainPanel) ensureNodeDetailPanel() {
	if p.nodeDetailPanel != nil {
		return
	}
	p.nodeDetailPanel = nodedetail.NewDetailPanel()
	p.nodeDetailPanel.SetOnBack(func() {
		p.currentDetailNodeName = "" // Clear tracking on back navigation
		p.app.NavigateBack()
	})
	p.nodeDetailPanel.SetOnPodSelected(func(namespace, podName string) {
		p.app.NavigateToPodDetail(namespace, podName)
	})
	// Set up focus callback for tab cycling within the detail panel
	p.nodeDetailPanel.SetAppFocus(func(prim tview.Primitive) {
		p.app.Focus(prim)
	})
	p.app.AddDetailPage("node_detail", p.nodeDetailPanel.GetRootView())
}

// ensurePodDetailPanel creates the pod detail panel if not already created
func (p *MainPanel) ensurePodDetailPanel() {
	if p.podDetailPanel != nil {
		return
	}
	p.podDetailPanel = poddetail.NewDetailPanel()
	p.podDetailPanel.SetOnBack(func() {
		p.currentDetailPodKey = "" // Clear tracking on back navigation
		p.app.NavigateBack()
	})
	p.podDetailPanel.SetOnNodeNavigate(func(nodeName string) {
		p.app.NavigateToNodeDetail(nodeName)
	})
	p.app.AddDetailPage("pod_detail", p.podDetailPanel.GetRootView())
}

// showNodeDetail navigates to the node detail view
func (p *MainPanel) showNodeDetail(nodeName string) {
	// Ensure the detail panel exists (lazy initialization)
	p.ensureNodeDetailPanel()

	// Find the node model in cached data
	var nodeModel *model.NodeModel
	for i := range p.cachedNodeModels {
		if p.cachedNodeModels[i].Name == nodeName {
			// Copy the model to avoid pointer to slice element issues
			nm := p.cachedNodeModels[i]
			nodeModel = &nm
			break
		}
	}

	if nodeModel == nil {
		return // Node not found
	}

	// Find pods on this node
	var podsOnNode []*model.PodModel
	for i := range p.cachedPodModels {
		if p.cachedPodModels[i].Node == nodeName {
			pm := p.cachedPodModels[i]
			podsOnNode = append(podsOnNode, &pm)
		}
	}

	// Create detail data
	detailData := &model.NodeDetailData{
		NodeModel:  nodeModel,
		PodsOnNode: podsOnNode,
	}

	ctx := context.Background()

	// Fetch metrics history for sparklines (if available)
	if p.metricsSource != nil && p.metricsSource.SupportsHistory() {
		historyDuration := 5 * time.Minute
		sparklineWidth := 15 // Number of data points for sparkline

		// Fetch CPU history
		cpuHistory, err := p.metricsSource.GetNodeHistory(ctx, nodeName, metrics.HistoryQuery{
			Resource:  metrics.ResourceCPU,
			Duration:  historyDuration,
			MaxPoints: sparklineWidth,
		})

		// Fetch Memory history
		memHistory, err2 := p.metricsSource.GetNodeHistory(ctx, nodeName, metrics.HistoryQuery{
			Resource:  metrics.ResourceMemory,
			Duration:  historyDuration,
			MaxPoints: sparklineWidth,
		})

		// Convert to MetricSample format for the detail view
		if err == nil && cpuHistory != nil && err2 == nil && memHistory != nil {
			// Get allocatable values for calculating ratios
			allocCPU := int64(1) // Default to avoid division by zero
			allocMem := int64(1)
			if nodeModel.AllocatableCpuQty != nil {
				allocCPU = nodeModel.AllocatableCpuQty.MilliValue()
			}
			if nodeModel.AllocatableMemQty != nil {
				allocMem = nodeModel.AllocatableMemQty.Value()
			}

			// Merge CPU and memory history into MetricSamples
			detailData.MetricsHistory = mergeHistoryToSamples(cpuHistory, memHistory, allocCPU, allocMem)
		}
	}

	// Fetch the raw Node object for conditions, labels, etc.
	ctrl := p.app.GetK8sClient().Controller()
	if node, err := ctrl.GetNode(ctx, nodeName); err == nil {
		detailData.Node = node
	}

	// Fetch events for this node
	if events, err := ctrl.GetEventsForNode(ctx, nodeName); err == nil {
		detailData.Events = events
	}

	// Track that we're showing node detail for live updates
	p.currentDetailNodeName = nodeName
	p.currentDetailPodKey = "" // Clear pod tracking

	// Update and show the detail panel
	p.nodeDetailPanel.DrawBody(detailData)
	p.app.ShowDetailPage("node_detail")
	p.nodeDetailPanel.InitFocus() // Set up initial focus on events panel
}

// showPodDetail navigates to the pod detail view
func (p *MainPanel) showPodDetail(namespace, podName string) {
	// Ensure the detail panel exists (lazy initialization)
	p.ensurePodDetailPanel()

	// Find the pod model in cached data
	var podModel *model.PodModel
	for i := range p.cachedPodModels {
		if p.cachedPodModels[i].Namespace == namespace && p.cachedPodModels[i].Name == podName {
			podModel = &p.cachedPodModels[i]
			break
		}
	}

	if podModel == nil {
		return // Pod not found
	}

	// Create detail data
	detailData := &model.PodDetailData{
		PodModel: podModel,
	}

	ctx := context.Background()

	// Fetch metrics history for sparklines (if available)
	if p.metricsSource != nil && p.metricsSource.SupportsHistory() {
		historyDuration := 5 * time.Minute
		sparklineWidth := 15 // Number of data points for sparkline

		// Fetch CPU history
		cpuHistory, err := p.metricsSource.GetPodHistory(ctx, namespace, podName, metrics.HistoryQuery{
			Resource:  metrics.ResourceCPU,
			Duration:  historyDuration,
			MaxPoints: sparklineWidth,
		})

		// Fetch Memory history
		memHistory, err2 := p.metricsSource.GetPodHistory(ctx, namespace, podName, metrics.HistoryQuery{
			Resource:  metrics.ResourceMemory,
			Duration:  historyDuration,
			MaxPoints: sparklineWidth,
		})

		// Convert to MetricSample format for the detail view
		if err == nil && cpuHistory != nil && err2 == nil && memHistory != nil {
			// For pods, we use requested resources as the baseline for ratios
			// Default values if no requests are set
			allocCPU := int64(1000)    // 1 core default
			allocMem := int64(1 << 30) // 1 GB default
			if podModel.PodRequestedCpuQty != nil && podModel.PodRequestedCpuQty.MilliValue() > 0 {
				allocCPU = podModel.PodRequestedCpuQty.MilliValue()
			} else if podModel.NodeAllocatableCpuQty != nil && podModel.NodeAllocatableCpuQty.MilliValue() > 0 {
				// Fallback to node allocatable for pods without requests
				allocCPU = podModel.NodeAllocatableCpuQty.MilliValue()
			}
			if podModel.PodRequestedMemQty != nil && podModel.PodRequestedMemQty.Value() > 0 {
				allocMem = podModel.PodRequestedMemQty.Value()
			} else if podModel.NodeAllocatableMemQty != nil && podModel.NodeAllocatableMemQty.Value() > 0 {
				// Fallback to node allocatable for pods without requests
				allocMem = podModel.NodeAllocatableMemQty.Value()
			}

			// Store as pod-level aggregate (single entry in map)
			samples := mergeHistoryToSamples(cpuHistory, memHistory, allocCPU, allocMem)
			if len(samples) > 0 {
				detailData.MetricsHistory = map[string][]model.MetricSample{
					"pod": samples,
				}
			}
		}
	}

	// Fetch events for this pod
	ctrl := p.app.GetK8sClient().Controller()
	if events, err := ctrl.GetEventsForPod(ctx, namespace, podName); err == nil {
		detailData.Events = events
	}

	// Track that we're showing pod detail for live updates
	p.currentDetailPodKey = namespace + "/" + podName
	p.currentDetailNodeName = "" // Clear node tracking

	// Update and show the detail panel
	p.podDetailPanel.DrawBody(detailData)
	p.app.ShowDetailPage("pod_detail")
	p.app.Focus(p.podDetailPanel.GetRootView())
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

	// Pre-fetch node detail data if detail view is visible (do network calls outside QueueUpdateDraw)
	var nodeDetailData *model.NodeDetailData
	if p.currentDetailNodeName != "" && p.nodeDetailPanel != nil {
		nodeDetailData = p.buildNodeDetailData(ctx, p.currentDetailNodeName, nodeModels)
	}

	// Queue UI update on main goroutine to avoid race with Draw()
	// This is called from controller goroutine, so we must synchronize
	p.app.QueueUpdateDraw(func() {
		// Cache the updated models for detail view access
		p.cachedNodeModels = nodeModels
		p.nodePanel.Clear()
		p.nodePanel.DrawBody(nodeModels)

		// If node detail is currently displayed, update it with pre-fetched data
		if nodeDetailData != nil && p.nodeDetailPanel != nil {
			p.nodeDetailPanel.DrawBody(nodeDetailData)
		}
	})

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

	// Update models with metrics first (before caching)
	updatedModels := models
	if p.metricsSource != nil && err == nil {
		// Build a map for fast lookup: namespace/name -> PodMetrics
		metricsMap := make(map[string]*metrics.PodMetrics, len(allPodMetrics))
		for _, pm := range allPodMetrics {
			key := pm.Namespace + "/" + pm.PodName
			metricsMap[key] = pm
		}

		// Update models with metrics from the map
		updatedModels = make([]model.PodModel, 0, len(models))
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
			updatedModels = append(updatedModels, existingModel)
		}
	}

	// Pre-fetch pod detail data if detail view is visible (do network calls outside QueueUpdateDraw)
	var podDetailData *model.PodDetailData
	if p.currentDetailPodKey != "" && p.podDetailPanel != nil {
		podDetailData = p.buildPodDetailData(ctx, p.currentDetailPodKey, updatedModels)
	}

	// Queue UI update on main goroutine to avoid race with Draw()
	// This is called from controller goroutine, so we must synchronize
	p.app.QueueUpdateDraw(func() {
		// Cache the updated models (with metrics) for immediate re-filtering
		p.cachedPodModels = updatedModels
		// Apply namespace filter and display
		p.displayFilteredPodsInternal()

		// If pod detail is currently displayed, update it with pre-fetched data
		if podDetailData != nil && p.podDetailPanel != nil {
			p.podDetailPanel.DrawBody(podDetailData)
		}
	})

	return nil
}

// displayFilteredPods filters cached pod models and displays them
// Called from namespace filter callback (runs on main goroutine)
func (p *MainPanel) displayFilteredPods() {
	if p.cachedPodModels == nil {
		return
	}

	// Already on main goroutine from input handler, safe to update directly
	p.displayFilteredPodsInternal()

	// Refresh screen
	if p.refresh != nil {
		p.refresh()
	}
}

// displayFilteredPodsInternal does the actual filtering and display
// Must be called from main goroutine (either directly or via QueueUpdateDraw)
func (p *MainPanel) displayFilteredPodsInternal() {
	if p.cachedPodModels == nil {
		return
	}

	// Apply namespace filter
	filteredModels := p.cachedPodModels
	if p.namespaceFilter != "" {
		filteredModels = make([]model.PodModel, 0, len(p.cachedPodModels))
		filterLower := strings.ToLower(p.namespaceFilter)
		for _, m := range p.cachedPodModels {
			if strings.Contains(strings.ToLower(m.Namespace), filterLower) {
				filteredModels = append(filteredModels, m)
			}
		}
	}

	// Sorting is now handled by the panel itself in DrawBody
	p.podPanel.Clear()
	p.podPanel.DrawBody(filteredModels)
}

// buildNodeDetailData builds the node detail data for live updates.
// This performs network calls and must be called outside QueueUpdateDraw.
func (p *MainPanel) buildNodeDetailData(ctx context.Context, nodeName string, nodeModels []model.NodeModel) *model.NodeDetailData {
	if nodeName == "" {
		return nil
	}

	// Find the node model in the provided models
	var nodeModel *model.NodeModel
	for i := range nodeModels {
		if nodeModels[i].Name == nodeName {
			// Copy the model to avoid pointer to slice element issues
			nm := nodeModels[i]
			nodeModel = &nm
			break
		}
	}

	if nodeModel == nil {
		return nil // Node no longer exists
	}

	// Find pods on this node from cached pods
	var podsOnNode []*model.PodModel
	for i := range p.cachedPodModels {
		if p.cachedPodModels[i].Node == nodeName {
			pm := p.cachedPodModels[i]
			podsOnNode = append(podsOnNode, &pm)
		}
	}

	// Create detail data
	detailData := &model.NodeDetailData{
		NodeModel:  nodeModel,
		PodsOnNode: podsOnNode,
	}

	// Fetch metrics history for sparklines (if available)
	if p.metricsSource != nil && p.metricsSource.SupportsHistory() {
		historyDuration := 5 * time.Minute
		sparklineWidth := 15

		cpuHistory, err := p.metricsSource.GetNodeHistory(ctx, nodeName, metrics.HistoryQuery{
			Resource:  metrics.ResourceCPU,
			Duration:  historyDuration,
			MaxPoints: sparklineWidth,
		})

		memHistory, err2 := p.metricsSource.GetNodeHistory(ctx, nodeName, metrics.HistoryQuery{
			Resource:  metrics.ResourceMemory,
			Duration:  historyDuration,
			MaxPoints: sparklineWidth,
		})

		if err == nil && cpuHistory != nil && err2 == nil && memHistory != nil {
			allocCPU := int64(1)
			allocMem := int64(1)
			if nodeModel.AllocatableCpuQty != nil {
				allocCPU = nodeModel.AllocatableCpuQty.MilliValue()
			}
			if nodeModel.AllocatableMemQty != nil {
				allocMem = nodeModel.AllocatableMemQty.Value()
			}

			detailData.MetricsHistory = mergeHistoryToSamples(cpuHistory, memHistory, allocCPU, allocMem)
		}
	}

	// Fetch the raw Node object for conditions, labels, etc.
	ctrl := p.app.GetK8sClient().Controller()
	if node, err := ctrl.GetNode(ctx, nodeName); err == nil {
		detailData.Node = node
	}

	// Fetch events for this node
	if events, err := ctrl.GetEventsForNode(ctx, nodeName); err == nil {
		detailData.Events = events
	}

	return detailData
}

// buildPodDetailData builds the pod detail data for live updates.
// This performs network calls and must be called outside QueueUpdateDraw.
func (p *MainPanel) buildPodDetailData(ctx context.Context, podKey string, podModels []model.PodModel) *model.PodDetailData {
	if podKey == "" {
		return nil
	}

	// Parse namespace/name from key
	parts := strings.SplitN(podKey, "/", 2)
	if len(parts) != 2 {
		return nil
	}
	namespace, podName := parts[0], parts[1]

	// Find the pod model in provided models
	var podModel *model.PodModel
	for i := range podModels {
		if podModels[i].Namespace == namespace && podModels[i].Name == podName {
			podModel = &podModels[i]
			break
		}
	}

	if podModel == nil {
		return nil // Pod no longer exists
	}

	// Create detail data
	detailData := &model.PodDetailData{
		PodModel: podModel,
	}

	// Fetch metrics history for sparklines (if available)
	if p.metricsSource != nil && p.metricsSource.SupportsHistory() {
		historyDuration := 5 * time.Minute
		sparklineWidth := 15

		cpuHistory, err := p.metricsSource.GetPodHistory(ctx, namespace, podName, metrics.HistoryQuery{
			Resource:  metrics.ResourceCPU,
			Duration:  historyDuration,
			MaxPoints: sparklineWidth,
		})

		memHistory, err2 := p.metricsSource.GetPodHistory(ctx, namespace, podName, metrics.HistoryQuery{
			Resource:  metrics.ResourceMemory,
			Duration:  historyDuration,
			MaxPoints: sparklineWidth,
		})

		if err == nil && cpuHistory != nil && err2 == nil && memHistory != nil {
			allocCPU := int64(1000)
			allocMem := int64(1 << 30)
			if podModel.PodRequestedCpuQty != nil && podModel.PodRequestedCpuQty.MilliValue() > 0 {
				allocCPU = podModel.PodRequestedCpuQty.MilliValue()
			} else if podModel.NodeAllocatableCpuQty != nil && podModel.NodeAllocatableCpuQty.MilliValue() > 0 {
				allocCPU = podModel.NodeAllocatableCpuQty.MilliValue()
			}
			if podModel.PodRequestedMemQty != nil && podModel.PodRequestedMemQty.Value() > 0 {
				allocMem = podModel.PodRequestedMemQty.Value()
			} else if podModel.NodeAllocatableMemQty != nil && podModel.NodeAllocatableMemQty.Value() > 0 {
				allocMem = podModel.NodeAllocatableMemQty.Value()
			}

			samples := mergeHistoryToSamples(cpuHistory, memHistory, allocCPU, allocMem)
			if len(samples) > 0 {
				detailData.MetricsHistory = map[string][]model.MetricSample{
					"pod": samples,
				}
			}
		}
	}

	// Fetch events for this pod
	ctrl := p.app.GetK8sClient().Controller()
	if events, err := ctrl.GetEventsForPod(ctx, namespace, podName); err == nil {
		detailData.Events = events
	}

	return detailData
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
	// Queue UI update on main goroutine to avoid race with Draw()
	// This is called from controller goroutine, so we must synchronize
	p.app.QueueUpdateDraw(func() {
		p.clusterSummaryPanel.Clear()
		p.clusterSummaryPanel.DrawBody(summary)
	})
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

// mergeHistoryToSamples converts ResourceHistory data points to MetricSamples for sparklines.
// CPU values are in millicores, memory values are in bytes.
// allocCPU is in millicores, allocMem is in bytes.
func mergeHistoryToSamples(cpuHistory, memHistory *metrics.ResourceHistory, allocCPU, allocMem int64) []model.MetricSample {
	// Use the shorter of the two histories
	cpuLen := 0
	memLen := 0
	if cpuHistory != nil {
		cpuLen = len(cpuHistory.DataPoints)
	}
	if memHistory != nil {
		memLen = len(memHistory.DataPoints)
	}

	// Determine result size
	resultLen := cpuLen
	if memLen < resultLen {
		resultLen = memLen
	}
	if resultLen == 0 {
		return nil
	}

	samples := make([]model.MetricSample, resultLen)
	for i := 0; i < resultLen; i++ {
		sample := model.MetricSample{}

		// CPU ratio: value is in millicores, divide by allocatable millicores
		if cpuHistory != nil && i < len(cpuHistory.DataPoints) {
			dp := cpuHistory.DataPoints[i]
			sample.Timestamp = dp.Timestamp.UnixMilli()
			if allocCPU > 0 {
				sample.CPURatio = dp.Value / float64(allocCPU)
				if sample.CPURatio > 1.0 {
					sample.CPURatio = 1.0
				}
			}
		}

		// Memory ratio: value is in bytes, divide by allocatable bytes
		if memHistory != nil && i < len(memHistory.DataPoints) {
			dp := memHistory.DataPoints[i]
			if sample.Timestamp == 0 {
				sample.Timestamp = dp.Timestamp.UnixMilli()
			}
			if allocMem > 0 {
				sample.MemRatio = dp.Value / float64(allocMem)
				if sample.MemRatio > 1.0 {
					sample.MemRatio = 1.0
				}
			}
		}

		samples[i] = sample
	}

	return samples
}
