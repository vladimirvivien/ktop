package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

func TestTabBar_ZeroGapAndThemedInactiveColors(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ResolveAndApplyTheme("dark", "truecolor")
	})

	rt, err := ResolveAndApplyTheme("dark", "truecolor")
	if err != nil {
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
	tb.SetActive(0)

	ts.Draw(tb.GetView())
	line := ts.Lines()[0]
	if !strings.Contains(line, "[1] Overview  [2] Workloads") {
		t.Fatalf("expected zero gap between adjacent tabs (single trailing + single leading pad), got %q", line)
	}

	cells, _, _ := ts.Screen().GetContents()
	firstStart := strings.Index(line, "[1] Overview") - 1
	secondStart := strings.Index(line, "[2] Workloads") - 1
	if firstStart < 0 || secondStart != firstStart+len(" [1] Overview ") {
		t.Fatalf("unexpected tab offsets first=%d second=%d in %q", firstStart, secondStart, line)
	}

	// Cell right before second tab must carry active TabActiveBg; first cell of second tab must carry TabInactiveBg.
	activeFG, activeBG, _ := cells[secondStart-1].Style.Decompose()
	inactiveFG, inactiveBG, _ := cells[secondStart].Style.Decompose()
	if activeFG != rt.Palette.TabActiveFg || activeBG != rt.Palette.TabActiveBg {
		t.Fatalf("active tab style = (%v, %v), want (%v, %v)", activeFG, activeBG, rt.Palette.TabActiveFg, rt.Palette.TabActiveBg)
	}
	if inactiveFG != rt.Palette.TabInactiveFg || inactiveBG != rt.Palette.TabInactiveBg {
		t.Fatalf("inactive tab style = (%v, %v), want (%v, %v)", inactiveFG, inactiveBG, rt.Palette.TabInactiveFg, rt.Palette.TabInactiveBg)
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

func TestTabFrame_BorderTabsBreadcrumbAndContent(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ResolveAndApplyTheme("dark", "truecolor")
	})
	rt, err := ResolveAndApplyTheme("dark", "truecolor")
	if err != nil {
		t.Fatalf("ResolveAndApplyTheme failed: %v", err)
	}

	ts, err := NewTestScreen(100, 6)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	content := tview.NewTextView().SetText("Inner Page Content")
	tf := NewTabFrame(content)
	tf.SetTabs([]TabItem{
		{ID: "overview", Title: "Overview", Key: '1'},
		{ID: "workloads", Title: "Workloads", Key: '2'},
	})
	tf.SetActive(0)
	tf.SetBreadcrumb("Overview › node/worker-1")

	ts.Draw(tf)
	lines := ts.Lines()
	top := lines[0]
	bottom := lines[5]

	if !strings.HasPrefix(top, "┌─ [1] Overview  [2] Workloads ") {
		t.Fatalf("expected top border to begin with corner + contiguous tabs, got %q", top)
	}
	if !strings.HasSuffix(top, " Overview › node/worker-1 ─┐") {
		t.Fatalf("expected right-aligned breadcrumb on top border, got %q", top)
	}
	if !strings.HasPrefix(bottom, "└") || !strings.HasSuffix(bottom, "┘") {
		t.Fatalf("expected bottom border corners, got %q", bottom)
	}
	if !strings.Contains(lines[1], "Inner Page Content") {
		t.Fatalf("expected inner content rendered inside frame row 1, got %q", lines[1])
	}

	cells, _, _ := ts.Screen().GetContents()
	// Row 0 cell 0 is '┌' with BorderNormal color.
	borderFG, _, _ := cells[0].Style.Decompose()
	if borderFG != rt.Palette.BorderNormal {
		t.Fatalf("border color = %v, want %v", borderFG, rt.Palette.BorderNormal)
	}

	// Row 0 cell 2 starts active tab " [1] Overview " (len 14), cell 16 starts inactive tab " [2] Workloads ".
	activeFG, activeBG, _ := cells[15].Style.Decompose()
	inactiveFG, inactiveBG, _ := cells[16].Style.Decompose()
	if activeFG != rt.Palette.TabActiveFg || activeBG != rt.Palette.TabActiveBg {
		t.Fatalf("active tab style = (%v, %v), want (%v, %v)", activeFG, activeBG, rt.Palette.TabActiveFg, rt.Palette.TabActiveBg)
	}
	if inactiveFG != rt.Palette.TabInactiveFg || inactiveBG != rt.Palette.TabInactiveBg {
		t.Fatalf("inactive tab style = (%v, %v), want (%v, %v)", inactiveFG, inactiveBG, rt.Palette.TabInactiveFg, rt.Palette.TabInactiveBg)
	}
}

func TestTabFrame_MonochromeZeroColorsAndFocusDelegation(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ResolveAndApplyTheme("dark", "truecolor")
	})
	if _, err := ResolveAndApplyTheme("monochrome", "none"); err != nil {
		t.Fatalf("ResolveAndApplyTheme failed: %v", err)
	}

	ts, err := NewTestScreen(100, 5)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	child := tview.NewTextView().SetText("Body")
	tf := NewTabFrame(child)
	tf.SetTabs([]TabItem{
		{ID: "overview", Title: "Overview", Key: '1'},
		{ID: "workloads", Title: "Workloads", Key: '2'},
	})
	tf.SetActive(1)
	tf.SetBreadcrumb("Workloads › deploy/default/api")

	ts.Draw(tf)
	cells, w, h := ts.Screen().GetContents()
	for i := 0; i < w*h; i++ {
		fg, bg, _ := cells[i].Style.Decompose()
		if fg != tcell.ColorDefault || bg != tcell.ColorDefault {
			t.Fatalf("cell %d emitted non-default monochrome colors fg=%v bg=%v", i, fg, bg)
		}
	}

	// Active tab "[2] Workloads" starts at column 2 + len(" [1] Overview ") = 16.
	_, _, activeAttrs := cells[17].Style.Decompose()
	if activeAttrs&tcell.AttrReverse == 0 || activeAttrs&tcell.AttrBold == 0 {
		t.Fatalf("expected active monochrome tab to use AttrReverse|AttrBold, got %v", activeAttrs)
	}
	_, _, inactiveAttrs := cells[3].Style.Decompose()
	if inactiveAttrs&tcell.AttrReverse != 0 {
		t.Fatalf("expected inactive monochrome tab not to use AttrReverse, got %v", inactiveAttrs)
	}

	// Verify Focus delegates to wrapped child primitive.
	var focused tview.Primitive
	tf.Focus(func(p tview.Primitive) {
		focused = p
		p.Focus(func(tview.Primitive) {})
	})
	if focused != child {
		t.Fatalf("expected Focus to delegate to child primitive")
	}
	if !tf.HasFocus() {
		t.Fatalf("expected TabFrame.HasFocus() to be true when child has focus")
	}
	tf.Blur()
	if tf.HasFocus() {
		t.Fatalf("expected TabFrame.HasFocus() to be false after Blur()")
	}
}
