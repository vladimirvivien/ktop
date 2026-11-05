package ui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestTestScreen_DrawAndInput(t *testing.T) {
	ts, err := NewTestScreen(80, 12)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	table := tview.NewTable().SetBorders(false)
	table.SetCell(0, 0, tview.NewTableCell("NAME"))
	table.SetCell(1, 0, tview.NewTableCell("node-alpha"))
	table.SetCell(2, 0, tview.NewTableCell("node-beta"))

	var lastRune rune
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyRune {
			lastRune = event.Rune()
			table.SetCell(0, 0, tview.NewTableCell("NAME ▲"))
		}
		return event
	})

	box := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(table, 0, 1, true)
	box.SetBorder(true).SetTitle(" Nodes (2) ")

	ts.Draw(box)
	if !ts.ContainsText("Nodes (2)") {
		t.Fatalf("expected title 'Nodes (2)' in rendered screen, got:\n%s", ts.Text())
	}
	if !ts.ContainsText("node-alpha") || !ts.ContainsText("node-beta") {
		t.Fatalf("expected table rows in rendered screen, got:\n%s", ts.Text())
	}

	ts.SendRune(box, 'n')
	if lastRune != 'n' {
		t.Fatalf("expected input capture to receive 'n', got %q", lastRune)
	}
	if !ts.ContainsText("NAME ▲") {
		t.Fatalf("expected updated header 'NAME ▲' after key press, got:\n%s", ts.Text())
	}

	htmlOut := ts.HTML("Test Snapshot")
	if len(htmlOut) == 0 {
		t.Fatal("expected non-empty HTML snapshot")
	}
}
