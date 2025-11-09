package ui

import "testing"

func TestColorKeysFromSlice(t *testing.T) {
	testCases := []struct {
		name     string
		slice    []string
		expected ColorKeys
	}{
		{
			name:     "no color",
			slice:    nil,
			expected: ColorKeys{},
		},

		{
			name:     "2 colors",
			slice:    []string{"green", "red"},
			expected: ColorKeys{50: "green", 100: "red"},
		},

		{
			name:     "3 colors",
			slice:    []string{"yellow", "green", "red"},
			expected: ColorKeys{33: "yellow", 66: "green", 100: "red"},
		},

		{
			name:     "4 colors",
			slice:    []string{"yellow", "green", "red", "blue"},
			expected: ColorKeys{25: "yellow", 50: "green", 75: "red", 100: "blue"},
		},

		{
			name:     "5 colors",
			slice:    []string{"yellow", "green", "red", "blue", "magenta"},
			expected: ColorKeys{20: "yellow", 40: "green", 60: "red", 80: "blue", 100: "magenta"},
		},
	}

	for _, tc := range testCases {
		t.Logf("Running test %s", tc.name)
		actual := ColorKeysFromSlice(tc.slice)
		if len(actual) != len(tc.expected) {
			t.Errorf("expecting color keys count %d, got %d", len(tc.expected), len(actual))
		}

		for k, c := range actual {
			if tc.expected[k] != c {
				t.Errorf("expecting ColorKeys %#v, got %#v", tc.expected, actual)
			}
		}
	}
}

func TestBarGraph(t *testing.T) {
	testCases := []struct {
		name      string
		scale     int
		ratio     Ratio
		colorKeys ColorKeys
		expected  string
	}{
		{
			name:      "no scale",
			scale:     0,
			ratio:     1,
			colorKeys: ColorKeys{},
			expected:  "",
		},
		{
			name:      "no ratio",
			scale:     5,
			ratio:     0,
			colorKeys: ColorKeys{},
			expected:  "[silver]     ",
		},
		{
			name:      "no color",
			scale:     10,
			ratio:     0.10,
			colorKeys: nil,
			expected:  "[white]|         ",
		},
		{
			name:      "2-color, unspecified color",
			scale:     10,
			ratio:     0.10,
			colorKeys: ColorKeys{15: "green", 30: "red"},
			expected:  "[white]|         ",
		},
		{
			name:      "2-color, select color 1",
			scale:     10,
			ratio:     0.20,
			colorKeys: ColorKeys{15: "green", 30: "red"},
			expected:  "[green]||        ",
		},
		{
			name:      "2-color, select color 2",
			scale:     10,
			ratio:     0.30,
			colorKeys: ColorKeys{15: "green", 30: "red"},
			expected:  "[red]|||       ",
		},
	}

	for _, tc := range testCases {
		t.Logf("running test %s", tc.name)
		actual := BarGraph(tc.scale, tc.ratio, tc.colorKeys)
		if actual != tc.expected {
			t.Errorf("expecting graph [%s], got [%s]", tc.expected, actual)
		}
	}
}
