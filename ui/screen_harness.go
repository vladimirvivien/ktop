package ui

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// TestScreen wraps a tcell.SimulationScreen for deterministic headless TUI testing
// and visual frame snapshot generation.
type TestScreen struct {
	screen  tcell.SimulationScreen
	width   int
	height  int
	focused tview.Primitive
}

// NewTestScreen initializes a headless UTF-8 simulation screen with the given dimensions.
func NewTestScreen(width, height int) (*TestScreen, error) {
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize simulation screen: %w", err)
	}
	sim.SetSize(width, height)
	return &TestScreen{
		screen: sim,
		width:  width,
		height: height,
	}, nil
}

// Close releases resources held by the underlying simulation screen.
func (ts *TestScreen) Close() {
	if ts.screen != nil {
		ts.screen.Fini()
	}
}

// Screen returns the underlying tcell.SimulationScreen.
func (ts *TestScreen) Screen() tcell.SimulationScreen {
	return ts.screen
}

// Focus sets focus on the target primitive and records the active leaf primitive.
func (ts *TestScreen) Focus(root tview.Primitive) {
	if root == nil {
		return
	}
	ts.focused = root
	root.Focus(func(p tview.Primitive) {
		if p != nil {
			ts.Focus(p)
		}
	})
}

// Draw clears the simulation screen, sizes root to the screen bounds, draws it, and flushes cells.
func (ts *TestScreen) Draw(root tview.Primitive) {
	if root == nil {
		return
	}
	if ts.focused == nil {
		ts.Focus(root)
	}
	ts.screen.Clear()
	root.SetRect(0, 0, ts.width, ts.height)
	root.Draw(ts.screen)
	ts.screen.Show()
}

// SendKey dispatches a key event to the focused primitive (or root) and redraws root.
func (ts *TestScreen) SendKey(root tview.Primitive, key tcell.Key, ch rune, mod tcell.ModMask) {
	if root == nil {
		return
	}
	if ts.focused == nil {
		ts.Focus(root)
	}
	ev := tcell.NewEventKey(key, ch, mod)
	target := ts.focused
	if target == nil {
		target = root
	}
	if handler := target.InputHandler(); handler != nil {
		handler(ev, func(p tview.Primitive) {
			if p != nil {
				ts.Focus(p)
			}
		})
	} else if rootHandler := root.InputHandler(); rootHandler != nil {
		rootHandler(ev, func(p tview.Primitive) {
			if p != nil {
				ts.Focus(p)
			}
		})
	}
	ts.Draw(root)
}

// SendRune dispatches a printable rune key event and redraws root.
func (ts *TestScreen) SendRune(root tview.Primitive, ch rune) {
	ts.SendKey(root, tcell.KeyRune, ch, tcell.ModNone)
}

// SendText dispatches each rune in text sequentially and redraws root.
func (ts *TestScreen) SendText(root tview.Primitive, text string) {
	for _, ch := range text {
		ts.SendRune(root, ch)
	}
}

// Lines returns the rendered screen rows as plain strings with trailing whitespace trimmed.
func (ts *TestScreen) Lines() []string {
	cells, w, h := ts.screen.GetContents()
	lines := make([]string, h)
	var row strings.Builder
	row.Grow(w)
	for y := 0; y < h; y++ {
		row.Reset()
		for x := 0; x < w; x++ {
			idx := y*w + x
			if idx >= len(cells) {
				row.WriteByte(' ')
				continue
			}
			runes := cells[idx].Runes
			if len(runes) == 0 || runes[0] == 0 {
				row.WriteByte(' ')
			} else {
				row.WriteRune(runes[0])
			}
		}
		lines[y] = strings.TrimRight(row.String(), " ")
	}
	return lines
}

// Text returns the full rendered screen content as a newline-delimited string.
func (ts *TestScreen) Text() string {
	return strings.Join(ts.Lines(), "\n")
}

// ContainsText reports whether sub appears anywhere in the rendered screen text.
func (ts *TestScreen) ContainsText(sub string) bool {
	return strings.Contains(ts.Text(), sub)
}

// HTML renders the current screen buffer into a styled HTML terminal block
// preserving foreground/background colors and bold attributes.
func (ts *TestScreen) HTML(title string) string {
	cells, w, h := ts.screen.GetContents()
	var b strings.Builder
	b.Grow(w * h * 2)
	b.WriteString(`<div style="background:#0d1117;border:1px solid #30363d;border-radius:8px;padding:12px;margin:8px 0;overflow-x:auto;">`)
	if title != "" {
		b.WriteString(`<div style="color:#8b949e;font-family:monospace;font-size:12px;margin-bottom:8px;border-bottom:1px solid #21262d;padding-bottom:4px;">`)
		b.WriteString(html.EscapeString(title))
		b.WriteString(`</div>`)
	}
	b.WriteString(`<pre style="margin:0;font-family:'JetBrains Mono','Fira Code','Cascadia Mono',monospace;font-size:12px;line-height:1.25;color:#e6edf3;background:#0d1117;">`)

	for y := 0; y < h; y++ {
		curStyle := ""
		var run strings.Builder
		flushRun := func(isLineEnd bool) {
			if run.Len() == 0 {
				return
			}
			s := run.String()
			if isLineEnd && curStyle == "" {
				s = strings.TrimRight(s, " ")
				if len(s) == 0 {
					run.Reset()
					return
				}
			}
			escaped := html.EscapeString(s)
			if curStyle == "" {
				b.WriteString(escaped)
			} else {
				b.WriteString(`<span style="`)
				b.WriteString(curStyle)
				b.WriteString(`">`)
				b.WriteString(escaped)
				b.WriteString(`</span>`)
			}
			run.Reset()
		}

		for x := 0; x < w; x++ {
			idx := y*w + x
			ch := ' '
			var style tcell.Style
			if idx < len(cells) {
				if len(cells[idx].Runes) > 0 && cells[idx].Runes[0] != 0 {
					ch = cells[idx].Runes[0]
				}
				style = cells[idx].Style
			}
			fg, bg, attr := style.Decompose()
			if attr&tcell.AttrReverse != 0 {
				fg, bg = bg, fg
			}
			fgCSS := tcellColorToCSS(fg, "#e6edf3")
			bgCSS := tcellColorToCSS(bg, "")
			bold := attr&tcell.AttrBold != 0

			cellStyle := ""
			if fgCSS != "#e6edf3" || bgCSS != "" || bold {
				var sb strings.Builder
				sb.WriteString("color:")
				sb.WriteString(fgCSS)
				if bgCSS != "" {
					sb.WriteString(";background:")
					sb.WriteString(bgCSS)
				}
				if bold {
					sb.WriteString(";font-weight:700")
				}
				cellStyle = sb.String()
			}

			if cellStyle != curStyle {
				flushRun(false)
				curStyle = cellStyle
			}
			run.WriteRune(ch)
		}
		flushRun(true)
		if y < h-1 {
			b.WriteByte('\n')
		}
	}
	b.WriteString(`</pre></div>`)
	return b.String()
}

func tcellColorToCSS(c tcell.Color, fallback string) string {
	if c == tcell.ColorDefault || !c.Valid() {
		return fallback
	}
	r, g, b := c.RGB()
	if r >= 0 && g >= 0 && b >= 0 {
		return fmt.Sprintf("#%02x%02x%02x", r, g, b)
	}
	return fallback
}

// WriteSnapshotIfEnv writes an HTML frame snapshot to $KTOP_SNAPSHOT_DIR/<name>.html when KTOP_SNAPSHOT_DIR is set.
func (ts *TestScreen) WriteSnapshotIfEnv(name, title string) error {
	dir := os.Getenv("KTOP_SNAPSHOT_DIR")
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, name+".html")
	return os.WriteFile(path, []byte(ts.HTML(title)), 0o644)
}
