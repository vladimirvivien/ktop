package network

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/ui/theme"
	"github.com/vladimirvivien/ktop/views/model"
)

const networkYAMLModalID = "network-yaml-modal"

type networkTableConfig[T any] struct {
	title          string
	icon           string
	columns        []string
	expandCol      string
	columnToKey    map[string]rune
	defaultSortCol string
	defaultSortAsc bool
	sortFn         func(items []T, col string, asc bool)
	namespaceFn    func(T) string
	filterCellsFn  func(T) []string
	cellFn         func(item T, col string, pal theme.Palette, mono bool) *tview.TableCell
	yamlInfoFn     func(item T) (title string, obj any)
	yankTargetFn   func(item T) string
}

type networkTable[T any] struct {
	app             *application.Application
	cfg             networkTableConfig[T]
	root            *tview.Flex
	table           *tview.Table
	ctrl            *ui.TableController
	allItems        []T
	displayed       []T
	namespaceFilter string
	colMap          map[string]int
}

func newNetworkTable[T any](app *application.Application, cfg networkTableConfig[T]) *networkTable[T] {
	nt := &networkTable[T]{
		app:    app,
		cfg:    cfg,
		colMap: make(map[string]int, len(cfg.columns)),
	}
	nt.layout()
	nt.drawHeader()
	nt.updateTitle(0)
	return nt
}

func (n *networkTable[T]) layout() {
	n.table = tview.NewTable()
	n.table.SetFixed(1, 0)
	n.table.SetBorder(false)
	n.table.SetBorders(false)
	ui.StyleTable(n.table)
	n.table.SetFocusFunc(func() {
		n.table.SetSelectable(true, false)
		n.table.SetSelectedStyle(ui.SelectedRowStyle())
		if n.table.GetRowCount() > 1 {
			row, col := n.table.GetSelection()
			if row < 1 {
				row = 1
			}
			n.table.Select(row, col)
		}
	})
	n.table.SetBlurFunc(func() {
		n.table.SetSelectable(false, false)
	})

	n.ctrl = ui.NewTableController(n.table, ui.TableControllerConfig{
		DefaultSortColumn: n.cfg.defaultSortCol,
		DefaultSortAsc:    n.cfg.defaultSortAsc,
		ColumnToKey:       n.cfg.columnToKey,
		OnRedraw:          n.redraw,
		OnSelect: func(row int) bool {
			idx := row - 1
			if idx < 0 || idx >= len(n.displayed) || n.app == nil || n.cfg.yamlInfoFn == nil {
				return false
			}
			title, src := n.cfg.yamlInfoFn(n.displayed[idx])
			yamlText, err := ui.FormatObjectYAML(src)
			if err != nil {
				n.app.ShowToast(fmt.Sprintf("Unable to render YAML: %v", err), ui.ToastError, 3*time.Second)
				return true
			}
			modal := ui.NewYAMLModal(ui.YAMLModalConfig{
				Title:   title,
				Content: yamlText,
				OnClose: func() {
					n.app.DismissModalPage(networkYAMLModalID)
					n.app.Focus(n.table)
				},
				OnYank: func(_ string) {
					n.app.ShowToast(fmt.Sprintf("Copied %s YAML to clipboard", title), ui.ToastSuccess, 2*time.Second)
				},
			})
			n.app.ShowModalPage(networkYAMLModalID, modal.GetRootView())
			return true
		},
		OnYank: func(row int) bool {
			idx := row - 1
			if idx < 0 || idx >= len(n.displayed) || n.cfg.yankTargetFn == nil {
				return false
			}
			target := n.cfg.yankTargetFn(n.displayed[idx])
			if target == "" {
				return false
			}
			_ = ui.CopyToClipboard(target)
			if n.app != nil {
				n.app.ShowToast(fmt.Sprintf("Copied %q to clipboard", target), ui.ToastSuccess, 2*time.Second)
			}
			return true
		},
	})
	n.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return n.ctrl.HandleInput(event)
	})

	n.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(n.table, 0, 1, true)
	n.root.SetBorder(true)
	n.root.SetTitleAlign(tview.AlignLeft)
	ui.SetFlexFocused(n.root, false)
}

func (n *networkTable[T]) drawHeader() {
	n.ctrl.SetVisibleColumns(n.cfg.columns)
	ui.StyleTable(n.table)
	n.colMap = make(map[string]int, len(n.cfg.columns))

	for i, col := range n.cfg.columns {
		headerText := n.ctrl.FormatColumnHeader(col)
		exp := 0
		if col == n.cfg.expandCol {
			exp = 1
		}
		n.table.SetCell(0, i,
			ui.StyleHeaderCell(tview.NewTableCell(headerText).
				SetAlign(tview.AlignLeft).
				SetExpansion(exp).
				SetSelectable(false)),
		)
		n.colMap[col] = i
	}
	n.table.SetFixed(1, 0)
}

func (n *networkTable[T]) setItems(items []T) {
	n.allItems = slices.Clone(items)
	n.applyItems()
}

func (n *networkTable[T]) applyItems() {
	selRow, selCol := n.table.GetSelection()

	sorted := slices.Clone(n.allItems)
	sortCol, sortAsc := n.ctrl.GetSortParams()
	if n.cfg.sortFn != nil {
		n.cfg.sortFn(sorted, sortCol, sortAsc)
	}

	filter := n.ctrl.Filter()
	filter.TotalRows = len(sorted)

	nsLower := strings.ToLower(strings.TrimSpace(n.namespaceFilter))
	hasNS := nsLower != ""
	hasRowFilter := filter.IsFiltering() && filter.Text != ""

	visible := make([]T, 0, len(sorted))
	for _, item := range sorted {
		if hasNS && n.cfg.namespaceFn != nil && !strings.Contains(strings.ToLower(n.cfg.namespaceFn(item)), nsLower) {
			continue
		}
		if hasRowFilter && n.cfg.filterCellsFn != nil && !filter.MatchesRow(n.cfg.filterCellsFn(item)) {
			continue
		}
		visible = append(visible, item)
	}

	filter.MatchRows = len(visible)
	n.displayed = visible

	n.table.Clear()
	n.drawHeader()
	n.updateTitle(len(visible))

	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()
	for i, item := range visible {
		rowIdx := i + 1
		for _, col := range n.cfg.columns {
			colIdx := n.colMap[col]
			n.table.SetCell(rowIdx, colIdx, n.cfg.cellFn(item, col, pal, mono))
		}
	}

	if len(visible) > 0 && n.table.HasFocus() {
		if selRow < 1 {
			selRow = 1
		} else if selRow > len(visible) {
			selRow = len(visible)
		}
		n.table.Select(selRow, selCol)
	}
}

func (n *networkTable[T]) redraw() {
	n.applyItems()
	if n.app != nil {
		n.app.Refresh()
	}
}

func (n *networkTable[T]) updateTitle(count int) {
	if n.root == nil {
		return
	}
	var suffix string
	if n.app != nil && n.app.IsAPIDisconnected() {
		suffix = " " + ui.ErrorMarkup("[DISCONNECTED - Press R to reconnect]")
	}
	if n.ctrl != nil {
		n.root.SetTitle(n.ctrl.FormatTitleWithScroll(n.cfg.title, n.cfg.icon, count, suffix))
	} else {
		n.root.SetTitle(fmt.Sprintf(" %s %s (%d)%s ", n.cfg.icon, n.cfg.title, count, suffix))
	}
}

func (n *networkTable[T]) setNamespaceFilter(ns string) {
	if n.namespaceFilter == ns {
		return
	}
	n.namespaceFilter = ns
	n.applyItems()
}

func (n *networkTable[T]) Layout(_ any)                 {}
func (n *networkTable[T]) DrawHeader(_ any)             {}
func (n *networkTable[T]) DrawBody(_ any)               {}
func (n *networkTable[T]) DrawFooter(_ any)             {}
func (n *networkTable[T]) Clear()                       {}
func (n *networkTable[T]) GetTitle() string             { return n.cfg.title }
func (n *networkTable[T]) GetRootView() tview.Primitive { return n.root }
func (n *networkTable[T]) GetChildrenViews() []tview.Primitive {
	return []tview.Primitive{n.table}
}
func (n *networkTable[T]) SetFocused(focused bool) { ui.SetFlexFocused(n.root, focused) }

// Page is the top-level [5] Network view showing Summary, Services, and Ingresses.
type Page struct {
	app             *application.Application
	title           string
	root            *tview.Flex
	summary         *summaryPanel
	services        *networkTable[model.ServiceModel]
	ingresses       *networkTable[model.IngressModel]
	children        []tview.Primitive
	childPanels     []ui.Panel
	namespaceFilter string
	liveSnap        model.NetworkSnapshot
}

// New constructs the Network top-level page controller.
func New(app *application.Application, title string) *Page {
	p := &Page{
		app:   app,
		title: title,
	}
	p.Layout(nil)
	return p
}

// Layout initializes the summary strip, Services table, and Ingresses table.
func (p *Page) Layout(_ any) {
	if p.root != nil {
		return
	}

	p.summary = newSummaryPanel(fmt.Sprintf(" %s  Summary ", ui.Icons.Thermometer))

	p.services = newNetworkTable(p.app, networkTableConfig[model.ServiceModel]{
		title:     "Services",
		icon:      ui.Icons.Controller,
		columns:   []string{"NS", "NAME", "TYPE", "CLUSTER-IP", "EXTERNAL-IP", "PORTS", "ENDPOINTS", "AGE"},
		expandCol: "NAME",
		columnToKey: map[string]rune{
			"NS":          'n',
			"NAME":        'm',
			"TYPE":        't',
			"CLUSTER-IP":  'c',
			"EXTERNAL-IP": 'e',
			"PORTS":       'p',
			"ENDPOINTS":   'd',
			"AGE":         'a',
		},
		defaultSortCol: "NS",
		defaultSortAsc: true,
		sortFn:         model.SortServiceModelsBy,
		namespaceFn:    func(svc model.ServiceModel) string { return svc.Namespace },
		filterCellsFn: func(svc model.ServiceModel) []string {
			return []string{svc.Namespace, svc.Name, svc.Type, svc.ClusterIP, svc.ExternalIP, svc.Ports}
		},
		cellFn: renderServiceCell,
		yamlInfoFn: func(svc model.ServiceModel) (string, any) {
			var src any = svc.Raw
			if svc.Raw == nil {
				src = svc
			}
			return fmt.Sprintf("Service YAML: %s/%s", svc.Namespace, svc.Name), src
		},
		yankTargetFn: func(svc model.ServiceModel) string {
			return svc.Namespace + "/" + svc.Name
		},
	})

	p.ingresses = newNetworkTable(p.app, networkTableConfig[model.IngressModel]{
		title:     "Ingresses",
		icon:      ui.Icons.Rocket,
		columns:   []string{"NS", "NAME", "CLASS", "HOSTS", "ADDRESS", "PORTS", "AGE"},
		expandCol: "HOSTS",
		columnToKey: map[string]rune{
			"NS":      'n',
			"NAME":    'm',
			"CLASS":   'c',
			"HOSTS":   'h',
			"ADDRESS": 'd',
			"PORTS":   'p',
			"AGE":     'a',
		},
		defaultSortCol: "NS",
		defaultSortAsc: true,
		sortFn:         model.SortIngressModelsBy,
		namespaceFn:    func(ing model.IngressModel) string { return ing.Namespace },
		filterCellsFn: func(ing model.IngressModel) []string {
			return []string{ing.Namespace, ing.Name, ing.ClassName, strings.Join(ing.Hosts, ","), ing.Address, ing.Ports}
		},
		cellFn: renderIngressCell,
		yamlInfoFn: func(ing model.IngressModel) (string, any) {
			var src any = ing.Raw
			if ing.Raw == nil {
				src = ing
			}
			return fmt.Sprintf("Ingress YAML: %s/%s", ing.Namespace, ing.Name), src
		},
		yankTargetFn: func(ing model.IngressModel) string {
			return ing.Namespace + "/" + ing.Name
		},
	})

	p.children = []tview.Primitive{p.services.table, p.ingresses.table}
	p.childPanels = []ui.Panel{p.services, p.ingresses}

	p.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.summary.GetRootView(), 3, 0, false).
		AddItem(p.services.GetRootView(), 0, 3, true).
		AddItem(p.ingresses.GetRootView(), 0, 2, false)
}

// Run subscribes to namespace filter changes and registers the network refresh handler.
func (p *Page) Run(_ context.Context) error {
	p.Layout(nil)
	if p.app != nil {
		p.app.AddNamespaceFilterListener(func(namespace string) {
			p.SetNamespaceFilter(namespace)
			p.app.Refresh()
		})
		if client := p.app.GetK8sClient(); client != nil {
			client.Controller().SetNetworkRefreshFunc(p.RefreshNetwork)
		}
	}
	return nil
}

// RefreshNetwork receives updated network snapshots from the k8s controller.
func (p *Page) RefreshNetwork(_ context.Context, snap model.NetworkSnapshot) error {
	if p.app != nil {
		p.app.QueueUpdateDraw(func() {
			p.applySnapshot(snap)
		})
	} else {
		p.applySnapshot(snap)
	}
	return nil
}

func (p *Page) applySnapshot(snap model.NetworkSnapshot) {
	p.liveSnap = snap
	p.services.setItems(snap.Services)
	p.ingresses.setItems(snap.Ingresses)
	p.renderSummary()
	if p.app != nil {
		p.app.Refresh()
	}
}

// SetNamespaceFilter updates the active namespace filter synced with the app header.
func (p *Page) SetNamespaceFilter(ns string) {
	trimmed := strings.TrimSpace(ns)
	if p.namespaceFilter == trimmed {
		return
	}
	p.namespaceFilter = trimmed
	p.services.setNamespaceFilter(trimmed)
	p.ingresses.setNamespaceFilter(trimmed)
	p.renderSummary()
}

func (p *Page) renderSummary() {
	if p.summary == nil {
		return
	}
	nsLower := strings.ToLower(strings.TrimSpace(p.namespaceFilter))
	if nsLower == "" {
		summary := p.liveSnap.Summary
		if summary.ServicesTotal == 0 && summary.IngressesTotal == 0 && (len(p.liveSnap.Services) > 0 || len(p.liveSnap.Ingresses) > 0) {
			summary = model.ComputeNetworkSummary(p.liveSnap.Services, p.liveSnap.Ingresses, p.liveSnap.NodeNetwork, summary.HasMetrics)
		}
		p.summary.DrawBody(summary)
		return
	}

	var scopedSvcs []model.ServiceModel
	for _, svc := range p.liveSnap.Services {
		if strings.Contains(strings.ToLower(svc.Namespace), nsLower) {
			scopedSvcs = append(scopedSvcs, svc)
		}
	}
	var scopedIngs []model.IngressModel
	for _, ing := range p.liveSnap.Ingresses {
		if strings.Contains(strings.ToLower(ing.Namespace), nsLower) {
			scopedIngs = append(scopedIngs, ing)
		}
	}
	p.summary.DrawBody(model.ComputeNetworkSummary(scopedSvcs, scopedIngs, p.liveSnap.NodeNetwork, p.liveSnap.Summary.HasMetrics))
}

// IsFilterEditing reports whether any child table filter input is actively editing.
func (p *Page) IsFilterEditing() bool {
	return p.services.ctrl.IsFilterEditing() || p.ingresses.ctrl.IsFilterEditing()
}

// HasEscapableState implements ui.EscapablePanel.
func (p *Page) HasEscapableState() bool {
	return p.services.ctrl.HasEscapableState() || p.ingresses.ctrl.HasEscapableState()
}

// HandleEscape implements ui.EscapablePanel.
func (p *Page) HandleEscape() bool {
	if p.services.ctrl.HandleEscape() {
		return true
	}
	return p.ingresses.ctrl.HandleEscape()
}

func (p *Page) DrawHeader(_ any) {}
func (p *Page) DrawBody(data any) {
	if snap, ok := data.(model.NetworkSnapshot); ok {
		p.applySnapshot(snap)
	}
}
func (p *Page) DrawFooter(_ any)                    {}
func (p *Page) Clear()                              {}
func (p *Page) GetTitle() string                    { return p.title }
func (p *Page) GetRootView() tview.Primitive        { return p.root }
func (p *Page) GetChildrenViews() []tview.Primitive { return p.children }

// GetChildPanel returns the ordered child Panel for Tab focus highlighting.
func (p *Page) GetChildPanel(index int) ui.Panel {
	if index < 0 || index >= len(p.childPanels) {
		return nil
	}
	return p.childPanels[index]
}

func renderServiceCell(svc model.ServiceModel, col string, pal theme.Palette, mono bool) *tview.TableCell {
	alert := svc.HasEndpointAlert()
	rowColor := pal.DataSecondary
	mutedColor := pal.StatusMuted
	if mono {
		rowColor = tcell.ColorDefault
		mutedColor = tcell.ColorDefault
	} else if alert {
		rowColor = pal.StatusError
	}

	switch col {
	case "NS":
		return ui.NewDataCell(dashIfEmpty(svc.Namespace)).SetTextColor(rowColor).SetMaxWidth(16)
	case "NAME":
		return ui.NewDataCell(dashIfEmpty(svc.Name)).SetTextColor(rowColor).SetMaxWidth(26).SetExpansion(1)
	case "TYPE":
		return ui.NewDataCell(dashIfEmpty(svc.Type)).SetTextColor(rowColor)
	case "CLUSTER-IP":
		return ui.NewDataCell(dashIfEmpty(svc.ClusterIP)).SetTextColor(rowColor)
	case "EXTERNAL-IP":
		return ui.NewDataCell(dashIfEmpty(svc.ExternalIP)).SetTextColor(rowColor).SetMaxWidth(24)
	case "PORTS":
		return ui.NewDataCell(dashIfEmpty(svc.Ports)).SetTextColor(rowColor).SetMaxWidth(24)
	case "ENDPOINTS":
		if !svc.HasEndpoints {
			return ui.NewDataCell("n/a").SetTextColor(mutedColor)
		}
		epCell := ui.NewDataCell(fmt.Sprintf("%d/%d", svc.ReadyEndpoints, svc.TotalEndpoints))
		if mono {
			epCell.SetTextColor(tcell.ColorDefault)
			if alert {
				epCell.SetAttributes(tcell.AttrBold)
			}
			return epCell
		}
		if alert {
			epCell.SetTextColor(pal.StatusError)
		} else if svc.ReadyEndpoints > 0 {
			epCell.SetTextColor(pal.StatusOK)
		} else {
			epCell.SetTextColor(rowColor)
		}
		return epCell
	case "AGE":
		return ui.NewDataCell(dashIfEmpty(svc.Age)).SetTextColor(rowColor)
	default:
		return ui.NewDataCell("").SetTextColor(rowColor)
	}
}

func renderIngressCell(ing model.IngressModel, col string, pal theme.Palette, mono bool) *tview.TableCell {
	rowColor := pal.DataSecondary
	if mono {
		rowColor = tcell.ColorDefault
	}

	switch col {
	case "NS":
		return ui.NewDataCell(dashIfEmpty(ing.Namespace)).SetTextColor(rowColor).SetMaxWidth(16)
	case "NAME":
		return ui.NewDataCell(dashIfEmpty(ing.Name)).SetTextColor(rowColor).SetMaxWidth(26)
	case "CLASS":
		return ui.NewDataCell(dashIfEmpty(ing.ClassName)).SetTextColor(rowColor).SetMaxWidth(16)
	case "HOSTS":
		return ui.NewDataCell(dashIfEmpty(strings.Join(ing.Hosts, ","))).SetTextColor(rowColor).SetExpansion(1)
	case "ADDRESS":
		return ui.NewDataCell(dashIfEmpty(ing.Address)).SetTextColor(rowColor).SetMaxWidth(24)
	case "PORTS":
		return ui.NewDataCell(dashIfEmpty(ing.Ports)).SetTextColor(rowColor)
	case "AGE":
		return ui.NewDataCell(dashIfEmpty(ing.Age)).SetTextColor(rowColor)
	default:
		return ui.NewDataCell("").SetTextColor(rowColor)
	}
}

func dashIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
