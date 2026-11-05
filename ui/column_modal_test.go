package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestColumnSelectorModal_ToggleAndApply(t *testing.T) {
	allCols := []string{"NAME", "STATUS", "CPU", "MEM"}
	currCols := []string{"NAME", "STATUS"}
	descriptions := map[string]string{
		"NAME":   "Node name",
		"STATUS": "Node condition",
		"CPU":    "CPU usage",
		"MEM":    "Memory usage",
	}

	var applied []string
	cancelled := false

	modal := NewColumnSelectorModal(ColumnSelectorConfig{
		Title:          "Nodes",
		AllColumns:     allCols,
		CurrentColumns: currCols,
		Descriptions:   descriptions,
		OnApply: func(selected []string) {
			applied = selected
		},
		OnCancel: func() {
			cancelled = true
		},
	})

	// Initial selection is row 0 (NAME). Toggle NAME -> now unchecked
	modal.HandleInput(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))
	// Move down to STATUS (row 1)
	modal.HandleInput(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone))
	// Move down to CPU (row 2)
	modal.HandleInput(tcell.NewEventKey(tcell.KeyRune, 'j', tcell.ModNone))
	// Toggle CPU (row 2) -> now checked
	modal.HandleInput(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))

	// Press Enter to apply
	modal.HandleInput(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))

	expectedApplied := []string{"STATUS", "CPU"}
	if !slices.Equal(applied, expectedApplied) {
		t.Fatalf("expected applied %v, got %v", expectedApplied, applied)
	}

	// Press Esc to cancel
	modal.HandleInput(tcell.NewEventKey(tcell.KeyEsc, 0, tcell.ModNone))
	if !cancelled {
		t.Fatalf("expected cancelled to be true")
	}
}

func TestColumnSelectorModal_PreventEmptySelection(t *testing.T) {
	allCols := []string{"NAME", "STATUS"}
	currCols := []string{"NAME"}

	modal := NewColumnSelectorModal(ColumnSelectorConfig{
		Title:          "Nodes",
		AllColumns:     allCols,
		CurrentColumns: currCols,
	})

	// Only NAME is selected. Attempt to toggle it off.
	modal.HandleInput(tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone))

	selected := modal.getSelectedColumns()
	if !slices.Equal(selected, []string{"NAME"}) {
		t.Fatalf("should not allow deselecting the only selected column: got %v", selected)
	}
}

func TestColumnSelectorModal_HeadlessTUI(t *testing.T) {
	allCols := []string{"NAME", "STATUS", "CPU", "MEM"}
	currCols := []string{"NAME", "CPU"}
	descriptions := map[string]string{
		"NAME":   "Node name",
		"STATUS": "Node condition",
		"CPU":    "CPU usage",
		"MEM":    "Memory usage",
	}

	modal := NewColumnSelectorModal(ColumnSelectorConfig{
		Title:          "Nodes",
		AllColumns:     allCols,
		CurrentColumns: currCols,
		Descriptions:   descriptions,
	})

	ts, err := NewTestScreen(80, 24)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	root := modal.GetRootView()
	ts.Draw(root)

	text := ts.Text()
	if !strings.Contains(text, "Configure Columns: Nodes") {
		t.Fatalf("expected modal title in rendered output, got:\n%s", text)
	}
	if !strings.Contains(text, "Node name") || !strings.Contains(text, "CPU usage") {
		t.Fatalf("expected descriptions in rendered output, got:\n%s", text)
	}
	if !strings.Contains(text, "[X]") || !strings.Contains(text, "[ ]") {
		t.Fatalf("expected checkbox indicators, got:\n%s", text)
	}
}
