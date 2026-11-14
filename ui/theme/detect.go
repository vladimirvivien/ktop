package theme

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// EnvFunc mirrors os.LookupEnv for deterministic testing.
type EnvFunc func(string) (string, bool)

// ResolveOptions configures theme and color-mode auto-detection.
type ResolveOptions struct {
	Theme        string
	ColorMode    string
	LookupEnv    EnvFunc
	ProbeLightBg func() (isLight bool, ok bool)
}

// Resolve computes the concrete Palette and ColorMode according to precedence:
// explicit theme -> $NO_COLOR / $TERM=dumb -> $COLORFGBG -> OSC 11 probe -> dark.
func Resolve(opts ResolveOptions) (ResolvedTheme, error) {
	lookupEnv := opts.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	probeLightBg := opts.ProbeLightBg
	if probeLightBg == nil {
		probeLightBg = ProbeTerminalLightBg
	}

	themeReq := strings.ToLower(strings.TrimSpace(opts.Theme))
	if themeReq == "" {
		themeReq = "auto"
	}

	modeReq := ColorMode(strings.ToLower(strings.TrimSpace(opts.ColorMode)))
	if modeReq == "" {
		modeReq = ColorModeAuto
	}
	if err := validateColorMode(modeReq); err != nil {
		return ResolvedTheme{}, err
	}

	_, noColorSet := lookupEnv("NO_COLOR")
	termVal, _ := lookupEnv("TERM")
	termDumb := strings.EqualFold(strings.TrimSpace(termVal), "dumb")

	var palette Palette
	autoThemeSelected := false

	if themeReq == "auto" {
		autoThemeSelected = true
		switch {
		case noColorSet || termDumb:
			palette = MonochromePalette()
		default:
			if colorFGBG, ok := lookupEnv("COLORFGBG"); ok {
				if isLight, parsed := ParseCOLORFGBG(colorFGBG); parsed {
					if isLight {
						palette = LightPalette()
					} else {
						palette = DarkPalette()
					}
					break
				}
			}
			if isLight, ok := probeLightBg(); ok && isLight {
				palette = LightPalette()
			} else {
				palette = DarkPalette()
			}
		}
	} else {
		p, ok := ByName(themeReq)
		if !ok {
			return ResolvedTheme{}, fmt.Errorf("unknown theme %q (valid: %s)", opts.Theme, strings.Join(ValidThemeNames(), ", "))
		}
		palette = p
	}

	mode := modeReq
	if mode == ColorModeAuto {
		switch {
		case palette.Name == "monochrome" || termDumb || (autoThemeSelected && noColorSet):
			mode = ColorModeNone
		case palette.Name == "ansi":
			mode = ColorMode16
		default:
			mode = detectColorTier(lookupEnv)
		}
	}

	if mode == ColorModeNone {
		palette = MonochromePalette()
	}

	return ResolvedTheme{
		Palette: palette.Resolve(mode),
		Mode:    mode,
	}, nil
}

func validateColorMode(mode ColorMode) error {
	switch mode {
	case ColorModeAuto, ColorModeTrueColor, ColorMode256, ColorMode16, ColorModeNone:
		return nil
	default:
		return fmt.Errorf("unknown color mode %q (valid: %s)", string(mode), strings.Join(ValidColorModes(), ", "))
	}
}

func detectColorTier(lookupEnv EnvFunc) ColorMode {
	if colorTerm, ok := lookupEnv("COLORTERM"); ok {
		ct := strings.ToLower(strings.TrimSpace(colorTerm))
		if ct == "truecolor" || ct == "24bit" {
			return ColorModeTrueColor
		}
	}
	if termVal, ok := lookupEnv("TERM"); ok {
		if strings.Contains(strings.ToLower(termVal), "256color") {
			return ColorMode256
		}
	}
	return ColorMode16
}

// ParseCOLORFGBG parses "$COLORFGBG" (e.g. "15;0" or "default;default;7") and
// reports whether the trailing background index indicates a light terminal.
func ParseCOLORFGBG(val string) (isLight bool, ok bool) {
	val = strings.TrimSpace(val)
	if val == "" {
		return false, false
	}
	parts := strings.Split(val, ";")
	bgStr := strings.TrimSpace(parts[len(parts)-1])
	bg, err := strconv.Atoi(bgStr)
	if err != nil {
		return false, false
	}
	if bg == 7 || bg == 15 {
		return true, true
	}
	if (bg >= 0 && bg <= 6) || bg == 8 {
		return false, true
	}
	return false, false
}

// ProbeTerminalLightBg sends an OSC 11 background color query on interactive TTYs
// and computes relative luminance within a 50ms read deadline.
func ProbeTerminalLightBg() (isLight bool, ok bool) {
	inFd := int(os.Stdin.Fd())
	outFd := int(os.Stdout.Fd())
	if !term.IsTerminal(inFd) || !term.IsTerminal(outFd) {
		return false, false
	}

	oldState, err := term.MakeRaw(inFd)
	if err != nil {
		return false, false
	}
	defer func() {
		_ = term.Restore(inFd, oldState)
	}()

	// Character TTY devices that do not support Go's netpoll deadlines return os.ErrNoDeadline;
	// abort before writing/reading so startup never blocks waiting for user input.
	if err := os.Stdin.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		return false, false
	}
	defer func() {
		_ = os.Stdin.SetReadDeadline(time.Time{})
	}()

	if _, err := os.Stdout.WriteString("\x1b]11;?\x1b\\"); err != nil {
		return false, false
	}

	buf := make([]byte, 64)
	total := 0
	for total < len(buf) {
		n, readErr := os.Stdin.Read(buf[total:])
		if n > 0 {
			total += n
			s := string(buf[:total])
			if strings.Contains(s, "\x1b\\") || strings.Contains(s, "\x07") {
				break
			}
		}
		if readErr != nil {
			break
		}
	}

	lum, parsed := ParseOSC11Luminance(string(buf[:total]))
	if !parsed {
		return false, false
	}
	return lum > 0.5, true
}

// ParseOSC11Luminance parses an OSC 11 "rgb:RRRR/GGGG/BBBB" response and returns
// its ITU-R BT.709 relative luminance in [0, 1].
func ParseOSC11Luminance(resp string) (float64, bool) {
	idx := strings.Index(resp, "rgb:")
	if idx < 0 {
		return 0, false
	}
	rest := resp[idx+len("rgb:"):]
	if end := strings.IndexAny(rest, "\x1b\x07\r\n"); end >= 0 {
		rest = rest[:end]
	}

	channels := strings.Split(rest, "/")
	if len(channels) != 3 {
		return 0, false
	}

	r, okR := parseHexChannel(channels[0])
	g, okG := parseHexChannel(channels[1])
	b, okB := parseHexChannel(channels[2])
	if !okR || !okG || !okB {
		return 0, false
	}

	lum := 0.2126*r + 0.7152*g + 0.0722*b
	return lum, true
}

func parseHexChannel(h string) (float64, bool) {
	h = strings.TrimSpace(h)
	if len(h) == 0 || len(h) > 4 {
		return 0, false
	}
	val, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 0, false
	}
	maxVal := float64((uint32(1) << (4 * len(h))) - 1)
	return float64(val) / maxVal, true
}
