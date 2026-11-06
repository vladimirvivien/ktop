package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestTableController_Sort(t *testing.T) {
	redrawCount := 0
	table := tview.NewTable()
	tc := NewTableController(table, TableControllerConfig{
		DefaultSortColumn: "NAME",
		DefaultSortAsc:    true,
		ColumnToKey: map[string]rune{
			"NAME": 'n',
			"CPU":  'c',
			"MEM":  'm',
		},
		OnRedraw: func() {
			redrawCount++
		},
	})

	col, asc := tc.GetSortParams()
	if col != "NAME" || !asc {
		t.Fatalf("expected initial sort NAME ▲, got %s %v", col, asc)
	}

	// Toggle same column -> descending
	tc.ToggleSort("NAME")
	col, asc = tc.GetSortParams()
	if col != "NAME" || asc {
		t.Fatalf("expected NAME ▼, got %s %v", col, asc)
	}
	if redrawCount != 1 {
		t.Errorf("expected 1 redraw, got %d", redrawCount)
	}

	// Switch column -> ascending
	tc.ToggleSort("CPU")
	col, asc = tc.GetSortParams()
	if col != "CPU" || !asc {
		t.Fatalf("expected CPU ▲, got %s %v", col, asc)
	}

	// Handle sort key 'm'
	if !tc.HandleSortKey('m') {
		t.Errorf("expected 'm' to be handled")
	}
	col, asc = tc.GetSortParams()
	if col != "MEM" || !asc {
		t.Fatalf("expected MEM ▲, got %s %v", col, asc)
	}

	// Unmapped key 'z'
	if tc.HandleSortKey('z') {
		t.Errorf("expected unmapped key 'z' to return false")
	}

	// Test visible columns restriction
	tc.SetVisibleColumns([]string{"NAME", "CPU"})
	if tc.HandleSortKey('m') {
		t.Errorf("expected 'm' (MEM) to be rejected when not in visible columns")
	}
}

func TestTableController_FormatHeader(t *testing.T) {
	table := tview.NewTable()
	tc := NewTableController(table, TableControllerConfig{
		DefaultSortColumn: "CPU",
		DefaultSortAsc:    true,
		ColumnToKey: map[string]rune{
			"CPU":  'c',
			"NAME": 'n',
		},
	})

	hdrCPU := tc.FormatColumnHeader("CPU")
	if !strings.Contains(hdrCPU, "▲") {
		t.Errorf("expected CPU header to have sort indicator ▲, got %s", hdrCPU)
	}

	hdrName := tc.FormatColumnHeader("NAME")
	if strings.Contains(hdrName, "▲") || strings.Contains(hdrName, "▼") {
		t.Errorf("NAME header should not have sort indicator, got %s", hdrName)
	}

	tc.ToggleSort("CPU")
	hdrCPUDesc := tc.FormatColumnHeader("CPU")
	if !strings.Contains(hdrCPUDesc, "▼") {
		t.Errorf("expected CPU header to have sort indicator ▼, got %s", hdrCPUDesc)
	}
}

func TestTableController_FilterLifecycle(t *testing.T) {
	redrawCount := 0
	table := tview.NewTable()
	tc := NewTableController(table, TableControllerConfig{
		DefaultSortColumn: "NAME",
		DefaultSortAsc:    true,
		ColumnToKey: map[string]rune{
			"NAME": 'n',
		},
		OnRedraw: func() {
			redrawCount++
		},
	})

	// Start filter with '/'
	evSlash := tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone)
	if res := tc.HandleInput(evSlash); res != nil {
		t.Errorf("expected '/' to be consumed")
	}
	if !tc.Filter().Editing {
		t.Fatalf("expected filter to be in editing mode")
	}

	// Type 't', 'e', 's', 't'
	for _, r := range "test" {
		ev := tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)
		tc.HandleInput(ev)
	}
	if tc.Filter().Text != "test" {
		t.Fatalf("expected filter text 'test', got %q", tc.Filter().Text)
	}

	// Backspace
	evBksp := tcell.NewEventKey(tcell.KeyBackspace, 0, tcell.ModNone)
	tc.HandleInput(evBksp)
	if tc.Filter().Text != "tes" {
		t.Fatalf("expected filter text 'tes', got %q", tc.Filter().Text)
	}

	// Confirm Enter
	evEnter := tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone)
	tc.HandleInput(evEnter)
	if tc.Filter().Editing || !tc.Filter().Active {
		t.Fatalf("expected filter active and not editing")
	}

	// Clear Esc
	evEsc := tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone)
	tc.HandleInput(evEsc)
	if tc.Filter().Active || tc.Filter().Text != "" {
		t.Fatalf("expected filter cleared")
	}
}

func TestTableController_Sparklines(t *testing.T) {
	cache := make(map[string]*Sparkline)
	colors := DefaultColorKeys()

	sp1 := GetSparkline(cache, "node1", 10, colors)
	if sp1 == nil || len(cache) != 1 {
		t.Fatalf("expected sparkline created in cache")
	}

	sp2 := GetSparkline(cache, "node1", 15, colors)
	if sp1 != sp2 {
		t.Errorf("expected cached sparkline to be reused")
	}

	GetSparkline(cache, "node2", 10, colors)
	if len(cache) != 2 {
		t.Fatalf("expected 2 sparklines in cache")
	}

	// Prune node1
	PruneSparklines(cache, map[string]bool{"node2": true})
	if len(cache) != 1 || cache["node2"] == nil {
		t.Fatalf("expected node1 pruned and node2 retained")
	}
}

func TestTableController_ConfigureColumns(t *testing.T) {
	configured := false
	table := tview.NewTable()
	tc := NewTableController(table, TableControllerConfig{
		DefaultSortColumn: "NAME",
		ColumnToKey: map[string]rune{
			"CPU": 'c',
		},
		OnConfigureColumns: func() {
			configured = true
		},
	})

	// Pressing lowercase 'c' should sort by CPU, NOT trigger column configuration
	evLowerC := tcell.NewEventKey(tcell.KeyRune, 'c', tcell.ModNone)
	tc.HandleInput(evLowerC)
	if configured {
		t.Fatalf("lowercase 'c' should not trigger OnConfigureColumns")
	}
	col, _ := tc.GetSortParams()
	if col != "CPU" {
		t.Fatalf("expected sort column CPU, got %s", col)
	}

	// Pressing '=' should trigger OnConfigureColumns
	evEqual := tcell.NewEventKey(tcell.KeyRune, '=', tcell.ModNone)
	tc.HandleInput(evEqual)
	if !configured {
		t.Fatalf("'=' should trigger OnConfigureColumns")
	}

	// In filter edit mode, '=' should be typed into the filter, not trigger configuration
	configured = false
	tc.Filter().StartEditing()
	tc.HandleInput(evEqual)
	if configured {
		t.Fatalf("'=' in filter edit mode should not trigger OnConfigureColumns")
	}
	if tc.Filter().Text != "=" {
		t.Fatalf("expected filter text '=', got '%s'", tc.Filter().Text)
	}
}
