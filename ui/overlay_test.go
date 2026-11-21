package ui

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
)

func TestOverlayFrame_PreservesUnderlyingChromeInMargins(t *testing.T) {
	ts, err := NewTestScreen(100, 24)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	bg := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().SetText("HEADER_VISIBLE_ROW"), 3, 0, false).
		AddItem(tview.NewTextView().SetText("TABS_VISIBLE_ROW"), 1, 0, false).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(tview.NewTextView().SetText("FOOTER_VISIBLE_ROW"), 3, 0, false)

	modalBody := tview.NewTextView().SetText("FLOATING_POD_MODAL")
	modalBody.SetBorder(true).SetTitle(" Pod Inspector ")

	overlay := NewOverlayFrame(modalBody, 4, 3, 3, 3)

	root := tview.NewPages()
	root.AddPage("main", bg, true, true)
	root.AddPage("modal", overlay, true, true)

	ts.Draw(root)
	text := ts.Text()
	for _, want := range []string{"HEADER_VISIBLE_ROW", "TABS_VISIBLE_ROW", "FLOATING_POD_MODAL", "Pod Inspector", "FOOTER_VISIBLE_ROW"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q visible with OverlayFrame, got:\n%s", want, text)
		}
	}
}
