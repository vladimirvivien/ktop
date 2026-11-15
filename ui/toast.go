package ui

import (
	"fmt"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/ui/theme"
)

// ToastLevel defines the severity level of a toast notification
type ToastLevel int

const (
	ToastInfo ToastLevel = iota
	ToastSuccess
	ToastWarning
	ToastError
)

// ToastCallback is called when a button is pressed on the toast
// buttonLabel is the text of the button that was pressed
type ToastCallback func(buttonLabel string)

// ToastModal is a theme-aware centered modal used for toast notifications.
// Unlike tview.Modal, it exposes frame border and button styling on every Draw.
type ToastModal struct {
	*tview.Box

	frame     *tview.Frame
	form      *tview.Form
	message   string
	level     ToastLevel
	buttons   []string
	textColor tcell.Color
	done      func(buttonIndex int, buttonLabel string)
}

func newToastModal(message string, level ToastLevel, buttons []string, done func(buttonIndex int, buttonLabel string)) *ToastModal {
	m := &ToastModal{
		Box:     tview.NewBox(),
		message: message,
		level:   level,
		done:    done,
	}
	if len(buttons) > 0 {
		m.buttons = append([]string(nil), buttons...)
	}

	m.form = tview.NewForm().SetButtonsAlign(tview.AlignCenter)
	m.form.SetBorderPadding(0, 0, 0, 0)
	m.form.SetCancelFunc(func() {
		if m.done != nil {
			m.done(-1, "")
		}
	})

	for index, label := range m.buttons {
		func(i int, l string) {
			m.form.AddButton(tview.Escape(l), func() {
				if m.done != nil {
					m.done(i, l)
				}
			})
			button := m.form.GetButton(m.form.GetButtonCount() - 1)
			button.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
				switch event.Key() {
				case tcell.KeyDown, tcell.KeyRight:
					return tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone)
				case tcell.KeyUp, tcell.KeyLeft:
					return tcell.NewEventKey(tcell.KeyBacktab, 0, tcell.ModNone)
				}
				return event
			})
		}(index, label)
	}

	m.frame = tview.NewFrame(m.form).SetBorders(0, 0, 1, 0, 0, 0)
	m.frame.SetBorder(true).SetBorderPadding(1, 1, 1, 1)
	m.applyTheme()
	return m
}

func getToastBorderColor(level ToastLevel, p theme.Palette) tcell.Color {
	switch level {
	case ToastSuccess:
		if p.IsDark {
			return p.GaugeLow
		}
		return p.StatusOK
	case ToastError:
		return p.StatusError
	case ToastWarning:
		return p.StatusWarning
	default:
		return p.StatusInfo
	}
}

func (m *ToastModal) applyTheme() {
	rt := ActiveTheme()
	p := rt.Palette
	if rt.IsMonochrome() {
		m.textColor = tcell.ColorDefault
		m.form.SetBackgroundColor(tcell.ColorDefault)
		m.form.SetButtonBackgroundColor(tcell.ColorDefault)
		m.form.SetButtonTextColor(tcell.ColorDefault)
		m.frame.SetBackgroundColor(tcell.ColorDefault)
		m.frame.SetBorderColor(tcell.ColorDefault)
		m.frame.SetBorderAttributes(tcell.AttrBold)
		for i, rawLabel := range m.buttons {
			if i >= m.form.GetButtonCount() {
				break
			}
			esc := tview.Escape(rawLabel)
			btn := m.form.GetButton(i)
			if btn.HasFocus() {
				btn.SetLabel("[::rb]" + esc + "[::-]")
			} else {
				btn.SetLabel(esc)
			}
		}
		return
	}

	_, bgColor := getIconAndColor(m.level)
	borderColor := getToastBorderColor(m.level, p)
	m.textColor = p.ToastFg
	m.frame.SetBackgroundColor(bgColor)
	m.frame.SetBorderColor(borderColor)
	m.frame.SetBorderAttributes(tcell.AttrNone)
	m.form.SetBackgroundColor(bgColor)

	var btnBg tcell.Color
	btnFg := p.ToastFg
	if !p.IsDark {
		btnBg = p.CanvasBg
	} else {
		btnBg = theme.ResolveColor(tcell.ColorBlack, rt.Mode)
	}
	m.form.SetButtonBackgroundColor(btnBg)
	m.form.SetButtonTextColor(btnFg)

	for i, rawLabel := range m.buttons {
		if i >= m.form.GetButtonCount() {
			break
		}
		m.form.GetButton(i).SetLabel(tview.Escape(rawLabel))
	}
}

// Draw draws the toast modal centered on screen with active theme colors.
func (m *ToastModal) Draw(screen tcell.Screen) {
	m.applyTheme()

	buttonsWidth := 0
	count := m.form.GetButtonCount()
	for i := 0; i < count; i++ {
		buttonsWidth += tview.TaggedStringWidth(m.form.GetButton(i).GetLabel()) + 4 + 2
	}
	if buttonsWidth > 0 {
		buttonsWidth -= 2
	}

	screenWidth, screenHeight := screen.Size()
	width := screenWidth / 3
	if width < buttonsWidth {
		width = buttonsWidth
	}
	if width < 1 {
		width = 1
	}

	icon, _ := getIconAndColor(m.level)
	plain := fmt.Sprintf("%s %s", icon, m.message)
	mono := IsMonochrome()
	if mono && m.level == ToastError {
		plain = "! " + plain
	}

	lines := tview.WordWrap(tview.Escape(plain), width)
	m.frame.Clear()
	for _, line := range lines {
		if mono && m.level == ToastError {
			line = "[::bu]" + line + "[::-]"
		}
		m.frame.AddText(line, true, tview.AlignCenter, m.textColor)
	}

	height := len(lines) + 6
	width += 4
	x := (screenWidth - width) / 2
	y := (screenHeight - height) / 2
	m.SetRect(x, y, width, height)
	m.frame.SetRect(x, y, width, height)
	m.frame.Draw(screen)
}

// Focus delegates focus to the underlying form controls.
func (m *ToastModal) Focus(delegate func(p tview.Primitive)) {
	delegate(m.form)
}

// HasFocus reports whether the modal or any of its buttons is focused.
func (m *ToastModal) HasFocus() bool {
	return m.form.HasFocus()
}

// MouseHandler handles mouse clicks for the modal buttons and container.
func (m *ToastModal) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return m.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		consumed, capture = m.form.MouseHandler()(action, event, setFocus)
		if !consumed && action == tview.MouseLeftClick && m.InRect(event.Position()) {
			setFocus(m)
			consumed = true
		}
		return
	})
}

// InputHandler forwards keyboard events to the inner modal frame.
func (m *ToastModal) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return m.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if m.frame.HasFocus() {
			if handler := m.frame.InputHandler(); handler != nil {
				handler(event, setFocus)
				return
			}
		}
	})
}

// NewToast creates a styled ToastModal for toast notifications (no buttons).
func NewToast(message string, level ToastLevel) *ToastModal {
	return newToastModal(message, level, nil, nil)
}

// NewToastWithEscHandler creates a toast that handles ESC key to quit.
func NewToastWithEscHandler(message string, level ToastLevel, callback ToastCallback) *ToastModal {
	modal := newToastModal(message, level, nil, nil)

	// Handle ESC key to quit
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			if callback != nil {
				callback("Quit")
			}
			return nil
		}
		return event
	})

	return modal
}

// NewToastWithButtons creates a modal toast with buttons.
// The callback is called with the button label when any button is pressed.
// ESC key triggers "Quit" callback if Quit button exists, otherwise dismisses.
func NewToastWithButtons(message string, level ToastLevel, buttons []string, callback ToastCallback) *ToastModal {
	modal := newToastModal(message, level, buttons, func(buttonIndex int, buttonLabel string) {
		if callback != nil {
			callback(buttonLabel)
		}
	})

	// Handle ESC key to trigger Quit (or dismiss if no Quit button)
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEsc {
			if callback != nil {
				hasQuit := false
				for _, b := range buttons {
					if b == "Quit" {
						hasQuit = true
						break
					}
				}
				if hasQuit {
					callback("Quit")
				} else {
					callback("")
				}
			}
			return nil
		}
		return event
	})

	return modal
}

func getIconAndColor(level ToastLevel) (string, tcell.Color) {
	p := ActivePalette()
	switch level {
	case ToastSuccess:
		return "✓", p.ToastSuccessBg
	case ToastError:
		return "✗", p.ToastErrorBg
	case ToastWarning:
		return "⚠", p.ToastWarnBg
	default: // ToastInfo
		return "ℹ", p.ToastInfoBg
	}
}

// Toast represents a temporary notification message with expiration
type Toast struct {
	Message  string
	Level    ToastLevel
	Duration time.Duration
	Expires  time.Time
}

// ToastManager handles toast notification display and lifecycle
type ToastManager struct {
	current *Toast
	mu      sync.RWMutex
}

// NewToastManager creates a new toast manager
func NewToastManager() *ToastManager {
	return &ToastManager{}
}

// Show displays a toast message for the specified duration
func (t *ToastManager) Show(message string, level ToastLevel, duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.current = &Toast{
		Message:  message,
		Level:    level,
		Duration: duration,
		Expires:  time.Now().Add(duration),
	}
}

// ShowInfo displays an info toast
func (t *ToastManager) ShowInfo(message string, duration time.Duration) {
	t.Show(message, ToastInfo, duration)
}

// ShowWarning displays a warning toast
func (t *ToastManager) ShowWarning(message string, duration time.Duration) {
	t.Show(message, ToastWarning, duration)
}

// ShowError displays an error toast
func (t *ToastManager) ShowError(message string, duration time.Duration) {
	t.Show(message, ToastError, duration)
}

// ShowSuccess displays a success toast
func (t *ToastManager) ShowSuccess(message string, duration time.Duration) {
	t.Show(message, ToastSuccess, duration)
}

// GetActive returns the currently active toast, or nil if expired/none
func (t *ToastManager) GetActive() *Toast {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.current == nil {
		return nil
	}

	if time.Now().After(t.current.Expires) {
		return nil
	}

	return t.current
}

// Clear removes the current toast immediately
func (t *ToastManager) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current = nil
}

// HasActive returns true if there's an active (non-expired) toast
func (t *ToastManager) HasActive() bool {
	return t.GetActive() != nil
}
