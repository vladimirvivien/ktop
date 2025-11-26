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
			expected:  "[darkgray]⠀⠀⠀⠀⠀",
		},
		{
			name:      "no color",
			scale:     10,
			ratio:     0.10,
			colorKeys: nil,
			// 10% of 10 chars = 1 full braille char (8 dots), remaining 9 empty
			expected: "[white]⣿[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀",
		},
		{
			name:      "2-color, unspecified color",
			scale:     10,
			ratio:     0.10,
			colorKeys: ColorKeys{15: "green", 30: "red"},
			// 10% is below 15% threshold, so uses default white
			expected: "[white]⣿[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀",
		},
		{
			name:      "2-color, select color 1",
			scale:     10,
			ratio:     0.20,
			colorKeys: ColorKeys{15: "green", 30: "red"},
			// 20% = 16 dots out of 80 = 2 full chars
			// pos 0 (0%) -> white (below 15%), pos 1 (10%) -> white (below 15%)
			expected: "[white]⣿[white]⣿[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀",
		},
		{
			name:      "2-color, select color 2",
			scale:     10,
			ratio:     0.30,
			colorKeys: ColorKeys{15: "green", 30: "red"},
			// 30% = 24 dots out of 80 = 3 full chars
			// pos 0 (0%) -> white, pos 1 (10%) -> white, pos 2 (20%) -> green (>=15%)
			expected: "[white]⣿[white]⣿[green]⣿[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀[darkgray]⠀",
		},
		{
			name:      "full bar",
			scale:     5,
			ratio:     1.0,
			colorKeys: ColorKeys{0: "green", 50: "yellow", 90: "red"},
			// 100% = all full, colors: pos0(0%)=green, pos1(20%)=green, pos2(40%)=green, pos3(60%)=yellow, pos4(80%)=yellow
			expected: "[green]⣿[green]⣿[green]⣿[yellow]⣿[yellow]⣿",
		},
		{
			name:      "fractional fill",
			scale:     5,
			ratio:     0.45,
			colorKeys: ColorKeys{0: "green", 50: "yellow"},
			// 45% of 40 dots = 18 dots = 2 full (16 dots) + 2 dots partial
			// pos0(0%)=green full, pos1(20%)=green full, pos2(40%)=green partial(⡄)
			expected: "[green]⣿[green]⣿[green]⡄[darkgray]⠀[darkgray]⠀",
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
