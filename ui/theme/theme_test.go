package theme

import (
	"math"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestBuiltinsAndByName(t *testing.T) {
	cases := []struct {
		name   string
		isDark bool
	}{
		{"dark", true},
		{"LIGHT", false},
		{"ansi", true},
		{"monochrome", true},
	}

	for _, tc := range cases {
		p, ok := ByName(tc.name)
		if !ok {
			t.Fatalf("ByName(%q) returned false", tc.name)
		}
		if p.Name != strings.ToLower(tc.name) {
			t.Errorf("ByName(%q).Name = %q", tc.name, p.Name)
		}
		if p.IsDark != tc.isDark {
			t.Errorf("ByName(%q).IsDark = %v, want %v", tc.name, p.IsDark, tc.isDark)
		}
	}

	if _, ok := ByName("solarized"); ok {
		t.Fatal("expected unknown theme name to return false")
	}

	// Verify ANSI palette uses strictly ColorDefault or ANSI 0..15.
	ansi := ANSIPalette()
	ansiColors := []tcell.Color{
		ansi.CanvasBg, ansi.CanvasFg, ansi.BorderNormal, ansi.BorderFocused,
		ansi.HeaderBg, ansi.HeaderFg, ansi.HeaderHotkey, ansi.RowSelectedBg,
		ansi.StatusOK, ansi.StatusWarning, ansi.StatusError, ansi.GaugeHigh,
	}
	for _, c := range ansiColors {
		if c == tcell.ColorDefault {
			continue
		}
		if c.IsRGB() || c < tcell.ColorValid || c >= tcell.ColorValid+16 {
			t.Errorf("ANSI palette contains non-16-color token: %v", c)
		}
	}

	// Verify Monochrome palette has only ColorDefault.
	mono := MonochromePalette()
	monoColors := []tcell.Color{
		mono.CanvasBg, mono.CanvasFg, mono.BorderNormal, mono.BorderFocused,
		mono.HeaderBg, mono.HeaderFg, mono.StatusError, mono.ToastErrorBg,
	}
	for _, c := range monoColors {
		if c != tcell.ColorDefault {
			t.Errorf("Monochrome palette contains non-default color: %v", c)
		}
	}
}

func TestResolveColorTiersAndRoundTrip(t *testing.T) {
	light := LightPalette()
	sample := light.HeaderBg // #1f3a52

	// Tier 1: TrueColor preserves exact RGB.
	tc := ResolveColor(sample, ColorModeTrueColor)
	if tc != sample || !tc.IsRGB() {
		t.Fatalf("TrueColor modified RGB color: got %v, want %v", tc, sample)
	}
	if tag := ColorTag(sample, ColorModeTrueColor); tag != "#1f3a52" {
		t.Errorf("TrueColor ColorTag = %q, want #1f3a52", tag)
	}

	// Tier 2: 256-color quantizes RGB into PaletteColor(16..255) without IsRGB().
	c256 := ResolveColor(sample, ColorMode256)
	if c256.IsRGB() || c256 < tcell.ColorValid+16 || c256 >= tcell.ColorValid+256 {
		t.Fatalf("ColorMode256 produced invalid palette color: %v", c256)
	}
	tag256 := ColorTag(sample, ColorMode256)
	if len(tag256) != 3 || tag256[0] != 'p' {
		t.Errorf("ColorMode256 ColorTag = %q, expected 3-letter pure alpha p<xx>", tag256)
	}
	if roundTrip := tcell.GetColor(tag256); roundTrip != c256 {
		t.Errorf("tcell.GetColor(%q) = %v, want %v", tag256, roundTrip, c256)
	}

	// Tier 3: 16-color maps any color into ANSI 0..15.
	c16 := ResolveColor(sample, ColorMode16)
	if c16.IsRGB() || c16 < tcell.ColorValid || c16 >= tcell.ColorValid+16 {
		t.Fatalf("ColorMode16 produced non-16-color: %v", c16)
	}
	tag16 := ColorTag(sample, ColorMode16)
	if roundTrip := tcell.GetColor(tag16); roundTrip != c16 {
		t.Errorf("tcell.GetColor(%q) = %v, want %v", tag16, roundTrip, c16)
	}

	// Tier 4: Monochrome strips all color tokens to ColorDefault / "-".
	if cNone := ResolveColor(sample, ColorModeNone); cNone != tcell.ColorDefault {
		t.Errorf("ColorModeNone = %v, want ColorDefault", cNone)
	}
	if tagNone := ColorTag(sample, ColorModeNone); tagNone != "-" {
		t.Errorf("ColorModeNone ColorTag = %q, want -", tagNone)
	}

	// Verify DarkPalette named tags remain exact W3C names under TrueColor.
	dark := DarkPalette()
	for color, want := range map[tcell.Color]string{
		dark.GaugeLow:        "green",
		dark.GaugeMedium:     "yellow",
		dark.GaugeElevated:   "orange",
		dark.GaugeHigh:       "red",
		dark.StatusMuted:     "gray",
		dark.StatusOK:        "olivedrab",
		dark.BarEmpty:        "darkgray",
		dark.BorderFocused:   "dodgerblue",
		dark.DataSecondary:   "white",
	} {
		if got := ColorTag(color, ColorModeTrueColor); got != want {
			t.Errorf("DarkPalette ColorTag(%v) = %q, want %q", color, got, want)
		}
	}
}

func makeEnv(m map[string]string) EnvFunc {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestResolvePrecedence(t *testing.T) {
	noProbe := func() (bool, bool) { return false, false }

	t.Run("NO_COLOR non-empty selects monochrome", func(t *testing.T) {
		rt, err := Resolve(ResolveOptions{
			Theme:        "auto",
			ColorMode:    "auto",
			LookupEnv:    makeEnv(map[string]string{"NO_COLOR": "1", "COLORTERM": "truecolor"}),
			ProbeLightBg: noProbe,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !rt.IsMonochrome() || rt.Palette.Name != "monochrome" || rt.Mode != ColorModeNone {
			t.Fatalf("got %+v, want monochrome + ColorModeNone", rt)
		}
	})

	t.Run("NO_COLOR empty string selects monochrome", func(t *testing.T) {
		rt, err := Resolve(ResolveOptions{
			Theme:        "auto",
			ColorMode:    "auto",
			LookupEnv:    makeEnv(map[string]string{"NO_COLOR": "", "TERM": "xterm-256color"}),
			ProbeLightBg: noProbe,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !rt.IsMonochrome() || rt.Mode != ColorModeNone {
			t.Fatalf("got %+v, want monochrome + ColorModeNone", rt)
		}
	})

	t.Run("TERM=dumb selects monochrome", func(t *testing.T) {
		rt, err := Resolve(ResolveOptions{
			LookupEnv:    makeEnv(map[string]string{"TERM": "dumb"}),
			ProbeLightBg: noProbe,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !rt.IsMonochrome() {
			t.Fatalf("expected monochrome for TERM=dumb, got %+v", rt)
		}
	})

	t.Run("explicit theme overrides NO_COLOR unless color-mode is none", func(t *testing.T) {
		rt, err := Resolve(ResolveOptions{
			Theme:        "light",
			ColorMode:    "truecolor",
			LookupEnv:    makeEnv(map[string]string{"NO_COLOR": "1"}),
			ProbeLightBg: noProbe,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rt.Palette.Name != "light" || rt.Mode != ColorModeTrueColor {
			t.Fatalf("got %+v, want light + truecolor", rt)
		}
	})

	t.Run("COLORFGBG light and dark detection", func(t *testing.T) {
		rtLight, err := Resolve(ResolveOptions{
			LookupEnv:    makeEnv(map[string]string{"COLORFGBG": "0;15", "COLORTERM": "truecolor"}),
			ProbeLightBg: noProbe,
		})
		if err != nil || rtLight.Palette.Name != "light" {
			t.Fatalf("expected light for COLORFGBG=0;15, got %+v (err=%v)", rtLight, err)
		}

		rtLightMulti, err := Resolve(ResolveOptions{
			LookupEnv:    makeEnv(map[string]string{"COLORFGBG": "default;default;7", "TERM": "xterm-256color"}),
			ProbeLightBg: noProbe,
		})
		if err != nil || rtLightMulti.Palette.Name != "light" || rtLightMulti.Mode != ColorMode256 {
			t.Fatalf("expected light + 256 for COLORFGBG=default;default;7, got %+v (err=%v)", rtLightMulti, err)
		}

		rtDark, err := Resolve(ResolveOptions{
			LookupEnv:    makeEnv(map[string]string{"COLORFGBG": "15;0", "TERM": "xterm"}),
			ProbeLightBg: func() (bool, bool) { return true, true },
		})
		if err != nil || rtDark.Palette.Name != "dark" || rtDark.Mode != ColorMode16 {
			t.Fatalf("expected dark + 16 for COLORFGBG=15;0, got %+v (err=%v)", rtDark, err)
		}
	})

	t.Run("OSC 11 probe fallback when COLORFGBG absent", func(t *testing.T) {
		rt, err := Resolve(ResolveOptions{
			LookupEnv:    makeEnv(map[string]string{"COLORTERM": "24bit"}),
			ProbeLightBg: func() (bool, bool) { return true, true },
		})
		if err != nil || rt.Palette.Name != "light" || rt.Mode != ColorModeTrueColor {
			t.Fatalf("expected light + truecolor from OSC 11 probe, got %+v (err=%v)", rt, err)
		}

		rtFallback, err := Resolve(ResolveOptions{
			LookupEnv:    makeEnv(map[string]string{}),
			ProbeLightBg: noProbe,
		})
		if err != nil || rtFallback.Palette.Name != "dark" || rtFallback.Mode != ColorMode16 {
			t.Fatalf("expected dark + 16 fallback, got %+v (err=%v)", rtFallback, err)
		}
	})

	t.Run("invalid theme or color-mode returns error", func(t *testing.T) {
		if _, err := Resolve(ResolveOptions{Theme: "neon"}); err == nil {
			t.Error("expected error for unknown theme")
		}
		if _, err := Resolve(ResolveOptions{ColorMode: "8bit"}); err == nil {
			t.Error("expected error for unknown color mode")
		}
	})
}

func TestParseOSC11Luminance(t *testing.T) {
	darkLum, ok := ParseOSC11Luminance("\x1b]11;rgb:1a1a/1b1b/2626\x1b\\")
	if !ok {
		t.Fatal("failed to parse dark OSC 11 response")
	}
	if darkLum >= 0.5 {
		t.Errorf("dark OSC 11 luminance = %f, expected < 0.5", darkLum)
	}

	lightLum, ok := ParseOSC11Luminance("\x1b]11;rgb:fafa/fafa/fafa\x07")
	if !ok {
		t.Fatal("failed to parse light OSC 11 response")
	}
	if lightLum <= 0.5 || math.Abs(lightLum-0.98) > 0.02 {
		t.Errorf("light OSC 11 luminance = %f, expected ~0.98", lightLum)
	}

	shortLum, ok := ParseOSC11Luminance("]11;rgb:ff/ff/ff\x1b\\")
	if !ok || math.Abs(shortLum-1.0) > 0.01 {
		t.Errorf("2-digit hex OSC 11 luminance = %f (ok=%v), want 1.0", shortLum, ok)
	}

	if _, ok := ParseOSC11Luminance("garbage"); ok {
		t.Error("expected invalid OSC 11 string to return false")
	}
}

func TestFormatTagAndWrapError(t *testing.T) {
	dark := DarkPalette()

	if got := FormatTag(tcell.ColorYellow, tcell.ColorDefault, "b", ColorModeTrueColor); got != "[yellow::b]" {
		t.Errorf("FormatTag color bold = %q, want [yellow::b]", got)
	}
	if got := FormatTag(tcell.ColorYellow, tcell.ColorDefault, "b", ColorModeNone); got != "[::b]" {
		t.Errorf("FormatTag mono bold = %q, want [::b]", got)
	}
	if got := FormatTag(tcell.ColorYellow, tcell.ColorDefault, "", ColorModeNone); got != "" {
		t.Errorf("FormatTag mono plain = %q, want empty", got)
	}

	if got := WrapError("CrashLoop", dark, ColorModeTrueColor); got != "[red]CrashLoop[-]" {
		t.Errorf("WrapError dark = %q, want [red]CrashLoop[-]", got)
	}
	if got := WrapError("CrashLoop", MonochromePalette(), ColorModeNone); got != "[::bu]! CrashLoop[::-]" {
		t.Errorf("WrapError mono = %q, want [::bu]! CrashLoop[::-]", got)
	}
	if got := ResetTag(true, false, true); got != "[-::-]" {
		t.Errorf("ResetTag = %q, want [-::-]", got)
	}
}
