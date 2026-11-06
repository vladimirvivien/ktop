package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/metrics"
)

// PSISparklineRow displays PSI pressure stall sparklines in a horizontal row:
// Column 0: CPU Pressure Stall
// Column 1: Memory Pressure Stall
// Column 2: I/O Pressure Stall
//
// Each Sparkline is self-sizing and adapts to its container during Draw().
type PSISparklineRow struct {
	*tview.Box

	cpuSparkline *Sparkline
	memSparkline *Sparkline
	ioSparkline  *Sparkline

	colorKeys ColorKeys
}

// NewPSISparklineRow creates a new row for PSI pressure stall sparklines.
func NewPSISparklineRow() *PSISparklineRow {
	colorKeys := DefaultColorKeys()

	r := &PSISparklineRow{
		Box:          tview.NewBox(),
		cpuSparkline: NewSparkline().SetColorKeys(colorKeys),
		memSparkline: NewSparkline().SetColorKeys(colorKeys),
		ioSparkline:  NewSparkline().SetColorKeys(colorKeys),
		colorKeys:    colorKeys,
	}

	r.cpuSparkline.SetBorder(true).SetBorderColor(tcell.ColorLightGray).SetTitleAlign(tview.AlignCenter)
	r.memSparkline.SetBorder(true).SetBorderColor(tcell.ColorLightGray).SetTitleAlign(tview.AlignCenter)
	r.ioSparkline.SetBorder(true).SetBorderColor(tcell.ColorLightGray).SetTitleAlign(tview.AlignCenter)

	return r
}

// Update updates the sparklines with the latest PSI metrics and supported flag.
func (r *PSISparklineRow) Update(psi metrics.PSIMetrics, supported bool) {
	r.updateAxis(r.cpuSparkline, "CPU", psi.CPUStallPct, supported)
	r.updateAxis(r.memSparkline, "MEM", psi.MemStallPct, supported)
	r.updateAxis(r.ioSparkline, "IO", psi.IOStallPct, supported)
}

func (r *PSISparklineRow) updateAxis(s *Sparkline, label string, pct float64, supported bool) {
	if !supported {
		s.Push(0).SetTitle(fmt.Sprintf(" %s Stall [yellow]⚠ N/A[-] ", label))
		return
	}

	ratio := pct / 100.0
	if ratio > 1.0 {
		ratio = 1.0
	}
	s.Push(ratio)

	trend := s.TrendIndicator(pct)
	var valStr string
	switch {
	case pct < 0.01:
		valStr = "[gray]0%[-]"
	case pct < 1.0:
		valStr = fmt.Sprintf("[%s]%.2f%%[-]", stallColorName(pct), pct)
	case pct < 10.0:
		valStr = fmt.Sprintf("[%s]%.1f%%[-]", stallColorName(pct), pct)
	default:
		valStr = fmt.Sprintf("[%s]%.0f%%[-]", stallColorName(pct), pct)
	}

	if trend != "" {
		s.SetTitle(fmt.Sprintf(" %s Stall %s %s ", label, valStr, trend))
	} else {
		s.SetTitle(fmt.Sprintf(" %s Stall %s ", label, valStr))
	}
}

func stallColorName(pct float64) string {
	switch {
	case pct < 2:
		return "green"
	case pct < 10:
		return "yellow"
	case pct < 25:
		return "orange"
	default:
		return "red"
	}
}

// Reset clears all sparkline history.
func (r *PSISparklineRow) Reset() {
	r.cpuSparkline.Clear()
	r.memSparkline.Clear()
	r.ioSparkline.Clear()
}

// GetCPUSparkline returns the underlying CPU stall sparkline (primarily for testing).
func (r *PSISparklineRow) GetCPUSparkline() *Sparkline {
	return r.cpuSparkline
}

// GetMEMSparkline returns the underlying Memory stall sparkline (primarily for testing).
func (r *PSISparklineRow) GetMEMSparkline() *Sparkline {
	return r.memSparkline
}

// GetIOSparkline returns the underlying I/O stall sparkline (primarily for testing).
func (r *PSISparklineRow) GetIOSparkline() *Sparkline {
	return r.ioSparkline
}

// Draw implements tview.Primitive.
func (r *PSISparklineRow) Draw(screen tcell.Screen) {
	r.Box.DrawForSubclass(screen, r)

	x, y, width, height := r.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	numCols := 3
	colWidth := width / numCols
	remainder := width % numCols

	// 3 columns: CPU Stall, MEM Stall, I/O Stall
	r.cpuSparkline.SetRect(x, y, colWidth, height)
	r.cpuSparkline.Draw(screen)

	r.memSparkline.SetRect(x+colWidth, y, colWidth, height)
	r.memSparkline.Draw(screen)

	// Last column gets remainder
	r.ioSparkline.SetRect(x+2*colWidth, y, colWidth+remainder, height)
	r.ioSparkline.Draw(screen)
}
