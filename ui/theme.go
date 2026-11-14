package ui

import (
	"sync/atomic"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/ui/theme"
)

var activeTheme atomic.Pointer[theme.ResolvedTheme]

func init() {
	ApplyTheme(theme.ResolvedTheme{
		Palette: theme.DarkPalette(),
		Mode:    theme.ColorModeTrueColor,
	})
}

// ActiveTheme returns the currently active resolved theme and color mode.
func ActiveTheme() theme.ResolvedTheme {
	if rt := activeTheme.Load(); rt != nil {
		return *rt
	}
	return theme.ResolvedTheme{
		Palette: theme.DarkPalette(),
		Mode:    theme.ColorModeTrueColor,
	}
}

// ActivePalette returns the active semantic Palette.
func ActivePalette() theme.Palette {
	return ActiveTheme().Palette
}

// IsMonochrome reports whether UI rendering must use attributes without color codes.
func IsMonochrome() bool {
	return ActiveTheme().IsMonochrome()
}

// ResolveAndApplyTheme resolves theme/color-mode settings against the environment and applies them.
func ResolveAndApplyTheme(themeName, colorMode string) (theme.ResolvedTheme, error) {
	rt, err := theme.Resolve(theme.ResolveOptions{
		Theme:     themeName,
		ColorMode: colorMode,
	})
	if err != nil {
		return theme.ResolvedTheme{}, err
	}
	ApplyTheme(rt)
	return rt, nil
}

// ApplyTheme activates rt across ui.Theme tokens and tview.Styles defaults.
func ApplyTheme(rt theme.ResolvedTheme) {
	resolved := rt
	activeTheme.Store(&resolved)

	p := resolved.Palette
	mode := resolved.Mode
	tag := func(c tcell.Color) string {
		return theme.ColorTag(c, mode)
	}

	Theme.HeaderBackground = tag(p.HeaderBg)
	Theme.HeaderForeground = tag(p.HeaderFg)
	Theme.HeaderShortcutKey = tag(p.HeaderHotkey)
	Theme.HeaderSortIndicator = tag(p.HeaderSortArrow)

	Theme.SelectionBackground = tag(p.RowSelectedBg)
	Theme.SelectionForeground = tag(p.RowSelectedFg)

	Theme.StatusOK = tag(p.StatusOK)
	Theme.StatusWarning = tag(p.StatusWarning)
	Theme.StatusError = tag(p.StatusError)
	Theme.StatusUnknown = tag(p.StatusMuted)
	Theme.StatusInfo = tag(p.StatusInfo)

	Theme.DataPrimary = tag(p.DataPrimary)
	Theme.DataSecondary = tag(p.DataSecondary)
	Theme.DataHighlight = tag(p.Accent)

	Theme.GraphLow = tag(p.GaugeLow)
	Theme.GraphMedium = tag(p.GaugeMedium)
	Theme.GraphHigh = tag(p.GaugeHigh)

	Theme.BorderColor = tag(p.BorderNormal)
	Theme.SeparatorColor = tag(p.StatusMuted)

	Theme.ResourceLowColor = tag(p.GaugeLow)
	Theme.ResourceMediumColor = tag(p.GaugeMedium)
	Theme.ResourceHighColor = tag(p.GaugeHigh)

	Theme.RestartsLowColor = tag(p.GaugeLow)
	Theme.RestartsMediumColor = tag(p.GaugeMedium)
	Theme.RestartsHighColor = tag(p.GaugeHigh)

	Theme.SparklineNormal = tag(p.SparklineNormal)
	Theme.SparklineMedium = tag(p.GaugeMedium)
	Theme.SparklineHigh = tag(p.SparklineAlert)
	Theme.SparklineEmpty = tag(p.SparklineEmpty)

	Theme.TrendNormalColor = tag(p.SparklineNormal)
	Theme.TrendHighColor = tag(p.SparklineAlert)

	Theme.FocusBorderColor = tag(p.BorderFocused)
	Theme.UnfocusBorderColor = tag(p.BorderNormal)

	if resolved.IsMonochrome() {
		tview.Styles = tview.Theme{
			PrimitiveBackgroundColor:    tcell.ColorDefault,
			ContrastBackgroundColor:     tcell.ColorDefault,
			MoreContrastBackgroundColor: tcell.ColorDefault,
			BorderColor:                 tcell.ColorDefault,
			TitleColor:                  tcell.ColorDefault,
			GraphicsColor:               tcell.ColorDefault,
			PrimaryTextColor:            tcell.ColorDefault,
			SecondaryTextColor:          tcell.ColorDefault,
			TertiaryTextColor:           tcell.ColorDefault,
			InverseTextColor:            tcell.ColorDefault,
			ContrastSecondaryTextColor:  tcell.ColorDefault,
		}
		return
	}

	tview.Styles = tview.Theme{
		PrimitiveBackgroundColor:    p.CanvasBg,
		ContrastBackgroundColor:     p.HeaderBg,
		MoreContrastBackgroundColor: p.RowSelectedBg,
		BorderColor:                 p.BorderNormal,
		TitleColor:                  p.TitleNormal,
		GraphicsColor:               p.BorderNormal,
		PrimaryTextColor:            p.CanvasFg,
		SecondaryTextColor:          p.DataPrimary,
		TertiaryTextColor:           p.StatusOK,
		InverseTextColor:            p.HeaderFg,
		ContrastSecondaryTextColor:  p.StatusMuted,
	}
}

// TagFg returns a tview foreground color tag string for c under the active theme mode.
func TagFg(c tcell.Color) string {
	return theme.ColorTag(c, ActiveTheme().Mode)
}

// ErrorMarkup formats error text for inline tview markup using semantic error styling.
func ErrorMarkup(text string) string {
	rt := ActiveTheme()
	return theme.WrapError(text, rt.Palette, rt.Mode)
}

// StyleHeaderCell applies theme styling to a table column header cell.
func StyleHeaderCell(cell *tview.TableCell) *tview.TableCell {
	if cell == nil {
		return nil
	}
	rt := ActiveTheme()
	if rt.IsMonochrome() {
		return cell.
			SetTextColor(tcell.ColorDefault).
			SetBackgroundColor(tcell.ColorDefault).
			SetAttributes(tcell.AttrReverse | tcell.AttrBold)
	}
	return cell.
		SetTextColor(rt.Palette.HeaderFg).
		SetBackgroundColor(rt.Palette.HeaderBg).
		SetAttributes(tcell.AttrBold)
}

// StyleStatusCell applies semantic status color or monochrome text attributes to a status cell.
func StyleStatusCell(cell *tview.TableCell, status, resourceType string) *tview.TableCell {
	if cell == nil {
		return nil
	}
	rt := ActiveTheme()
	statusColor := GetStatusColor(status, resourceType)
	if rt.IsMonochrome() {
		cell.SetTextColor(tcell.ColorDefault)
		switch statusColor {
		case Theme.StatusError:
			if len(cell.Text) < 2 || cell.Text[:2] != "! " {
				cell.SetText("! " + cell.Text)
			}
			cell.SetAttributes(tcell.AttrUnderline | tcell.AttrBold)
		case Theme.StatusWarning:
			cell.SetAttributes(tcell.AttrBold)
		case Theme.StatusUnknown:
			cell.SetAttributes(tcell.AttrDim)
		default:
			cell.SetAttributes(tcell.AttrNone)
		}
		return cell
	}
	return cell.SetTextColor(GetTcellColor(statusColor))
}

// SelectedRowStyle returns the tcell.Style for highlighted table rows.
func SelectedRowStyle() tcell.Style {
	rt := ActiveTheme()
	if rt.IsMonochrome() {
		return tcell.StyleDefault.
			Foreground(tcell.ColorDefault).
			Background(tcell.ColorDefault).
			Reverse(true)
	}
	return tcell.StyleDefault.
		Foreground(rt.Palette.RowSelectedFg).
		Background(rt.Palette.RowSelectedBg)
}

// Theme contains all color and style constants for the ktop UI
// This allows for centralized theming and easy color customization
var Theme = struct {
	// Header colors (used for table column headers)
	HeaderBackground    string
	HeaderForeground    string
	HeaderShortcutKey   string // Color for keyboard shortcut highlighting
	HeaderSortIndicator string // Color for sort arrows (▲/▼)

	// Selection colors
	SelectionBackground string
	SelectionForeground string

	// Status colors
	StatusOK      string
	StatusWarning string
	StatusError   string
	StatusUnknown string
	StatusInfo    string // For informational states

	// Data display colors
	DataPrimary   string // Primary data text
	DataSecondary string // Secondary/dimmed data text
	DataHighlight string // Emphasized data

	// Graph/meter colors
	GraphLow    string // For low usage (0-50%)
	GraphMedium string // For medium usage (50-90%)
	GraphHigh   string // For high usage (90-100%)

	// Border and separator colors
	BorderColor    string
	SeparatorColor string

	// Resource usage thresholds and colors
	ResourceLowThreshold    float64 // 0-70%
	ResourceMediumThreshold float64 // 70-90%
	ResourceHighThreshold   float64 // 90-100%
	ResourceLowColor        string
	ResourceMediumColor     string
	ResourceHighColor       string

	// Restart count thresholds and colors
	RestartsLowThreshold    int // 0-2
	RestartsMediumThreshold int // 3-9
	RestartsHighThreshold   int // 10+
	RestartsLowColor        string
	RestartsMediumColor     string
	RestartsHighColor       string

	// Sparkline colors (single-line uses 2 colors, multi-line uses 3)
	SparklineNormal    string  // Normal/low usage (olivedrab)
	SparklineMedium    string  // Medium usage (multi-line only)
	SparklineHigh      string  // High usage (red)
	SparklineEmpty     string  // Zero/empty values (gray)
	SparklineThreshold float64 // Threshold for high color (0.70 = 70%)

	// Trend arrow colors (↑/↓)
	TrendNormalColor   string  // Normal color for trend arrows (olivedrab)
	TrendHighColor     string  // Color when percentage >= 80% (red)
	TrendThreshold     float64 // Percentage change threshold (0.05 = 5%)
	TrendHighThreshold float64 // Percentage threshold for red arrow (0.80 = 80%)

	// Panel focus colors
	FocusBorderColor   string // Border color when panel is focused (yellow)
	UnfocusBorderColor string // Border color when panel is unfocused (white)
}{
	// Header colors
	HeaderBackground:    "darkcyan",
	HeaderForeground:    "white",
	HeaderShortcutKey:   "orange", // Keyboard shortcut key color
	HeaderSortIndicator: "white",  // Sort arrow color

	// Selection colors
	SelectionBackground: "lightgray",
	SelectionForeground: "black",

	// Status colors
	StatusOK:      "olivedrab",
	StatusWarning: "yellow",
	StatusError:   "red",
	StatusUnknown: "gray",
	StatusInfo:    "cyan",

	// Data display colors
	DataPrimary:   "yellow",
	DataSecondary: "white",
	DataHighlight: "cyan",

	// Graph/meter colors (used in bar graphs)
	GraphLow:    "green",
	GraphMedium: "yellow",
	GraphHigh:   "red",

	// Border and separator colors
	BorderColor:    "white",
	SeparatorColor: "gray",

	// Resource usage thresholds and colors
	ResourceLowThreshold:    70.0,
	ResourceMediumThreshold: 90.0,
	ResourceHighThreshold:   100.0,
	ResourceLowColor:        "green",
	ResourceMediumColor:     "yellow",
	ResourceHighColor:       "red",

	// Restart count thresholds and colors
	RestartsLowThreshold:    2,
	RestartsMediumThreshold: 9,
	RestartsHighThreshold:   10,
	RestartsLowColor:        "green",
	RestartsMediumColor:     "yellow",
	RestartsHighColor:       "red",

	// Sparkline colors
	SparklineNormal:    "olivedrab", // Normal/low usage (< 70%)
	SparklineMedium:    "yellow",    // Medium usage (multi-line only)
	SparklineHigh:      "red",       // High usage (>= 70%)
	SparklineEmpty:     "gray",      // Zero/empty values
	SparklineThreshold: 0.70,        // 70% threshold for high color

	// Trend arrow colors
	TrendNormalColor:   "olivedrab", // Normal trend arrows (matches table text)
	TrendHighColor:     "red",       // Red when percentage >= 80%
	TrendThreshold:     0.05,        // 5% change triggers arrow
	TrendHighThreshold: 0.80,        // 80% threshold for red arrow

	// Panel focus colors
	FocusBorderColor:   "dodgerblue", // Border color when panel is focused
	UnfocusBorderColor: "lightgray",  // Border color when panel is unfocused
}

// FormatTag returns a tview color/style tag string.
func FormatTag(foreground, background, attributes string) string {
	rt := ActiveTheme()
	if rt.IsMonochrome() {
		return theme.FormatTag(tcell.ColorDefault, tcell.ColorDefault, attributes, theme.ColorModeNone)
	}
	if background == "" && attributes == "" {
		return "[" + foreground + "]"
	}
	return "[" + foreground + ":" + background + ":" + attributes + "]"
}

// ResetTag returns a tview reset tag that resets to default colors
// Optionally specify which fields to reset: foreground, background, attributes
func ResetTag(resetForeground, resetBackground, resetAttributes bool) string {
	fg := ""
	if resetForeground {
		fg = "-"
	}
	bg := ""
	if resetBackground {
		bg = "-"
	}
	attr := ""
	if resetAttributes {
		attr = "-"
	}
	return "[" + fg + ":" + bg + ":" + attr + "]"
}

// GetStatusColor returns the appropriate color for a given status
// resourceType can be "node" or "pod"
func GetStatusColor(status string, resourceType string) string {
	if resourceType == "node" {
		switch status {
		case "Ready":
			return Theme.StatusOK
		case "NotReady":
			return Theme.StatusError
		case "SchedulingDisabled":
			return Theme.StatusWarning
		case "Unknown":
			return Theme.StatusUnknown
		default:
			return Theme.StatusUnknown
		}
	}

	if resourceType == "pod" {
		switch status {
		case "Running":
			return Theme.StatusOK
		case "Completed":
			return Theme.StatusInfo
		case "Pending", "Terminating":
			return Theme.StatusWarning
		case "ContainerCreating":
			return Theme.StatusInfo
		case "CrashLoopBackOff", "Error", "Failed", "ImagePullBackOff":
			return Theme.StatusError
		default:
			return Theme.StatusUnknown
		}
	}

	return Theme.StatusUnknown
}

// GetStatusIcon returns the appropriate icon for a given status
// resourceType can be "node" or "pod"
func GetStatusIcon(status string, resourceType string) string {
	if resourceType == "node" {
		switch status {
		case "Ready":
			return Icons.Healthy
		case "NotReady":
			return Icons.Error
		case "SchedulingDisabled":
			return Icons.Warning
		case "Unknown":
			return Icons.Unknown
		default:
			return Icons.Unknown
		}
	}

	if resourceType == "pod" {
		switch status {
		case "Running":
			return Icons.Healthy
		case "Completed":
			return Icons.Completed
		case "Pending":
			return Icons.Pending
		case "Terminating":
			return Icons.Warning
		case "ContainerCreating":
			return Icons.Info
		case "CrashLoopBackOff", "Error", "Failed", "ImagePullBackOff":
			return Icons.Error
		default:
			return Icons.Unknown
		}
	}

	return Icons.Unknown
}

// GetRestartsColor returns color based on restart count thresholds
func GetRestartsColor(restarts int) string {
	if restarts <= Theme.RestartsLowThreshold {
		return Theme.RestartsLowColor
	} else if restarts <= Theme.RestartsMediumThreshold {
		return Theme.RestartsMediumColor
	}
	return Theme.RestartsHighColor
}

// GetResourcePercentageColor returns color based on resource usage percentage
// Uses same 2-color scheme as sparklines for consistency (olivedrab < 70%, red >= 70%)
func GetResourcePercentageColor(percentage float64) string {
	if percentage >= Theme.SparklineThreshold*100 {
		return Theme.SparklineHigh
	}
	return Theme.SparklineNormal
}

// GetReadyColor returns color based on ready/total ratio
// As containers drift from ready state, color shifts toward red
func GetReadyColor(ready, total int) string {
	if total == 0 {
		return Theme.StatusUnknown
	}
	ratio := float64(ready) / float64(total)
	if ratio >= 1.0 {
		return Theme.StatusOK // All ready - green
	} else if ratio >= 0.5 {
		return Theme.StatusWarning // Some ready - yellow
	}
	return Theme.StatusError // Few/none ready - red
}

// GetTcellColor converts a theme color string to tcell.Color.
func GetTcellColor(color string) tcell.Color {
	if color == "" || color == "-" || color == "default" {
		return tcell.ColorDefault
	}
	if IsMonochrome() {
		return tcell.ColorDefault
	}
	switch color {
	case "green":
		return tcell.ColorGreen
	case "olive":
		return tcell.ColorOlive
	case "olivedrab":
		return tcell.ColorOliveDrab
	case "red":
		return tcell.ColorRed
	case "yellow":
		return tcell.ColorYellow
	case "cyan":
		return tcell.ColorDarkCyan
	case "darkcyan":
		return tcell.ColorDarkCyan
	case "gray":
		return tcell.ColorGray
	case "lightgray":
		return tcell.ColorLightGray
	case "orange":
		return tcell.ColorOrange
	case "blue":
		return tcell.ColorBlue
	case "dodgerblue":
		return tcell.ColorDodgerBlue
	case "black":
		return tcell.ColorBlack
	case "white":
		return tcell.ColorWhite
	default:
		if c := tcell.GetColor(color); c != tcell.ColorDefault {
			return c
		}
		return tcell.ColorYellow
	}
}

// GetRowColorForStatus returns the tcell.Color for an entire row based on resource status.
func GetRowColorForStatus(status string, resourceType string) tcell.Color {
	if IsMonochrome() {
		return tcell.ColorDefault
	}
	statusColor := GetStatusColor(status, resourceType)
	if statusColor == Theme.StatusOK {
		return ActivePalette().DataPrimary
	}
	return GetTcellColor(statusColor)
}
