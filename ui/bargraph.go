package ui

import (
	"math"
	"sort"
	"strings"
)

const (
	colorZeroGraph = "silver"
	colorNoKeys    = "white"
)

// Ratio float64 type used to represents ratio values
type Ratio float64

// ColorKeys represents color gradients mapping of a percentage value
// (expressed as integer i.e. 20 for 20%) to a color.
type ColorKeys map[int]string

// Keys returns a ascending sorted slice of color keys
func (ck ColorKeys) Keys() []int {
	var keys []int
	for k := range ck {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// ColorKeysFromSlice automatically builds a color key mapping by
// equally dividing the slice items into a key value.  For instance,
// []string{"green", "yellow", "red"} returns ColorKeys{33:"green",66:"yellow",100:"red"}
func ColorKeysFromSlice(colors []string) ColorKeys {
	count := len(colors)
	keys := ColorKeys{}
	for i, color := range colors {
		key := (float64(i+1) / float64(count)) * 100
		keys[int(key)] = color
	}
	return keys
}

// BarGraph returns a colorized string using braille characters with fractional precision.
// Each character position can show 8 levels of fill (using braille dot patterns),
// giving much finer granularity than simple filled/empty blocks.
// Colors provide key mapping to colorize the graph based on the value of ratio.
func BarGraph(scale int, ratio Ratio, colors ColorKeys) string {
	if scale == 0 {
		return ""
	}

	var graph strings.Builder

	// nothing to graph - show empty bar
	if ratio == 0 {
		graph.WriteString("[darkgray]")
		for j := 0; j < scale; j++ {
			graph.WriteString(Icons.BargraphEmpty)
		}
		return graph.String()
	}

	// Clamp ratio to [0, 1]
	if ratio > 1 {
		ratio = 1
	}

	// Calculate total "dots" available (8 dots per character * scale characters)
	totalDots := scale * 8
	filledDots := int(math.Round(float64(ratio) * float64(totalDots)))

	// Build color keys for threshold lookup
	colorKeys := colors.Keys()

	// Draw each character position
	for i := 0; i < scale; i++ {
		// Calculate how many dots should be filled in this character (0-8)
		startDot := i * 8
		endDot := startDot + 8
		dotsInThisChar := 0

		if filledDots >= endDot {
			// This character is fully filled
			dotsInThisChar = 8
		} else if filledDots > startDot {
			// This character is partially filled
			dotsInThisChar = filledDots - startDot
		}
		// else: dotsInThisChar = 0 (empty)

		// Calculate what percentage this position represents (for color selection)
		positionPercent := int((float64(i) / float64(scale)) * 100)

		// Find the appropriate color for this position
		segmentColor := colorNoKeys
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
			// Empty - use darkgray for consistency
			graph.WriteString("[darkgray]")
			graph.WriteString(Icons.BargraphEmpty)
			continue
		} else if dotsInThisChar == 8 {
			// Fully filled
			brailleChar = Icons.BargraphFull
		} else {
			// Partially filled (1-7 dots)
			brailleChar = Icons.BargraphPartials[dotsInThisChar-1]
		}

		// Write this character with its color
		graph.WriteString("[")
		graph.WriteString(segmentColor)
		graph.WriteString("]")
		graph.WriteString(brailleChar)
	}

	return graph.String()
}

// GetRatio returns a ration between val0/val1.
// If val <= 0, it return 0.
func GetRatio(val0, val1 float64) Ratio {
	if val1 <= 0 {
		return 0
	}
	return Ratio(val0 / val1)
}
