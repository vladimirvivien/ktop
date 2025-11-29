package ui

var Icons = struct {
	// Braille bargraph characters (fractional precision using 8-dot braille)
	// Pattern fills from bottom-left, going up then right: ⡀ ⡄ ⡆ ⡇ ⣇ ⣧ ⣷ ⣿
	BargraphFull     string   // ⣿ - all 8 dots (100% of character)
	BargraphEmpty    string   // ⠀ - blank braille (0% of character)
	BargraphPartials []string // Progressive fill: 1/8 through 7/8
	BargraphLBorder  string
	BargraphRBorder  string
	Factory          string
	Battery         string
	Package         string
	Anchor          string
	Rocket          string
	Thermometer     string
	Sun             string
	Knobs           string
	Drum            string
	M               string
	Plane           string
	Controller      string
	Clock           string
	TrafficLight    string
	// Status icons for visual indicators (using strings for multi-byte emojis)
	Healthy   string
	Error     string
	Warning   string
	Pending   string
	Info      string
	Unknown   string
	Completed string
	// Trend indicators for sparklines (skinny arrows)
	TrendUp   string
	TrendDown string
}{
	BargraphFull:  "⣿",
	BargraphEmpty: "⠀",
	// Progressive fill patterns: 1/8, 2/8, 3/8, 4/8, 5/8, 6/8, 7/8
	BargraphPartials: []string{"⡀", "⡄", "⡆", "⡇", "⣇", "⣧", "⣷"},
	BargraphLBorder:  "[",
	BargraphRBorder:  "]",
	Factory:          "🏭",
	Battery:         "🔋",
	Package:         "📦",
	Anchor:          "⚓",
	Rocket:          "🚀",
	Thermometer:     "🌡",
	Sun:             "☀",
	Knobs:           "🎛",
	Drum:            "🥁",
	M:               "Ⓜ",
	Plane:           "🛩",
	Controller:      "🛂",
	Clock:           "⏰",
	TrafficLight:    "🚦",
	// Status icons
	Healthy:   "✅",
	Error:     "❌",
	Warning:   "⚠️",
	Pending:   "⏳",
	Info:      "ℹ️",
	Unknown:   "⛔️",
	Completed: "✅",
	// Trend indicators for sparklines (skinny arrows)
	TrendUp:   "↑",
	TrendDown: "↓",
}
