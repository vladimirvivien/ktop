package events

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
)

// DetailModalConfig configures the floating Event Detail modal.
type DetailModalConfig struct {
	Event   model.EventModel
	OnClose func()
	OnJump  func(evt model.EventModel)
	OnYAML  func(evt model.EventModel)
	OnYank  func(evt model.EventModel)
}

// DetailModal renders a centered floating modal with full event metadata and
// an untruncated word-wrapped message, plus optional jump/YAML/copy actions.
type DetailModal struct {
	overlay *ui.OverlayFrame
	box     *tview.Flex
	msgView *tview.TextView
	event   model.EventModel
	canJump bool
	onClose func()
	onJump  func(evt model.EventModel)
	onYAML  func(evt model.EventModel)
	onYank  func(evt model.EventModel)
}

// NewDetailModal constructs a floating Event Detail modal overlay.
func NewDetailModal(cfg DetailModalConfig) *DetailModal {
	canJump := application.IsInspectableObjectKind(cfg.Event.InvolvedKind) && cfg.Event.InvolvedName != ""
	m := &DetailModal{
		event:   cfg.Event,
		canJump: canJump,
		onClose: cfg.OnClose,
		onJump:  cfg.OnJump,
		onYAML:  cfg.OnYAML,
		onYank:  cfg.OnYank,
	}

	pal := ui.ActivePalette()
	mono := ui.IsMonochrome()

	metaTable := tview.NewTable()
	metaTable.SetBorder(false)
	metaTable.SetBorders(false)
	metaTable.SetSelectable(false, false)
	ui.StyleTable(metaTable)

	ev := cfg.Event
	typeColor := pal.StatusOK
	if ev.IsWarning() {
		typeColor = pal.StatusWarning
	}
	if mono {
		typeColor = tcell.ColorDefault
	}

	objectStr := ev.ObjectRef()
	if ns := ev.EffectiveNamespace(); ns != "" && !strings.EqualFold(ev.InvolvedKind, "Node") {
		objectStr = fmt.Sprintf("%s (%s)", objectStr, ns)
	}
	if objectStr == "" {
		objectStr = "-"
	}

	sourceStr := ev.Source
	if sourceStr == "" {
		sourceStr = "-"
	}

	firstSeenStr := formatEventTimestamp(ev.FirstSeen)
	lastSeenStr := formatEventTimestamp(ev.LastSeen)

	row := 0
	addRow := func(label, value string, valColor tcell.Color) {
		lblCell := ui.NewDataCell(label + ":").SetSelectable(false)
		valCell := ui.NewDataCell(value).SetSelectable(false)
		if mono {
			lblCell.SetTextColor(tcell.ColorDefault)
			valCell.SetTextColor(tcell.ColorDefault)
		} else {
			lblCell.SetTextColor(pal.DataPrimary)
			valCell.SetTextColor(valColor)
		}
		metaTable.SetCell(row, 0, lblCell)
		metaTable.SetCell(row, 1, valCell)
		row++
	}

	defaultValColor := pal.DataSecondary
	if mono {
		defaultValColor = tcell.ColorDefault
	}
	addRow("Type", ev.Type, typeColor)
	addRow("Reason", emptyDash(ev.Reason), typeColor)
	addRow("Object", objectStr, defaultValColor)
	addRow("Count", fmt.Sprintf("%d", ev.Count), defaultValColor)
	addRow("First Seen", firstSeenStr, defaultValColor)
	addRow("Last Seen", lastSeenStr, defaultValColor)
	addRow("Source", sourceStr, defaultValColor)
	addRow("Event Name", emptyDash(ev.Name), defaultValColor)

	msgView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(true).
		SetWordWrap(true)
	if mono {
		msgView.SetTextColor(tcell.ColorDefault)
		msgView.SetBackgroundColor(tcell.ColorDefault)
	} else {
		msgView.SetTextColor(pal.CanvasFg)
		msgView.SetBackgroundColor(pal.CanvasBg)
	}
	msgHeader := "[::b]Message[::-]\n"
	if !mono {
		msgHeader = fmt.Sprintf("[%s::b]Message[-::-]\n", ui.TagFg(pal.DataPrimary))
	}
	msgBody := ev.Message
	if msgBody == "" {
		msgBody = "(no message)"
	}
	msgView.SetText(msgHeader + tview.Escape(msgBody))
	msgView.ScrollToBeginning()
	msgView.SetInputCapture(m.HandleInput)
	m.msgView = msgView

	instructions := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(m.formatInstructions(ui.TagFg(pal.DataPrimary), mono))
	if mono {
		instructions.SetTextColor(tcell.ColorDefault)
		instructions.SetBackgroundColor(tcell.ColorDefault)
	} else {
		instructions.SetTextColor(pal.CanvasFg)
		instructions.SetBackgroundColor(pal.CanvasBg)
	}

	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(metaTable, row+1, 0, false).
		AddItem(msgView, 0, 1, true).
		AddItem(instructions, 1, 0, false)
	if mono {
		box.SetBackgroundColor(tcell.ColorDefault)
	} else {
		box.SetBackgroundColor(pal.CanvasBg)
	}

	title := fmt.Sprintf(" Event Detail: %s / %s ", ev.Type, emptyDash(ev.Reason))
	box.SetBorder(true).
		SetTitle(title).
		SetTitleAlign(tview.AlignCenter)
	ui.SetFlexFocused(box, true)
	m.box = box
	m.overlay = ui.NewOverlayFrame(box, 4, 3, 6, 6)

	return m
}

func (m *DetailModal) formatInstructions(keyTag string, mono bool) string {
	var parts []string
	add := func(key, label string) {
		if mono {
			parts = append(parts, fmt.Sprintf("[::b]%s[::-] %s", key, label))
		} else {
			parts = append(parts, fmt.Sprintf("[%s]%s[-] %s", keyTag, key, label))
		}
	}
	add("↑↓/j/k", "Scroll")
	if m.canJump {
		add("g", "Inspect "+m.event.InvolvedKind)
	}
	add("v", "YAML")
	add("y", "Copy")
	add("Esc/q", "Close")
	return strings.Join(parts, "   ")
}

// GetRootView returns the floating modal root primitive for ShowModalPage layering.
func (m *DetailModal) GetRootView() tview.Primitive {
	return m.overlay
}

// CanJump reports whether the event's involved object supports direct inspection.
func (m *DetailModal) CanJump() bool {
	return m.canJump
}

// HandleInput processes scrolling, jump, YAML, copy, and close keys for the Event Detail modal.
func (m *DetailModal) HandleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		if m.onClose != nil {
			m.onClose()
		}
		return nil
	case tcell.KeyUp:
		row, col := m.msgView.GetScrollOffset()
		if row > 0 {
			m.msgView.ScrollTo(row-1, col)
		}
		return nil
	case tcell.KeyDown:
		row, col := m.msgView.GetScrollOffset()
		m.msgView.ScrollTo(row+1, col)
		return nil
	case tcell.KeyPgUp:
		row, col := m.msgView.GetScrollOffset()
		_, _, _, h := m.msgView.GetInnerRect()
		m.msgView.ScrollTo(max(row-max(h, 1), 0), col)
		return nil
	case tcell.KeyPgDn:
		row, col := m.msgView.GetScrollOffset()
		_, _, _, h := m.msgView.GetInnerRect()
		m.msgView.ScrollTo(row+max(h, 1), col)
		return nil
	case tcell.KeyHome:
		m.msgView.ScrollToBeginning()
		return nil
	case tcell.KeyEnd:
		m.msgView.ScrollToEnd()
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case 'q':
			if m.onClose != nil {
				m.onClose()
			}
			return nil
		case 'j':
			row, col := m.msgView.GetScrollOffset()
			m.msgView.ScrollTo(row+1, col)
			return nil
		case 'k':
			row, col := m.msgView.GetScrollOffset()
			if row > 0 {
				m.msgView.ScrollTo(row-1, col)
			}
			return nil
		case 'g':
			if m.canJump && m.onJump != nil {
				m.onJump(m.event)
			}
			return nil
		case 'v':
			if m.onYAML != nil {
				m.onYAML(m.event)
			}
			return nil
		case 'y':
			_ = ui.CopyToClipboard(m.event.Message)
			if m.onYank != nil {
				m.onYank(m.event)
			}
			return nil
		}
	}
	return nil
}

func formatEventTimestamp(t time.Time) string {
	if t.IsZero() {
		return "n/a"
	}
	return fmt.Sprintf("%s (%s ago)", t.Local().Format("2006-01-02 15:04:05"), formatRelativeAge(t, time.Now()))
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
