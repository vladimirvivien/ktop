package workloads

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

// Page is the top-level Workloads multi-page view managing Summary,
// Deployments, StatefulSets, DaemonSets, Jobs, and Workload Detail drill-down.
type Page struct {
	app             *application.Application
	title           string
	root            *tview.Flex
	summary         *summaryPanel
	deployments     *workloadTable[model.DeploymentModel]
	statefulsets    *workloadTable[model.StatefulSetModel]
	daemonsets      *workloadTable[model.DaemonSetModel]
	jobs            *workloadTable[model.JobModel]
	children        []tview.Primitive
	childPanels     []ui.Panel
	namespaceFilter string
	lastSnap        model.WorkloadsSnapshot
	layoutSig       string

	detailMu     sync.RWMutex
	detailActive bool
	detailKind   model.WorkloadKind
	detailNS     string
	detailName   string
	detailPanel  *DetailPanel
}

// New constructs the Workloads top-level page controller.
func New(app *application.Application, title string) *Page {
	p := &Page{
		app:   app,
		title: title,
	}
	p.Layout(nil)
	return p
}

func (p *Page) Layout(_ any) {
	if p.root != nil {
		return
	}

	p.summary = newSummaryPanel(fmt.Sprintf(" %s Workload Summary ", ui.Icons.Thermometer))

	p.deployments = newWorkloadTable(p.app, workloadTableConfig[model.DeploymentModel]{
		title: "Deployments",
		icon:  ui.Icons.Rocket,
		columns: []string{
			"NAMESPACE", "NAME", "READY", "UP-TO-DATE", "AVAIL", "AGE", "EFF", "STATUS",
		},
		columnToKey: map[string]rune{
			"NAMESPACE":  'n',
			"NAME":       'd',
			"READY":      'r',
			"UP-TO-DATE": 'u',
			"AVAIL":      'v',
			"AGE":        'a',
			"EFF":        'e',
			"STATUS":     's',
		},
		defaultSortCol: "NAMESPACE",
		defaultSortAsc: true,
		sortFn:         model.SortDeploymentModelsBy,
		namespaceFn:    func(d model.DeploymentModel) string { return d.Namespace },
		nameFn:         func(d model.DeploymentModel) string { return d.Name },
		statusFn:       func(d model.DeploymentModel) string { return d.Status },
		filterCellsFn: func(d model.DeploymentModel) []string {
			return []string{d.Namespace, d.Name, d.Status, fmt.Sprintf("%d/%d", d.Ready, d.Desired)}
		},
		cellFn: renderDeploymentCell,
		onSelect: func(d model.DeploymentModel) {
			if p.app != nil {
				p.app.NavigateToWorkloadDetail(string(model.WorkloadKindDeployment), d.Namespace, d.Name)
			}
		},
	})

	p.statefulsets = newWorkloadTable(p.app, workloadTableConfig[model.StatefulSetModel]{
		title: "StatefulSets",
		icon:  ui.Icons.Package,
		columns: []string{
			"NAMESPACE", "NAME", "READY", "AGE", "EFF", "STATUS",
		},
		columnToKey: map[string]rune{
			"NAMESPACE": 'n',
			"NAME":      'f',
			"READY":     'r',
			"AGE":       'a',
			"EFF":       'e',
			"STATUS":    's',
		},
		defaultSortCol: "NAMESPACE",
		defaultSortAsc: true,
		sortFn:         model.SortStatefulSetModelsBy,
		namespaceFn:    func(s model.StatefulSetModel) string { return s.Namespace },
		nameFn:         func(s model.StatefulSetModel) string { return s.Name },
		statusFn:       func(s model.StatefulSetModel) string { return s.Status },
		filterCellsFn: func(s model.StatefulSetModel) []string {
			return []string{s.Namespace, s.Name, s.Status, fmt.Sprintf("%d/%d", s.Ready, s.Desired)}
		},
		cellFn: renderStatefulSetCell,
		onSelect: func(s model.StatefulSetModel) {
			if p.app != nil {
				p.app.NavigateToWorkloadDetail(string(model.WorkloadKindStatefulSet), s.Namespace, s.Name)
			}
		},
	})

	p.daemonsets = newWorkloadTable(p.app, workloadTableConfig[model.DaemonSetModel]{
		title: "DaemonSets",
		icon:  ui.Icons.Controller,
		columns: []string{
			"NAMESPACE", "NAME", "DESIRED", "CURRENT", "READY", "AGE", "EFF", "STATUS",
		},
		columnToKey: map[string]rune{
			"NAMESPACE": 'n',
			"NAME":      'd',
			"DESIRED":   'i',
			"CURRENT":   'c',
			"READY":     'r',
			"AGE":       'a',
			"EFF":       'e',
			"STATUS":    's',
		},
		defaultSortCol: "NAMESPACE",
		defaultSortAsc: true,
		sortFn:         model.SortDaemonSetModelsBy,
		namespaceFn:    func(ds model.DaemonSetModel) string { return ds.Namespace },
		nameFn:         func(ds model.DaemonSetModel) string { return ds.Name },
		statusFn:       func(ds model.DaemonSetModel) string { return ds.Status },
		filterCellsFn: func(ds model.DaemonSetModel) []string {
			return []string{ds.Namespace, ds.Name, ds.Status}
		},
		cellFn: renderDaemonSetCell,
		onSelect: func(ds model.DaemonSetModel) {
			if p.app != nil {
				p.app.NavigateToWorkloadDetail(string(model.WorkloadKindDaemonSet), ds.Namespace, ds.Name)
			}
		},
	})

	p.jobs = newWorkloadTable(p.app, workloadTableConfig[model.JobModel]{
		title: "Jobs",
		icon:  ui.Icons.Clock,
		columns: []string{
			"NAMESPACE", "NAME", "COMPLETIONS", "AGE", "STATUS",
		},
		columnToKey: map[string]rune{
			"NAMESPACE":   'n',
			"NAME":        'j',
			"COMPLETIONS": 'c',
			"AGE":         'a',
			"STATUS":      's',
		},
		defaultSortCol: "NAMESPACE",
		defaultSortAsc: true,
		sortFn:         model.SortJobModelsBy,
		namespaceFn:    func(j model.JobModel) string { return j.Namespace },
		nameFn:         func(j model.JobModel) string { return j.Name },
		statusFn:       func(j model.JobModel) string { return j.Status },
		filterCellsFn: func(j model.JobModel) []string {
			return []string{j.Namespace, j.Name, j.Status}
		},
		cellFn: renderJobCell,
		onSelect: func(j model.JobModel) {
			if p.app != nil {
				p.app.NavigateToWorkloadDetail(string(model.WorkloadKindJob), j.Namespace, j.Name)
			}
		},
	})

	p.children = []tview.Primitive{
		p.summary.GetRootView(),
		p.deployments.TablePrimitive(),
		p.statefulsets.TablePrimitive(),
		p.daemonsets.TablePrimitive(),
		p.jobs.TablePrimitive(),
	}
	p.childPanels = []ui.Panel{
		p.summary,
		p.deployments,
		p.statefulsets,
		p.daemonsets,
		p.jobs,
	}

	p.root = tview.NewFlex().SetDirection(tview.FlexRow)
	p.rebuildLayout(false)
}

func (p *Page) Run(ctx context.Context) error {
	p.Layout(nil)

	if p.app != nil {
		p.app.AddNamespaceFilterListener(func(namespace string) {
			p.SetNamespaceFilter(namespace)
			p.app.Refresh()
		})
		p.app.SetWorkloadDetailCallback(p.showWorkloadDetail)
		p.app.AddDetailResetListener(func() {
			p.clearDetailState()
		})

		if client := p.app.GetK8sClient(); client != nil {
			ctrl := client.Controller()
			ctrl.SetWorkloadsRefreshFunc(p.RefreshWorkloads)
		}
	}
	return nil
}

// RefreshWorkloads updates all workload tables, summary counters, and any open
// Workload Detail view on the UI goroutine.
func (p *Page) RefreshWorkloads(ctx context.Context, snap model.WorkloadsSnapshot) error {
	var detailData *model.WorkloadDetailData
	detailActive, detailKind, detailNS, detailName := p.getDetailTarget()
	if detailActive && p.app != nil && p.app.GetK8sClient() != nil {
		if d, err := p.app.GetK8sClient().Controller().BuildWorkloadDetailData(ctx, detailKind, detailNS, detailName); err == nil {
			detailData = d
		}
	}

	if p.app != nil {
		p.app.QueueUpdateDraw(func() {
			p.applySnapshot(snap)
			if detailData != nil && p.detailPanel != nil {
				active, curKind, curNS, curName := p.getDetailTarget()
				if active && curKind == detailKind && curNS == detailNS && curName == detailName {
					p.detailPanel.DrawBody(detailData)
				}
			}
		})
	} else {
		p.applySnapshot(snap)
	}
	return nil
}

func (p *Page) applySnapshot(snap model.WorkloadsSnapshot) {
	p.lastSnap = snap
	p.deployments.DrawBody(snap.Deployments)
	p.statefulsets.DrawBody(snap.StatefulSets)
	p.daemonsets.DrawBody(snap.DaemonSets)
	p.jobs.DrawBody(snap.Jobs)
	p.summary.DrawBody(model.ComputeWorkloadSummary(snap, p.namespaceFilter))
	p.rebuildLayoutIfNeeded()
}

// SetNamespaceFilter updates the namespace filter across all workload tables and summary counts.
func (p *Page) SetNamespaceFilter(ns string) {
	p.namespaceFilter = strings.TrimSpace(ns)
	p.deployments.SetNamespaceFilter(p.namespaceFilter)
	p.statefulsets.SetNamespaceFilter(p.namespaceFilter)
	p.daemonsets.SetNamespaceFilter(p.namespaceFilter)
	p.jobs.SetNamespaceFilter(p.namespaceFilter)
	p.summary.SetNamespaceFilter(p.namespaceFilter)
	p.summary.DrawBody(model.ComputeWorkloadSummary(p.lastSnap, p.namespaceFilter))
	p.rebuildLayoutIfNeeded()
}

func (p *Page) rebuildLayoutIfNeeded() {
	sig := fmt.Sprintf("%t/%t/%t/%t",
		p.deployments.Count() > 0,
		p.statefulsets.Count() > 0,
		p.daemonsets.Count() > 0,
		p.jobs.Count() > 0,
	)
	if sig == p.layoutSig {
		return
	}
	p.layoutSig = sig
	p.rebuildLayout(true)
}

func (p *Page) rebuildLayout(_ bool) {
	if p.root == nil {
		return
	}

	depPop := p.deployments.Count() > 0
	stsPop := p.statefulsets.Count() > 0
	dsPop := p.daemonsets.Count() > 0
	jobPop := p.jobs.Count() > 0
	anyPop := depPop || stsPop || dsPop || jobPop

	p.root.Clear()
	p.root.AddItem(p.summary.GetRootView(), 4, 0, false)

	addSection := func(panel ui.Panel, populated bool, popWeight int) {
		if !anyPop {
			p.root.AddItem(panel.GetRootView(), 0, 1, false)
			return
		}
		if populated {
			p.root.AddItem(panel.GetRootView(), 0, popWeight, false)
		} else {
			p.root.AddItem(panel.GetRootView(), 4, 0, false)
		}
	}

	addSection(p.deployments, depPop, 2)
	addSection(p.statefulsets, stsPop, 1)
	addSection(p.daemonsets, dsPop, 1)
	addSection(p.jobs, jobPop, 1)
}

func (p *Page) ensureDetailPanel() {
	if p.detailPanel != nil {
		return
	}
	p.detailPanel = NewDetailPanel()
	p.detailPanel.SetOnBack(func() {
		p.clearDetailState()
		p.app.NavigateBack()
	})
	p.detailPanel.SetOnPodSelected(func(namespace, podName string) {
		p.clearDetailState()
		p.app.NavigateToPodDetail(namespace, podName)
	})
	p.detailPanel.SetAppFocus(func(prim tview.Primitive) {
		p.app.Focus(prim)
	})
	p.detailPanel.SetOnFooterContextChange(func(focusedPanel string) {
		p.app.SetFooterContext(ui.WorkloadDetailContext{FocusedPanel: focusedPanel})
	})
	p.detailPanel.SetOnYank(func(target string) {
		if p.app != nil {
			p.app.ShowToast(fmt.Sprintf("Copied %q to clipboard", target), ui.ToastSuccess, 2_000_000_000)
		}
	})
	p.detailPanel.SetGetTerminalHeightFunc(func() int {
		if p.app != nil {
			return p.app.GetTerminalHeight()
		}
		return 50
	})
	p.app.AddDetailPage("workload_detail", p.detailPanel.GetRootView())
}

func (p *Page) showWorkloadDetail(kindRaw, namespace, name string) {
	if p.app == nil {
		return
	}
	p.ensureDetailPanel()
	kind := model.ParseWorkloadKind(kindRaw)
	p.setDetailTarget(kind, namespace, name)

	if client := p.app.GetK8sClient(); client != nil {
		if detail, err := client.Controller().BuildWorkloadDetailData(context.Background(), kind, namespace, name); err == nil && detail != nil {
			p.detailPanel.DrawBody(detail)
		}
	}
	p.app.ShowDetailPage("workload_detail")
	p.detailPanel.InitFocus()
}

func (p *Page) setDetailTarget(kind model.WorkloadKind, ns, name string) {
	p.detailMu.Lock()
	defer p.detailMu.Unlock()
	p.detailActive = true
	p.detailKind = kind
	p.detailNS = ns
	p.detailName = name
}

func (p *Page) clearDetailState() {
	p.detailMu.Lock()
	defer p.detailMu.Unlock()
	p.detailActive = false
	p.detailKind = ""
	p.detailNS = ""
	p.detailName = ""
}

func (p *Page) getDetailTarget() (bool, model.WorkloadKind, string, string) {
	p.detailMu.RLock()
	defer p.detailMu.RUnlock()
	return p.detailActive, p.detailKind, p.detailNS, p.detailName
}

// GetActiveDetailPanel implements application detail focus/escape resolution.
func (p *Page) GetActiveDetailPanel() ui.EscapablePanel {
	p.detailMu.RLock()
	active := p.detailActive
	p.detailMu.RUnlock()
	if active && p.detailPanel != nil {
		return p.detailPanel
	}
	return nil
}

// IsFilterEditing reports whether any child workload table is actively editing its filter.
func (p *Page) IsFilterEditing() bool {
	return p.deployments.IsFilterEditing() ||
		p.statefulsets.IsFilterEditing() ||
		p.daemonsets.IsFilterEditing() ||
		p.jobs.IsFilterEditing()
}

// HasEscapableState implements ui.EscapablePanel.
func (p *Page) HasEscapableState() bool {
	return p.deployments.HasEscapableState() ||
		p.statefulsets.HasEscapableState() ||
		p.daemonsets.HasEscapableState() ||
		p.jobs.HasEscapableState()
}

// HandleEscape implements ui.EscapablePanel.
func (p *Page) HandleEscape() bool {
	for _, tbl := range []ui.EscapablePanel{p.deployments, p.statefulsets, p.daemonsets, p.jobs} {
		if tbl.HandleEscape() {
			return true
		}
	}
	return false
}

func (p *Page) DrawHeader(_ any) {}
func (p *Page) DrawBody(data any) {
	if snap, ok := data.(model.WorkloadsSnapshot); ok {
		p.applySnapshot(snap)
	}
}
func (p *Page) DrawFooter(_ any)                    {}
func (p *Page) Clear()                              {}
func (p *Page) GetTitle() string                    { return p.title }
func (p *Page) GetRootView() tview.Primitive        { return p.root }
func (p *Page) GetChildrenViews() []tview.Primitive { return p.children }

// GetChildPanel returns the ordered Panel for Tab focus highlighting.
func (p *Page) GetChildPanel(index int) ui.Panel {
	if index < 0 || index >= len(p.childPanels) {
		return nil
	}
	return p.childPanels[index]
}

func renderDeploymentCell(d model.DeploymentModel, col string, rowColor tcell.Color) *tview.TableCell {
	switch col {
	case "NAMESPACE":
		return ui.NewDataCell(d.Namespace).SetTextColor(rowColor)
	case "NAME":
		return ui.NewDataCell(d.Name).SetTextColor(rowColor).SetMaxWidth(32)
	case "READY":
		text := fmt.Sprintf("%d/%d", d.Ready, d.Desired)
		c := ui.GetTcellColor(ui.GetReadyColor(int(d.Ready), int(max(d.Desired, 1))))
		if d.Desired == 0 {
			c = rowColor
		}
		return ui.NewDataCell(text).SetTextColor(c)
	case "UP-TO-DATE":
		return ui.NewDataCell(fmt.Sprintf("%d", d.Updated)).SetTextColor(rowColor)
	case "AVAIL":
		return ui.NewDataCell(fmt.Sprintf("%d", d.Available)).SetTextColor(rowColor)
	case "AGE":
		return ui.NewDataCell(d.Age).SetTextColor(mutedCellColor())
	case "EFF":
		return ui.NewDataCell(ui.FormatEfficiencyCell(d.Efficiency.EffPct, d.Efficiency.HasEfficiency)).SetTextColor(rowColor)
	case "STATUS":
		return ui.StyleStatusCell(ui.NewDataCell(d.Status), d.Status, "workload")
	default:
		return ui.NewDataCell("").SetTextColor(rowColor)
	}
}

func renderStatefulSetCell(s model.StatefulSetModel, col string, rowColor tcell.Color) *tview.TableCell {
	switch col {
	case "NAMESPACE":
		return ui.NewDataCell(s.Namespace).SetTextColor(rowColor)
	case "NAME":
		return ui.NewDataCell(s.Name).SetTextColor(rowColor).SetMaxWidth(32)
	case "READY":
		text := fmt.Sprintf("%d/%d", s.Ready, s.Desired)
		c := ui.GetTcellColor(ui.GetReadyColor(int(s.Ready), int(max(s.Desired, 1))))
		if s.Desired == 0 {
			c = rowColor
		}
		return ui.NewDataCell(text).SetTextColor(c)
	case "AGE":
		return ui.NewDataCell(s.Age).SetTextColor(mutedCellColor())
	case "EFF":
		return ui.NewDataCell(ui.FormatEfficiencyCell(s.Efficiency.EffPct, s.Efficiency.HasEfficiency)).SetTextColor(rowColor)
	case "STATUS":
		return ui.StyleStatusCell(ui.NewDataCell(s.Status), s.Status, "workload")
	default:
		return ui.NewDataCell("").SetTextColor(rowColor)
	}
}

func renderDaemonSetCell(ds model.DaemonSetModel, col string, rowColor tcell.Color) *tview.TableCell {
	switch col {
	case "NAMESPACE":
		return ui.NewDataCell(ds.Namespace).SetTextColor(rowColor)
	case "NAME":
		return ui.NewDataCell(ds.Name).SetTextColor(rowColor).SetMaxWidth(32)
	case "DESIRED":
		return ui.NewDataCell(fmt.Sprintf("%d", ds.Desired)).SetTextColor(rowColor)
	case "CURRENT":
		return ui.NewDataCell(fmt.Sprintf("%d", ds.Current)).SetTextColor(rowColor)
	case "READY":
		text := fmt.Sprintf("%d", ds.Ready)
		c := ui.GetTcellColor(ui.GetReadyColor(int(ds.Ready), int(max(ds.Desired, 1))))
		if ds.Desired == 0 {
			c = rowColor
		}
		return ui.NewDataCell(text).SetTextColor(c)
	case "AGE":
		return ui.NewDataCell(ds.Age).SetTextColor(mutedCellColor())
	case "EFF":
		return ui.NewDataCell(ui.FormatEfficiencyCell(ds.Efficiency.EffPct, ds.Efficiency.HasEfficiency)).SetTextColor(rowColor)
	case "STATUS":
		return ui.StyleStatusCell(ui.NewDataCell(ds.Status), ds.Status, "workload")
	default:
		return ui.NewDataCell("").SetTextColor(rowColor)
	}
}

func renderJobCell(j model.JobModel, col string, rowColor tcell.Color) *tview.TableCell {
	switch col {
	case "NAMESPACE":
		return ui.NewDataCell(j.Namespace).SetTextColor(rowColor)
	case "NAME":
		return ui.NewDataCell(j.Name).SetTextColor(rowColor).SetMaxWidth(32)
	case "COMPLETIONS":
		return ui.NewDataCell(j.FormatCompletions()).SetTextColor(rowColor)
	case "AGE":
		return ui.NewDataCell(j.Age).SetTextColor(mutedCellColor())
	case "STATUS":
		return ui.StyleStatusCell(ui.NewDataCell(j.Status), j.Status, "workload")
	default:
		return ui.NewDataCell("").SetTextColor(rowColor)
	}
}

func mutedCellColor() tcell.Color {
	if ui.IsMonochrome() {
		return tcell.ColorDefault
	}
	return ui.ActivePalette().StatusMuted
}
