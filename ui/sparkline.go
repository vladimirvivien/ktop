package ui

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Sparkline is a tview primitive that displays a sparkline chart.
// It embeds tview.Box and maintains internal state for smooth animation.
type Sparkline struct {
	*tview.Box

	// Internal state
	state     *SparklineState
	colorKeys ColorKeys

	// Dimensions (used when creating/resizing state)
	sparkWidth  int
	sparkHeight int
}

// NewSparkline creates a new sparkline primitive.
func NewSparkline() *Sparkline {
	s := &Sparkline{
		Box:         tview.NewBox(),
		colorKeys:   DefaultColorKeys(),
		sparkWidth:  20,
		sparkHeight: 1,
	}
	s.state = NewSparklineStateWithHeight(s.sparkWidth, s.sparkHeight, s.colorKeys)
	return s
}

// SetDimensions sets the sparkline data dimensions (width = data points, height = rows).
func (s *Sparkline) SetDimensions(width, height int) *Sparkline {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	s.sparkWidth = width
	s.sparkHeight = height
	s.state = NewSparklineStateWithHeight(width, height, s.colorKeys)
	return s
}

// SetColorKeys sets the color thresholds.
func (s *Sparkline) SetColorKeys(colors ColorKeys) *Sparkline {
	s.colorKeys = colors
	s.state = NewSparklineStateWithHeight(s.sparkWidth, s.sparkHeight, s.colorKeys)
	return s
}

// SetTitle sets the panel title (chainable, returns *Sparkline).
func (s *Sparkline) SetTitle(title string) *Sparkline {
	s.Box.SetTitle(title)
	return s
}

// SetBorder enables/disables the border (chainable, returns *Sparkline).
func (s *Sparkline) SetBorder(show bool) *Sparkline {
	s.Box.SetBorder(show)
	return s
}

// SetBorderColor sets the border color (chainable, returns *Sparkline).
func (s *Sparkline) SetBorderColor(color tcell.Color) *Sparkline {
	s.Box.SetBorderColor(color)
	return s
}

// SetTitleAlign sets the title alignment (chainable, returns *Sparkline).
func (s *Sparkline) SetTitleAlign(align int) *Sparkline {
	s.Box.SetTitleAlign(align)
	return s
}

// Push adds a new value (0.0 to 1.0), shifting old values left.
func (s *Sparkline) Push(value float64) *Sparkline {
	s.state.Push(value)
	return s
}

// Clear resets all values to zero.
func (s *Sparkline) Clear() *Sparkline {
	s.state.Clear()
	return s
}

// TrendIndicator returns trend arrow for the given percentage.
func (s *Sparkline) TrendIndicator(percent float64) string {
	return s.state.TrendIndicator(percent)
}

// Width returns the current sparkline width (data points).
func (s *Sparkline) Width() int {
	return s.sparkWidth
}

// Height returns the current sparkline height (rows).
func (s *Sparkline) Height() int {
	return s.sparkHeight
}

// Resize changes the sparkline width (number of data points).
func (s *Sparkline) Resize(width int) *Sparkline {
	if width > 0 {
		s.state.Resize(width)
		s.sparkWidth = width
	}
	return s
}

// RenderText returns the sparkline as a colored string for inline use (e.g., table cells).
func (s *Sparkline) RenderText() string {
	return s.state.Render()
}

// Draw implements tview.Primitive.
// Like tview's Table and TextArea, this method adapts to container dimensions.
func (s *Sparkline) Draw(screen tcell.Screen) {
	s.Box.DrawForSubclass(screen, s)

	// Get inner dimensions (accounting for border)
	x, y, width, height := s.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	// Detect dimension changes (tview pattern)
	widthChanged := s.state.Width() != width
	heightChanged := s.state.Height() != height

	if widthChanged || heightChanged {
		// Preserve existing data
		oldValues := s.state.Values()

		if widthChanged && !heightChanged {
			// Width only - use efficient Resize
			s.state.Resize(width)
		} else {
			// Height changed - recreate state with new dimensions
			s.state = NewSparklineStateWithHeight(width, height, s.colorKeys)
			// Restore data
			start := 0
			if len(oldValues) > width {
				start = len(oldValues) - width
			}
			for _, v := range oldValues[start:] {
				s.state.Push(v)
			}
		}
		s.sparkWidth = width
		s.sparkHeight = height
	}

	// Render sparkline content - handle multi-line output
	content := s.state.Render()
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if y+i < y+height { // Don't overflow container
			tview.Print(screen, line, x, y+i, width, tview.AlignLeft, tcell.ColorDefault)
		}
	}
}

// DefaultColorKeys returns standard color thresholds for sparklines.
func DefaultColorKeys() ColorKeys {
	return ColorKeys{0: "olivedrab", 50: "yellow", 90: "red"}
}
