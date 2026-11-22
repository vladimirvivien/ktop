package overview

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/config"
	"github.com/vladimirvivien/ktop/internal/snapshot"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	containerdetail "github.com/vladimirvivien/ktop/views/container"
	"github.com/vladimirvivien/ktop/views/model"
	nodedetail "github.com/vladimirvivien/ktop/views/node"
	poddetail "github.com/vladimirvivien/ktop/views/pod"
	v1 "k8s.io/api/core/v1"
	// metrics package imported for SourceTypePrometheus constant
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const podInspectorModalID = "pod-inspector"

type MainPanel struct {
	app                 *application.Application
	metricsSource       metrics.MetricsSource
	snapshotBuilder     *snapshot.Builder
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

	// Detail panels & shared Pod/Container Inspector modal
	nodeDetailPanel      *nodedetail.DetailPanel
	podDetailPanel       *poddetail.DetailPanel
	containerDetailPanel *containerdetail.DetailPanel
	podInspectorOverlay  *ui.OverlayFrame
	podInspectorPages    *tview.Pages
	podInspectorOpen     bool
	activeModalPodNS     string
	activeModalPodName   string

	// Centralized view state manager - single source of truth for current page/resource
	// Thread-safe, replaces individual tracking variables
	viewState *ViewStateManager

	// Dynamic layout tracking
	lastHeightCategory int // 0=small, 1=medium, 2=large, 3=extraLarge - for detecting resize category changes
	lastTerminalHeight int // Track height for dynamic resizing in ExtraLarge category

	initialNodeSortCol string
	initialNodeSortAsc bool
	initialPodSortCol  string
	initialPodSortAsc  bool
}

// SetInitialSort sets the initial sort column and direction for nodes and pods tables
func (p *MainPanel) SetInitialSort(nodeCol string, nodeAsc bool, podCol string, podAsc bool) {
	p.initialNodeSortCol = nodeCol
	p.initialNodeSortAsc = nodeAsc
	p.initialPodSortCol = podCol
	p.initialPodSortAsc = podAsc
}

func New(app *application.Application, title string) *MainPanel {
	return NewWithColumnOptions(app, title, false, nil, nil)
}

func NewWithColumnOptions(app *application.Application, title string, showAllColumns bool, nodeColumns, podColumns []string) *MainPanel {
	ctrl := &MainPanel{
		app:            app,
		title:          title,
		selPanelIndex:  -1,
		showAllColumns: showAllColumns,
		nodeColumns:    nodeColumns,
		podColumns:     podColumns,
		viewState:      NewViewStateManager(), // Centralized state tracking
	}

	if app != nil {
		ctrl.metricsSource = app.GetMetricsSource()
		ctrl.refresh = app.Refresh
	}

	return ctrl
}

func (p *MainPanel) Layout(data any) {
	nodeColumnsToDisplay := config.DefaultNodeColumns
	podColumnsToDisplay := config.DefaultPodColumns

	if p.showAllColumns {
		nodeColumnsToDisplay = config.AllNodeColumns
		podColumnsToDisplay = config.AllPodColumns
	} else {
		if len(p.nodeColumns) > 0 {
			nodeColumnsToDisplay = filterColumns(config.AllNodeColumns, p.nodeColumns)
		}

		if len(p.podColumns) > 0 {
			podColumnsToDisplay = filterColumns(config.AllPodColumns, p.podColumns)
		}
	}

	p.nodePanel = NewNodePanel(p.app, fmt.Sprintf(" %s Nodes ", ui.Icons.Factory))
	p.nodePanel.DrawHeader(nodeColumnsToDisplay)

	// Set up node selection and column configuration callback
	if np, ok := p.nodePanel.(*nodePanel); ok {
		if p.initialNodeSortCol != "" {
			np.tableCtrl.SetSortParams(p.initialNodeSortCol, p.initialNodeSortAsc)
		}
		np.SetOnNodeSelected(func(nodeName string) {
			p.app.NavigateToNodeDetail(nodeName)
		})
		np.SetOnConfigureColumns(func() {
			p.openColumnSelector("Nodes", config.AllNodeColumns, np.GetVisibleColumns(), func(selected []string) {
				p.nodeColumns = selected
				p.showAllColumns = false
				np.SetVisibleColumns(selected)
				if err := config.SaveOverviewColumns(p.nodeColumns, p.podColumns); err != nil {
					slog.Warn("failed to persist node columns", "error", err)
				}
			})
		})
	}

	p.clusterSummaryPanel = NewClusterSummaryPanel(p.app, fmt.Sprintf(" %s Cluster Summary ", ui.Icons.Thermometer))
	p.clusterSummaryPanel.Layout(nil)
	p.clusterSummaryPanel.DrawHeader(nil)

	p.podPanel = NewPodPanel(p.app, fmt.Sprintf(" %s Pods ", ui.Icons.Package))
	p.podPanel.DrawHeader(podColumnsToDisplay)

	// Set up pod selection and column configuration callback
	if pp, ok := p.podPanel.(*podPanel); ok {
		if p.initialPodSortCol != "" {
			pp.tableCtrl.SetSortParams(p.initialPodSortCol, p.initialPodSortAsc)
		}
		pp.SetOnPodSelected(func(namespace, podName string) {
			p.app.NavigateToPodDetail(namespace, podName)
		})
		pp.SetOnConfigureColumns(func() {
			p.openColumnSelector("Pods", config.AllPodColumns, pp.GetVisibleColumns(), func(selected []string) {
				p.podColumns = selected
				p.showAllColumns = false
				pp.SetVisibleColumns(selected)
				if err := config.SaveOverviewColumns(p.nodeColumns, p.podColumns); err != nil {
					slog.Warn("failed to persist pod columns", "error", err)
				}
			})
		})
	}

	p.children = []tview.Primitive{
		p.nodePanel.GetRootView(),
		p.podPanel.GetRootView(),
	}

	// Store panels in order for focus management
	p.childPanels = []ui.Panel{
		p.nodePanel,
		p.podPanel,
	}

	// Calculate dynamic panel heights based on terminal size
	// Use default height (50) during initial layout since root doesn't exist yet
	// The resize check in refreshWorkloadSummary will adjust once app is running
	isPrometheus := p.metricsSource != nil && p.metricsSource.GetSourceInfo().Type == metrics.SourceTypePrometheus
	terminalHeight := 50 // Default to medium during initial layout
	summaryHeight, nodeHeight := p.calculatePanelHeights(terminalHeight, isPrometheus)
	p.lastHeightCategory = ui.GetHeightCategory(terminalHeight)
	p.lastTerminalHeight = terminalHeight

	view := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.clusterSummaryPanel.GetRootView(), summaryHeight, 0, false).
		AddItem(p.nodePanel.GetRootView(), nodeHeight, 0, true).
		AddItem(p.podPanel.GetRootView(), 0, 1, true) // Takes remaining space

	p.root = view
}

// calculatePanelHeights returns summary and node panel heights based on terminal height
// Sparkline content heights are designed to match detail pages:
// - Small (≤45): 2 rows content
// - Medium+ (46+): 3 rows content
func (p *MainPanel) calculatePanelHeights(terminalHeight int, isPrometheus bool) (summaryHeight, nodeHeight int) {
	switch ui.GetHeightCategory(terminalHeight) {
	case ui.HeightCategoryTooSmall:
		// Below minimum - use minimal heights
		summaryHeight, nodeHeight = 5, 5
		return
	case ui.HeightCategorySmall:
		// Small: stats hidden, sparklines only with 2-row content (matches detail pages)
		// graphFlex=4 (2 content + 2 border) + outerBorder=2 = 6
		summaryHeight, nodeHeight = 6, 8
		return // Don't add Prometheus bonus at small heights
	case ui.HeightCategoryMedium:
		summaryHeight, nodeHeight = 10, 12
	case ui.HeightCategoryLarge:
		// Large: same summary height as medium, but more room for nodes
		summaryHeight, nodeHeight = 10, 15
	case ui.HeightCategoryExtraLarge:
		// ExtraLarge: Summary fixed, Nodes gets 45% of remaining space
		summaryHeight = 10
		if isPrometheus {
			summaryHeight = 13
		}
		// Available = terminal - header(3) - tabframe borders(2) - footer(3) - summary
		available := terminalHeight - 8 - summaryHeight
		// Nodes gets 45% of remaining, Pods gets the rest
		nodeHeight = available * 45 / 100
		return // Already handled Prometheus bonus above
	default:
		summaryHeight, nodeHeight = 10, 15
	}
	if isPrometheus {
		// Prometheus needs extra row for enhanced stats while maintaining 3-row sparklines
		// graphFlex=5 (3 content + 2 border) + stats=3 + enhanced=3 + outerBorder=2 = 13
		summaryHeight += 3
	}
	return
}

// checkAndRebuildLayout checks if terminal size category changed and rebuilds layout if needed
func (p *MainPanel) checkAndRebuildLayout() {
	terminalHeight := p.app.GetTerminalHeight()
	currentCategory := ui.GetHeightCategory(terminalHeight)

	// Check if we need to rebuild
	needsRebuild := false
	if currentCategory != p.lastHeightCategory {
		// Category changed - always rebuild
		needsRebuild = true
	} else if currentCategory == ui.HeightCategoryExtraLarge && terminalHeight != p.lastTerminalHeight {
		// In ExtraLarge, rebuild when height changes (dynamic sizing)
		needsRebuild = true
	}

	if !needsRebuild {
		return
	}

	isPrometheus := p.metricsSource != nil && p.metricsSource.GetSourceInfo().Type == metrics.SourceTypePrometheus
	summaryHeight, nodeHeight := p.calculatePanelHeights(terminalHeight, isPrometheus)

	// Clear and rebuild the flex layout
	p.root.Clear()
	p.root.AddItem(p.clusterSummaryPanel.GetRootView(), summaryHeight, 0, false)
	p.root.AddItem(p.nodePanel.GetRootView(), nodeHeight, 0, true)
	p.root.AddItem(p.podPanel.GetRootView(), 0, 1, true)

	p.lastHeightCategory = currentCategory
	p.lastTerminalHeight = terminalHeight
}

func (p *MainPanel) DrawHeader(_ any) {}
func (p *MainPanel) DrawBody(_ any)   {}
func (p *MainPanel) DrawFooter(_ any) {}
func (p *MainPanel) Clear()           {}

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

// IsFilterEditing returns true if any child panel is currently editing its filter
func (p *MainPanel) IsFilterEditing() bool {
	if np, ok := p.nodePanel.(interface{ IsFilterEditing() bool }); ok && np.IsFilterEditing() {
		return true
	}
	if pp, ok := p.podPanel.(interface{ IsFilterEditing() bool }); ok && pp.IsFilterEditing() {
		return true
	}
	return false
}

// GetActiveDetailPanel returns the currently active depth-1 detail subpage panel if any.
func (p *MainPanel) GetActiveDetailPanel() ui.EscapablePanel {
	if _, ok := p.viewState.GetNodeDetail(); ok && p.nodeDetailPanel != nil {
		return p.nodeDetailPanel
	}
	return nil
}

func (p *MainPanel) Run(ctx context.Context) error {
	p.Layout(nil)

	if p.app != nil {
		p.app.SetNamespaceFilterCallback(func(namespace string) {
			p.namespaceFilter = namespace
			p.displayFilteredPods()
		})

		p.app.SetNodeDetailCallback(p.showNodeDetail)
		p.app.SetPodDetailCallback(p.showPodDetail)
		p.app.SetContainerLogsCallback(p.showContainerLogs)
		p.app.SetDetailResetCallback(func() {
			p.viewState.SetOverview()
			p.closePodInspector()
		})

		if client := p.app.GetK8sClient(); client != nil {
			ctrl := client.Controller()
			ctrl.SetMetricsSource(p.metricsSource)
			ctrl.SetClusterSummaryRefreshFunc(p.refreshWorkloadSummary)
			ctrl.SetNodeRefreshFunc(p.refreshNodeView)
			ctrl.SetPodRefreshFunc(p.refreshPods)
		}
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
		p.viewState.SetOverview() // Clear tracking on back navigation
		p.app.NavigateBack()
	})
	p.nodeDetailPanel.SetOnPodSelected(func(namespace, podName string) {
		p.app.NavigateToPodDetail(namespace, podName)
	})
	p.nodeDetailPanel.SetOnShowYAML(func(node *v1.Node) {
		if node == nil || p.app == nil {
			return
		}
		nodeCopy := node.DeepCopy()
		nodeCopy.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Node"}
		yamlText, err := ui.FormatObjectYAML(nodeCopy)
		if err != nil {
			p.app.ShowToast(fmt.Sprintf("Unable to render Node YAML: %v", err), ui.ToastError, 3*time.Second)
			return
		}
		const nodeYAMLModalID = "node-yaml-modal"
		modal := ui.NewYAMLModal(ui.YAMLModalConfig{
			Title:   fmt.Sprintf("Node YAML: %s", node.Name),
			Content: yamlText,
			OnClose: func() {
				p.app.DismissModalPage(nodeYAMLModalID)
				if p.nodeDetailPanel != nil {
					p.nodeDetailPanel.InitFocus()
				}
			},
			OnYank: func(_ string) {
				p.app.ShowToast(fmt.Sprintf("Copied Node YAML: %s to clipboard", node.Name), ui.ToastSuccess, 2*time.Second)
			},
		})
		p.app.ShowModalPage(nodeYAMLModalID, modal.GetRootView())
	})
	// Set up focus callback for tab cycling within the detail panel
	p.nodeDetailPanel.SetAppFocus(func(prim tview.Primitive) {
		p.app.Focus(prim)
	})
	// Set up footer context callback for focus changes
	p.nodeDetailPanel.SetOnFooterContextChange(func(focusedPanel string) {
		p.app.SetFooterContext(ui.NodeDetailContext{FocusedPanel: focusedPanel})
	})
	p.app.AddDetailPage("node_detail", p.nodeDetailPanel.GetRootView())
}

func (p *MainPanel) ensurePodInspectorModal() {
	if p.podInspectorPages != nil {
		return
	}
	p.podInspectorPages = tview.NewPages()
	// Leave top header+tabs (4 rows), bottom footer (3 rows), and side parent edges (3 cols) visible.
	p.podInspectorOverlay = ui.NewOverlayFrame(p.podInspectorPages, 4, 3, 3, 3)
}

func (p *MainPanel) closePodInspector() {
	if p.containerDetailPanel != nil {
		p.containerDetailPanel.Cleanup()
	}
	p.viewState.ClearModal()
	if p.podInspectorOpen {
		p.podInspectorOpen = false
		if p.podInspectorPages != nil {
			p.podInspectorPages.RemovePage("yaml")
		}
		if p.app != nil {
			p.app.DismissModalPage(podInspectorModalID)
		}
	}
}

// ensurePodDetailPanel creates the pod detail panel hosted inside the shared Pod Inspector modal.
func (p *MainPanel) ensurePodDetailPanel() {
	if p.podDetailPanel != nil {
		return
	}
	p.ensurePodInspectorModal()
	p.podDetailPanel = poddetail.NewDetailPanel()
	p.podDetailPanel.SetOnBack(func() {
		p.closePodInspector()
	})
	p.podDetailPanel.SetOnContainerSelected(func(namespace, podName, containerName string) {
		p.app.NavigateToContainerLogs(namespace, podName, containerName)
	})
	p.podDetailPanel.SetOnShowYAML(func(pod *v1.Pod) {
		if pod == nil {
			return
		}
		podCopy := pod.DeepCopy()
		podCopy.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
		yamlText, err := ui.FormatObjectYAML(podCopy)
		if err != nil {
			p.app.ShowToast(fmt.Sprintf("Unable to render Pod YAML: %v", err), ui.ToastError, 3*time.Second)
			return
		}
		p.showInspectorYAML(fmt.Sprintf("Pod YAML: %s/%s", pod.Namespace, pod.Name), yamlText, "pod")
	})
	p.podDetailPanel.SetAppFocus(func(prim tview.Primitive) {
		p.app.Focus(prim)
	})
	p.podDetailPanel.SetOnFooterContextChange(func(focusedPanel string) {
		p.app.SetFooterContext(ui.PodDetailContext{FocusedPanel: focusedPanel})
	})
	p.podInspectorPages.AddPage("pod", p.podDetailPanel.GetRootView(), true, false)
}

// ensureContainerDetailPanel creates the container logs panel hosted inside the shared Pod Inspector modal.
func (p *MainPanel) ensureContainerDetailPanel() {
	if p.containerDetailPanel != nil {
		return
	}
	p.ensurePodInspectorModal()
	p.containerDetailPanel = containerdetail.NewDetailPanel()
	p.containerDetailPanel.SetOnBack(func() {
		p.containerDetailPanel.Cleanup()
		p.returnToPodModal()
	})
	p.containerDetailPanel.SetOnShowSpec(func(namespace, podName, containerName string, containerSpec *v1.Container) {
		yamlText, err := ui.FormatObjectYAML(containerSpec)
		if err != nil {
			p.app.ShowToast(fmt.Sprintf("Unable to render Container Spec: %v", err), ui.ToastError, 3*time.Second)
			return
		}
		p.showInspectorYAML(fmt.Sprintf("Container Spec: %s (%s/%s)", containerName, namespace, podName), yamlText, "logs")
	})
	p.containerDetailPanel.SetAppFocus(func(prim tview.Primitive) {
		p.app.Focus(prim)
	})
	p.containerDetailPanel.SetLogStreamFunc(func(ctx context.Context, namespace, podName string, opts k8s.LogOptions) (io.ReadCloser, error) {
		return p.app.GetK8sClient().Controller().GetPodLogs(ctx, namespace, podName, opts)
	})
	p.containerDetailPanel.SetGetPodFunc(func(ctx context.Context, namespace, podName string) (*v1.Pod, error) {
		return p.app.GetK8sClient().Controller().GetPod(ctx, namespace, podName)
	})
	p.containerDetailPanel.SetGetPodMetricsFunc(func(ctx context.Context, namespace, podName string) (*metrics.PodMetrics, error) {
		if p.metricsSource == nil {
			return nil, fmt.Errorf("no metrics source")
		}
		return p.metricsSource.GetPodMetrics(ctx, namespace, podName)
	})
	p.containerDetailPanel.SetGetTerminalHeightFunc(func() int {
		return p.app.GetTerminalHeight()
	})
	p.containerDetailPanel.SetQueueUpdateFunc(func(fn func()) {
		p.app.QueueUpdateDraw(fn)
	})
	p.containerDetailPanel.SetOnFooterContextChange(func(focusedPanel string) {
		p.app.SetFooterContext(ui.ContainerDetailContext{FocusedPanel: focusedPanel})
	})
	p.podInspectorPages.AddPage("logs", p.containerDetailPanel.GetRootView(), true, false)
}

func (p *MainPanel) returnToPodModal() {
	if p.podDetailPanel == nil || !p.podInspectorOpen {
		p.closePodInspector()
		return
	}
	p.viewState.SetPodModal(p.activeModalPodNS, p.activeModalPodName)
	p.podInspectorPages.SwitchToPage("pod")
	p.podDetailPanel.SetFocused(true)
	p.podDetailPanel.InitFocus()
	p.app.SetFooterContext(ui.PodDetailContext{FocusedPanel: "containers"})
}

func (p *MainPanel) showInspectorYAML(title, yamlText, returnPage string) {
	p.ensurePodInspectorModal()
	box, textView := ui.NewYAMLBox(ui.YAMLModalConfig{
		Title:   title,
		Content: yamlText,
		OnClose: func() {
			p.podInspectorPages.RemovePage("yaml")
			if returnPage == "logs" && p.containerDetailPanel != nil {
				p.podInspectorPages.SwitchToPage("logs")
				p.containerDetailPanel.SetFocused(true)
				p.containerDetailPanel.InitFocus()
				return
			}
			p.returnToPodModal()
		},
		OnYank: func(_ string) {
			p.app.ShowToast(fmt.Sprintf("Copied %s to clipboard", title), ui.ToastSuccess, 2*time.Second)
		},
	})
	p.podInspectorPages.RemovePage("yaml")
	p.podInspectorPages.AddAndSwitchToPage("yaml", box, true)
	p.app.Focus(textView)
}

// showContainerLogs transitions the shared Pod Inspector modal to container logs.
func (p *MainPanel) showContainerLogs(namespace, podName, containerName string) {
	p.ensureContainerDetailPanel()

	p.activeModalPodNS = namespace
	p.activeModalPodName = podName
	p.viewState.SetContainerModal(namespace, podName, containerName)

	p.containerDetailPanel.ShowContainer(namespace, podName, containerName)
	p.podInspectorPages.SwitchToPage("logs")
	if !p.podInspectorOpen {
		p.podInspectorOpen = true
		p.app.ShowModalPage(podInspectorModalID, p.podInspectorOverlay)
	}
	p.containerDetailPanel.SetFocused(true)
	p.containerDetailPanel.InitFocus()
}

// showNodeDetail navigates to the node detail view
func (p *MainPanel) showNodeDetail(nodeName string) {
	if p.containerDetailPanel != nil {
		p.containerDetailPanel.Cleanup()
	}
	// Ensure the detail panel exists (lazy initialization)
	p.ensureNodeDetailPanel()

	// Set tracking IMMEDIATELY to prevent race with refreshNodeView
	// If we set this after network calls, refreshNodeView might update with old node data
	p.viewState.SetNodeDetail(nodeName)

	detailData := p.buildNodeDetailData(context.Background(), nodeName, p.cachedNodeModels)
	if detailData == nil {
		return // Node not found
	}

	// Update and show the detail panel
	p.nodeDetailPanel.DrawBody(detailData)
	p.app.ShowDetailPage("node_detail")
	p.nodeDetailPanel.InitFocus() // Set up initial focus on events panel
}

// showPodDetail opens the shared floating Pod Inspector modal for the given pod.
func (p *MainPanel) showPodDetail(namespace, podName string) {
	if p.containerDetailPanel != nil {
		p.containerDetailPanel.Cleanup()
	}
	p.ensurePodDetailPanel()

	p.activeModalPodNS = namespace
	p.activeModalPodName = podName
	p.viewState.SetPodModal(namespace, podName)

	detailData := p.buildPodDetailData(context.Background(), namespace+"/"+podName, p.cachedPodModels)
	if detailData == nil {
		p.viewState.ClearModal()
		return
	}

	p.podDetailPanel.DrawBody(detailData)
	p.podInspectorPages.SwitchToPage("pod")
	if !p.podInspectorOpen {
		p.podInspectorOpen = true
		p.app.ShowModalPage(podInspectorModalID, p.podInspectorOverlay)
	}
	p.podDetailPanel.SetFocused(true)
	p.podDetailPanel.InitFocus()
	p.app.SetFooterContext(ui.PodDetailContext{FocusedPanel: "containers"})
}

func (p *MainPanel) refreshNodeView(ctx context.Context, models []model.NodeModel) error {
	// GetNodeModels already populates UsageCpuQty, UsageMemQty, and PSI from MetricsSource.
	nodeModels := models

	// Pre-fetch node detail data if detail view is visible (do network calls outside QueueUpdateDraw)
	// Use ViewStateManager for thread-safe state access
	// Capture the node name at fetch time so we can verify it later
	var nodeDetailData *model.NodeDetailData
	var fetchedNodeName string
	if nodeName, ok := p.viewState.GetNodeDetail(); ok && p.nodeDetailPanel != nil {
		fetchedNodeName = nodeName
		nodeDetailData = p.buildNodeDetailData(ctx, nodeName, nodeModels)
	}

	// Queue UI update on main goroutine to avoid race with Draw()
	// This is called from controller goroutine, so we must synchronize
	p.app.QueueUpdateDraw(func() {
		// Cache the updated models for detail view access
		p.cachedNodeModels = nodeModels
		p.nodePanel.Clear()
		if p.snapshotBuilder == nil {
			p.snapshotBuilder = snapshot.NewBuilder()
		}
		if snap, err := p.snapshotBuilder.Build(ctx, snapshot.BuildInput{Nodes: nodeModels}); err == nil {
			p.nodePanel.DrawBody(snap)
		} else {
			p.nodePanel.DrawBody(nodeModels)
		}

		// If node detail is currently displayed, update it with pre-fetched data
		// CRITICAL: Re-verify the view state matches what we fetched - user may have
		// navigated to a different node/page between fetch and now
		if nodeDetailData != nil && p.nodeDetailPanel != nil && fetchedNodeName != "" {
			if currentNodeName, ok := p.viewState.GetNodeDetail(); ok {
				if currentNodeName == fetchedNodeName {
					p.nodeDetailPanel.DrawBody(nodeDetailData)
				}
				// If names don't match, skip this update - the data is stale
			}
			// If not on node detail page anymore, skip update
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

	// Build a map for each pod's node-kernel-PSI-support flag, so the pod
	// table can render the per-row warning glyph without a separate lookup.
	// Snapshot of cachedNodeModels — a transient race here yields at most a
	// one-cycle warning delay, which is acceptable.
	nodeKernelSupport := make(map[string]bool, len(p.cachedNodeModels))
	for _, n := range p.cachedNodeModels {
		nodeKernelSupport[n.Name] = n.PSIKernelSupported
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
				// Update the model's usage metrics only if we got actual metrics
				totalCpu, totalMem := podMetricsTotals(podMetrics)

				// If metrics are available (non-zero), use them
				// Otherwise keep the existing model which may have requests/limits
				if totalCpu.Value() > 0 || totalMem.Value() > 0 {
					existingModel.PodUsageCpuQty = totalCpu
					existingModel.PodUsageMemQty = totalMem
				}
				existingModel.PSI = podMetrics.PSI
				existingModel.CPUThrottledPct = podMetrics.CPUThrottledPct
			}
			existingModel.NodePSIKernelSupported = nodeKernelSupport[existingModel.Node]
			updatedModels = append(updatedModels, existingModel)
		}
	}

	// Pre-fetch pod detail data if detail view is visible (do network calls outside QueueUpdateDraw)
	// Use ViewStateManager for thread-safe state access
	// Capture the pod key at fetch time so we can verify it later
	var podDetailData *model.PodDetailData
	var fetchedPodKey string
	if namespace, podName, ok := p.viewState.GetPodModal(); ok && p.podDetailPanel != nil {
		fetchedPodKey = namespace + "/" + podName
		podDetailData = p.buildPodDetailData(ctx, fetchedPodKey, updatedModels)
	}

	// Pre-fetch container metrics if container modal view is visible
	var containerCPU, containerMem string
	var fetchedContainerKey string
	if namespace, podName, containerName, ok := p.viewState.GetContainerModal(); ok && p.containerDetailPanel != nil {
		fetchedContainerKey = namespace + "/" + podName + "/" + containerName
		if p.metricsSource != nil {
			if podMetrics, err := p.metricsSource.GetPodMetrics(ctx, namespace, podName); err == nil && podMetrics != nil {
				// Try actual container name first
				for _, cm := range podMetrics.Containers {
					if cm.Name == containerName {
						if cm.CPUUsage != nil {
							containerCPU = fmt.Sprintf("%dm", cm.CPUUsage.MilliValue())
						}
						if cm.MemoryUsage != nil {
							containerMem = strings.TrimSpace(ui.FormatMemory(cm.MemoryUsage))
						}
						break
					}
				}
				// Fallback to "main" for single-container static pods
				if containerCPU == "" && containerMem == "" {
					for _, cm := range podMetrics.Containers {
						if cm.Name == "main" {
							if cm.CPUUsage != nil {
								containerCPU = fmt.Sprintf("%dm", cm.CPUUsage.MilliValue())
							}
							if cm.MemoryUsage != nil {
								containerMem = strings.TrimSpace(ui.FormatMemory(cm.MemoryUsage))
							}
							break
						}
					}
				}
			}
		}
	}

	// Queue UI update on main goroutine to avoid race with Draw()
	// This is called from controller goroutine, so we must synchronize
	p.app.QueueUpdateDraw(func() {
		// Cache the updated models (with metrics) for immediate re-filtering
		p.cachedPodModels = updatedModels
		// Apply namespace filter and display
		p.displayFilteredPodsInternal()

		// If pod detail is currently displayed, update it with pre-fetched data
		// CRITICAL: Re-verify the view state matches what we fetched - user may have
		// navigated to a different pod/page between fetch and now
		if podDetailData != nil && p.podDetailPanel != nil && fetchedPodKey != "" {
			if ns, pod, ok := p.viewState.GetPodModal(); ok {
				currentKey := ns + "/" + pod
				if currentKey == fetchedPodKey {
					if podDetailData.PodModel != nil &&
						podDetailData.PodModel.Namespace == ns &&
						podDetailData.PodModel.Name == pod {
						p.podDetailPanel.DrawBody(podDetailData)
					}
				}
			}
		}

		// If container modal is currently displayed, update metrics with pre-fetched data
		if fetchedContainerKey != "" && p.containerDetailPanel != nil {
			if ns, pod, container, ok := p.viewState.GetContainerModal(); ok {
				currentKey := ns + "/" + pod + "/" + container
				if currentKey == fetchedContainerKey {
					p.containerDetailPanel.UpdateMetrics(containerCPU, containerMem)
				}
			}
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

	if pp, ok := p.podPanel.(interface{ SetNamespaceFilter(string) }); ok {
		pp.SetNamespaceFilter(p.namespaceFilter)
	}

	// Sorting and filtering are handled by the panel itself in DrawBody
	p.podPanel.Clear()
	if p.snapshotBuilder == nil {
		p.snapshotBuilder = snapshot.NewBuilder()
	}
	if snap, err := p.snapshotBuilder.Build(context.Background(), snapshot.BuildInput{Pods: p.cachedPodModels}); err == nil {
		p.podPanel.DrawBody(snap)
	} else {
		p.podPanel.DrawBody(p.cachedPodModels)
	}
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

	// Set metrics source type for conditional display in detail panel
	if p.metricsSource != nil {
		detailData.MetricsSourceType = p.metricsSource.GetSourceInfo().Type

		// Fetch per-node network/disk rates and fresh CPU/memory usage for sparklines (Prometheus only)
		// GetNodeMetrics uses label filter {node: nodeName} to get only this node's metrics
		if nodeMetrics, err := p.metricsSource.GetNodeMetrics(ctx, nodeName); err == nil && nodeMetrics != nil {
			detailData.NetworkRxRate = nodeMetrics.NetworkRxRate
			detailData.NetworkTxRate = nodeMetrics.NetworkTxRate
			detailData.DiskReadRate = nodeMetrics.DiskReadRate
			detailData.DiskWriteRate = nodeMetrics.DiskWriteRate
			nodeModel.UsageCpuQty = nodeMetrics.CPUUsage
			if nodeMetrics.MemoryUsage != nil {
				nodeModel.UsageMemQty = nodeMetrics.MemoryUsage
			}
			nodeModel.PSI = nodeMetrics.PSI
		}
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
	if p.app != nil && p.app.GetK8sClient() != nil {
		ctrl := p.app.GetK8sClient().Controller()
		if node, err := ctrl.GetNode(ctx, nodeName); err == nil {
			detailData.Node = node
		}

		// Fetch events for this node
		if events, err := ctrl.GetEventsForNode(ctx, nodeName); err == nil {
			detailData.Events = events
		}
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
	// CRITICAL: Copy the value instead of storing pointer to slice element
	// Pointers to slice elements become unstable when the slice is replaced during refresh
	var podModel *model.PodModel
	for i := range podModels {
		if podModels[i].Namespace == namespace && podModels[i].Name == podName {
			pm := podModels[i] // Copy the value
			podModel = &pm     // Pointer to local copy
			break
		}
	}

	if podModel == nil {
		// Fallback when opened from another tab (e.g., --page=workloads) before Overview's pod cache refreshes.
		if p.app != nil && p.app.GetK8sClient() != nil {
			if pod, err := p.app.GetK8sClient().Controller().GetPod(ctx, namespace, podName); err == nil && pod != nil {
				pm := model.NewPodModel(pod, nil, nil)
				podModel = pm
			}
		}
	}

	if podModel == nil {
		return nil // Pod no longer exists
	}

	// Create detail data
	detailData := &model.PodDetailData{
		PodModel: podModel,
	}

	// Set metrics source type for conditional display in detail panel
	if p.metricsSource != nil {
		detailData.MetricsSourceType = p.metricsSource.GetSourceInfo().Type

		// Fetch pod-level network/disk rates for sparklines (Prometheus only)
		if netRx, netTx, diskRead, diskWrite, err := p.metricsSource.GetPodNetworkDiskMetrics(ctx, namespace, podName); err == nil {
			detailData.NetworkRxRate = netRx
			detailData.NetworkTxRate = netTx
			detailData.DiskReadRate = diskRead
			detailData.DiskWriteRate = diskWrite
		}
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
			// Use node allocatable as denominator to match real-time sparkline calculation
			allocCPU := int64(1000)
			allocMem := int64(1 << 30)
			if podModel.NodeAllocatableCpuQty != nil && podModel.NodeAllocatableCpuQty.MilliValue() > 0 {
				allocCPU = podModel.NodeAllocatableCpuQty.MilliValue()
			}
			if podModel.NodeAllocatableMemQty != nil && podModel.NodeAllocatableMemQty.Value() > 0 {
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

	// Fetch the full Pod object for detailed info
	if p.app != nil && p.app.GetK8sClient() != nil {
		ctrl := p.app.GetK8sClient().Controller()
		if pod, err := ctrl.GetPod(ctx, namespace, podName); err == nil {
			detailData.Pod = pod
		}

		// Fetch events for this pod
		if events, err := ctrl.GetEventsForPod(ctx, namespace, podName); err == nil {
			detailData.Events = events
		}
	}

	// Fetch container metrics for the containers table
	if p.metricsSource != nil {
		if podMetrics, err := p.metricsSource.GetPodMetrics(ctx, namespace, podName); err == nil && podMetrics != nil {
			detailData.ContainerMetrics = make(map[string]model.ContainerUsage)
			for _, cm := range podMetrics.Containers {
				usage := model.ContainerUsage{}
				if cm.CPUUsage != nil {
					usage.CPUUsage = fmt.Sprintf("%dm", cm.CPUUsage.MilliValue())
				}
				if cm.MemoryUsage != nil {
					usage.MemoryUsage = ui.FormatMemory(cm.MemoryUsage)
				}
				usage.CPUThrottledPct = cm.CPUThrottled
				detailData.ContainerMetrics[cm.Name] = usage
			}
		}
	}

	return detailData
}

// podMetricsTotals sums up CPU and memory usage across all containers in a pod directly from *metrics.PodMetrics
func podMetricsTotals(podMetrics *metrics.PodMetrics) (totalCpu, totalMem *resource.Quantity) {
	totalCpu = resource.NewQuantity(0, resource.DecimalSI)
	totalMem = resource.NewQuantity(0, resource.DecimalSI)

	if podMetrics == nil {
		return
	}

	for _, c := range podMetrics.Containers {
		if c.CPUUsage != nil {
			totalCpu.Add(*c.CPUUsage)
		}
		if c.MemoryUsage != nil {
			totalMem.Add(*c.MemoryUsage)
		}
	}

	return
}

func (p *MainPanel) refreshWorkloadSummary(ctx context.Context, summary model.ClusterSummary) error {
	// Queue UI update on main goroutine to avoid race with Draw()
	// This is called from controller goroutine, so we must synchronize
	p.app.QueueUpdateDraw(func() {
		// Check if terminal size category changed and rebuild layout if needed
		p.checkAndRebuildLayout()

		p.clusterSummaryPanel.Clear()
		if p.snapshotBuilder == nil {
			p.snapshotBuilder = snapshot.NewBuilder()
		}
		if snap, err := p.snapshotBuilder.Build(ctx, snapshot.BuildInput{Summary: summary}); err == nil {
			p.clusterSummaryPanel.DrawBody(snap)
		} else {
			p.clusterSummaryPanel.DrawBody(summary)
		}
	})
	return nil
}

func (p *MainPanel) openColumnSelector(title string, allColumns, currentColumns []string, onApply func([]string)) {
	modalID := "column-selector"
	modal := ui.NewColumnSelectorModal(ui.ColumnSelectorConfig{
		Title:          title,
		AllColumns:     allColumns,
		CurrentColumns: currentColumns,
		Descriptions:   config.ColumnDescriptions,
		OnApply: func(selected []string) {
			p.app.DismissModalPage(modalID)
			onApply(selected)
		},
		OnCancel: func() {
			p.app.DismissModalPage(modalID)
		},
	})
	p.app.ShowModalPage(modalID, modal.GetRootView())
}

// filterColumns filters the allColumns based on the user-provided filterCols
// It returns a slice of columns that match the case-insensitive filter, preserving specified order
func filterColumns(allColumns []string, filterCols []string) []string {
	if len(filterCols) == 0 {
		return allColumns
	}

	result := []string{}
	for _, fc := range filterCols {
		for _, col := range allColumns {
			if strings.EqualFold(col, fc) && !slices.Contains(result, col) {
				result = append(result, col)
				break
			}
		}
	}

	// If no matches found, return at least the first column (usually NAME or NAMESPACE)
	if len(result) == 0 && len(allColumns) > 0 {
		return []string{allColumns[0]}
	}

	return result
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
