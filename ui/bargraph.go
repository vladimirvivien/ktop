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

// BarGraph returns a colorized string, like ||||||||, which represents
// a bargrah built using scale and ratio values.  Colors provide key mapping
// to colorize the graph basede on the value of ratio.
// If ratio is zero (nothing to graph), the function returns a series of dots.
func BarGraph(scale int, ratio Ratio, colors ColorKeys) string {
	if scale == 0 {
		return ""
	}

	normVal := float64(ratio) * float64(scale)
	graphVal := int(math.Ceil(normVal))

	var graph strings.Builder
	var color string

	// nothing to graph
	if normVal == 0 {
		if c, found := colors[0]; !found {
			color = colorZeroGraph
		} else {
			color = c
		}

		graph.WriteString("[")
		graph.WriteString(color)
		graph.WriteString("]")
		for j := 0; j < (scale - graphVal); j++ {
			graph.WriteString(".")
		}
		return graph.String()
	}

	// assign color
	if colors == nil || len(colors) == 0 {
		color = colorNoKeys
	}

	key := int(float64(ratio) * 100)

	for _, k := range colors.Keys() {
		if key >= k {
			color = colors[k]
		}
	}

	if color == "" {
		color = colorNoKeys
	}

	// draw graph
	graph.WriteString("[")
	graph.WriteString(color)
	graph.WriteString("]")

	for i := 0; i < int(math.Min(float64(scale), float64(graphVal))); i++ {
		graph.WriteString("|")
	}

	for j := 0; j < (scale - graphVal); j++ {
		graph.WriteString(" ")
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
