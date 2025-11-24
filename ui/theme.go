package ui

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
	StatusOK:      "green",
	StatusWarning: "yellow",
	StatusError:   "red",
	StatusUnknown: "gray",

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
