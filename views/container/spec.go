package container

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/ui"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// SpecPanel displays the full container spec in YAML format
type SpecPanel struct {
	root     *tview.Flex
	specView *tview.TextView
	footer   *tview.TextView
	laidout  bool

	// Container identity
	namespace     string
	podName       string
	containerName string
	containerSpec *corev1.Container

	// Callbacks
	onBack func()
}

// NewSpecPanel creates a new container spec panel
func NewSpecPanel() *SpecPanel {
	p := &SpecPanel{
		root: tview.NewFlex().SetDirection(tview.FlexRow),
	}
	p.setupInputCapture()
	return p
}

// SetOnBack sets the callback for when user wants to go back
func (p *SpecPanel) SetOnBack(callback func()) {
	p.onBack = callback
}

// GetRootView returns the root view for this panel
func (p *SpecPanel) GetRootView() tview.Primitive {
	return p.root
}

// ShowSpec displays the container spec
func (p *SpecPanel) ShowSpec(namespace, podName, containerName string, containerSpec *corev1.Container) {
	p.namespace = namespace
	p.podName = podName
	p.containerName = containerName
	p.containerSpec = containerSpec

	// Build UI if not laid out
	if !p.laidout {
		p.buildLayout()
		p.laidout = true
	}

	// Update title
	p.root.SetTitle(fmt.Sprintf(" %s Container Spec: %s ", ui.Icons.Drum, containerName))

	// Render the spec
	p.renderSpec()
}

func (p *SpecPanel) buildLayout() {
	// Spec view - scrollable TextView with YAML content
	p.specView = tview.NewTextView()
	p.specView.SetDynamicColors(true)
	p.specView.SetScrollable(true)
	p.specView.SetBorder(false)

	// Footer with shortcuts
	p.footer = tview.NewTextView()
	p.footer.SetDynamicColors(true)
	p.footer.SetTextAlign(tview.AlignCenter)
	p.footer.SetText("[yellow]↑↓/j/k[white] scroll  [yellow]PgUp/PgDn[white] page  [yellow]g/G[white] top/bottom  [yellow]ESC[white] back")

	// Assemble layout
	p.root.Clear()
	p.root.AddItem(p.specView, 0, 1, true) // Spec takes all space
	p.root.AddItem(p.footer, 1, 0, false)  // Footer: 1 row

	p.root.SetBorder(true)
	p.root.SetTitleAlign(tview.AlignLeft)
	p.root.SetBorderColor(tcell.ColorWhite)
}

func (p *SpecPanel) renderSpec() {
	if p.containerSpec == nil {
		p.specView.SetText("[red]No container spec available[-]")
		return
	}

	// Marshal container spec to YAML
	yamlBytes, err := yaml.Marshal(p.containerSpec)
	if err != nil {
		p.specView.SetText(fmt.Sprintf("[red]Error rendering spec: %v[-]", err))
		return
	}

	// Add syntax highlighting
	highlighted := highlightYAML(string(yamlBytes))
	p.specView.SetText(highlighted)

	// Scroll to top
	p.specView.ScrollToBeginning()
}

// highlightYAML adds tview color codes to YAML content
func highlightYAML(content string) string {
	var result strings.Builder

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines
		if trimmed == "" {
			result.WriteString("\n")
			continue
		}

		// Comments in gray
		if strings.HasPrefix(trimmed, "#") {
			result.WriteString("[gray]")
			result.WriteString(line)
			result.WriteString("[-]\n")
			continue
		}

		// List items (lines starting with -)
		if strings.HasPrefix(trimmed, "-") {
			// Find leading whitespace
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			rest := strings.TrimPrefix(trimmed, "- ")

			// Check if rest has a key: value
			if idx := strings.Index(rest, ":"); idx > 0 {
				key := rest[:idx]
				value := rest[idx:]
				result.WriteString(indent)
				result.WriteString("[white]- [aqua]")
				result.WriteString(key)
				result.WriteString("[white]")
				result.WriteString(value)
				result.WriteString("[-]\n")
			} else {
				result.WriteString(indent)
				result.WriteString("[white]- ")
				result.WriteString(rest)
				result.WriteString("[-]\n")
			}
			continue
		}

		// Key: value pairs
		if idx := strings.Index(line, ":"); idx > 0 {
			// Preserve leading whitespace
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			keyValue := strings.TrimLeft(line, " ")
			colonIdx := strings.Index(keyValue, ":")

			key := keyValue[:colonIdx]
			value := keyValue[colonIdx:]

			result.WriteString(indent)
			result.WriteString("[aqua]")
			result.WriteString(key)
			result.WriteString("[white]")
			result.WriteString(value)
			result.WriteString("[-]\n")
			continue
		}

		// Default: just output the line
		result.WriteString(line)
		result.WriteString("\n")
	}

	return result.String()
}

func (p *SpecPanel) setupInputCapture() {
	p.root.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			if p.onBack != nil {
				p.onBack()
			}
			return nil

		case tcell.KeyUp:
			row, col := p.specView.GetScrollOffset()
			if row > 0 {
				p.specView.ScrollTo(row-1, col)
			}
			return nil

		case tcell.KeyDown:
			row, col := p.specView.GetScrollOffset()
			p.specView.ScrollTo(row+1, col)
			return nil

		case tcell.KeyPgUp:
			row, col := p.specView.GetScrollOffset()
			_, _, _, height := p.specView.GetInnerRect()
			newRow := row - height
			if newRow < 0 {
				newRow = 0
			}
			p.specView.ScrollTo(newRow, col)
			return nil

		case tcell.KeyPgDn:
			row, col := p.specView.GetScrollOffset()
			_, _, _, height := p.specView.GetInnerRect()
			p.specView.ScrollTo(row+height, col)
			return nil

		case tcell.KeyRune:
			switch event.Rune() {
			case 'j':
				row, col := p.specView.GetScrollOffset()
				p.specView.ScrollTo(row+1, col)
				return nil
			case 'k':
				row, col := p.specView.GetScrollOffset()
				if row > 0 {
					p.specView.ScrollTo(row-1, col)
				}
				return nil
			case 'g':
				p.specView.ScrollToBeginning()
				return nil
			case 'G':
				p.specView.ScrollToEnd()
				return nil
			}
		}
		return event
	})
}

// SetFocused implements ui.FocusablePanel
func (p *SpecPanel) SetFocused(focused bool) {
	ui.SetFlexFocused(p.root, focused)
}

// HasEscapableState implements ui.EscapablePanel
func (p *SpecPanel) HasEscapableState() bool {
	return true // Always allow ESC to go back
}

// HandleEscape implements ui.EscapablePanel
func (p *SpecPanel) HandleEscape() bool {
	if p.onBack != nil {
		p.onBack()
		return true
	}
	return false
}
