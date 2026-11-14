package theme

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// ColorTag formats a tcell.Color into a tview markup color token for mode.
func ColorTag(c tcell.Color, mode ColorMode) string {
	if mode == ColorModeNone || c == tcell.ColorDefault {
		return "-"
	}

	rc := ResolveColor(c, mode)
	if rc == tcell.ColorDefault {
		return "-"
	}

	if !rc.IsRGB() {
		if mode == ColorModeTrueColor || mode == ColorModeAuto || mode == "" {
			if name, ok := canonicalColorNames[rc]; ok {
				return name
			}
		}
		if mode == ColorMode16 && rc >= tcell.ColorValid && rc < tcell.ColorValid+16 {
			if name, ok := canonicalColorNames[rc]; ok {
				return name
			}
		}
		if rc >= tcell.ColorValid && rc < tcell.ColorValid+16 {
			if name, ok := canonicalColorNames[rc]; ok {
				return name
			}
		}
		if rc >= tcell.ColorValid && rc < tcell.ColorValid+256 {
			return palette256TagNames[int(rc-tcell.ColorValid)]
		}
		if name, ok := canonicalColorNames[rc]; ok {
			return name
		}
	}

	return fmt.Sprintf("#%06x", rc.Hex())
}

// FormatTag builds a tview markup tag string for the given colors and attributes.
func FormatTag(fg, bg tcell.Color, attrs string, mode ColorMode) string {
	var fgStr, bgStr string
	if mode != ColorModeNone {
		if fg != tcell.ColorDefault {
			fgStr = ColorTag(fg, mode)
		}
		if bg != tcell.ColorDefault {
			bgStr = ColorTag(bg, mode)
		}
	}

	if bgStr == "" && attrs == "" {
		if fgStr == "" {
			return ""
		}
		return "[" + fgStr + "]"
	}
	return "[" + fgStr + ":" + bgStr + ":" + attrs + "]"
}

// Wrap wraps text with a tview markup tag and matching reset tag.
func Wrap(text string, fg tcell.Color, attrs string, mode ColorMode) string {
	tag := FormatTag(fg, tcell.ColorDefault, attrs, mode)
	if tag == "" {
		return text
	}
	if mode == ColorModeNone {
		return tag + text + ResetTag(false, false, attrs != "")
	}
	return tag + text + ResetTag(fg != tcell.ColorDefault, false, attrs != "")
}

// WrapError formats error text semantically, using bold+underline+! prefix in monochrome.
func WrapError(text string, p Palette, mode ColorMode) string {
	if mode == ColorModeNone || p.Name == "monochrome" {
		if !strings.HasPrefix(text, "! ") {
			text = "! " + text
		}
		return "[::bu]" + text + "[::-]"
	}
	return "[" + ColorTag(p.StatusError, mode) + "]" + text + "[-]"
}

// ResetTag builds a tview reset sequence for the specified style channels.
func ResetTag(resetFg, resetBg, resetAttr bool) string {
	var fg, bg, attr string
	if resetFg {
		fg = "-"
	}
	if resetBg {
		bg = "-"
	}
	if resetAttr {
		attr = "-"
	}
	return "[" + fg + ":" + bg + ":" + attr + "]"
}
