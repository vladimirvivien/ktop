package storage

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

const storageYAMLModalID = "storage-yaml-modal"

type storageTableConfig[T any] struct {
	title          string
	icon           string
	columns        []string
	expandCol      string
	columnToKey    map[string]rune
	defaultSortCol string
	defaultSortAsc bool
	sortFn         func(items []T, col string, asc bool)
	filterMatchFn  func(item T, nsLower string) bool
	filterCellsFn  func(item T) []string
	cellFn         func(item T, col string, pal theme.Palette, mono bool) *tview.TableCell
	yamlInfoFn     func(item T) (title string, obj any)
	yankTargetFn   func(item T) string
}

type storageTable[T any] struct {
	app             *application.Application
	cfg             storageTableConfig[T]
	root            *tview.Flex
	table           *tview.Table
	ctrl            *ui.TableController
	allItems        []T
	displayed       []T
	namespaceFilter string
	colMap          map[string]int
}

func newStorageTable[T any](app *application.Application, cfg storageTableConfig[T]) *storageTable[T] {
	st := &storageTable[T]{
		app:    app,
		cfg:    cfg,
		colMap: make(map[string]int, len(cfg.columns)),
	}
	st.layout()
	st.drawHeader()
	st.updateTitle(0)
	return st
}

func (s *storageTable[T]) layout() {
	s.table = tview.NewTable()
	s.table.SetFixed(1, 0)
	s.table.SetBorder(false)
	s.table.SetBorders(false)
	ui.StyleTable(s.table)
	s.table.SetFocusFunc(func() {
		s.table.SetSelectable(true, false)
		s.table.SetSelectedStyle(ui.SelectedRowStyle())
		if s.table.GetRowCount() > 1 {
			row, col := s.table.GetSelection()
			if row < 1 {
				row = 1
			}
			s.table.Select(row, col)
		}
	})
	s.table.SetBlurFunc(func() {
		s.table.SetSelectable(false, false)
	})

	s.ctrl = ui.NewTableController(s.table, ui.TableControllerConfig{
		DefaultSortColumn: s.cfg.defaultSortCol,
		DefaultSortAsc:    s.cfg.defaultSortAsc,
		ColumnToKey:       s.cfg.columnToKey,
		OnRedraw:          s.redraw,
		OnSelect: func(row int) bool {
			idx := row - 1
			if idx < 0 || idx >= len(s.displayed) || s.app == nil || s.cfg.yamlInfoFn == nil {
				return false
			}
			title, src := s.cfg.yamlInfoFn(s.displayed[idx])
			yamlText, err := ui.FormatObjectYAML(src)
			if err != nil {
				s.app.ShowToast(fmt.Sprintf("Unable to render YAML: %v", err), ui.ToastError, 3*time.Second)
				return true
			}
			modal := ui.NewYAMLModal(ui.YAMLModalConfig{
				Title:   title,
				Content: yamlText,
				OnClose: func() {
					s.app.DismissModalPage(storageYAMLModalID)
					s.app.Focus(s.table)
				},
				OnYank: func(_ string) {
					s.app.ShowToast(fmt.Sprintf("Copied %s YAML to clipboard", title), ui.ToastSuccess, 2*time.Second)
				},
			})
			s.app.ShowModalPage(storageYAMLModalID, modal.GetRootView())
			return true
		},
		OnYank: func(row int) bool {
			idx := row - 1
			if idx < 0 || idx >= len(s.displayed) || s.cfg.yankTargetFn == nil {
				return false
			}
			target := s.cfg.yankTargetFn(s.displayed[idx])
			if target == "" {
				return false
			}
			_ = ui.CopyToClipboard(target)
			if s.app != nil {
				s.app.ShowToast(fmt.Sprintf("Copied %q to clipboard", target), ui.ToastSuccess, 2*time.Second)
			}
			return true
		},
	})
	s.table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return s.ctrl.HandleInput(event)
	})

	s.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(s.table, 0, 1, true)
	s.root.SetBorder(true)
	s.root.SetTitleAlign(tview.AlignLeft)
	ui.SetFlexFocused(s.root, false)
}

func (s *storageTable[T]) drawHeader() {
	s.ctrl.SetVisibleColumns(s.cfg.columns)
	ui.StyleTable(s.table)
	s.colMap = make(map[string]int, len(s.cfg.columns))

	for i, col := range s.cfg.columns {
		headerText := s.ctrl.FormatColumnHeader(col)
		exp := 0
		if col == s.cfg.expandCol {
			exp = 1
		}
		s.table.SetCell(0, i,
			ui.StyleHeaderCell(tview.NewTableCell(headerText).
				SetAlign(tview.AlignLeft).
				SetExpansion(exp).
				SetSelectable(false)),
		)
		s.colMap[col] = i
	}
	s.table.SetFixed(1, 0)
}

func (s *storageTable[T]) setItems(items []T) {
	s.allItems = slices.Clone(items)
	s.applyItems()
}

func (s *storageTable[T]) applyItems() {
	selRow, selCol := s.table.GetSelection()

	sorted := slices.Clone(s.allItems)
	sortCol, sortAsc := s.ctrl.GetSortParams()
	if s.cfg.sortFn != nil {
		s.cfg.sortFn(sorted, sortCol, sortAsc)
	}

	filter := s.ctrl.Filter()
	filter.TotalRows = len(sorted)

	nsLower := strings.ToLower(strings.TrimSpace(s.namespaceFilter))
	hasNS := nsLower != ""
	hasRowFilter := filter.IsFiltering() && filter.Text != ""

	visible := make([]T, 0, len(sorted))
	for _, item := range sorted {
		if hasNS && s.cfg.filterMatchFn != nil && !s.cfg.filterMatchFn(item, nsLower) {
			continue
		}
		if hasRowFilter && s.cfg.filterCellsFn != nil && !filter.MatchesRow(s.cfg.filterCellsFn(item)) {
			continue
		}
		visible = append(visible, item)
	}

	filter.MatchRows = len(visible)
	s.displayed = visible

	s.table.Clear()
	s.drawHeader()
	s.updateTitle(len(visible))

	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()
	for i, item := range visible {
		rowIdx := i + 1
		for _, col := range s.cfg.columns {
			colIdx := s.colMap[col]
			s.table.SetCell(rowIdx, colIdx, s.cfg.cellFn(item, col, pal, mono))
		}
	}

	if len(visible) > 0 && s.table.HasFocus() {
		if selRow < 1 {
			selRow = 1
		} else if selRow > len(visible) {
			selRow = len(visible)
		}
		s.table.Select(selRow, selCol)
	}
}

func (s *storageTable[T]) redraw() {
	s.applyItems()
	if s.app != nil {
		s.app.Refresh()
	}
}

func (s *storageTable[T]) updateTitle(count int) {
	if s.root == nil {
		return
	}
	var suffix string
	if s.app != nil && s.app.IsAPIDisconnected() {
		suffix = " " + ui.ErrorMarkup("[DISCONNECTED - Press R to reconnect]")
	}
	if s.ctrl != nil {
		s.root.SetTitle(s.ctrl.FormatTitleWithScroll(s.cfg.title, s.cfg.icon, count, suffix))
	} else {
		s.root.SetTitle(fmt.Sprintf(" %s %s (%d)%s ", s.cfg.icon, s.cfg.title, count, suffix))
	}
}

func (s *storageTable[T]) setNamespaceFilter(ns string) {
	if s.namespaceFilter == ns {
		return
	}
	s.namespaceFilter = ns
	s.applyItems()
}

func (s *storageTable[T]) Layout(_ any)                 {}
func (s *storageTable[T]) DrawHeader(_ any)             {}
func (s *storageTable[T]) DrawBody(_ any)               {}
func (s *storageTable[T]) DrawFooter(_ any)             {}
func (s *storageTable[T]) Clear()                       {}
func (s *storageTable[T]) GetTitle() string             { return s.cfg.title }
func (s *storageTable[T]) GetRootView() tview.Primitive { return s.root }
func (s *storageTable[T]) GetChildrenViews() []tview.Primitive {
	return []tview.Primitive{s.table}
}
func (s *storageTable[T]) SetFocused(focused bool) { ui.SetFlexFocused(s.root, focused) }

// Page is the top-level [4] Storage view showing Summary, PersistentVolumeClaims, and PersistentVolumes.
type Page struct {
	app             *application.Application
	title           string
	root            *tview.Flex
	summary         *summaryPanel
	pvcs            *storageTable[model.PVCModel]
	pvs             *storageTable[model.PVModel]
	children        []tview.Primitive
	childPanels     []ui.Panel
	namespaceFilter string
	liveSnap        model.StorageSnapshot
}

// New constructs the Storage top-level page controller.
func New(app *application.Application, title string) *Page {
	p := &Page{
		app:   app,
		title: title,
	}
	p.Layout(nil)
	return p
}

// Layout initializes the summary strip, PVCs table, and PVs table.
func (p *Page) Layout(_ any) {
	if p.root != nil {
		return
	}

	p.summary = newSummaryPanel(fmt.Sprintf(" %s  Summary ", ui.Icons.Thermometer))

	p.pvcs = newStorageTable(p.app, storageTableConfig[model.PVCModel]{
		title:     "PersistentVolumeClaims",
		icon:      ui.Icons.Package,
		columns:   []string{"NS", "NAME", "STATUS", "CAPACITY", "USED", "FILL%", "INODES%", "CLASS", "VOLUME", "AGE"},
		expandCol: "NAME",
		columnToKey: map[string]rune{
			"NS":       'n',
			"NAME":     'm',
			"STATUS":   's',
			"CAPACITY": 'c',
			"USED":     'u',
			"FILL%":    'f',
			"INODES%":  'i',
			"CLASS":    'l',
			"VOLUME":   'v',
			"AGE":      'a',
		},
		defaultSortCol: "NS",
		defaultSortAsc: true,
		sortFn:         model.SortPVCModelsBy,
		filterMatchFn: func(pvc model.PVCModel, nsLower string) bool {
			return strings.Contains(strings.ToLower(pvc.Namespace), nsLower)
		},
		filterCellsFn: func(pvc model.PVCModel) []string {
			return []string{pvc.Namespace, pvc.Name, pvc.Status, pvc.StorageClass, pvc.VolumeName}
		},
		cellFn: renderPVCCell,
		yamlInfoFn: func(pvc model.PVCModel) (string, any) {
			var src any = pvc.Raw
			if pvc.Raw == nil {
				src = pvc
			}
			return fmt.Sprintf("PVC YAML: %s/%s", pvc.Namespace, pvc.Name), src
		},
		yankTargetFn: func(pvc model.PVCModel) string {
			return pvc.Namespace + "/" + pvc.Name
		},
	})

	p.pvs = newStorageTable(p.app, storageTableConfig[model.PVModel]{
		title:     "PersistentVolumes",
		icon:      ui.Icons.Controller,
		columns:   []string{"NAME", "STATUS", "CAPACITY", "ACCESS", "RECLAIM", "CLASS", "CLAIM", "AGE"},
		expandCol: "NAME",
		columnToKey: map[string]rune{
			"NAME":     'm',
			"STATUS":   's',
			"CAPACITY": 'c',
			"ACCESS":   'x',
			"RECLAIM":  'r',
			"CLASS":    'l',
			"CLAIM":    'p',
			"AGE":      'a',
		},
		defaultSortCol: "NAME",
		defaultSortAsc: true,
		sortFn:         model.SortPVModelsBy,
		filterMatchFn: func(pv model.PVModel, nsLower string) bool {
			ns, _, ok := strings.Cut(pv.ClaimRef, "/")
			if !ok {
				return false
			}
			return strings.Contains(strings.ToLower(ns), nsLower)
		},
		filterCellsFn: func(pv model.PVModel) []string {
			return []string{pv.Name, pv.Status, pv.StorageClass, pv.ClaimRef, strings.Join(pv.AccessModes, ",")}
		},
		cellFn: renderPVCell,
		yamlInfoFn: func(pv model.PVModel) (string, any) {
			var src any = pv.Raw
			if pv.Raw == nil {
				src = pv
			}
			return fmt.Sprintf("PV YAML: %s", pv.Name), src
		},
		yankTargetFn: func(pv model.PVModel) string {
			return pv.Name
		},
	})

	p.children = []tview.Primitive{p.pvcs.table, p.pvs.table}
	p.childPanels = []ui.Panel{p.pvcs, p.pvs}

	p.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.summary.GetRootView(), 3, 0, false).
		AddItem(p.pvcs.GetRootView(), 0, 3, true).
		AddItem(p.pvs.GetRootView(), 0, 2, false)
}

// Run subscribes to namespace filter changes and registers the storage refresh handler.
func (p *Page) Run(_ context.Context) error {
	p.Layout(nil)
	if p.app != nil {
		p.app.AddNamespaceFilterListener(func(namespace string) {
			p.SetNamespaceFilter(namespace)
			p.app.Refresh()
		})
		if client := p.app.GetK8sClient(); client != nil {
			client.Controller().SetStorageRefreshFunc(p.RefreshStorage)
		}
	}
	return nil
}

// RefreshStorage receives updated storage snapshots from the k8s controller.
func (p *Page) RefreshStorage(_ context.Context, snap model.StorageSnapshot) error {
	if p.app != nil {
		p.app.QueueUpdateDraw(func() {
			p.applySnapshot(snap)
		})
	} else {
		p.applySnapshot(snap)
	}
	return nil
}

func (p *Page) applySnapshot(snap model.StorageSnapshot) {
	p.liveSnap = snap
	p.pvcs.setItems(snap.PVCs)
	p.pvs.setItems(snap.PVs)
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
	p.pvcs.setNamespaceFilter(trimmed)
	p.pvs.setNamespaceFilter(trimmed)
	p.renderSummary()
}

func (p *Page) renderSummary() {
	if p.summary == nil {
		return
	}
	nsLower := strings.ToLower(strings.TrimSpace(p.namespaceFilter))
	if nsLower == "" {
		summary := p.liveSnap.Summary
		if summary.PVCsTotal == 0 && summary.PVsTotal == 0 && (len(p.liveSnap.PVCs) > 0 || len(p.liveSnap.PVs) > 0) {
			summary = model.ComputeStorageSummary(p.liveSnap.PVCs, p.liveSnap.PVs, p.liveSnap.StorageClasses, summary.DiskReadRateBps, summary.DiskWriteRateBps)
		}
		p.summary.DrawBody(summary)
		return
	}

	var scopedPVCs []model.PVCModel
	for _, pvc := range p.liveSnap.PVCs {
		if strings.Contains(strings.ToLower(pvc.Namespace), nsLower) {
			scopedPVCs = append(scopedPVCs, pvc)
		}
	}
	var scopedPVs []model.PVModel
	for _, pv := range p.liveSnap.PVs {
		if claimNS, _, ok := strings.Cut(pv.ClaimRef, "/"); ok && strings.Contains(strings.ToLower(claimNS), nsLower) {
			scopedPVs = append(scopedPVs, pv)
		}
	}
	summary := model.ComputeStorageSummary(
		scopedPVCs,
		scopedPVs,
		p.liveSnap.StorageClasses,
		p.liveSnap.Summary.DiskReadRateBps,
		p.liveSnap.Summary.DiskWriteRateBps,
	)
	p.summary.DrawBody(summary)
}

// IsFilterEditing reports whether any child table filter input is actively editing.
func (p *Page) IsFilterEditing() bool {
	return p.pvcs.ctrl.IsFilterEditing() || p.pvs.ctrl.IsFilterEditing()
}

// HasEscapableState implements ui.EscapablePanel.
func (p *Page) HasEscapableState() bool {
	return p.pvcs.ctrl.HasEscapableState() || p.pvs.ctrl.HasEscapableState()
}

// HandleEscape implements ui.EscapablePanel.
func (p *Page) HandleEscape() bool {
	if p.pvcs.ctrl.HandleEscape() {
		return true
	}
	return p.pvs.ctrl.HandleEscape()
}

func (p *Page) DrawHeader(_ any) {}
func (p *Page) DrawBody(data any) {
	if snap, ok := data.(model.StorageSnapshot); ok {
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

func renderPVCCell(pvc model.PVCModel, col string, pal theme.Palette, mono bool) *tview.TableCell {
	rowColor := pal.DataSecondary
	mutedColor := pal.StatusMuted
	if mono {
		rowColor = tcell.ColorDefault
		mutedColor = tcell.ColorDefault
	}

	switch col {
	case "NS":
		return ui.NewDataCell(dashIfEmpty(pvc.Namespace)).SetTextColor(rowColor).SetMaxWidth(16)
	case "NAME":
		return ui.NewDataCell(dashIfEmpty(pvc.Name)).SetTextColor(rowColor).SetMaxWidth(28).SetExpansion(1)
	case "STATUS":
		return storageStatusCell(pvc.Status, pal, mono)
	case "CAPACITY":
		if pvc.CapacityBytes <= 0 {
			return ui.NewDataCell("n/a").SetTextColor(mutedColor)
		}
		return ui.NewDataCell(ui.FormatBytes(pvc.CapacityBytes)).SetTextColor(rowColor)
	case "USED":
		if !pvc.HasUsage {
			return ui.NewDataCell("n/a").SetTextColor(mutedColor)
		}
		return ui.NewDataCell(ui.FormatBytes(pvc.UsedBytes)).SetTextColor(rowColor)
	case "FILL%":
		return thresholdPctCell(pvc.HasUsage, pvc.FillPct, pal, mono, rowColor, mutedColor)
	case "INODES%":
		return thresholdPctCell(pvc.HasInodes, pvc.InodesUsedPct, pal, mono, rowColor, mutedColor)
	case "CLASS":
		return ui.NewDataCell(dashIfEmpty(pvc.StorageClass)).SetTextColor(rowColor).SetMaxWidth(18)
	case "VOLUME":
		return ui.NewDataCell(dashIfEmpty(pvc.VolumeName)).SetTextColor(rowColor).SetMaxWidth(24)
	case "AGE":
		return ui.NewDataCell(dashIfEmpty(pvc.Age)).SetTextColor(rowColor)
	default:
		return ui.NewDataCell("").SetTextColor(rowColor)
	}
}

func renderPVCell(pv model.PVModel, col string, pal theme.Palette, mono bool) *tview.TableCell {
	rowColor := pal.DataSecondary
	mutedColor := pal.StatusMuted
	if mono {
		rowColor = tcell.ColorDefault
		mutedColor = tcell.ColorDefault
	}

	switch col {
	case "NAME":
		return ui.NewDataCell(dashIfEmpty(pv.Name)).SetTextColor(rowColor).SetMaxWidth(28).SetExpansion(1)
	case "STATUS":
		return storageStatusCell(pv.Status, pal, mono)
	case "CAPACITY":
		if pv.CapacityBytes <= 0 {
			return ui.NewDataCell("n/a").SetTextColor(mutedColor)
		}
		return ui.NewDataCell(ui.FormatBytes(pv.CapacityBytes)).SetTextColor(rowColor)
	case "ACCESS":
		return ui.NewDataCell(dashIfEmpty(strings.Join(pv.AccessModes, ","))).SetTextColor(rowColor).SetMaxWidth(14)
	case "RECLAIM":
		return ui.NewDataCell(dashIfEmpty(pv.ReclaimPolicy)).SetTextColor(rowColor)
	case "CLASS":
		return ui.NewDataCell(dashIfEmpty(pv.StorageClass)).SetTextColor(rowColor).SetMaxWidth(18)
	case "CLAIM":
		return ui.NewDataCell(dashIfEmpty(pv.ClaimRef)).SetTextColor(rowColor).SetMaxWidth(28)
	case "AGE":
		return ui.NewDataCell(dashIfEmpty(pv.Age)).SetTextColor(rowColor)
	default:
		return ui.NewDataCell("").SetTextColor(rowColor)
	}
}

func storageStatusCell(status string, pal theme.Palette, mono bool) *tview.TableCell {
	c := ui.NewDataCell(dashIfEmpty(status))
	if mono {
		c.SetTextColor(tcell.ColorDefault)
		if status != "Bound" && status != "Available" {
			c.SetAttributes(tcell.AttrBold)
		}
		return c
	}
	switch status {
	case "Bound", "Available":
		c.SetTextColor(pal.StatusOK)
	case "Pending", "Released":
		c.SetTextColor(pal.StatusWarning)
	case "Lost", "Failed":
		c.SetTextColor(pal.StatusError)
	default:
		c.SetTextColor(pal.DataSecondary)
	}
	return c
}

func thresholdPctCell(ok bool, pct float64, pal theme.Palette, mono bool, rowColor, mutedColor tcell.Color) *tview.TableCell {
	if !ok {
		return ui.NewDataCell("n/a").SetTextColor(mutedColor)
	}
	cell := ui.NewDataCell(fmt.Sprintf("%.0f%%", pct))
	if mono {
		cell.SetTextColor(tcell.ColorDefault)
		if pct >= 85 {
			cell.SetAttributes(tcell.AttrBold)
		}
		return cell
	}
	switch {
	case pct >= 95:
		cell.SetTextColor(pal.StatusError)
	case pct >= 85:
		cell.SetTextColor(pal.StatusWarning)
	default:
		cell.SetTextColor(rowColor)
	}
	return cell
}

func dashIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
