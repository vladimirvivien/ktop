package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func findSimToken(cells []tcell.SimCell, w, h int, token string) (int, int, bool) {
	target := []rune(token)
	for y := 0; y < h; y++ {
		base := y * w
		for x := 0; x <= w-len(target); x++ {
			match := true
			for i, want := range target {
				runes := cells[base+x+i].Runes
				if len(runes) == 0 || runes[0] != want {
					match = false
					break
				}
			}
			if match {
				return x, y, true
			}
		}
	}
	return -1, -1, false
}

func TestNewToast(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ResolveAndApplyTheme("dark", "truecolor")
	})
	if _, err := ResolveAndApplyTheme("dark", "truecolor"); err != nil {
		t.Fatalf("ResolveAndApplyTheme failed: %v", err)
	}

	tests := []struct {
		name          string
		message       string
		level         ToastLevel
		expectedIcon  string
		expectedColor tcell.Color
	}{
		{
			name:          "Info toast",
			message:       "Loading metrics",
			level:         ToastInfo,
			expectedIcon:  "ℹ",
			expectedColor: tcell.ColorDarkBlue,
		},
		{
			name:          "Success toast",
			message:       "Connected",
			level:         ToastSuccess,
			expectedIcon:  "✓",
			expectedColor: tcell.ColorDarkGreen,
		},
		{
			name:          "Warning toast",
			message:       "Slow response",
			level:         ToastWarning,
			expectedIcon:  "⚠",
			expectedColor: tcell.NewHexColor(0x92400e),
		},
		{
			name:          "Error toast",
			message:       "Connection failed",
			level:         ToastError,
			expectedIcon:  "✗",
			expectedColor: tcell.ColorDarkRed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toast := NewToast(tt.message, tt.level)
			if toast == nil {
				t.Fatal("NewToast returned nil")
			}
		})
	}
}

func TestGetIconAndColor(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ResolveAndApplyTheme("dark", "truecolor")
	})
	if _, err := ResolveAndApplyTheme("dark", "truecolor"); err != nil {
		t.Fatalf("ResolveAndApplyTheme failed: %v", err)
	}

	tests := []struct {
		level         ToastLevel
		expectedIcon  string
		expectedColor tcell.Color
	}{
		{ToastInfo, "ℹ", tcell.ColorDarkBlue},
		{ToastSuccess, "✓", tcell.ColorDarkGreen},
		{ToastWarning, "⚠", tcell.NewHexColor(0x92400e)},
		{ToastError, "✗", tcell.ColorDarkRed},
	}

	for _, tt := range tests {
		t.Run(tt.expectedIcon, func(t *testing.T) {
			icon, color := getIconAndColor(tt.level)
			if icon != tt.expectedIcon {
				t.Errorf("Expected icon %q, got %q", tt.expectedIcon, icon)
			}
			if color != tt.expectedColor {
				t.Errorf("Expected color %v, got %v", tt.expectedColor, color)
			}
		})
	}
}

func TestToastModalThemeColorsAndButtons(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ResolveAndApplyTheme("dark", "truecolor")
	})

	themes := []struct {
		name string
		mode string
	}{
		{"dark", "truecolor"},
		{"light", "truecolor"},
		{"ansi", "16"},
		{"monochrome", "none"},
	}

	levels := []struct {
		name  string
		level ToastLevel
	}{
		{"info", ToastInfo},
		{"success", ToastSuccess},
		{"warning", ToastWarning},
		{"error", ToastError},
	}

	for _, tc := range themes {
		for _, lv := range levels {
			t.Run(tc.name+"/"+lv.name, func(t *testing.T) {
				rt, err := ResolveAndApplyTheme(tc.name, tc.mode)
				if err != nil {
					t.Fatalf("ResolveAndApplyTheme(%q, %q) failed: %v", tc.name, tc.mode, err)
				}

				toast := NewToastWithButtons("API error [403]", lv.level, []string{"Retry", "Quit"}, nil)
				ts, err := NewTestScreen(100, 30)
				if err != nil {
					t.Fatalf("NewTestScreen failed: %v", err)
				}
				defer ts.Close()

				ts.Focus(toast)
				ts.Draw(toast)

				text := ts.Text()
				if !strings.Contains(text, "[403]") {
					t.Fatalf("expected escaped literal '[403]' in toast output, got:\n%s", text)
				}

				cells, w, h := ts.Screen().GetContents()
				bx, by, okBorder := findSimToken(cells, w, h, "╔")
				if !okBorder {
					bx, by, okBorder = findSimToken(cells, w, h, "┌")
				}
				if !okBorder {
					t.Fatalf("expected top-left border cell '╔' or '┌' in toast output:\n%s", text)
				}
				rx, ry, okRetry := findSimToken(cells, w, h, "Retry")
				if !okRetry {
					t.Fatalf("expected 'Retry' button in toast output:\n%s", text)
				}
				qx, qy, okQuit := findSimToken(cells, w, h, "Quit")
				if !okQuit {
					t.Fatalf("expected 'Quit' button in toast output:\n%s", text)
				}
				ax, ay, okMsg := findSimToken(cells, w, h, "API")
				if !okMsg {
					t.Fatalf("expected message 'API' in toast output:\n%s", text)
				}

				if rt.IsMonochrome() {
					for i, cell := range cells {
						fg, bg, _ := cell.Style.Decompose()
						if fg != tcell.ColorDefault || bg != tcell.ColorDefault {
							t.Fatalf("monochrome cell %d has non-default color fg=%v bg=%v", i, fg, bg)
						}
					}
					_, _, bAttr := cells[by*w+bx].Style.Decompose()
					if bAttr&tcell.AttrBold == 0 {
						t.Fatalf("monochrome border cell missing AttrBold")
					}
					for dx := 0; dx < len("Retry"); dx++ {
						_, _, attr := cells[ry*w+rx+dx].Style.Decompose()
						if attr&tcell.AttrReverse == 0 {
							t.Fatalf("monochrome focused Retry button char %d missing AttrReverse", dx)
						}
					}
					if lv.level == ToastError {
						if !strings.Contains(text, "! ") {
							t.Fatalf("monochrome error toast missing '! ' prefix:\n%s", text)
						}
						_, _, mAttr := cells[ay*w+ax].Style.Decompose()
						if mAttr&tcell.AttrUnderline == 0 {
							t.Fatalf("monochrome error toast message missing AttrUnderline")
						}
					}
					return
				}

				if tc.name == "dark" || tc.name == "light" {
					_, expectedBg := getIconAndColor(lv.level)
					expectedBorder := getToastBorderColor(lv.level, rt.Palette)

					msgFg, msgBg, _ := cells[ay*w+ax].Style.Decompose()
					if msgBg != expectedBg {
						t.Fatalf("%s/%s toast message bg = %v, want %v", tc.name, lv.name, msgBg, expectedBg)
					}
					if msgFg != rt.Palette.ToastFg {
						t.Fatalf("%s/%s toast message fg = %v, want %v", tc.name, lv.name, msgFg, rt.Palette.ToastFg)
					}

					bFg, bBg, _ := cells[by*w+bx].Style.Decompose()
					if bBg != expectedBg {
						t.Fatalf("%s/%s toast border bg = %v, want %v", tc.name, lv.name, bBg, expectedBg)
					}
					if bFg != expectedBorder {
						t.Fatalf("%s/%s toast border fg = %v, want %v", tc.name, lv.name, bFg, expectedBorder)
					}

					for dx := 0; dx < len("Retry"); dx++ {
						fg, bg, _ := cells[ry*w+rx+dx].Style.Decompose()
						if fg == bg || fg == tcell.ColorDefault || bg == tcell.ColorDefault {
							t.Fatalf("%s/%s focused button cell invalid colors fg=%v bg=%v", tc.name, lv.name, fg, bg)
						}
					}
					for dx := 0; dx < len("Quit"); dx++ {
						fg, bg, _ := cells[qy*w+qx+dx].Style.Decompose()
						if fg == bg || fg == tcell.ColorDefault || bg == tcell.ColorDefault {
							t.Fatalf("%s/%s unfocused button cell invalid colors fg=%v bg=%v", tc.name, lv.name, fg, bg)
						}
					}
				}
			})
		}
	}
}
