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
	colModal := ui.NewColumnSelectorModal(ui.ColumnSelectorConfig{
		Title:          "Nodes",
		AllColumns:     []string{"NAME", "STATUS", "CPU"},
		CurrentColumns: []string{"NAME", "CPU"},
	}).GetRootView()
	toast := ui.NewToast("connection lost", ui.ToastError)

	return []struct {
		name      string
		primitive tview.Primitive
	}{
		{"SummaryPanel", sum.GetRootView()},
		{"NodePanel", nodePanel.GetRootView()},
		{"PodPanel", podPanel.GetRootView()},
		{"NodeDetailPanel", nd.GetRootView()},
		{"PodDetailPanel", pd.GetRootView()},
		{"HelpModal", help},
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
					{"RowSelectedFg/RowSelectedBg", p.RowSelectedFg, p.RowSelectedBg},
					{"ToastFg/ToastInfoBg", p.ToastFg, p.ToastInfoBg},
					{"ToastFg/ToastErrorBg", p.ToastFg, p.ToastErrorBg},
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

			var sawTableReverse bool
			var sawBold bool
			var sawCrashLoopPrefix bool
			var sawCrashLoopUnderline bool

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
					if (v.name == "NodePanel" || v.name == "PodPanel") && attrs&tcell.AttrReverse != 0 {
						sawTableReverse = true
					}
				}

				if v.name == "PodPanel" {
					lines := ts.Lines()
					for y, line := range lines {
						if idx := strings.Index(line, "! CrashLoop"); idx >= 0 && y < h {
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

				assertNoLeakedMarkupTags(t, tc.name+"/"+v.name, ts.Text())
				ts.Close()
			}

			if !sawTableReverse {
				t.Errorf("%s: expected AttrReverse on monochrome table header/selection cells", tc.name)
			}
			if !sawBold {
				t.Errorf("%s: expected AttrBold structural styling in monochrome mode", tc.name)
			}
			if !sawCrashLoopPrefix || !sawCrashLoopUnderline {
				t.Errorf("%s: expected '! CrashLoop' prefix (%v) with AttrUnderline (%v)",
					tc.name, sawCrashLoopPrefix, sawCrashLoopUnderline)
			}
		})
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
