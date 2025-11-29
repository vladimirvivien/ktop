package ui

import (
	"math"
	"strings"

	"github.com/vladimirvivien/ktop/metrics"
)

// SparkStyle defines the rendering style for sparklines
type SparkStyle int

const (
	// SparkStyleBar renders histogram bars from bottom up
	SparkStyleBar SparkStyle = iota
	// SparkStyleLine renders a connected line graph (future)
	SparkStyleLine
	// SparkStyleArea renders filled area under line (future)
	SparkStyleArea
)

// Sparkline renders time-series data as a braille-based chart
type Sparkline struct {
	// Data points (raw values, will be normalized)
	Data []float64

	// Dimensions
	Width  int // Characters wide (each char = 2 data columns)
	Height int // Text rows tall (each row = 4 dot rows)

	// Appearance
	Colors     ColorKeys  // Color thresholds (reuses existing ColorKeys)
	EmptyColor string     // Color for empty space (default: darkgray)
	Style      SparkStyle // Rendering style

	// Scaling
	Min float64 // Minimum value (auto if 0)
	Max float64 // Maximum value (auto if 0)

	// Optional
	Inverted bool // Flip vertically (high=bad scenarios)
}

// Braille dot bit positions for 2x4 grid
// Unicode braille uses an 8-dot pattern where each dot maps to a bit:
//
//	┌───┬───┐
//	│ 0 │ 3 │  ← bits for dots 1, 4
//	├───┼───┤
//	│ 1 │ 4 │  ← bits for dots 2, 5
//	├───┼───┤
//	│ 2 │ 5 │  ← bits for dots 3, 6
//	├───┼───┤
//	│ 6 │ 7 │  ← bits for dots 7, 8
//	└───┴───┘
var brailleDotBits = [4][2]uint8{
	{0, 3}, // Row 0: dots 1, 4
	{1, 4}, // Row 1: dots 2, 5
	{2, 5}, // Row 2: dots 3, 6
	{6, 7}, // Row 3: dots 7, 8
}

// NewSparkline creates a new sparkline with the given data and dimensions
func NewSparkline(data []float64, width, height int) *Sparkline {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return &Sparkline{
		Data:       data,
		Width:      width,
		Height:     height,
		Colors:     ColorKeys{0: "green", 50: "yellow", 80: "red"},
		EmptyColor: "darkgray",
		Style:      SparkStyleBar,
	}
}

// normalize converts data to 0.0-1.0 range
func (s *Sparkline) normalize() []float64 {
	if len(s.Data) == 0 {
		return nil
	}

	min, max := s.Min, s.Max
	if min == 0 && max == 0 {
		// Auto-detect range
		min, max = math.MaxFloat64, -math.MaxFloat64
		for _, v := range s.Data {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
	}

	normalized := make([]float64, len(s.Data))
	rangeVal := max - min
	if rangeVal == 0 {
		// All values are the same - treat as 100% if non-zero, 0% if zero
		fillValue := 0.0
		if max > 0 {
			fillValue = 1.0
		}
		for i := range normalized {
			normalized[i] = fillValue
		}
		return normalized
	}

	for i, v := range s.Data {
		normalized[i] = (v - min) / rangeVal
		// Clamp to [0, 1]
		if normalized[i] < 0 {
			normalized[i] = 0
		}
		if normalized[i] > 1 {
			normalized[i] = 1
		}
	}
	return normalized
}

// resampleData resamples data to fit the pixel width
func (s *Sparkline) resampleData(data []float64, targetWidth int) []float64 {
	if len(data) == 0 {
		return make([]float64, targetWidth)
	}
	if len(data) == targetWidth {
		return data
	}

	resampled := make([]float64, targetWidth)

	if len(data) < targetWidth {
		// Right-align data: place actual values at the right, empty space on left
		// This gives "sliding right to left" appearance as new data appears on right
		offset := targetWidth - len(data)
		for i := 0; i < len(data); i++ {
			resampled[offset+i] = data[i]
		}
		// Left portion (0 to offset) remains zeros (empty)
	} else {
		// Compress data (average values)
		ratio := float64(len(data)) / float64(targetWidth)
		for i := 0; i < targetWidth; i++ {
			startIdx := int(float64(i) * ratio)
			endIdx := int(float64(i+1) * ratio)
			if endIdx > len(data) {
				endIdx = len(data)
			}
			if startIdx >= endIdx {
				startIdx = endIdx - 1
			}
			if startIdx < 0 {
				startIdx = 0
			}

			// Average the values in this bucket
			sum := 0.0
			count := 0
			for j := startIdx; j < endIdx; j++ {
				sum += data[j]
				count++
			}
			if count > 0 {
				resampled[i] = sum / float64(count)
			}
		}
	}

	return resampled
}

// plotBars fills the pixel grid with bar-style histogram
func (s *Sparkline) plotBars(grid [][]bool, data []float64) {
	pixelHeight := len(grid)
	pixelWidth := len(grid[0])

	for x := 0; x < pixelWidth && x < len(data); x++ {
		// Calculate bar height in pixels
		barHeight := int(math.Round(data[x] * float64(pixelHeight)))
		if barHeight > pixelHeight {
			barHeight = pixelHeight
		}
		// Minimum visibility: non-zero values show at least 1 pixel
		if data[x] > 0 && barHeight == 0 {
			barHeight = 1
		}

		// Fill from bottom up (or top down if inverted)
		if s.Inverted {
			for y := 0; y < barHeight; y++ {
				grid[y][x] = true
			}
		} else {
			for y := pixelHeight - barHeight; y < pixelHeight; y++ {
				grid[y][x] = true
			}
		}
	}
}

// gridToBraille converts a 2x4 section of the grid to a braille character
func gridToBraille(grid [][]bool, charRow, charCol int) rune {
	offset := uint8(0)

	for row := 0; row < 4; row++ {
		for col := 0; col < 2; col++ {
			pixelY := charRow*4 + row
			pixelX := charCol*2 + col

			if pixelY < len(grid) && pixelX < len(grid[0]) && grid[pixelY][pixelX] {
				offset |= 1 << brailleDotBits[row][col]
			}
		}
	}

	return rune(0x2800 + int(offset)) // Unicode braille block starts at U+2800
}

// colorForPosition determines the color for a character position based on data value
func (s *Sparkline) colorForPosition(data []float64, charCol int, colorKeys []int) string {
	if len(data) == 0 || len(s.Colors) == 0 {
		return "white"
	}

	// Get the data value at approximately this character position
	dataIdx := charCol * 2
	if dataIdx >= len(data) {
		dataIdx = len(data) - 1
	}
	if dataIdx < 0 {
		return "white"
	}

	// Convert normalized value (0-1) to percentage (0-100)
	percent := int(data[dataIdx] * 100)

	// Find the appropriate color
	color := "white"
	for _, k := range colorKeys {
		if percent >= k {
			color = s.Colors[k]
		}
	}

	return color
}

// Render returns the sparkline as a colored string
func (s *Sparkline) Render() string {
	if s.Width == 0 || s.Height == 0 {
		return ""
	}

	// Normalize and resample data
	data := s.normalize()
	pixelWidth := s.Width * 2
	pixelHeight := s.Height * 4

	data = s.resampleData(data, pixelWidth)

	// Build pixel grid
	grid := make([][]bool, pixelHeight)
	for i := range grid {
		grid[i] = make([]bool, pixelWidth)
	}

	// Plot based on style
	switch s.Style {
	case SparkStyleBar:
		s.plotBars(grid, data)
	case SparkStyleLine:
		s.plotBars(grid, data) // Fallback to bar for now
	case SparkStyleArea:
		s.plotBars(grid, data) // Fallback to bar for now
	}

	// Get sorted color keys
	colorKeys := s.Colors.Keys()

	// Convert to string with colors
	var result strings.Builder

	for charRow := 0; charRow < s.Height; charRow++ {
		for charCol := 0; charCol < s.Width; charCol++ {
			char := gridToBraille(grid, charRow, charCol)

			// Check if this character has any dots
			if char == 0x2800 {
				// Empty braille character
				result.WriteString("[")
				result.WriteString(s.EmptyColor)
				result.WriteString("]")
				result.WriteRune(char)
			} else {
				// Determine color based on data value at this position
				color := s.colorForPosition(data, charCol, colorKeys)
				result.WriteString("[")
				result.WriteString(color)
				result.WriteString("]")
				result.WriteRune(char)
			}
		}
		if charRow < s.Height-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// TrendIndicator returns a colored trend arrow (↑/↓) or empty string for stable.
// percentage is used to determine color: red if >= 80%, white otherwise.
// Returns empty string when trend is stable or there's insufficient data.
func (s *Sparkline) TrendIndicator(percentage float64) string {
	if len(s.Data) < 2 {
		return ""
	}

	// Compare last 20% of data to first 20%
	n := len(s.Data)
	window := n / 5
	if window < 1 {
		window = 1
	}

	var startAvg, endAvg float64
	for i := 0; i < window && i < n; i++ {
		startAvg += s.Data[i]
	}
	for i := 0; i < window && (n-1-i) >= 0; i++ {
		endAvg += s.Data[n-1-i]
	}
	startAvg /= float64(window)
	endAvg /= float64(window)

	// Determine trend direction
	var trendUp, trendDown bool

	if startAvg == 0 {
		if endAvg > Theme.TrendThreshold {
			trendUp = true
		}
	} else {
		diff := (endAvg - startAvg) / startAvg
		if diff > Theme.TrendThreshold {
			trendUp = true
		} else if diff < -Theme.TrendThreshold {
			trendDown = true
		}
	}

	// Return empty for stable
	if !trendUp && !trendDown {
		return ""
	}

	// Determine color: red if percentage >= 80%, white otherwise
	color := Theme.TrendNormalColor
	if percentage >= Theme.TrendHighThreshold*100 {
		color = Theme.TrendHighColor
	}

	if trendUp {
		return "[" + color + "]" + Icons.TrendUp + "[-]"
	}
	return "[" + color + "]" + Icons.TrendDown + "[-]"
}

// LastValue returns the most recent data point
func (s *Sparkline) LastValue() float64 {
	if len(s.Data) == 0 {
		return 0
	}
	return s.Data[len(s.Data)-1]
}

// AddPoint appends a new data point, trimming old if exceeds maxPoints
func (s *Sparkline) AddPoint(value float64, maxPoints int) {
	s.Data = append(s.Data, value)
	if maxPoints > 0 && len(s.Data) > maxPoints {
		s.Data = s.Data[len(s.Data)-maxPoints:]
	}
}

// NewSparklineFromHistory creates a sparkline from a ResourceHistory
func NewSparklineFromHistory(history *metrics.ResourceHistory, width, height int) *Sparkline {
	if history == nil || len(history.DataPoints) == 0 {
		return NewSparkline(nil, width, height)
	}

	data := make([]float64, len(history.DataPoints))
	for i, dp := range history.DataPoints {
		data[i] = dp.Value
	}

	spark := NewSparkline(data, width, height)

	// Use min/max from history if available for consistent scaling
	if history.MaxValue > history.MinValue {
		spark.Min = history.MinValue
		spark.Max = history.MaxValue
	}

	return spark
}

// HistoryToValues extracts float64 values from ResourceHistory
func HistoryToValues(history *metrics.ResourceHistory) []float64 {
	if history == nil || len(history.DataPoints) == 0 {
		return nil
	}

	data := make([]float64, len(history.DataPoints))
	for i, dp := range history.DataPoints {
		data[i] = dp.Value
	}
	return data
}

// SparkGraph renders a sparkline graph that replaces BarGraph.
// It shows historical data if available, otherwise shows empty graph.
// Data flows right-to-left: oldest data on left, newest on right.
// Width is in characters (like BarGraph scale).
// The ratio parameter is the current percentage (0-1) and is used to derive the scale
// so that bar heights reflect actual percentages (not relative to history max).
// Returns a colored string (caller should wrap in [ ] brackets).
func SparkGraph(width int, history *metrics.ResourceHistory, ratio Ratio, colors ColorKeys) string {
	if width == 0 {
		return ""
	}

	// If we have history data with multiple points, render sparkline
	if history != nil && len(history.DataPoints) >= 2 {
		// Calculate the scale (total capacity) from current ratio and last value
		// If ratio = currentValue/total, then total = currentValue/ratio
		var scale float64
		if ratio > 0 && len(history.DataPoints) > 0 {
			lastValue := history.DataPoints[len(history.DataPoints)-1].Value
			scale = lastValue / float64(ratio)
		} else if history.MaxValue > 0 {
			// Fallback to history max if ratio is 0
			scale = history.MaxValue
		} else {
			scale = 100 // Default scale
		}
		return sparkGraphFromHistory(width, history, colors, scale)
	}

	// No history: show empty graph (no fallback bargraph)
	return sparkGraphEmpty(width)
}

// Stacked bar braille characters - each represents a percentage range
// These use both left and right columns filled to the same height
var stackedBarChars = []rune{
	'⣀', // 0: bottom row, greyed out (0%)
	'⣀', // 1: bottom row only (1-25%)
	'⣤', // 2: bottom 2 rows (26-50%)
	'⣶', // 3: bottom 3 rows (51-75%)
	'⣿', // 4: all 4 rows (76-100%)
}

// sparkGraphFromHistory renders a sparkline from historical data
// Each braille character represents one data point, height shows percentage
// Scale is the total capacity (e.g., allocatable CPU) - values are divided by this
func sparkGraphFromHistory(width int, history *metrics.ResourceHistory, colors ColorKeys, scale float64) string {
	if history == nil || len(history.DataPoints) == 0 {
		return sparkGraphEmpty(width)
	}

	// Ensure valid scale
	if scale <= 0 {
		scale = history.MaxValue
		if scale <= 0 {
			scale = 1
		}
	}

	// Extract and normalize values to 0-1 range (percentage of total capacity)
	data := make([]float64, len(history.DataPoints))
	for i, dp := range history.DataPoints {
		data[i] = dp.Value / scale
		if data[i] > 1 {
			data[i] = 1
		}
		if data[i] < 0 {
			data[i] = 0
		}
	}

	// Resample data to fit width (compress if too many, right-align if too few)
	resampled := resampleForWidth(data, width)

	// Get sorted color keys for threshold lookup
	colorKeys := colors.Keys()

	// Build the sparkline string
	var graph strings.Builder
	for _, val := range resampled {
		// Determine bar height (0-4 levels)
		var level int
		if val <= 0 {
			level = 0 // empty
		} else if val <= 0.25 {
			level = 1 // ⣀
		} else if val <= 0.50 {
			level = 2 // ⣤
		} else if val <= 0.75 {
			level = 3 // ⣶
		} else {
			level = 4 // ⣿
		}

		// Determine color based on percentage value (not position)
		percent := int(val * 100)
		color := "white"
		for _, k := range colorKeys {
			if percent >= k {
				color = colors[k]
			}
		}

		// Zero values use gray color (matches NotReady status)
		if level == 0 {
			graph.WriteString("[gray]")
		} else {
			graph.WriteString("[")
			graph.WriteString(color)
			graph.WriteString("]")
		}
		graph.WriteRune(stackedBarChars[level])
	}

	return graph.String()
}

// resampleForWidth resamples data to exactly fit the target width
// For a smooth sliding effect, we take the last N values when data exceeds width
func resampleForWidth(data []float64, width int) []float64 {
	if len(data) == 0 {
		return make([]float64, width)
	}
	if len(data) == width {
		return data
	}

	result := make([]float64, width)

	if len(data) < width {
		// Right-align: place data at right, zeros on left
		offset := width - len(data)
		copy(result[offset:], data)
	} else {
		// Take only the last 'width' values for smooth sliding effect
		// This ensures new data appears on right, old data slides off left
		startIdx := len(data) - width
		copy(result, data[startIdx:])
	}

	return result
}

// sparkGraphFromRatio renders a single-value bar graph using braille (fallback when no history)
func sparkGraphFromRatio(width int, ratio Ratio, colors ColorKeys) string {
	if width == 0 {
		return ""
	}

	var graph strings.Builder

	// Nothing to graph - show empty bar
	if ratio == 0 {
		graph.WriteString("[darkgray]")
		for j := 0; j < width; j++ {
			graph.WriteString(Icons.BargraphEmpty)
		}
		return graph.String()
	}

	// Clamp ratio to [0, 1]
	if ratio > 1 {
		ratio = 1
	}

	// Calculate total "dots" available (8 dots per character * width characters)
	totalDots := width * 8
	filledDots := int(math.Round(float64(ratio) * float64(totalDots)))

	// Build color keys for threshold lookup
	colorKeys := colors.Keys()

	// Draw each character position
	for i := 0; i < width; i++ {
		// Calculate how many dots should be filled in this character (0-8)
		startDot := i * 8
		endDot := startDot + 8
		dotsInThisChar := 0

		if filledDots >= endDot {
			dotsInThisChar = 8
		} else if filledDots > startDot {
			dotsInThisChar = filledDots - startDot
		}

		// Calculate what percentage this position represents (for color selection)
		positionPercent := int((float64(i) / float64(width)) * 100)

		// Find the appropriate color for this position
		segmentColor := "white"
		if colors != nil && len(colors) > 0 {
			for _, k := range colorKeys {
				if positionPercent >= k {
					segmentColor = colors[k]
				}
			}
		}

		// Select the appropriate braille character
		var brailleChar string
		if dotsInThisChar == 0 {
			graph.WriteString("[darkgray]")
			graph.WriteString(Icons.BargraphEmpty)
			continue
		} else if dotsInThisChar == 8 {
			brailleChar = Icons.BargraphFull
		} else {
			brailleChar = Icons.BargraphPartials[dotsInThisChar-1]
		}

		graph.WriteString("[")
		graph.WriteString(segmentColor)
		graph.WriteString("]")
		graph.WriteString(brailleChar)
	}

	return graph.String()
}

// sparkGraphEmpty renders an empty sparkline graph
func sparkGraphEmpty(width int) string {
	var graph strings.Builder
	graph.WriteString("[darkgray]")
	for j := 0; j < width; j++ {
		graph.WriteString(Icons.BargraphEmpty)
	}
	return graph.String()
}

// SparkGraphTrend returns a trend indicator for the given history
// Uses 0 as percentage since historical data doesn't have current percentage context
func SparkGraphTrend(history *metrics.ResourceHistory) string {
	if history == nil || len(history.DataPoints) < 2 {
		return ""
	}

	data := make([]float64, len(history.DataPoints))
	for i, dp := range history.DataPoints {
		data[i] = dp.Value
	}

	spark := &Sparkline{Data: data}
	// Use last value as percentage for color determination
	lastPercentage := 0.0
	if len(data) > 0 {
		lastPercentage = data[len(data)-1] * 100
	}
	return spark.TrendIndicator(lastPercentage)
}
