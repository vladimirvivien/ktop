package ui

import "github.com/gdamore/tcell/v2"

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
}{
	// Header colors
	HeaderBackground:    "darkgreen",
	HeaderForeground:    "white",
	HeaderShortcutKey:   "orange", // Keyboard shortcut key color
	HeaderSortIndicator: "white",  // Sort arrow color

	// Selection colors
	SelectionBackground: "yellow",
	SelectionForeground: "blue",

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
}

// FormatTag returns a tview color/style tag string
// Usage: FormatTag(Theme.HeaderShortcutKey, "", "b") returns "[orange::b]"
func FormatTag(foreground, background, attributes string) string {
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
func GetResourcePercentageColor(percentage float64) string {
	if percentage < Theme.ResourceLowThreshold {
		return Theme.ResourceLowColor
	} else if percentage < Theme.ResourceMediumThreshold {
		return Theme.ResourceMediumColor
	}
	return Theme.ResourceHighColor
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

// GetTcellColor converts a theme color string to tcell.Color
func GetTcellColor(color string) tcell.Color {
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
	case "gray":
		return tcell.ColorGray
	case "orange":
		return tcell.ColorOrange
	case "blue":
		return tcell.ColorBlue
	case "white":
		return tcell.ColorWhite
	default:
		return tcell.ColorYellow
	}
}

// GetRowColorForStatus returns the tcell.Color for an entire row based on resource status
// Healthy resources get default yellow text, unhealthy resources get their status color
func GetRowColorForStatus(status string, resourceType string) tcell.Color {
	statusColor := GetStatusColor(status, resourceType)

	// Healthy resources use default yellow row color
	if statusColor == Theme.StatusOK {
		return tcell.ColorYellow
	}

	// Unhealthy resources: entire row uses the status color
	return GetTcellColor(statusColor)
}
