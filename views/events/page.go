package events

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
	"github.com/vladimirvivien/ktop/views/model"
)

const (
	eventDetailModalID = "event-detail-modal"
	eventYAMLModalID   = "event-yaml-modal"
)

// Page is the top-level [3] Events cluster-wide chronological event stream view.
type Page struct {
	app             *application.Application
	title           string
	root            *tview.Flex
	summary         *summaryPanel
	tablePanel      *tview.Flex
	table           *tview.Table
	tableCtrl       *ui.TableController
	children        []tview.Primitive
	childPanels     []ui.Panel
	columns         []string
	colMap          map[string]int
	namespaceFilter string

	warningsOnly bool
	paused       bool
	liveSnap     model.EventsSnapshot
	pendingSnap  *model.EventsSnapshot
	displayed    []model.EventModel
}

// New constructs the standalone Events top-level page controller.
func New(app *application.Application, title string) *Page {
	p := &Page{
		app:     app,
		title:   title,
		columns: []string{"LAST SEEN", "TYPE", "REASON", "OBJECT", "NS", "COUNT", "MESSAGE"},
		colMap:  make(map[string]int, 7),
	}
	p.Layout(nil)
	return p
}

// Layout initializes the summary strip and interactive Events table.
func (p *Page) Layout(_ any) {
	if p.root != nil {
		return
	}

	p.summary = newSummaryPanel(fmt.Sprintf(" %s  Summary ", ui.Icons.Clock))

	p.table = tview.NewTable()
	p.table.SetFixed(1, 0)
	p.table.SetBorder(false)
	p.table.SetBorders(false)
	ui.StyleTable(p.table)
	p.table.SetFocusFunc(func() {
		p.table.SetSelectable(true, false)
		p.table.SetSelectedStyle(ui.SelectedRowStyle())
		if p.table.GetRowCount() > 1 {
			row, col := p.table.GetSelection()
			if row < 1 {
				row = 1
			}
			p.table.Select(row, col)
		}
	})
	p.table.SetBlurFunc(func() {
		p.table.SetSelectable(false, false)
	})

	colKeys := map[string]rune{
		"LAST SEEN": 'l',
		"TYPE":      't',
		"REASON":    'r',
		"OBJECT":    'o',
		"NS":        'n',
		"COUNT":     'c',
		"MESSAGE":   'm',
	}
	p.tableCtrl = ui.NewTableController(p.table, ui.TableControllerConfig{
		DefaultSortColumn: "LAST SEEN",
		DefaultSortAsc:    false,
		ColumnToKey:       colKeys,
		OnRedraw:          p.redraw,
		OnSelect: func(row int) bool {
			idx := row - 1
			if idx >= 0 && idx < len(p.displayed) {
				p.openDetailModal(p.displayed[idx])
				return true
			}
			return false
		},
		OnYank: func(row int) bool {
			idx := row - 1
			if idx < 0 || idx >= len(p.displayed) {
				return false
			}
			evt := p.displayed[idx]
			line := fmt.Sprintf("%s %s: %s", evt.ObjectRef(), evt.Reason, evt.Message)
			_ = ui.CopyToClipboard(line)
			if p.app != nil {
				p.app.ShowToast(fmt.Sprintf("Copied event %s (%s) to clipboard", evt.ObjectRef(), evt.Reason), ui.ToastSuccess, 2*time.Second)
			}
			return true
		},
	})

	p.table.SetInputCapture(p.handleTableInput)

	p.tablePanel = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.table, 0, 1, true)
	p.tablePanel.SetBorder(true)
	p.tablePanel.SetTitleAlign(tview.AlignLeft)
	ui.SetFlexFocused(p.tablePanel, false)

	p.children = []tview.Primitive{p.table}
	p.childPanels = []ui.Panel{&tablePanelAdapter{page: p}}

	p.root = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(p.summary.GetRootView(), 3, 0, false).
		AddItem(p.tablePanel, 0, 1, true)

	p.drawHeader()
	p.updateTableTitle()
}

func (p *Page) handleTableInput(event *tcell.EventKey) *tcell.EventKey {
	// Never intercept 'w'/'f' while the user is typing inside the '/' filter bar.
	if p.tableCtrl != nil && p.tableCtrl.IsFilterEditing() {
		return p.tableCtrl.HandleInput(event)
	}

	if event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case 'w':
			p.ToggleWarningsOnly()
			return nil
		case 'f':
			p.TogglePaused()
			return nil
		}
	}
	if p.tableCtrl != nil {
		return p.tableCtrl.HandleInput(event)
	}
	return event
}

// Run subscribes to namespace filter changes and registers the events refresh handler.
func (p *Page) Run(ctx context.Context) error {
	p.Layout(nil)
	if p.app != nil {
		p.app.AddNamespaceFilterListener(func(namespace string) {
			p.SetNamespaceFilter(namespace)
			p.app.Refresh()
		})
		if client := p.app.GetK8sClient(); client != nil {
			client.Controller().SetEventsRefreshFunc(p.RefreshEvents)
		}
	}
	return nil
}

// RefreshEvents receives updated event snapshots from the k8s controller.
func (p *Page) RefreshEvents(_ context.Context, snap model.EventsSnapshot) error {
	if p.app != nil {
		p.app.QueueUpdateDraw(func() {
			p.applySnapshot(snap)
		})
	} else {
		p.applySnapshot(snap)
	}
	return nil
}

func (p *Page) applySnapshot(snap model.EventsSnapshot) {
	if p.paused {
		cp := snap
		p.pendingSnap = &cp
		p.renderSummary()
		return
	}
	p.liveSnap = snap
	p.pendingSnap = nil
	p.redraw()
}

// ToggleWarningsOnly switches between showing all events and Warning events only.
func (p *Page) ToggleWarningsOnly() {
	p.warningsOnly = !p.warningsOnly
	p.redraw()
	p.syncFooter()
}

// TogglePaused freezes or resumes live table updates. Resuming immediately applies
// any buffered snapshot received while paused.
func (p *Page) TogglePaused() {
	p.paused = !p.paused
	if !p.paused && p.pendingSnap != nil {
		p.liveSnap = *p.pendingSnap
		p.pendingSnap = nil
		p.redraw()
	} else {
		p.renderSummary()
		p.updateTableTitle()
		if p.app != nil {
			p.app.Refresh()
		}
	}
	p.syncFooter()
}

// WarningsOnly reports whether the Warnings-only filter is active.
func (p *Page) WarningsOnly() bool { return p.warningsOnly }

// Paused reports whether live updates are paused.
func (p *Page) Paused() bool { return p.paused }

// SetNamespaceFilter updates the active namespace filter synced with the app header.
func (p *Page) SetNamespaceFilter(ns string) {
	trimmed := strings.TrimSpace(ns)
	if p.namespaceFilter == trimmed {
		return
	}
	p.namespaceFilter = trimmed
	p.redraw()
}

func (p *Page) syncFooter() {
	if p.app == nil {
		return
	}
	p.app.SetFooterContext(ui.EventsContext{
		FocusedPanel: "events",
		PageCount:    p.app.PageCount(),
		WarningsOnly: p.warningsOnly,
		Paused:       p.paused,
	})
}

func (p *Page) drawHeader() {
	p.tableCtrl.SetVisibleColumns(p.columns)
	ui.StyleTable(p.table)
	p.colMap = make(map[string]int, len(p.columns))

	for i, col := range p.columns {
		headerText := p.tableCtrl.FormatColumnHeader(col)
		exp := 0
		if col == "MESSAGE" {
			exp = 1
		}
		p.table.SetCell(0, i,
			ui.StyleHeaderCell(tview.NewTableCell(headerText).
				SetAlign(tview.AlignLeft).
				SetExpansion(exp).
				SetSelectable(false)),
		)
		p.colMap[col] = i
	}
	p.table.SetFixed(1, 0)
}

func (p *Page) redraw() {
	selRow, selCol := p.table.GetSelection()

	nsLower := strings.ToLower(strings.TrimSpace(p.namespaceFilter))
	hasNS := nsLower != ""

	scoped := make([]model.EventModel, 0, len(p.liveSnap.Events))
	for _, ev := range p.liveSnap.Events {
		if hasNS && !strings.Contains(strings.ToLower(ev.EffectiveNamespace()), nsLower) {
			continue
		}
		scoped = append(scoped, ev)
	}

	sortCol, sortAsc := p.tableCtrl.GetSortParams()
	sorted := slices.Clone(scoped)
	model.SortEventModelsBy(sorted, sortCol, sortAsc)

	filter := p.tableCtrl.Filter()
	filter.TotalRows = len(sorted)
	hasRowFilter := filter.IsFiltering() && filter.Text != ""

	visible := make([]model.EventModel, 0, len(sorted))
	for _, ev := range sorted {
		if p.warningsOnly && !ev.IsWarning() {
			continue
		}
		if hasRowFilter && !filter.MatchesRow([]string{
			ev.Type,
			ev.Reason,
			ev.ObjectRef(),
			ev.EffectiveNamespace(),
			ev.Source,
			ev.Message,
		}) {
			continue
		}
		visible = append(visible, ev)
	}

	filter.MatchRows = len(visible)
	p.displayed = visible

	p.table.Clear()
	p.drawHeader()

	now := time.Now()
	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()

	for i, ev := range visible {
		rowIdx := i + 1
		rowColor := pal.DataSecondary
		typeColor := pal.StatusOK
		if ev.IsWarning() {
			rowColor = pal.StatusWarning
			typeColor = pal.StatusWarning
		}
		if mono {
			rowColor = tcell.ColorDefault
			typeColor = tcell.ColorDefault
		}

		msg := strings.Join(strings.Fields(ev.Message), " ")
		if msg == "" {
			msg = "-"
		}

		p.table.SetCell(rowIdx, p.colMap["LAST SEEN"], ui.NewDataCell(formatRelativeAge(ev.LastSeen, now)).SetTextColor(rowColor))
		typeCell := ui.NewDataCell(ev.Type).SetTextColor(typeColor)
		if mono && ev.IsWarning() {
			typeCell.SetAttributes(tcell.AttrBold)
		}
		p.table.SetCell(rowIdx, p.colMap["TYPE"], typeCell)
		p.table.SetCell(rowIdx, p.colMap["REASON"], ui.NewDataCell(emptyDash(ev.Reason)).SetTextColor(rowColor).SetMaxWidth(24))
		p.table.SetCell(rowIdx, p.colMap["OBJECT"], ui.NewDataCell(emptyDash(ev.ObjectRef())).SetTextColor(rowColor).SetMaxWidth(34))
		p.table.SetCell(rowIdx, p.colMap["NS"], ui.NewDataCell(emptyDash(ev.EffectiveNamespace())).SetTextColor(rowColor).SetMaxWidth(16))
		p.table.SetCell(rowIdx, p.colMap["COUNT"], ui.NewDataCell(fmt.Sprintf("%d", ev.Count)).SetTextColor(rowColor))
		p.table.SetCell(rowIdx, p.colMap["MESSAGE"], ui.NewDataCell(msg).SetTextColor(rowColor).SetExpansion(1))
	}

	if len(visible) > 0 && p.table.HasFocus() {
		if selRow < 1 {
			selRow = 1
		} else if selRow > len(visible) {
			selRow = len(visible)
		}
		p.table.Select(selRow, selCol)
	}

	p.updateTableTitle()
	p.renderSummary()
	if p.app != nil {
		p.app.Refresh()
	}
}

func (p *Page) renderSummary() {
	if p.summary == nil {
		return
	}
	nsLower := strings.ToLower(strings.TrimSpace(p.namespaceFilter))
	var summary model.EventsSummary
	if nsLower == "" {
		summary = p.liveSnap.Summary
		if summary.Total == 0 && len(p.liveSnap.Events) > 0 {
			summary = model.ComputeEventsSummary(p.liveSnap.Events)
		}
	} else {
		var scoped []model.EventModel
		for _, ev := range p.liveSnap.Events {
			if strings.Contains(strings.ToLower(ev.EffectiveNamespace()), nsLower) {
				scoped = append(scoped, ev)
			}
		}
		summary = model.ComputeEventsSummary(scoped)
	}

	pendingDelta := 0
	if p.paused && p.pendingSnap != nil {
		pendingDelta = countNewEvents(p.liveSnap.Events, p.pendingSnap.Events)
	}

	p.summary.DrawBody(summaryState{
		Summary:      summary,
		WarningsOnly: p.warningsOnly,
		Paused:       p.paused,
		PendingDelta: pendingDelta,
		Truncated:    p.liveSnap.Truncated && nsLower == "",
		Shown:        len(p.liveSnap.Events),
	})
}

func (p *Page) updateTableTitle() {
	if p.tablePanel == nil {
		return
	}
	base := "Events"
	if p.warningsOnly {
		base = "Events (Warnings)"
	}
	var suffix string
	if p.paused {
		suffix += " [PAUSED]"
	}
	if p.app != nil && p.app.IsAPIDisconnected() {
		suffix += " " + ui.ErrorMarkup("[DISCONNECTED - Press R to reconnect]")
	}
	if p.tableCtrl != nil {
		p.tablePanel.SetTitle(p.tableCtrl.FormatTitleWithScroll(base, ui.Icons.TrafficLight, len(p.displayed), suffix))
	} else {
		p.tablePanel.SetTitle(fmt.Sprintf(" %s %s (%d)%s ", ui.Icons.TrafficLight, base, len(p.displayed), suffix))
	}
}

func (p *Page) openDetailModal(evt model.EventModel) {
	if p.app == nil {
		return
	}
	modal := NewDetailModal(DetailModalConfig{
		Event: evt,
		OnClose: func() {
			p.app.DismissModalPage(eventDetailModalID)
			p.app.Focus(p.table)
		},
		OnJump: func(ev model.EventModel) {
			p.app.DismissModalPage(eventDetailModalID)
			if ok, reason := p.app.JumpToObject(ev.InvolvedKind, ev.EffectiveNamespace(), ev.InvolvedName); !ok {
				p.app.Focus(p.table)
				p.app.ShowToast(reason, ui.ToastWarning, 3*time.Second)
			}
		},
		OnYAML: func(ev model.EventModel) {
			var src any = ev.Raw
			if ev.Raw == nil {
				src = ev
			}
			yamlText, err := ui.FormatObjectYAML(src)
			if err != nil {
				p.app.ShowToast(fmt.Sprintf("Unable to render Event YAML: %v", err), ui.ToastError, 3*time.Second)
				return
			}
			yamlModal := ui.NewYAMLModal(ui.YAMLModalConfig{
				Title:   fmt.Sprintf("Event YAML: %s/%s", ev.EffectiveNamespace(), ev.Name),
				Content: yamlText,
				OnClose: func() {
					p.app.DismissModalPage(eventYAMLModalID)
					p.app.SetFooterContext(ui.EventDetailModalContext{
						CanJump:  application.IsInspectableObjectKind(ev.InvolvedKind) && ev.InvolvedName != "",
						JumpKind: ev.InvolvedKind,
					})
				},
				OnYank: func(_ string) {
					p.app.ShowToast(fmt.Sprintf("Copied Event %s YAML to clipboard", ev.Name), ui.ToastSuccess, 2*time.Second)
				},
			})
			p.app.ShowModalPage(eventYAMLModalID, yamlModal.GetRootView())
		},
		OnYank: func(ev model.EventModel) {
			p.app.ShowToast(fmt.Sprintf("Copied message for %s (%s) to clipboard", ev.ObjectRef(), ev.Reason), ui.ToastSuccess, 2*time.Second)
		},
	})

	p.app.ShowModalPage(eventDetailModalID, modal.GetRootView())
	p.app.SetFooterContext(ui.EventDetailModalContext{
		CanJump:  modal.CanJump(),
		JumpKind: evt.InvolvedKind,
	})
}

// IsFilterEditing reports whether the Events table filter input is actively editing.
func (p *Page) IsFilterEditing() bool {
	return p.tableCtrl != nil && p.tableCtrl.IsFilterEditing()
}

// HasEscapableState implements ui.EscapablePanel.
func (p *Page) HasEscapableState() bool {
	return p.tableCtrl != nil && p.tableCtrl.HasEscapableState()
}

// HandleEscape implements ui.EscapablePanel.
func (p *Page) HandleEscape() bool {
	if p.tableCtrl != nil {
		return p.tableCtrl.HandleEscape()
	}
	return false
}

func (p *Page) DrawHeader(_ any) {}
func (p *Page) DrawBody(data any) {
	if snap, ok := data.(model.EventsSnapshot); ok {
		p.applySnapshot(snap)
	}
}
func (p *Page) DrawFooter(_ any)                    {}
func (p *Page) Clear()                              {}
func (p *Page) GetTitle() string                    { return p.title }
func (p *Page) GetRootView() tview.Primitive        { return p.root }
func (p *Page) GetChildrenViews() []tview.Primitive { return p.children }

// GetChildPanel returns the focusable child panel for Tab border highlighting.
func (p *Page) GetChildPanel(index int) ui.Panel {
	if index < 0 || index >= len(p.childPanels) {
		return nil
	}
	return p.childPanels[index]
}

// tablePanelAdapter lets Application highlight the bordered Events table frame on Tab focus.
type tablePanelAdapter struct {
	page *Page
}

func (a *tablePanelAdapter) Layout(_ any)                        {}
func (a *tablePanelAdapter) DrawHeader(_ any)                    {}
func (a *tablePanelAdapter) DrawBody(_ any)                      {}
func (a *tablePanelAdapter) DrawFooter(_ any)                    {}
func (a *tablePanelAdapter) Clear()                              {}
func (a *tablePanelAdapter) GetTitle() string                    { return "Events" }
func (a *tablePanelAdapter) GetRootView() tview.Primitive        { return a.page.tablePanel }
func (a *tablePanelAdapter) GetChildrenViews() []tview.Primitive { return a.page.children }
func (a *tablePanelAdapter) SetFocused(focused bool) {
	ui.SetFlexFocused(a.page.tablePanel, focused)
}

func formatRelativeAge(t, now time.Time) string {
	if t.IsZero() {
		return "n/a"
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func countNewEvents(current, next []model.EventModel) int {
	seen := make(map[string]int32, len(current))
	for _, ev := range current {
		key := ev.Namespace + "/" + ev.Name
		seen[key] = ev.Count
	}
	delta := 0
	for _, ev := range next {
		key := ev.Namespace + "/" + ev.Name
		prevCount, exists := seen[key]
		if !exists || ev.Count > prevCount {
			delta++
		}
	}
	return delta
}
