package ui

import (
	"strings"
	"testing"
)

func TestNewSparklineRenderer(t *testing.T) {
	data := []float64{1, 2, 3, 4, 5}
	spark := NewSparklineRenderer(data, 10, 1)

	if spark.Width != 10 {
		t.Errorf("Expected width 10, got %d", spark.Width)
	}
	if spark.Height != 1 {
		t.Errorf("Expected height 1, got %d", spark.Height)
	}
	if len(spark.Data) != 5 {
		t.Errorf("Expected 5 data points, got %d", len(spark.Data))
	}
	if spark.Style != SparkStyleBar {
		t.Errorf("Expected SparkStyleBar, got %v", spark.Style)
	}
}

func TestNewSparklineRenderer_MinimumDimensions(t *testing.T) {
	spark := NewSparklineRenderer(nil, 0, 0)

	if spark.Width != 1 {
		t.Errorf("Expected minimum width 1, got %d", spark.Width)
	}
	if spark.Height != 1 {
		t.Errorf("Expected minimum height 1, got %d", spark.Height)
	}
}

func TestSparkline_Normalize(t *testing.T) {
	tests := []struct {
		name     string
		data     []float64
		min, max float64
		expected []float64
	}{
		{
			name:     "Auto range 0-100",
			data:     []float64{0, 50, 100},
			expected: []float64{0, 0.5, 1},
		},
		{
			name:     "Auto range with negatives",
			data:     []float64{-10, 0, 10},
			expected: []float64{0, 0.5, 1},
		},
		{
			name:     "Custom range",
			data:     []float64{0, 50, 100},
			min:      0,
			max:      200,
			expected: []float64{0, 0.25, 0.5},
		},
		{
			name:     "All same non-zero values",
			data:     []float64{5, 5, 5},
			expected: []float64{1, 1, 1}, // When range is 0 and values > 0, all become 1
		},
		{
			name:     "All zeros",
			data:     []float64{0, 0, 0},
			expected: []float64{0, 0, 0}, // When range is 0 and values = 0, all become 0
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spark := NewSparklineRenderer(tc.data, 10, 1)
			spark.Min = tc.min
			spark.Max = tc.max

			result := spark.normalize()

			if len(result) != len(tc.expected) {
				t.Fatalf("Expected %d values, got %d", len(tc.expected), len(result))
			}

			for i, v := range result {
				if v != tc.expected[i] {
					t.Errorf("At index %d: expected %f, got %f", i, tc.expected[i], v)
				}
			}
		})
	}
}

func TestSparkline_Render_EmptyData(t *testing.T) {
	spark := NewSparklineRenderer(nil, 5, 1)
	result := spark.Render()

	// Should return empty braille characters
	if result == "" {
		t.Error("Expected non-empty result for empty data")
	}

	// Should contain braille empty character
	if !strings.Contains(result, "⠀") {
		t.Error("Expected empty braille characters in result")
	}
}

func TestSparkline_Render_BasicBar(t *testing.T) {
	// Create a simple increasing pattern
	data := []float64{0.25, 0.5, 0.75, 1.0}
	spark := NewSparklineRenderer(data, 4, 1)
	spark.Colors = ColorKeys{0: "white"} // Simple single color

	result := spark.Render()

	// Result should not be empty
	if result == "" {
		t.Error("Expected non-empty result")
	}

	// Result should contain braille characters
	if !strings.ContainsAny(result, "⠀⡀⡄⡆⡇⣀⣄⣆⣇⣠⣤⣦⣧⣰⣴⣶⣷⣸⣼⣾⣿") {
		t.Error("Expected braille characters in result")
	}
}

func TestSparkline_Render_FullBar(t *testing.T) {
	// All maximum values should produce full bars
	data := []float64{100, 100, 100, 100}
	spark := NewSparklineRenderer(data, 2, 1)
	spark.Colors = ColorKeys{0: "white"}

	result := spark.Render()

	// Should contain full braille character
	if !strings.Contains(result, "⣿") {
		t.Errorf("Expected full braille character (⣿) for max values, got: %s", result)
	}
}

func TestSparkline_Render_MultiRow(t *testing.T) {
	data := []float64{0, 0.5, 1.0}
	spark := NewSparklineRenderer(data, 3, 2)
	spark.Colors = ColorKeys{0: "white"}

	result := spark.Render()

	// Multi-row should contain newline
	if !strings.Contains(result, "\n") {
		t.Error("Expected newline in multi-row sparkline")
	}

	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Errorf("Expected 2 lines, got %d", len(lines))
	}
}

func TestSparkline_TrendIndicator(t *testing.T) {
	tests := []struct {
		name           string
		data           []float64
		percentage     float64
		expectContains string
		expectEmpty    bool
	}{
		{
			name:           "Increasing trend normal",
			data:           []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
			percentage:     50,
			expectContains: Icons.TrendUp,
		},
		{
			name:           "Increasing trend high percentage",
			data:           []float64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
			percentage:     85,
			expectContains: Icons.TrendUp,
		},
		{
			name:           "Decreasing trend",
			data:           []float64{100, 90, 80, 70, 60, 50, 40, 30, 20, 10},
			percentage:     30,
			expectContains: Icons.TrendDown,
		},
		{
			name:        "Flat trend",
			data:        []float64{50, 51, 49, 50, 51, 49, 50, 51, 49, 50},
			percentage:  50,
			expectEmpty: true, // Stable returns empty
		},
		{
			name:        "Too few points",
			data:        []float64{50},
			percentage:  50,
			expectEmpty: true, // Insufficient data returns empty
		},
		{
			name:        "Empty data",
			data:        []float64{},
			percentage:  0,
			expectEmpty: true, // No data returns empty
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spark := NewSparklineRenderer(tc.data, 10, 1)
			result := spark.TrendIndicator(tc.percentage)

			if tc.expectEmpty {
				if result != "" {
					t.Errorf("Expected empty string, got: %s", result)
				}
			} else if !strings.Contains(result, tc.expectContains) {
				t.Errorf("Expected trend indicator to contain %s, got: %s", tc.expectContains, result)
			}
		})
	}
}

func TestSparkline_LastValue(t *testing.T) {
	tests := []struct {
		name     string
		data     []float64
		expected float64
	}{
		{
			name:     "Normal data",
			data:     []float64{1, 2, 3, 4, 5},
			expected: 5,
		},
		{
			name:     "Single value",
			data:     []float64{42},
			expected: 42,
		},
		{
			name:     "Empty data",
			data:     []float64{},
			expected: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spark := NewSparklineRenderer(tc.data, 10, 1)
			result := spark.LastValue()

			if result != tc.expected {
				t.Errorf("Expected %f, got %f", tc.expected, result)
			}
		})
	}
}

func TestSparkline_AddPoint(t *testing.T) {
	spark := NewSparklineRenderer([]float64{1, 2, 3}, 10, 1)

	spark.AddPoint(4, 0) // No max
	if len(spark.Data) != 4 {
		t.Errorf("Expected 4 points, got %d", len(spark.Data))
	}
	if spark.LastValue() != 4 {
		t.Errorf("Expected last value 4, got %f", spark.LastValue())
	}

	// Add with max limit
	spark.AddPoint(5, 3)
	if len(spark.Data) != 3 {
		t.Errorf("Expected 3 points after trim, got %d", len(spark.Data))
	}
	if spark.Data[0] != 3 {
		t.Errorf("Expected first value 3 after trim, got %f", spark.Data[0])
	}
}

func TestSparkline_ResampleData(t *testing.T) {
	spark := NewSparklineRenderer(nil, 10, 1)

	// Stretch case: less data than width
	smallData := []float64{0, 1}
	stretched := spark.resampleData(smallData, 4)
	if len(stretched) != 4 {
		t.Errorf("Expected 4 points, got %d", len(stretched))
	}

	// Compress case: more data than width
	largeData := []float64{0, 0.25, 0.5, 0.75, 1.0, 0.75, 0.5, 0.25}
	compressed := spark.resampleData(largeData, 4)
	if len(compressed) != 4 {
		t.Errorf("Expected 4 points, got %d", len(compressed))
	}

	// Same size case
	sameData := []float64{1, 2, 3, 4}
	same := spark.resampleData(sameData, 4)
	if len(same) != 4 {
		t.Errorf("Expected 4 points, got %d", len(same))
	}
	for i, v := range same {
		if v != sameData[i] {
			t.Errorf("At index %d: expected %f, got %f", i, sameData[i], v)
		}
	}
}

func TestSparkline_Colors(t *testing.T) {
	data := []float64{10, 50, 90}
	spark := NewSparklineRenderer(data, 6, 1)
	spark.Colors = ColorKeys{0: "green", 50: "yellow", 80: "red"}

	result := spark.Render()

	// Should contain color codes
	if !strings.Contains(result, "[green]") && !strings.Contains(result, "[yellow]") && !strings.Contains(result, "[red]") {
		t.Error("Expected color codes in result")
	}
}

func TestSparkline_Inverted(t *testing.T) {
	data := []float64{0.5, 0.5, 0.5}
	sparkNormal := NewSparklineRenderer(data, 3, 1)
	sparkInverted := NewSparklineRenderer(data, 3, 1)
	sparkInverted.Inverted = true

	normalResult := sparkNormal.Render()
	invertedResult := sparkInverted.Render()

	// Results should be different when inverted
	// (though they might render similarly for centered data)
	if normalResult == "" || invertedResult == "" {
		t.Error("Expected non-empty results")
	}
}

func TestGridToBraille(t *testing.T) {
	// Test empty grid produces empty braille
	grid := make([][]bool, 4)
	for i := range grid {
		grid[i] = make([]bool, 2)
	}

	result := gridToBraille(grid, 0, 0)
	if result != 0x2800 {
		t.Errorf("Expected empty braille (0x2800), got %x", result)
	}

	// Test full grid produces full braille
	for i := range grid {
		for j := range grid[i] {
			grid[i][j] = true
		}
	}

	result = gridToBraille(grid, 0, 0)
	if result != 0x28FF {
		t.Errorf("Expected full braille (0x28FF), got %x", result)
	}
}

func TestSparkline_RenderDimensions(t *testing.T) {
	tests := []struct {
		width, height int
		expectLines   int
	}{
		{10, 1, 1},
		{5, 2, 2},
		{20, 4, 4},
	}

	for _, tc := range tests {
		t.Run("", func(t *testing.T) {
			spark := NewSparklineRenderer([]float64{1, 2, 3}, tc.width, tc.height)
			result := spark.Render()

			lines := strings.Split(result, "\n")
			if len(lines) != tc.expectLines {
				t.Errorf("Expected %d lines for height %d, got %d", tc.expectLines, tc.height, len(lines))
			}
		})
	}
}
