package tests

import (
	"context"
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/internal/snapshot"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	nodedetail "github.com/vladimirvivien/ktop/views/node"
	"github.com/vladimirvivien/ktop/views/overview"
	poddetail "github.com/vladimirvivien/ktop/views/pod"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var leakedPalette256TagRe = regexp.MustCompile(`\[p[a-j][a-z]\]`)

func buildThemeTestSnapshot(t *testing.T) *snapshot.Snapshot {
	t.Helper()
	created := metav1.NewTime(time.Now().Add(-3 * time.Hour))

	summary := model.ClusterSummary{
		HealthScore:             75,
		Uptime:                  created,
		NodesReady:              1,
		NodesCount:              1,
		Namespaces:              2,
		PodsRunning:             1,
		PodsAvailable:           2,
		DeploymentsTotal:        2,
		DeploymentsReady:        1,
		MetricsSourceType:       metrics.SourceTypePrometheus,
		AllocatableNodeCpuTotal: resource.NewMilliQuantity(4000, resource.DecimalSI),
		AllocatableNodeMemTotal: resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
		RequestedPodCpuTotal:    resource.NewMilliQuantity(500, resource.DecimalSI),
		RequestedPodMemTotal:    resource.NewQuantity(512*1024*1024, resource.BinarySI),
		UsageNodeCpuTotal:       resource.NewMilliQuantity(1200, resource.DecimalSI),
		UsageNodeMemTotal:       resource.NewQuantity(2*1024*1024*1024, resource.BinarySI),
		HasUsageCPU:             true,
		HasUsageMem:             true,
		PVsTotal:                resource.NewQuantity(0, resource.DecimalSI),
		PVCsTotal:               resource.NewQuantity(0, resource.DecimalSI),
	}

	nodes := []model.NodeModel{
		{
			Name:                  "node-1",
			Status:                "Ready",
			CreationTime:          created,
			TimeSinceStart:        "3h",
			InternalIP:            "10.0.0.10",
			PodsCount:             2,
			AllocatableCpuQty:     resource.NewMilliQuantity(4000, resource.DecimalSI),
			AllocatableMemQty:     resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			AllocatableStorageQty: resource.NewQuantity(50*1024*1024*1024, resource.BinarySI),
			RequestedPodCpuQty:    resource.NewMilliQuantity(500, resource.DecimalSI),
			RequestedPodMemQty:    resource.NewQuantity(512*1024*1024, resource.BinarySI),
			UsageCpuQty:           resource.NewMilliQuantity(1200, resource.DecimalSI),
			UsageMemQty:           resource.NewQuantity(2*1024*1024*1024, resource.BinarySI),
			PSIKernelSupported:    true,
		},
	}

	pods := []model.PodModel{
		{
			Namespace:              "default",
			Name:                   "api-0",
			Status:                 "Running",
			Node:                   "node-1",
			IP:                     "10.244.0.5",
			TimeSince:              "3h",
			CreationTime:           created,
			ReadyContainers:        1,
			TotalContainers:        1,
			PodRequestedCpuQty:     resource.NewMilliQuantity(250, resource.DecimalSI),
			PodRequestedMemQty:     resource.NewQuantity(256*1024*1024, resource.BinarySI),
			PodUsageCpuQty:         resource.NewMilliQuantity(150, resource.DecimalSI),
			PodUsageMemQty:         resource.NewQuantity(128*1024*1024, resource.BinarySI),
			NodeAllocatableCpuQty:  resource.NewMilliQuantity(4000, resource.DecimalSI),
			NodeAllocatableMemQty:  resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			NodePSIKernelSupported: true,
		},
		{
			Namespace:              "default",
			Name:                   "worker-1",
			Status:                 "CrashLoopBackOff",
			Node:                   "node-1",
			IP:                     "10.244.0.6",
			TimeSince:              "10m",
			CreationTime:           created,
			Restarts:               12,
			ReadyContainers:        0,
			TotalContainers:        1,
			PodRequestedCpuQty:     resource.NewMilliQuantity(250, resource.DecimalSI),
			PodRequestedMemQty:     resource.NewQuantity(256*1024*1024, resource.BinarySI),
			PodUsageCpuQty:         resource.NewMilliQuantity(50, resource.DecimalSI),
			PodUsageMemQty:         resource.NewQuantity(64*1024*1024, resource.BinarySI),
			NodeAllocatableCpuQty:  resource.NewMilliQuantity(4000, resource.DecimalSI),
			NodeAllocatableMemQty:  resource.NewQuantity(8*1024*1024*1024, resource.BinarySI),
			NodePSIKernelSupported: true,
		},
	}

	snap, err := snapshot.NewBuilder().Build(context.Background(), snapshot.BuildInput{
		Cluster: snapshot.ClusterInfo{Ready: true, MetricsSource: metrics.SourceTypePrometheus},
		Summary: summary,
		Nodes:   nodes,
		Pods:    pods,
	})
	if err != nil {
		t.Fatalf("SnapshotBuilder.Build failed: %v", err)
	}
	return snap
}

func renderThemeViews(t *testing.T, snap *snapshot.Snapshot) []struct {
	name      string
	primitive tview.Primitive
} {
	t.Helper()

	sum := overview.NewClusterSummaryPanel(nil, "Cluster Summary")
	sum.DrawBody(snap)

	nodePanel := overview.NewNodePanel(nil, "Nodes")
	nodePanel.DrawHeader([]string{"NAME", "STATUS", "RST", "PODS", "CPU", "MEM", "STALL"})
	nodePanel.DrawBody(snap)

	podPanel := overview.NewPodPanel(nil, "Pods")
	podPanel.DrawHeader([]string{"NAMESPACE", "POD", "READY", "STATUS", "RST", "CPU", "MEMORY"})
	podPanel.DrawBody(snap)

	nd := nodedetail.NewDetailPanel()
	nd.DrawBody(&snapshot.NodeDetailProjection{
		Snapshot: snap,
		NodeName: "node-1",
	})

	pd := poddetail.NewDetailPanel()
	pd.DrawBody(&snapshot.PodDetailProjection{
		Snapshot:  snap,
		Namespace: "default",
		PodName:   "worker-1",
	})

	help := ui.NewHelpModal(ui.HelpModalConfig{CurrentPage: "Overview"}).GetRootView()
	yamlModal := ui.NewYAMLModal(ui.YAMLModalConfig{
		Title:   "Deployment YAML: default/api",
		Content: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n",
	}).GetRootView()
	colModal := ui.NewColumnSelectorModal(ui.ColumnSelectorConfig{
		Title:          "Nodes",
		AllColumns:     []string{"NAME", "STATUS", "CPU"},
		CurrentColumns: []string{"NAME", "CPU"},
	}).GetRootView()
	toast := ui.NewToastWithButtons("connection lost [403]", ui.ToastError, []string{"Retry", "Quit"}, nil)

	tabBar := ui.NewTabBar()
	tabBar.SetTabs([]ui.TabItem{
		{ID: "overview", Title: "Overview", Key: '1'},
		{ID: "workloads", Title: "Workloads", Key: '2'},
	})
	tabBar.SetActive(0)
	tabBar.SetBreadcrumb("Overview › pod/default/worker-1")

	tabFrame := ui.NewTabFrame(sum.GetRootView())
	tabFrame.SetTabs([]ui.TabItem{
		{ID: "overview", Title: "Overview", Key: '1'},
		{ID: "workloads", Title: "Workloads", Key: '2'},
	})
	tabFrame.SetActive(0)
	tabFrame.SetBreadcrumb("Overview › pod/default/worker-1")

	return []struct {
		name      string
		primitive tview.Primitive
	}{
		{"TabBar", tabBar.GetView()},
		{"TabFrame", tabFrame.GetView()},
		{"SummaryPanel", sum.GetRootView()},
		{"NodePanel", nodePanel.GetRootView()},
		{"PodPanel", podPanel.GetRootView()},
		{"NodeDetailPanel", nd.GetRootView()},
		{"PodDetailPanel", pd.GetRootView()},
		{"HelpModal", help},
		{"YAMLModal", yamlModal},
		{"ColumnSelectorModal", colModal},
		{"ErrorToast", toast},
	}
}

func assertNoLeakedMarkupTags(t *testing.T, viewName, text string) {
	t.Helper()
	leaks := []string{"[#", "[::-", "[::b", "[::u", "[yellow]", "[red]", "[darkcyan]", "[-]"}
	for _, needle := range leaks {
		if strings.Contains(text, needle) {
			t.Errorf("%s rendered unparsed tview tag %q in output:\n%s", viewName, needle, text)
		}
	}
	if leakedPalette256TagRe.MatchString(text) {
		t.Errorf("%s rendered unparsed 256-color tag in output:\n%s", viewName, text)
	}
}

func srgbToLinear(c int32) float64 {
	v := float64(c) / 255.0
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func relativeLuminance(c tcell.Color, defaultRGB int32) float64 {
	if c == tcell.ColorDefault {
		c = tcell.NewHexColor(defaultRGB)
	}
	r, g, b := c.RGB()
	return 0.2126*srgbToLinear(r) + 0.7152*srgbToLinear(g) + 0.0722*srgbToLinear(b)
}

func wcagContrastRatio(fg, bg tcell.Color, bgDefaultRGB int32) float64 {
	l1 := relativeLuminance(fg, 0xffffff)
	l2 := relativeLuminance(bg, bgDefaultRGB)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func TestAllBuiltinThemesRenderAndContrast(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	themes := []struct {
		themeName string
		colorMode string
	}{
		{"dark", "truecolor"},
		{"light", "truecolor"},
		{"ansi", "16"},
		{"monochrome", "none"},
	}

	for _, tc := range themes {
		t.Run(tc.themeName, func(t *testing.T) {
			rt, err := ui.ResolveAndApplyTheme(tc.themeName, tc.colorMode)
			if err != nil {
				t.Fatalf("ResolveAndApplyTheme(%q, %q) failed: %v", tc.themeName, tc.colorMode, err)
			}

			snap := buildThemeTestSnapshot(t)
			views := renderThemeViews(t, snap)
			for _, v := range views {
				ts, err := ui.NewTestScreen(160, 45)
				if err != nil {
					t.Fatalf("NewTestScreen: %v", err)
				}
				ts.Draw(v.primitive)
				text := ts.Text()
				if strings.TrimSpace(text) == "" {
					t.Errorf("view %s rendered empty text under theme %s", v.name, tc.themeName)
				}
				assertNoLeakedMarkupTags(t, tc.themeName+"/"+v.name, text)
				ts.Close()
			}

			if tc.themeName == "dark" || tc.themeName == "light" {
				p := rt.Palette
				bgFallback := int32(0x000000)
				if !p.IsDark {
					bgFallback = 0xfafafa
				}
				pairs45 := []struct {
					name string
					fg   tcell.Color
					bg   tcell.Color
				}{
					{"CanvasFg/CanvasBg", p.CanvasFg, p.CanvasBg},
					{"HeaderFg/HeaderBg", p.HeaderFg, p.HeaderBg},
					{"TabActiveFg/TabActiveBg", p.TabActiveFg, p.TabActiveBg},
					{"TabInactiveFg/TabInactiveBg", p.TabInactiveFg, p.TabInactiveBg},
					{"RowSelectedFg/RowSelectedBg", p.RowSelectedFg, p.RowSelectedBg},
					{"ToastFg/ToastInfoBg", p.ToastFg, p.ToastInfoBg},
					{"ToastFg/ToastSuccessBg", p.ToastFg, p.ToastSuccessBg},
					{"ToastFg/ToastWarnBg", p.ToastFg, p.ToastWarnBg},
					{"ToastFg/ToastErrorBg", p.ToastFg, p.ToastErrorBg},
				}
				if tc.themeName == "light" {
					pairs45 = append(pairs45,
						struct {
							name string
							fg   tcell.Color
							bg   tcell.Color
						}{"HeaderHotkey/HeaderBg", p.HeaderHotkey, p.HeaderBg},
						struct {
							name string
							fg   tcell.Color
							bg   tcell.Color
						}{"HeaderSortArrow/HeaderBg", p.HeaderSortArrow, p.HeaderBg},
						struct {
							name string
							fg   tcell.Color
							bg   tcell.Color
						}{"StatusWarning/CanvasBg", p.StatusWarning, p.CanvasBg},
						struct {
							name string
							fg   tcell.Color
							bg   tcell.Color
						}{"GaugeMedium/CanvasBg", p.GaugeMedium, p.CanvasBg},
						struct {
							name string
							fg   tcell.Color
							bg   tcell.Color
						}{"SparklineNormal/CanvasBg", p.SparklineNormal, p.CanvasBg},
					)
				}
				for _, pair := range pairs45 {
					ratio := wcagContrastRatio(pair.fg, pair.bg, bgFallback)
					if ratio < 4.5 {
						t.Errorf("theme %s %s contrast ratio = %.2f (< 4.5)", tc.themeName, pair.name, ratio)
					}
				}
				pairs30 := []struct {
					name string
					fg   tcell.Color
					bg   tcell.Color
				}{
					{"StatusError/CanvasBg", p.StatusError, p.CanvasBg},
					{"DataPrimary/CanvasBg", p.DataPrimary, p.CanvasBg},
				}
				for _, pair := range pairs30 {
					ratio := wcagContrastRatio(pair.fg, pair.bg, bgFallback)
					if ratio < 3.0 {
						t.Errorf("theme %s %s contrast ratio = %.2f (< 3.0)", tc.themeName, pair.name, ratio)
					}
				}
			}
		})
	}
}

func findCellTokenOnRow(cells []tcell.SimCell, w, y int, token string) (int, bool) {
	target := []rune(token)
	if len(target) == 0 {
		return -1, false
	}
	base := y * w
	for x := 0; x <= w-len(target); x++ {
		match := true
		for i, want := range target {
			runes := cells[base+x+i].Runes
			if len(runes) == 0 || runes[0] != want {
				match = false
				break
			}
		}
		if match {
			return x, true
		}
	}
	return -1, false
}

func TestMonochromeAndNoColorStrictZeroColors(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	cases := []struct {
		name  string
		setup func(t *testing.T)
	}{
		{
			name: "MonochromeExplicit",
			setup: func(t *testing.T) {
				if _, err := ui.ResolveAndApplyTheme("monochrome", "none"); err != nil {
					t.Fatalf("ResolveAndApplyTheme failed: %v", err)
				}
			},
		},
		{
			name: "NoColorEnvVar",
			setup: func(t *testing.T) {
				t.Setenv("NO_COLOR", "1")
				rt, err := ui.ResolveAndApplyTheme("auto", "auto")
				if err != nil {
					t.Fatalf("ResolveAndApplyTheme failed: %v", err)
				}
				if !rt.IsMonochrome() {
					t.Fatalf("expected NO_COLOR=1 to resolve monochrome, got %+v", rt)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			snap := buildThemeTestSnapshot(t)
			views := renderThemeViews(t, snap)

			var sawHeaderBoldNoReverse bool
			var sawFocusedRowReverse bool
			var sawBold bool
			var sawCrashLoopPrefix bool
			var sawCrashLoopUnderline bool
			var sawSummaryCriticalPrefix bool

			for _, v := range views {
				ts, err := ui.NewTestScreen(160, 45)
				if err != nil {
					t.Fatalf("NewTestScreen: %v", err)
				}
				ts.Draw(v.primitive)

				cells, w, h := ts.Screen().GetContents()
				for i, cell := range cells {
					fg, bg, attrs := cell.Style.Decompose()
					if fg != tcell.ColorDefault || bg != tcell.ColorDefault {
						x, y := i%w, i/w
						t.Fatalf("%s/%s cell (%d,%d) emitted non-default color fg=%v bg=%v",
							tc.name, v.name, x, y, fg, bg)
					}
					if attrs&tcell.AttrBold != 0 {
						sawBold = true
					}
				}

				lines := ts.Lines()
				switch v.name {
				case "SummaryPanel":
					text := ts.Text()
					if strings.Contains(text, "! 1/2") {
						sawSummaryCriticalPrefix = true
					}
					if !strings.Contains(text, "CPU ") {
						t.Errorf("%s/SummaryPanel: expected unclipped 'CPU ' title, got:\n%s", tc.name, text)
					}
				case "NodePanel":
					text := ts.Text()
					if !strings.Contains(text, "Ready") || strings.Contains(text, "! Ready") {
						t.Errorf("%s/NodePanel: healthy node status should render 'Ready' without '! ', got:\n%s", tc.name, text)
					}
					for y, line := range lines {
						if strings.Contains(line, "NAME") && strings.Contains(line, "STATUS") && y < h {
							for _, word := range []string{"NAME", "STATUS", "PODS"} {
								if idx, ok := findCellTokenOnRow(cells, w, y, word); ok {
									sawHeaderBoldNoReverse = true
									for dx := 0; dx < len([]rune(word)) && idx+dx < w; dx++ {
										_, _, attrs := cells[y*w+idx+dx].Style.Decompose()
										if attrs&tcell.AttrBold == 0 {
											t.Errorf("%s/NodePanel: header %q rune %d missing AttrBold", tc.name, word, dx)
										}
										if attrs&tcell.AttrReverse != 0 {
											t.Errorf("%s/NodePanel: header %q rune %d should not use AttrReverse", tc.name, word, dx)
										}
									}
								}
							}
						}
					}
					// Focus table and verify selected row renders with AttrReverse
					ts.Focus(v.primitive)
					ts.Draw(v.primitive)
					focusedCells, fw, fh := ts.Screen().GetContents()
					for y, line := range ts.Lines() {
						if y >= fh {
							break
						}
						if strings.Contains(line, "node-1") {
							if idx, ok := findCellTokenOnRow(focusedCells, fw, y, "node-1"); ok {
								for dx := 0; dx < len("node-1") && idx+dx < fw; dx++ {
									_, _, attrs := focusedCells[y*fw+idx+dx].Style.Decompose()
									if attrs&tcell.AttrReverse != 0 {
										sawFocusedRowReverse = true
									}
								}
							}
						}
					}
				case "PodPanel":
					text := ts.Text()
					if !strings.Contains(text, "Running") || strings.Contains(text, "! Running") {
						t.Errorf("%s/PodPanel: healthy pod status should render 'Running' without '! ', got:\n%s", tc.name, text)
					}
					for y, line := range lines {
						if y < h && strings.Contains(line, "! CrashLoop") {
							if idx, ok := findCellTokenOnRow(cells, w, y, "! CrashLoop"); ok {
								sawCrashLoopPrefix = true
								for dx := 0; dx < len("! CrashLoop"); dx++ {
									col := idx + dx
									if col >= w {
										break
									}
									_, _, attrs := cells[y*w+col].Style.Decompose()
									if attrs&tcell.AttrUnderline != 0 {
										sawCrashLoopUnderline = true
									}
								}
							}
						}
					}
				}

				assertNoLeakedMarkupTags(t, tc.name+"/"+v.name, ts.Text())
				ts.Close()
			}

			// Verify healthy summary stats render without "! " prefix in monochrome
			healthySnap, err := snapshot.NewBuilder().Build(context.Background(), snapshot.BuildInput{
				Cluster: snapshot.ClusterInfo{Ready: true, MetricsSource: metrics.SourceTypePrometheus},
				Summary: model.ClusterSummary{
					HealthScore:      100,
					Uptime:           metav1.NewTime(snap.Summary.Uptime),
					NodesReady:       1,
					NodesCount:       1,
					Namespaces:       2,
					PodsRunning:      2,
					PodsAvailable:    2,
					DeploymentsReady: 2,
					DeploymentsTotal: 2,
				},
			})
			if err != nil {
				t.Fatalf("healthy SnapshotBuilder.Build failed: %v", err)
			}
			healthySum := overview.NewClusterSummaryPanel(nil, "Cluster Summary")
			healthySum.DrawBody(healthySnap)
			hts, err := ui.NewTestScreen(160, 20)
			if err != nil {
				t.Fatalf("NewTestScreen: %v", err)
			}
			hts.Draw(healthySum.GetRootView())
			healthyText := hts.Text()
			hts.Close()
			if !strings.Contains(healthyText, "Deploys: 2/2") || !strings.Contains(healthyText, "Pods: 2/2") {
				t.Errorf("%s: expected healthy summary 'Deploys: 2/2' and 'Pods: 2/2', got:\n%s", tc.name, healthyText)
			}
			if strings.Contains(healthyText, "! ") {
				t.Errorf("%s: healthy summary must not contain '! ', got:\n%s", tc.name, healthyText)
			}

			if !sawHeaderBoldNoReverse {
				t.Errorf("%s: expected monochrome table headers with AttrBold and without AttrReverse", tc.name)
			}
			if !sawFocusedRowReverse {
				t.Errorf("%s: expected AttrReverse on focused monochrome table row", tc.name)
			}
			if !sawBold {
				t.Errorf("%s: expected AttrBold structural styling in monochrome mode", tc.name)
			}
			if !sawSummaryCriticalPrefix {
				t.Errorf("%s: expected '! 1/2' prefix for critical Deploys/Pods ratio", tc.name)
			}
			if !sawCrashLoopPrefix || !sawCrashLoopUnderline {
				t.Errorf("%s: expected '! CrashLoop' prefix (%v) with AttrUnderline (%v)",
					tc.name, sawCrashLoopPrefix, sawCrashLoopUnderline)
			}
		})
	}
}

func TestLightThemeTableContrastAndClipping(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	rt, err := ui.ResolveAndApplyTheme("light", "truecolor")
	if err != nil {
		t.Fatalf("ResolveAndApplyTheme(light) failed: %v", err)
	}

	snap := buildThemeTestSnapshot(t)

	// Unfocused NodePanel data rows must paint LightPalette.CanvasBg with >= 4.5 contrast.
	nodePanel := overview.NewNodePanel(nil, "Nodes")
	nodePanel.DrawHeader([]string{"NAME", "STATUS", "RST", "PODS", "CPU", "MEM"})
	nodePanel.DrawBody(snap)

	ts, err := ui.NewTestScreen(160, 20)
	if err != nil {
		t.Fatalf("NewTestScreen: %v", err)
	}
	defer ts.Close()
	// Prime TestScreen focus on an unrelated dummy box so NodePanel draws unfocused first.
	ts.Draw(tview.NewBox())
	ts.Draw(nodePanel.GetRootView())

	cells, w, h := ts.Screen().GetContents()
	foundHeaderRow := false
	foundNodeRow := false
	for y, line := range ts.Lines() {
		if y >= h {
			break
		}
		if strings.Contains(line, "NAME") {
			foundHeaderRow = true
			hidx, ok := findCellTokenOnRow(cells, w, y, "NAME")
			if !ok {
				t.Fatalf("expected token 'NAME' in header row %q", line)
			}
			for dx := 0; dx < len("NAME"); dx++ {
				fg, bg, _ := cells[y*w+hidx+dx].Style.Decompose()
				if bg != rt.Palette.HeaderBg {
					t.Fatalf("light NodePanel header cell bg = %v, want %v", bg, rt.Palette.HeaderBg)
				}
				if ratio := wcagContrastRatio(fg, bg, 0xfafafa); ratio < 4.5 {
					t.Fatalf("light NodePanel header rune %d contrast = %.2f (< 4.5)", dx, ratio)
				}
			}
		}
		if strings.Contains(line, "node-1") {
			foundNodeRow = true
			// Check rendered cells on the data row span CanvasBg across the row.
			for x := 2; x < w-2; x++ {
				_, bg, _ := cells[y*w+x].Style.Decompose()
				if bg != rt.Palette.CanvasBg {
					t.Fatalf("light NodePanel unselected cell (%d,%d) bg = %v, want %v", x, y, bg, rt.Palette.CanvasBg)
				}
			}
			// Check readable contrast on key text tokens in the row.
			for _, tok := range []string{"node-1", "Ready"} {
				tidx, ok := findCellTokenOnRow(cells, w, y, tok)
				if !ok {
					t.Fatalf("expected token %q in node row %q", tok, line)
				}
				for dx := 0; dx < len(tok); dx++ {
					fg, bg, _ := cells[y*w+tidx+dx].Style.Decompose()
					if ratio := wcagContrastRatio(fg, bg, 0xfafafa); ratio < 4.5 {
						t.Fatalf("light NodePanel unselected %q rune %d contrast = %.2f (< 4.5)", tok, dx, ratio)
					}
				}
			}
		}
	}
	if !foundHeaderRow {
		t.Fatalf("expected 'NAME' header row in light NodePanel output:\n%s", ts.Text())
	}
	if !foundNodeRow {
		t.Fatalf("expected 'node-1' row in light NodePanel output:\n%s", ts.Text())
	}

	// Focused/selected row must use RowSelectedFg on RowSelectedBg with >= 4.5 contrast.
	ts.Focus(nodePanel.GetRootView())
	ts.Draw(nodePanel.GetRootView())
	scells, sw, sh := ts.Screen().GetContents()
	foundSelectedRow := false
	for y, line := range ts.Lines() {
		if y >= sh {
			break
		}
		if strings.Contains(line, "node-1") {
			foundSelectedRow = true
			idx, ok := findCellTokenOnRow(scells, sw, y, "node-1")
			if !ok {
				t.Fatalf("expected focused token 'node-1' in row %q", line)
			}
			for dx := 0; dx < len("node-1") && idx+dx < sw; dx++ {
				fg, bg, _ := scells[y*sw+idx+dx].Style.Decompose()
				if bg != rt.Palette.RowSelectedBg {
					t.Fatalf("light NodePanel selected cell bg = %v, want %v", bg, rt.Palette.RowSelectedBg)
				}
				if fg != rt.Palette.RowSelectedFg {
					t.Fatalf("light NodePanel selected cell fg = %v, want %v", fg, rt.Palette.RowSelectedFg)
				}
				if ratio := wcagContrastRatio(fg, bg, 0xfafafa); ratio < 4.5 {
					t.Fatalf("light NodePanel selected row contrast = %.2f (< 4.5)", ratio)
				}
			}
		}
	}
	if !foundSelectedRow {
		t.Fatalf("expected focused 'node-1' row in light NodePanel output")
	}

	// Check PodPanel data rows also paint CanvasBg cleanly both unfocused and for unselected rows when focused.
	podPanel := overview.NewPodPanel(nil, "Pods")
	podPanel.DrawHeader([]string{"NAMESPACE", "POD", "READY", "STATUS", "CPU", "MEMORY"})
	podPanel.DrawBody(snap)
	pts, err := ui.NewTestScreen(160, 20)
	if err != nil {
		t.Fatalf("NewTestScreen: %v", err)
	}
	defer pts.Close()
	pts.Draw(tview.NewBox())
	pts.Draw(podPanel.GetRootView())
	pcells, pw, ph := pts.Screen().GetContents()
	foundPodRow := false
	for y, line := range pts.Lines() {
		if y >= ph {
			break
		}
		if strings.Contains(line, "api-0") {
			foundPodRow = true
			for x := 2; x < pw-2; x++ {
				_, bg, _ := pcells[y*pw+x].Style.Decompose()
				if bg != rt.Palette.CanvasBg {
					t.Fatalf("light PodPanel unselected cell (%d,%d) bg = %v, want %v", x, y, bg, rt.Palette.CanvasBg)
				}
			}
		}
	}
	if !foundPodRow {
		t.Fatalf("expected 'api-0' row in light PodPanel output")
	}

	// Check SummaryPanel sparkline title does not clip leading "CPU " and bargraph
	// cells use LightPalette SparklineNormal instead of olive/green tones.
	sum := overview.NewClusterSummaryPanel(nil, "Cluster Summary")
	sum.DrawBody(snap)
	sts, err := ui.NewTestScreen(130, 25)
	if err != nil {
		t.Fatalf("NewTestScreen: %v", err)
	}
	defer sts.Close()
	sts.Draw(sum.GetRootView())
	sumText := sts.Text()
	if !strings.Contains(sumText, "CPU ") {
		t.Fatalf("expected unclipped 'CPU ' in light SummaryPanel, got:\n%s", sumText)
	}

	scellsSum, swSum, shSum := sts.Screen().GetContents()
	sawActiveBar := false
	oliveLegacy := tcell.ColorOliveDrab
	forestLegacy := tcell.NewHexColor(0x047857)
	for i := 0; i < swSum*shSum && i < len(scellsSum); i++ {
		if len(scellsSum[i].Runes) == 0 {
			continue
		}
		switch scellsSum[i].Runes[0] {
		case '▂', '▃', '▄', '▅', '▆', '▇', '█':
			sawActiveBar = true
			fg, bg, _ := scellsSum[i].Style.Decompose()
			if fg == oliveLegacy || fg == forestLegacy {
				t.Fatalf("light SummaryPanel bargraph used off-palette color %v, want %v", fg, rt.Palette.SparklineNormal)
			}
			if fg != rt.Palette.SparklineNormal {
				t.Fatalf("light SummaryPanel bargraph fg = %v, want %v", fg, rt.Palette.SparklineNormal)
			}
			if ratio := wcagContrastRatio(fg, bg, 0xfafafa); ratio < 4.5 {
				t.Fatalf("light SummaryPanel bargraph contrast = %.2f (< 4.5)", ratio)
			}
		}
	}
	if !sawActiveBar {
		t.Fatalf("expected active bargraph cells in light SummaryPanel output:\n%s", sumText)
	}
}

func TestColorModeQuantizationTiers(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	renderTier := func(mode string) (cells []tcell.SimCell, text string) {
		if _, err := ui.ResolveAndApplyTheme("light", mode); err != nil {
			t.Fatalf("ResolveAndApplyTheme(light, %q): %v", mode, err)
		}
		snap := buildThemeTestSnapshot(t)
		sum := overview.NewClusterSummaryPanel(nil, "Cluster Summary")
		sum.DrawBody(snap)
		podPanel := overview.NewPodPanel(nil, "Pods")
		podPanel.DrawHeader([]string{"NAMESPACE", "POD", "READY", "STATUS", "CPU", "MEMORY"})
		podPanel.DrawBody(snap)

		flex := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(sum.GetRootView(), 12, 0, false).
			AddItem(podPanel.GetRootView(), 0, 1, true)

		ts, err := ui.NewTestScreen(160, 45)
		if err != nil {
			t.Fatalf("NewTestScreen: %v", err)
		}
		defer ts.Close()
		ts.Draw(flex)
		simCells, _, _ := ts.Screen().GetContents()
		out := make([]tcell.SimCell, len(simCells))
		copy(out, simCells)
		return out, ts.Text()
	}

	t.Run("TrueColor", func(t *testing.T) {
		cells, text := renderTier("truecolor")
		assertNoLeakedMarkupTags(t, "light/truecolor", text)
		var sawRGB bool
		for _, cell := range cells {
			fg, bg, _ := cell.Style.Decompose()
			if fg.IsRGB() || bg.IsRGB() {
				sawRGB = true
				break
			}
		}
		if !sawRGB {
			t.Error("expected at least one 24-bit RGB color cell in truecolor mode")
		}
	})

	t.Run("256Color", func(t *testing.T) {
		cells, text := renderTier("256")
		assertNoLeakedMarkupTags(t, "light/256", text)
		var saw256 bool
		for _, cell := range cells {
			fg, bg, _ := cell.Style.Decompose()
			for _, c := range []tcell.Color{fg, bg} {
				if c == tcell.ColorDefault {
					continue
				}
				if c.IsRGB() {
					t.Fatalf("256-color mode emitted raw RGB color %v", c)
				}
				if c >= tcell.ColorValid+16 && c < tcell.ColorValid+256 {
					saw256 = true
				}
			}
		}
		if !saw256 {
			t.Error("expected quantized 256-color indexed palette cell (16..255)")
		}
	})

	t.Run("16Color", func(t *testing.T) {
		cells, text := renderTier("16")
		assertNoLeakedMarkupTags(t, "light/16", text)
		var sawANSI bool
		for _, cell := range cells {
			fg, bg, _ := cell.Style.Decompose()
			for _, c := range []tcell.Color{fg, bg} {
				if c == tcell.ColorDefault {
					continue
				}
				if c.IsRGB() || c < tcell.ColorValid || c >= tcell.ColorValid+16 {
					t.Fatalf("16-color mode emitted non-ANSI color %v", c)
				}
				sawANSI = true
			}
		}
		if !sawANSI {
			t.Error("expected ANSI 16-color cells in 16-color mode")
		}
	})
}
