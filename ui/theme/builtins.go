package theme

import (
	"strings"

	"github.com/gdamore/tcell/v2"
)

// DarkPalette returns the default high-contrast palette for dark terminal backgrounds.
func DarkPalette() Palette {
	return Palette{
		Name:        "dark",
		Description: "Dark terminal palette",
		IsDark:      true,

		CanvasBg:      tcell.ColorDefault,
		CanvasFg:      tcell.ColorWhite,
		BorderNormal:  tcell.ColorLightGray,
		BorderFocused: tcell.ColorDodgerBlue,
		TitleNormal:   tcell.ColorWhite,
		TitleFocused:  tcell.ColorAqua,

		HeaderBg:        tcell.NewHexColor(0x005f5f),
		HeaderFg:        tcell.ColorWhite,
		HeaderHotkey:    tcell.ColorOrange,
		HeaderSortArrow: tcell.ColorWhite,
		TabInactiveBg:   tcell.NewHexColor(0x222831),
		TabInactiveFg:   tcell.NewHexColor(0x9ca3af),
		RowSelectedBg:   tcell.ColorWhite,
		RowSelectedFg:   tcell.ColorBlack,
		RowAlternateBg:  tcell.ColorDefault,

		StatusOK:      tcell.ColorOliveDrab,
		StatusWarning: tcell.ColorYellow,
		StatusError:   tcell.ColorRed,
		StatusInfo:    tcell.ColorAqua,
		StatusMuted:   tcell.ColorGray,

		GaugeLow:        tcell.ColorGreen,
		GaugeMedium:     tcell.ColorYellow,
		GaugeElevated:   tcell.ColorOrange,
		GaugeHigh:       tcell.ColorRed,
		SparklineNormal: tcell.ColorOliveDrab,
		SparklineAlert:  tcell.ColorRed,
		SparklineEmpty:  tcell.ColorGray,
		BarEmpty:        tcell.ColorDarkGray,

		Accent:        tcell.ColorAqua,
		DataPrimary:   tcell.ColorYellow,
		DataSecondary: tcell.ColorWhite,

		ToastInfoBg:    tcell.ColorDarkBlue,
		ToastSuccessBg: tcell.ColorDarkGreen,
		ToastWarnBg:    tcell.NewHexColor(0x92400e),
		ToastErrorBg:   tcell.ColorDarkRed,
		ToastFg:        tcell.ColorWhite,
	}
}

// LightPalette returns a high-contrast palette tuned for light terminal backgrounds.
func LightPalette() Palette {
	return Palette{
		Name:        "light",
		Description: "Light terminal palette",
		IsDark:      false,

		CanvasBg:      tcell.NewHexColor(0xfafafa),
		CanvasFg:      tcell.NewHexColor(0x1a1a1a),
		BorderNormal:  tcell.NewHexColor(0x9ca3af),
		BorderFocused: tcell.NewHexColor(0x0284c7),
		TitleNormal:   tcell.NewHexColor(0x1e293b),
		TitleFocused:  tcell.NewHexColor(0x0f4c81),

		HeaderBg:        tcell.NewHexColor(0xe2e8f0),
		HeaderFg:        tcell.NewHexColor(0x1e293b),
		HeaderHotkey:    tcell.NewHexColor(0x0369a1),
		HeaderSortArrow: tcell.NewHexColor(0x0369a1),
		TabInactiveBg:   tcell.NewHexColor(0x94a3b8),
		TabInactiveFg:   tcell.NewHexColor(0x1e293b),
		RowSelectedBg:   tcell.NewHexColor(0xdbeafe),
		RowSelectedFg:   tcell.NewHexColor(0x0f172a),
		RowAlternateBg:  tcell.ColorDefault,

		StatusOK:      tcell.NewHexColor(0x047857),
		StatusWarning: tcell.NewHexColor(0x9a6700),
		StatusError:   tcell.NewHexColor(0xb91c1c),
		StatusInfo:    tcell.NewHexColor(0x0369a1),
		StatusMuted:   tcell.NewHexColor(0x6b7280),

		GaugeLow:        tcell.NewHexColor(0x047857),
		GaugeMedium:     tcell.NewHexColor(0x9a6700),
		GaugeElevated:   tcell.NewHexColor(0xc2410c),
		GaugeHigh:       tcell.NewHexColor(0xb91c1c),
		SparklineNormal: tcell.NewHexColor(0x0369a1),
		SparklineAlert:  tcell.NewHexColor(0xb91c1c),
		SparklineEmpty:  tcell.NewHexColor(0x9ca3af),
		BarEmpty:        tcell.NewHexColor(0x9ca3af),

		Accent:        tcell.NewHexColor(0x0f4c81),
		DataPrimary:   tcell.NewHexColor(0x0f4c81),
		DataSecondary: tcell.NewHexColor(0x1a1a1a),

		ToastInfoBg:    tcell.NewHexColor(0xe0f2fe),
		ToastSuccessBg: tcell.NewHexColor(0xdcfce7),
		ToastWarnBg:    tcell.NewHexColor(0xfef3c7),
		ToastErrorBg:   tcell.NewHexColor(0xfee2e2),
		ToastFg:        tcell.NewHexColor(0x0f172a),
	}
}

// ANSIPalette returns a 16-color palette that inherits the terminal's ANSI theme.
func ANSIPalette() Palette {
	return Palette{
		Name:        "ansi",
		Description: "16-color ANSI terminal palette",
		IsDark:      true,

		CanvasBg:      tcell.ColorDefault,
		CanvasFg:      tcell.ColorDefault,
		BorderNormal:  tcell.ColorSilver,
		BorderFocused: tcell.ColorBlue,
		TitleNormal:   tcell.ColorDefault,
		TitleFocused:  tcell.ColorAqua,

		HeaderBg:        tcell.ColorTeal,
		HeaderFg:        tcell.ColorWhite,
		HeaderHotkey:    tcell.ColorYellow,
		HeaderSortArrow: tcell.ColorWhite,
		TabInactiveBg:   tcell.ColorSilver,
		TabInactiveFg:   tcell.ColorBlack,
		RowSelectedBg:   tcell.ColorSilver,
		RowSelectedFg:   tcell.ColorBlack,
		RowAlternateBg:  tcell.ColorDefault,

		StatusOK:      tcell.ColorGreen,
		StatusWarning: tcell.ColorYellow,
		StatusError:   tcell.ColorRed,
		StatusInfo:    tcell.ColorAqua,
		StatusMuted:   tcell.ColorGray,

		GaugeLow:        tcell.ColorGreen,
		GaugeMedium:     tcell.ColorYellow,
		GaugeElevated:   tcell.ColorYellow,
		GaugeHigh:       tcell.ColorRed,
		SparklineNormal: tcell.ColorGreen,
		SparklineAlert:  tcell.ColorRed,
		SparklineEmpty:  tcell.ColorGray,
		BarEmpty:        tcell.ColorGray,

		Accent:        tcell.ColorAqua,
		DataPrimary:   tcell.ColorYellow,
		DataSecondary: tcell.ColorDefault,

		ToastInfoBg:    tcell.ColorNavy,
		ToastSuccessBg: tcell.ColorGreen,
		ToastWarnBg:    tcell.ColorOlive,
		ToastErrorBg:   tcell.ColorMaroon,
		ToastFg:        tcell.ColorWhite,
	}
}

// MonochromePalette returns an attributes-only palette with zero color values.
func MonochromePalette() Palette {
	return Palette{
		Name:        "monochrome",
		Description: "Monochrome attributes-only palette",
		IsDark:      true,

		CanvasBg:      tcell.ColorDefault,
		CanvasFg:      tcell.ColorDefault,
		BorderNormal:  tcell.ColorDefault,
		BorderFocused: tcell.ColorDefault,
		TitleNormal:   tcell.ColorDefault,
		TitleFocused:  tcell.ColorDefault,

		HeaderBg:        tcell.ColorDefault,
		HeaderFg:        tcell.ColorDefault,
		HeaderHotkey:    tcell.ColorDefault,
		HeaderSortArrow: tcell.ColorDefault,
		TabInactiveBg:   tcell.ColorDefault,
		TabInactiveFg:   tcell.ColorDefault,
		RowSelectedBg:   tcell.ColorDefault,
		RowSelectedFg:   tcell.ColorDefault,
		RowAlternateBg:  tcell.ColorDefault,

		StatusOK:      tcell.ColorDefault,
		StatusWarning: tcell.ColorDefault,
		StatusError:   tcell.ColorDefault,
		StatusInfo:    tcell.ColorDefault,
		StatusMuted:   tcell.ColorDefault,

		GaugeLow:        tcell.ColorDefault,
		GaugeMedium:     tcell.ColorDefault,
		GaugeElevated:   tcell.ColorDefault,
		GaugeHigh:       tcell.ColorDefault,
		SparklineNormal: tcell.ColorDefault,
		SparklineAlert:  tcell.ColorDefault,
		SparklineEmpty:  tcell.ColorDefault,
		BarEmpty:        tcell.ColorDefault,

		Accent:        tcell.ColorDefault,
		DataPrimary:   tcell.ColorDefault,
		DataSecondary: tcell.ColorDefault,

		ToastInfoBg:    tcell.ColorDefault,
		ToastSuccessBg: tcell.ColorDefault,
		ToastWarnBg:    tcell.ColorDefault,
		ToastErrorBg:   tcell.ColorDefault,
		ToastFg:        tcell.ColorDefault,
	}
}

// ByName returns the built-in Palette for name (case-insensitive).
func ByName(name string) (Palette, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dark":
		return DarkPalette(), true
	case "light":
		return LightPalette(), true
	case "ansi":
		return ANSIPalette(), true
	case "monochrome":
		return MonochromePalette(), true
	default:
		return Palette{}, false
	}
}

// ValidThemeNames returns the accepted values for theme configuration.
func ValidThemeNames() []string {
	return []string{"auto", "dark", "light", "ansi", "monochrome"}
}

// ValidColorModes returns the accepted values for color_mode configuration.
func ValidColorModes() []string {
	return []string{"auto", "truecolor", "256", "16", "none"}
}
