package theme

import "github.com/gdamore/tcell/v2"

// ColorMode controls color output degradation across terminal capabilities.
type ColorMode string

const (
	ColorModeAuto      ColorMode = "auto"
	ColorModeTrueColor ColorMode = "truecolor"
	ColorMode256       ColorMode = "256"
	ColorMode16        ColorMode = "16"
	ColorModeNone      ColorMode = "none"
)

// Palette defines the semantic color contract for ktop UI components.
type Palette struct {
	Name        string
	Description string
	IsDark      bool

	// Base Canvas & Chrome
	CanvasBg      tcell.Color
	CanvasFg      tcell.Color
	BorderNormal  tcell.Color
	BorderFocused tcell.Color
	TitleNormal   tcell.Color
	TitleFocused  tcell.Color

	// Tables & Lists
	HeaderBg        tcell.Color
	HeaderFg        tcell.Color
	HeaderHotkey    tcell.Color
	HeaderSortArrow tcell.Color
	TabInactiveBg   tcell.Color
	TabInactiveFg   tcell.Color
	RowSelectedBg   tcell.Color
	RowSelectedFg   tcell.Color
	RowAlternateBg  tcell.Color

	// Resource Status
	StatusOK      tcell.Color
	StatusWarning tcell.Color
	StatusError   tcell.Color
	StatusInfo    tcell.Color
	StatusMuted   tcell.Color

	// Metrics & Resource Saturation
	GaugeLow        tcell.Color
	GaugeMedium     tcell.Color
	GaugeElevated   tcell.Color
	GaugeHigh       tcell.Color
	SparklineNormal tcell.Color
	SparklineAlert  tcell.Color
	SparklineEmpty  tcell.Color
	BarEmpty        tcell.Color

	// Accents & Data
	Accent        tcell.Color
	DataPrimary   tcell.Color
	DataSecondary tcell.Color

	// Diagnostic Toasts
	ToastInfoBg    tcell.Color
	ToastSuccessBg tcell.Color
	ToastWarnBg    tcell.Color
	ToastErrorBg   tcell.Color
	ToastFg        tcell.Color
}

// ResolvedTheme holds an active palette quantized for a specific ColorMode.
type ResolvedTheme struct {
	Palette Palette
	Mode    ColorMode
}

// IsMonochrome reports whether styling must avoid all terminal color sequences.
func (rt ResolvedTheme) IsMonochrome() bool {
	return rt.Mode == ColorModeNone || rt.Palette.Name == "monochrome"
}

var (
	ansi16Colors = [16]tcell.Color{
		tcell.ColorBlack,
		tcell.ColorMaroon,
		tcell.ColorGreen,
		tcell.ColorOlive,
		tcell.ColorNavy,
		tcell.ColorPurple,
		tcell.ColorTeal,
		tcell.ColorSilver,
		tcell.ColorGray,
		tcell.ColorRed,
		tcell.ColorLime,
		tcell.ColorYellow,
		tcell.ColorBlue,
		tcell.ColorFuchsia,
		tcell.ColorAqua,
		tcell.ColorWhite,
	}

	xtermCubeLevels = [6]int32{0, 95, 135, 175, 215, 255}

	canonicalColorNames map[tcell.Color]string
	palette256TagNames  [256]string
)

// Register p0..p255 indexed palette names during package init (before any concurrent
// UI rendering starts) so tview markup tags resolve directly to PaletteColor indices.
func init() {
	canonicalColorNames = map[tcell.Color]string{
		tcell.ColorBlack:      "black",
		tcell.ColorMaroon:     "maroon",
		tcell.ColorGreen:      "green",
		tcell.ColorOlive:      "olive",
		tcell.ColorNavy:       "navy",
		tcell.ColorPurple:     "purple",
		tcell.ColorTeal:       "teal",
		tcell.ColorSilver:     "silver",
		tcell.ColorGray:       "gray",
		tcell.ColorRed:        "red",
		tcell.ColorLime:       "lime",
		tcell.ColorYellow:     "yellow",
		tcell.ColorBlue:       "blue",
		tcell.ColorFuchsia:    "fuchsia",
		tcell.ColorAqua:       "aqua",
		tcell.ColorWhite:      "white",
		tcell.ColorOrange:     "orange",
		tcell.ColorOliveDrab:  "olivedrab",
		tcell.ColorDarkGray:   "darkgray",
		tcell.ColorLightGray:  "lightgray",
		tcell.ColorDodgerBlue: "dodgerblue",
		tcell.ColorDarkCyan:   "darkcyan",
		tcell.ColorDarkBlue:   "darkblue",
		tcell.ColorDarkGreen:  "darkgreen",
		tcell.ColorDarkOrange: "darkorange",
		tcell.ColorDarkRed:    "darkred",
	}

	if _, exists := tcell.ColorNames["cyan"]; !exists {
		tcell.ColorNames["cyan"] = tcell.ColorAqua
	}

	// Register paa..pjv indexed palette names during package init (before any concurrent
	// UI rendering starts) because tview's color regex rejects digits in named tags.
	for i := 0; i < 256; i++ {
		name := paletteIndexTagName(i)
		palette256TagNames[i] = name
		tcell.ColorNames[name] = tcell.PaletteColor(i)
	}
}

func paletteIndexTagName(idx int) string {
	return string([]byte{'p', byte('a' + idx/26), byte('a' + idx%26)})
}

// ResolveColor adapts a single color token to the target ColorMode.
func ResolveColor(c tcell.Color, mode ColorMode) tcell.Color {
	if c == tcell.ColorDefault || mode == ColorModeNone {
		return tcell.ColorDefault
	}

	switch mode {
	case ColorModeTrueColor, ColorModeAuto, "":
		return c
	case ColorMode256:
		if !c.IsRGB() && c >= tcell.ColorValid && c < tcell.ColorValid+256 {
			return c
		}
		r, g, b := c.RGB()
		return nearestXterm256(r, g, b)
	case ColorMode16:
		if !c.IsRGB() && c >= tcell.ColorValid && c < tcell.ColorValid+16 {
			return c
		}
		r, g, b := c.RGB()
		return nearestANSI16(r, g, b)
	default:
		return c
	}
}

// Resolve returns a copy of p with every color field mapped to mode.
func (p Palette) Resolve(mode ColorMode) Palette {
	p.CanvasBg = ResolveColor(p.CanvasBg, mode)
	p.CanvasFg = ResolveColor(p.CanvasFg, mode)
	p.BorderNormal = ResolveColor(p.BorderNormal, mode)
	p.BorderFocused = ResolveColor(p.BorderFocused, mode)
	p.TitleNormal = ResolveColor(p.TitleNormal, mode)
	p.TitleFocused = ResolveColor(p.TitleFocused, mode)

	p.HeaderBg = ResolveColor(p.HeaderBg, mode)
	p.HeaderFg = ResolveColor(p.HeaderFg, mode)
	p.HeaderHotkey = ResolveColor(p.HeaderHotkey, mode)
	p.HeaderSortArrow = ResolveColor(p.HeaderSortArrow, mode)
	p.TabInactiveBg = ResolveColor(p.TabInactiveBg, mode)
	p.TabInactiveFg = ResolveColor(p.TabInactiveFg, mode)
	p.RowSelectedBg = ResolveColor(p.RowSelectedBg, mode)
	p.RowSelectedFg = ResolveColor(p.RowSelectedFg, mode)
	p.RowAlternateBg = ResolveColor(p.RowAlternateBg, mode)

	p.StatusOK = ResolveColor(p.StatusOK, mode)
	p.StatusWarning = ResolveColor(p.StatusWarning, mode)
	p.StatusError = ResolveColor(p.StatusError, mode)
	p.StatusInfo = ResolveColor(p.StatusInfo, mode)
	p.StatusMuted = ResolveColor(p.StatusMuted, mode)

	p.GaugeLow = ResolveColor(p.GaugeLow, mode)
	p.GaugeMedium = ResolveColor(p.GaugeMedium, mode)
	p.GaugeElevated = ResolveColor(p.GaugeElevated, mode)
	p.GaugeHigh = ResolveColor(p.GaugeHigh, mode)
	p.SparklineNormal = ResolveColor(p.SparklineNormal, mode)
	p.SparklineAlert = ResolveColor(p.SparklineAlert, mode)
	p.SparklineEmpty = ResolveColor(p.SparklineEmpty, mode)
	p.BarEmpty = ResolveColor(p.BarEmpty, mode)

	p.Accent = ResolveColor(p.Accent, mode)
	p.DataPrimary = ResolveColor(p.DataPrimary, mode)
	p.DataSecondary = ResolveColor(p.DataSecondary, mode)

	p.ToastInfoBg = ResolveColor(p.ToastInfoBg, mode)
	p.ToastSuccessBg = ResolveColor(p.ToastSuccessBg, mode)
	p.ToastWarnBg = ResolveColor(p.ToastWarnBg, mode)
	p.ToastErrorBg = ResolveColor(p.ToastErrorBg, mode)
	p.ToastFg = ResolveColor(p.ToastFg, mode)
	return p
}

func nearestXterm256(r, g, b int32) tcell.Color {
	bestIdx := 16
	bestDist := int64(1<<62 - 1)

	// 6x6x6 RGB cube (indices 16..231)
	for ri, cr := range xtermCubeLevels {
		for gi, cg := range xtermCubeLevels {
			for bi, cb := range xtermCubeLevels {
				dist := rgbDistSq(r, g, b, cr, cg, cb)
				if dist < bestDist {
					bestDist = dist
					bestIdx = 16 + ri*36 + gi*6 + bi
				}
			}
		}
	}

	// 24-step grayscale ramp (indices 232..255)
	for k := 0; k < 24; k++ {
		gray := int32(8 + 10*k)
		dist := rgbDistSq(r, g, b, gray, gray, gray)
		if dist < bestDist {
			bestDist = dist
			bestIdx = 232 + k
		}
	}

	return tcell.PaletteColor(bestIdx)
}

func nearestANSI16(r, g, b int32) tcell.Color {
	bestColor := tcell.ColorWhite
	bestDist := int64(1<<62 - 1)

	for _, candidate := range ansi16Colors {
		cr, cg, cb := candidate.RGB()
		dist := rgbDistSq(r, g, b, cr, cg, cb)
		if dist < bestDist {
			bestDist = dist
			bestColor = candidate
		}
	}
	return bestColor
}

func rgbDistSq(r1, g1, b1, r2, g2, b2 int32) int64 {
	dr := int64(r1 - r2)
	dg := int64(g1 - g2)
	db := int64(b1 - b2)
	return dr*dr + dg*dg + db*db
}
