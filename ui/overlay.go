package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// OverlayFrame positions a modal child inside outer margins without clearing
// the margin area, keeping underlying chrome (header, tabs, parent frame, footer)
// visible around the floating overlay.
type OverlayFrame struct {
	*tview.Box
	content                  tview.Primitive
	top, bottom, left, right int
}

// NewOverlayFrame wraps content in a transparent-margin overlay frame.
func NewOverlayFrame(content tview.Primitive, top, bottom, left, right int) *OverlayFrame {
	return &OverlayFrame{
		Box:     tview.NewBox(),
		content: content,
		top:     top,
		bottom:  bottom,
		left:    left,
		right:   right,
	}
}

// SetContent replaces the primitive rendered inside the overlay frame.
func (f *OverlayFrame) SetContent(content tview.Primitive) {
	f.content = content
}

// innerBounds computes the child rectangle, collapsing margins when the terminal is very small.
func (f *OverlayFrame) innerBounds() (x, y, w, h int) {
	rx, ry, rw, rh := f.GetRect()
	l, r, t, b := f.left, f.right, f.top, f.bottom
	if rw < l+r+40 {
		l, r = 0, 0
	}
	if rh < t+b+12 {
		t, b = 0, 0
	}
	return rx + l, ry + t, max(rw-l-r, 0), max(rh-t-b, 0)
}

// Draw renders only the inner content primitive so cells in the outer margins
// remain untouched from previously drawn pages beneath this overlay.
func (f *OverlayFrame) Draw(screen tcell.Screen) {
	if f.content == nil {
		return
	}
	ix, iy, iw, ih := f.innerBounds()
	if iw <= 0 || ih <= 0 {
		return
	}
	f.content.SetRect(ix, iy, iw, ih)
	f.content.Draw(screen)
}

// Focus delegates focus to the wrapped content primitive.
func (f *OverlayFrame) Focus(delegate func(p tview.Primitive)) {
	if f.content != nil {
		delegate(f.content)
		return
	}
	f.Box.Focus(delegate)
}

// Blur delegates blur to the wrapped content primitive.
func (f *OverlayFrame) Blur() {
	if f.content != nil {
		f.content.Blur()
	}
	f.Box.Blur()
}

// HasFocus reports whether the wrapped content primitive has focus.
func (f *OverlayFrame) HasFocus() bool {
	if f.content != nil && f.content.HasFocus() {
		return true
	}
	return f.Box.HasFocus()
}

// InputHandler delegates keyboard input to the wrapped content primitive.
func (f *OverlayFrame) InputHandler() func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
	return f.WrapInputHandler(func(event *tcell.EventKey, setFocus func(p tview.Primitive)) {
		if f.content != nil {
			if handler := f.content.InputHandler(); handler != nil {
				handler(event, setFocus)
			}
		}
	})
}

// MouseHandler delegates mouse events inside the overlay to the wrapped content primitive.
func (f *OverlayFrame) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return f.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (consumed bool, capture tview.Primitive) {
		if !f.InRect(event.Position()) {
			return false, nil
		}
		if f.content != nil {
			if handler := f.content.MouseHandler(); handler != nil {
				return handler(action, event, setFocus)
			}
		}
		return false, nil
	})
}
