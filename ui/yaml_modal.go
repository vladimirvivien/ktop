package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"sigs.k8s.io/yaml"
)

// YAMLModalConfig configures a scrollable YAML inspection modal overlay.
type YAMLModalConfig struct {
	Title   string
	Content string
	OnClose func()
	OnYank  func(content string)
}

// YAMLModal renders syntax-highlighted YAML inside a centered, scrollable modal overlay.
type YAMLModal struct {
	overlay  *OverlayFrame
	box      *tview.Flex
	textView *tview.TextView
	rawYAML  string
	onClose  func()
	onYank   func(content string)
}

// NewYAMLModal constructs a centered, responsive modal displaying highlighted YAML content
// with transparent outer margins so header, tabs, parent frame, and footer stay visible.
func NewYAMLModal(cfg YAMLModalConfig) *YAMLModal {
	m := newYAMLModalCore(cfg)
	m.overlay = NewOverlayFrame(m.box, 4, 3, 5, 5)
	return m
}

// NewYAMLBox constructs the bordered, scrollable YAML view without an outer overlay wrapper,
// suitable for embedding as a sub-view inside an existing modal container.
func NewYAMLBox(cfg YAMLModalConfig) (*tview.Flex, *tview.TextView) {
	m := newYAMLModalCore(cfg)
	return m.box, m.textView
}

func newYAMLModalCore(cfg YAMLModalConfig) *YAMLModal {
	m := &YAMLModal{
		rawYAML: cfg.Content,
		onClose: cfg.OnClose,
		onYank:  cfg.OnYank,
	}

	p := ActivePalette()
	mono := IsMonochrome()

	m.textView = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	if mono {
		m.textView.SetTextColor(tcell.ColorDefault)
		m.textView.SetBackgroundColor(tcell.ColorDefault)
	} else {
		m.textView.SetTextColor(p.CanvasFg)
		m.textView.SetBackgroundColor(p.CanvasBg)
	}
	m.textView.SetText(HighlightYAML(cfg.Content))
	m.textView.ScrollToBeginning()
	m.textView.SetInputCapture(m.HandleInput)

	title := " YAML "
	if strings.TrimSpace(cfg.Title) != "" {
		title = fmt.Sprintf(" %s ", strings.TrimSpace(cfg.Title))
	}

	instrText := "[::b]↑↓/j/k[::-] Scroll   [::b]PgUp/PgDn[::-] Page   [::b]g/G[::-] Top/Bottom   [::b]y[::-] Copy   [::b]Esc/q[::-] Close"
	if !mono {
		keyTag := TagFg(p.DataPrimary)
		instrText = fmt.Sprintf("[%s]↑↓/j/k[-] Scroll   [%s]PgUp/PgDn[-] Page   [%s]g/G[-] Top/Bottom   [%s]y[-] Copy   [%s]Esc/q[-] Close",
			keyTag, keyTag, keyTag, keyTag, keyTag)
	}
	instructions := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignCenter).
		SetText(instrText)
	if mono {
		instructions.SetTextColor(tcell.ColorDefault)
		instructions.SetBackgroundColor(tcell.ColorDefault)
	} else {
		instructions.SetTextColor(p.CanvasFg)
		instructions.SetBackgroundColor(p.CanvasBg)
	}

	box := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(m.textView, 0, 1, true).
		AddItem(instructions, 1, 0, false)
	if mono {
		box.SetBackgroundColor(tcell.ColorDefault)
	} else {
		box.SetBackgroundColor(p.CanvasBg)
	}
	box.SetBorder(true).
		SetTitle(title).
		SetTitleAlign(tview.AlignCenter)
	SetFlexFocused(box, true)
	m.box = box

	return m
}

// GetRootView returns the modal root primitive for ShowModalPage layering.
func (m *YAMLModal) GetRootView() tview.Primitive {
	return m.overlay
}

// HandleInput processes scrolling, copy, and dismissal key events for the YAML modal.
func (m *YAMLModal) HandleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEscape:
		if m.onClose != nil {
			m.onClose()
		}
		return nil
	case tcell.KeyUp:
		row, col := m.textView.GetScrollOffset()
		if row > 0 {
			m.textView.ScrollTo(row-1, col)
		}
		return nil
	case tcell.KeyDown:
		row, col := m.textView.GetScrollOffset()
		m.textView.ScrollTo(row+1, col)
		return nil
	case tcell.KeyPgUp:
		row, col := m.textView.GetScrollOffset()
		_, _, _, h := m.textView.GetInnerRect()
		m.textView.ScrollTo(max(row-max(h, 1), 0), col)
		return nil
	case tcell.KeyPgDn:
		row, col := m.textView.GetScrollOffset()
		_, _, _, h := m.textView.GetInnerRect()
		m.textView.ScrollTo(row+max(h, 1), col)
		return nil
	case tcell.KeyRune:
		switch event.Rune() {
		case 'q':
			if m.onClose != nil {
				m.onClose()
			}
			return nil
		case 'j':
			row, col := m.textView.GetScrollOffset()
			m.textView.ScrollTo(row+1, col)
			return nil
		case 'k':
			row, col := m.textView.GetScrollOffset()
			if row > 0 {
				m.textView.ScrollTo(row-1, col)
			}
			return nil
		case 'g':
			m.textView.ScrollToBeginning()
			return nil
		case 'G':
			m.textView.ScrollToEnd()
			return nil
		case 'y':
			_ = CopyToClipboard(m.rawYAML)
			if m.onYank != nil {
				m.onYank(m.rawYAML)
			}
			return nil
		}
	}
	return nil
}

// FormatObjectYAML marshals a Kubernetes object to YAML, omitting verbose
// metadata.managedFields without mutating the source object.
func FormatObjectYAML(obj any) (string, error) {
	if obj == nil {
		return "", fmt.Errorf("no object available")
	}
	rawJSON, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	var generic map[string]any
	if err := json.Unmarshal(rawJSON, &generic); err == nil && generic != nil {
		if meta, ok := generic["metadata"].(map[string]any); ok {
			delete(meta, "managedFields")
		}
		out, err := yaml.Marshal(generic)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	out, err := yaml.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// HighlightYAML adds theme-aware tview color tags to YAML text while escaping
// literal brackets so dynamic color parsing never mangles YAML sequences or JSON strings.
func HighlightYAML(content string) string {
	var result strings.Builder
	mono := IsMonochrome()
	var mutedOpen, accentOpen, valOpen, closeTag string
	if !mono {
		pal := ActivePalette()
		mutedOpen = "[" + TagFg(pal.StatusMuted) + "]"
		accentOpen = "[" + TagFg(pal.Accent) + "]"
		valOpen = "[" + TagFg(pal.DataSecondary) + "]"
		closeTag = "[-]"
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			result.WriteString("\n")
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			result.WriteString(mutedOpen)
			result.WriteString(tview.Escape(line))
			result.WriteString(closeTag)
			result.WriteString("\n")
			continue
		}

		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			rest := strings.TrimPrefix(trimmed, "- ")
			if trimmed == "-" {
				rest = ""
			}
			if idx := strings.Index(rest, ":"); idx > 0 {
				key := tview.Escape(rest[:idx])
				value := tview.Escape(rest[idx:])
				result.WriteString(indent)
				result.WriteString(valOpen)
				result.WriteString("- ")
				result.WriteString(accentOpen)
				result.WriteString(key)
				result.WriteString(valOpen)
				result.WriteString(value)
				result.WriteString(closeTag)
				result.WriteString("\n")
			} else {
				result.WriteString(indent)
				result.WriteString(valOpen)
				result.WriteString(tview.Escape(trimmed))
				result.WriteString(closeTag)
				result.WriteString("\n")
			}
			continue
		}

		if idx := strings.Index(line, ":"); idx > 0 {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			keyValue := strings.TrimLeft(line, " ")
			colonIdx := strings.Index(keyValue, ":")
			key := tview.Escape(keyValue[:colonIdx])
			value := tview.Escape(keyValue[colonIdx:])
			result.WriteString(indent)
			result.WriteString(accentOpen)
			result.WriteString(key)
			result.WriteString(valOpen)
			result.WriteString(value)
			result.WriteString(closeTag)
			result.WriteString("\n")
			continue
		}

		result.WriteString(valOpen)
		result.WriteString(tview.Escape(line))
		result.WriteString(closeTag)
		result.WriteString("\n")
	}

	return result.String()
}
