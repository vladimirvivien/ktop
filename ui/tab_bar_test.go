package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestTabBar_RenderAndSwitching(t *testing.T) {
	ts, err := NewTestScreen(100, 1)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	tb := NewTabBar()
	tb.SetTabs([]TabItem{
		{ID: "overview", Title: "Overview", Key: '1'},
		{ID: "workloads", Title: "Workloads", Key: '2'},
		{ID: "events", Title: "Events", Key: '3'},
	})

	ts.Draw(tb.GetView())
	initial := ts.Text()
	for _, want := range []string{"[1] Overview", "[2] Workloads", "[3] Events"} {
		if !strings.Contains(initial, want) {
			t.Fatalf("expected tab label %q in rendered bar, got %q", want, initial)
		}
	}

	tb.SetActive(1)
	if tb.ActiveIndex() != 1 {
		t.Fatalf("expected ActiveIndex() = 1, got %d", tb.ActiveIndex())
	}

	tb.SetBreadcrumb("Overview › node/worker-1")
	ts.Draw(tb.GetView())
	withCrumb := ts.Text()
	if !strings.Contains(withCrumb, "Overview › node/worker-1") {
		t.Fatalf("expected breadcrumb in rendered bar, got %q", withCrumb)
	}

	tb.SetBreadcrumb("")
	ts.Draw(tb.GetView())
	cleared := ts.Text()
	if strings.Contains(cleared, "node/worker-1") {
		t.Fatalf("expected cleared breadcrumb, got %q", cleared)
	}
}

func TestTabBar_MonochromeZeroColorsAndActiveAttr(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ResolveAndApplyTheme("dark", "truecolor")
	})
	if _, err := ResolveAndApplyTheme("monochrome", "none"); err != nil {
		t.Fatalf("ResolveAndApplyTheme failed: %v", err)
	}

	ts, err := NewTestScreen(100, 1)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	tb := NewTabBar()
	tb.SetTabs([]TabItem{
		{ID: "overview", Title: "Overview", Key: '1'},
		{ID: "workloads", Title: "Workloads", Key: '2'},
	})
	tb.SetActive(1)
	tb.SetBreadcrumb("Workloads › pod/default/api-0")

	ts.Draw(tb.GetView())
	line := ts.Lines()[0]
	cells, w, _ := ts.Screen().GetContents()

	for x := 0; x < w; x++ {
		fg, bg, _ := cells[x].Style.Decompose()
		if fg != tcell.ColorDefault || bg != tcell.ColorDefault {
			t.Fatalf("cell %d emitted non-default monochrome colors fg=%v bg=%v", x, fg, bg)
		}
	}

	activeIdx := strings.Index(line, "[2] Workloads")
	if activeIdx < 0 {
		t.Fatalf("missing active tab in %q", line)
	}
	_, _, activeAttrs := cells[activeIdx+1].Style.Decompose()
	if activeAttrs&tcell.AttrReverse == 0 || activeAttrs&tcell.AttrBold == 0 {
		t.Fatalf("expected active monochrome tab to use AttrReverse|AttrBold, got %v", activeAttrs)
	}

	inactiveIdx := strings.Index(line, "[1] Overview")
	if inactiveIdx < 0 {
		t.Fatalf("missing inactive tab in %q", line)
	}
	_, _, inactiveAttrs := cells[inactiveIdx+1].Style.Decompose()
	if inactiveAttrs&tcell.AttrReverse != 0 {
		t.Fatalf("expected inactive monochrome tab not to use AttrReverse, got %v", inactiveAttrs)
	}
}
