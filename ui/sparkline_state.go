package ui

import (
	"strings"
)

// SparklineStyle defines the character set used for rendering
type SparklineStyle int

const (
	// SparklineStyleBraille uses braille characters (4 levels)
	SparklineStyleBraille SparklineStyle = iota
	// SparklineStyleBlock uses block characters (8 levels)
	SparklineStyleBlock
)

// Braille stacked bar characters - 4 height levels + zero
var brailleBarChars = []rune{
	'⣀', // 0: baseline (0%, rendered in gray)
	'⣀', // 1: 1-25%
	'⣤', // 2: 26-50%
	'⣶', // 3: 51-75%
	'⣿', // 4: 76-100%
}

// Block bar characters - 8 height levels + zero
// These provide finer granularity than braille
var blockBarChars = []rune{
	'▁', // 0: baseline (0%, rendered in gray)
	'▁', // 1: 1-12%
	'▂', // 2: 13-25%
	'▃', // 3: 26-37%
	'▄', // 4: 38-50%
	'▅', // 5: 51-62%
	'▆', // 6: 63-75%
	'▇', // 7: 76-87%
	'█', // 8: 88-100%
}

// SparklineState maintains a sliding window buffer for smooth sparkline animation.
// Instead of re-querying history each render, this maintains its own state
// and new values are pushed in, causing old values to slide left.
type SparklineState struct {
	values []float64      // Normalized values (0-1), index 0 = oldest, last = newest
	width  int            // Number of characters wide
	height int            // Number of rows tall (1 = single line)
	colors ColorKeys      // Color thresholds
	style  SparklineStyle // Braille or Block
}

// NewSparklineState creates a new stateful sparkline with the given width.
// All values start at 0 (gray baseline). Uses block characters by default
// for better resolution at low values.
func NewSparklineState(width int, colors ColorKeys) *SparklineState {
	return NewSparklineStateWithHeight(width, 1, colors)
}

// NewSparklineStateWithHeight creates a multi-line stateful sparkline.
// Height > 1 enables multi-row rendering with stacked blocks.
func NewSparklineStateWithHeight(width, height int, colors ColorKeys) *SparklineState {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return &SparklineState{
		values: make([]float64, width),
		width:  width,
		height: height,
		colors: colors,
		style:  SparklineStyleBlock,
	}
}

// SetStyle changes the rendering style (braille or block characters)
func (s *SparklineState) SetStyle(style SparklineStyle) {
	s.style = style
}

// Height returns the current height of the sparkline
func (s *SparklineState) Height() int {
	return s.height
}

// Push adds a new value (0.0 to 1.0) to the right, shifting all values left.
// The oldest value (leftmost) is discarded.
func (s *SparklineState) Push(value float64) {
	// Clamp value to 0-1
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}

	// Shift all values left by one position
	copy(s.values, s.values[1:])

	// Add new value at the end (rightmost)
	s.values[s.width-1] = value
}

// Render returns the sparkline as a colored string.
// For single-line (height=1): each character represents one value with variable height.
// For multi-line (height>1): returns multiple lines separated by \n, bars stack vertically.
func (s *SparklineState) Render() string {
	if s.height == 1 {
		return s.renderSingleLine()
	}
	return s.renderMultiLine()
}

// RenderBottomAligned returns the sparkline padded to totalHeight with empty lines at top.
// This makes the sparkline appear bottom-aligned within a container of totalHeight rows.
// If totalHeight <= s.height, behaves the same as Render().
func (s *SparklineState) RenderBottomAligned(totalHeight int) string {
	content := s.Render()
	if totalHeight <= s.height {
		return content
	}

	// Add empty lines at the top to push content to bottom
	padding := totalHeight - s.height
	var result strings.Builder
	for i := 0; i < padding; i++ {
		result.WriteString("\n")
	}
	result.WriteString(content)
	return result.String()
}

// renderSingleLine renders a single-line sparkline with partial block characters
// Uses simplified 2-color scheme from Theme: normal (< threshold) and high (>= threshold)
func (s *SparklineState) renderSingleLine() string {
	var graph strings.Builder

	for _, val := range s.values {
		var level int
		var char rune

		if s.style == SparklineStyleBlock {
			// 9 levels (0-8) for block characters
			level = s.blockLevel(val)
			char = blockBarChars[level]
		} else {
			// 5 levels (0-4) for braille characters
			level = s.brailleLevel(val)
			char = brailleBarChars[level]
		}

		// Simple 2-color scheme for single-line using Theme constants
		var color string
		if val >= Theme.SparklineThreshold {
			color = Theme.SparklineHigh
		} else {
			color = Theme.SparklineNormal
		}

		// Zero values use empty color from Theme
		if level == 0 {
			graph.WriteString("[")
			graph.WriteString(Theme.SparklineEmpty)
			graph.WriteString("]")
		} else {
			graph.WriteString("[")
			graph.WriteString(color)
			graph.WriteString("]")
		}
		graph.WriteRune(char)
	}

	return graph.String()
}

// renderMultiLine renders a multi-line sparkline with improved resolution.
// Uses partial block characters (▁▂▃▄▅▆▇█) to show finer gradation within each row.
// This gives height × 8 effective resolution levels instead of just height levels.
// Uses 3-color scheme from ColorKeys for color coding.
func (s *SparklineState) renderMultiLine() string {
	colorKeys := s.colors.Keys()
	lines := make([]strings.Builder, s.height)
	rowHeight := 1.0 / float64(s.height)

	// Build each row from top (highest) to bottom (lowest)
	for row := 0; row < s.height; row++ {
		// Row boundaries: rowLower to rowUpper
		// Row 0 (top) = highest values, Row height-1 (bottom) = lowest values
		rowUpper := float64(s.height-row) / float64(s.height)
		rowLower := float64(s.height-row-1) / float64(s.height)

		isBottomRow := row == s.height-1

		for _, val := range s.values {
			// Determine color based on percentage value using ColorKeys
			percent := int(val * 100)
			color := Theme.SparklineNormal
			for _, k := range colorKeys {
				if percent >= k {
					color = s.colors[k]
				}
			}

			if val >= rowUpper {
				// Value exceeds this row's upper bound - draw full block
				lines[row].WriteString("[")
				lines[row].WriteString(color)
				lines[row].WriteString("]")
				lines[row].WriteRune('█')
			} else if val > rowLower {
				// Value is within this row's range - draw partial block
				// Calculate how much of this row is filled (0-1)
				fraction := (val - rowLower) / rowHeight
				// Map to block level (1-8), level 0 is baseline
				level := int(fraction * 8)
				if level > 8 {
					level = 8
				}
				if level < 1 {
					level = 1 // At least show minimal block if val > rowLower
				}
				lines[row].WriteString("[")
				lines[row].WriteString(color)
				lines[row].WriteString("]")
				lines[row].WriteRune(blockBarChars[level])
			} else if isBottomRow {
				// On bottom row, show baseline for empty/zero values
				lines[row].WriteString("[")
				lines[row].WriteString(Theme.SparklineEmpty)
				lines[row].WriteString("]")
				lines[row].WriteRune('▁')
			} else {
				// Value doesn't reach this row - draw space
				lines[row].WriteString(" ")
			}
		}
	}

	// Join lines with newlines
	var result strings.Builder
	for i, line := range lines {
		result.WriteString(line.String())
		if i < s.height-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// brailleLevel converts a 0-1 value to a braille level (0-4)
func (s *SparklineState) brailleLevel(val float64) int {
	if val <= 0 {
		return 0
	} else if val <= 0.25 {
		return 1
	} else if val <= 0.50 {
		return 2
	} else if val <= 0.75 {
		return 3
	}
	return 4
}

// blockLevel converts a 0-1 value to a block level (0-8)
func (s *SparklineState) blockLevel(val float64) int {
	if val <= 0 {
		return 0
	}
	// Map 0.01-1.0 to levels 1-8
	level := int(val*8) + 1
	if level > 8 {
		level = 8
	}
	return level
}

// Width returns the sparkline width
func (s *SparklineState) Width() int {
	return s.width
}

// Values returns a copy of the current values (for debugging/testing)
func (s *SparklineState) Values() []float64 {
	result := make([]float64, s.width)
	copy(result, s.values)
	return result
}

// Clear resets all values to zero
func (s *SparklineState) Clear() {
	for i := range s.values {
		s.values[i] = 0
	}
}

// Resize changes the sparkline width, preserving recent data.
// When width increases: old data stays right-aligned, zeros pad on left.
// When width decreases: old data truncated from left, recent data preserved.
func (s *SparklineState) Resize(newWidth int) {
	if newWidth == s.width || newWidth <= 0 {
		return
	}
	newValues := make([]float64, newWidth)
	if newWidth > s.width {
		// Larger: pad left with zeros, copy all old data to right
		copy(newValues[newWidth-s.width:], s.values)
	} else {
		// Smaller: truncate old data, keep most recent
		copy(newValues, s.values[s.width-newWidth:])
	}
	s.values = newValues
	s.width = newWidth
}

// TrendIndicator returns a colored trend arrow (↑/↓) or empty string for stable.
// Compares first 20% of buffer to last 20% to determine trend direction.
// percentage is used to determine color: red if >= 80%, white otherwise.
// Returns empty string when trend is stable or there's insufficient data.
func (s *SparklineState) TrendIndicator(percentage float64) string {
	// Need minimum data points for meaningful comparison
	if len(s.values) < 5 {
		return ""
	}

	// Compare first 20% average to last 20% average
	window := len(s.values) / 5
	if window < 1 {
		window = 1
	}

	var startAvg, endAvg float64
	for i := 0; i < window; i++ {
		startAvg += s.values[i]
		endAvg += s.values[len(s.values)-1-i]
	}
	startAvg /= float64(window)
	endAvg /= float64(window)

	// Determine trend direction
	var trendUp, trendDown bool

	if startAvg < 0.001 {
		// Starting from near-zero
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
