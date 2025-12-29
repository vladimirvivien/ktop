package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SparklineRow displays multiple sparklines in a horizontal row.
// Used for CPU/MEM (and optionally NET/DISK in prometheus mode).
// Each Sparkline is self-sizing and adapts to its container during Draw().
type SparklineRow struct {
	*tview.Box

	cpuSparkline  *Sparkline
	memSparkline  *Sparkline
	netSparkline  *Sparkline // nil if not prometheus mode
	diskSparkline *Sparkline // nil if not prometheus mode

	prometheusMode bool
	colorKeys      ColorKeys
}

// NewSparklineRow creates a row of metric sparklines.
func NewSparklineRow(prometheusMode bool) *SparklineRow {
	colorKeys := DefaultColorKeys()

	r := &SparklineRow{
		Box:            tview.NewBox(),
		cpuSparkline:   NewSparkline().SetColorKeys(colorKeys),
		memSparkline:   NewSparkline().SetColorKeys(colorKeys),
		prometheusMode: prometheusMode,
		colorKeys:      colorKeys,
	}

	r.cpuSparkline.SetBorder(true).SetBorderColor(tcell.ColorLightGray).SetTitleAlign(tview.AlignCenter)
	r.memSparkline.SetBorder(true).SetBorderColor(tcell.ColorLightGray).SetTitleAlign(tview.AlignCenter)

	if prometheusMode {
		r.netSparkline = NewSparkline().SetColorKeys(colorKeys)
		r.diskSparkline = NewSparkline().SetColorKeys(colorKeys)
		r.netSparkline.SetBorder(true).SetBorderColor(tcell.ColorLightGray).SetTitleAlign(tview.AlignCenter)
		r.diskSparkline.SetBorder(true).SetBorderColor(tcell.ColorLightGray).SetTitleAlign(tview.AlignCenter)
	}

	return r
}

// SetPrometheusMode changes whether NET/DISK sparklines are shown.
func (r *SparklineRow) SetPrometheusMode(enabled bool) *SparklineRow {
	if enabled == r.prometheusMode {
		return r
	}
	r.prometheusMode = enabled
	if enabled && r.netSparkline == nil {
		r.netSparkline = NewSparkline().SetColorKeys(r.colorKeys)
		r.diskSparkline = NewSparkline().SetColorKeys(r.colorKeys)
		r.netSparkline.SetBorder(true).SetBorderColor(tcell.ColorLightGray).SetTitleAlign(tview.AlignCenter)
		r.diskSparkline.SetBorder(true).SetBorderColor(tcell.ColorLightGray).SetTitleAlign(tview.AlignCenter)
	}
	return r
}

// IsPrometheusMode returns whether prometheus mode is enabled.
func (r *SparklineRow) IsPrometheusMode() bool {
	return r.prometheusMode
}

// NumColumns returns number of sparkline columns (2 or 4).
func (r *SparklineRow) NumColumns() int {
	if r.prometheusMode {
		return 4
	}
	return 2
}

// UpdateCPU pushes CPU ratio and updates title.
func (r *SparklineRow) UpdateCPU(ratio float64, title string) {
	r.cpuSparkline.Push(ratio).SetTitle(title)
}

// UpdateMEM pushes memory ratio and updates title.
func (r *SparklineRow) UpdateMEM(ratio float64, title string) {
	r.memSparkline.Push(ratio).SetTitle(title)
}

// UpdateNET pushes network ratio and updates title (prometheus mode only).
func (r *SparklineRow) UpdateNET(ratio float64, title string) {
	if r.netSparkline != nil {
		r.netSparkline.Push(ratio).SetTitle(title)
	}
}

// UpdateDisk pushes disk ratio and updates title (prometheus mode only).
func (r *SparklineRow) UpdateDisk(ratio float64, title string) {
	if r.diskSparkline != nil {
		r.diskSparkline.Push(ratio).SetTitle(title)
	}
}

// CPUTrend returns CPU trend indicator.
func (r *SparklineRow) CPUTrend(percent float64) string {
	return r.cpuSparkline.TrendIndicator(percent)
}

// MEMTrend returns memory trend indicator.
func (r *SparklineRow) MEMTrend(percent float64) string {
	return r.memSparkline.TrendIndicator(percent)
}

// Reset clears all sparklines.
func (r *SparklineRow) Reset() {
	r.cpuSparkline.Clear()
	r.memSparkline.Clear()
	if r.netSparkline != nil {
		r.netSparkline.Clear()
	}
	if r.diskSparkline != nil {
		r.diskSparkline.Clear()
	}
}

// Draw implements tview.Primitive.
func (r *SparklineRow) Draw(screen tcell.Screen) {
	r.Box.DrawForSubclass(screen, r)

	x, y, width, height := r.GetInnerRect()
	if width <= 0 || height <= 0 {
		return
	}

	// Calculate column widths - distribute evenly with remainder to last column
	numCols := r.NumColumns()
	colWidth := width / numCols
	remainder := width % numCols

	if r.prometheusMode && r.netSparkline != nil {
		// 4 columns: CPU, MEM, NET, DISK
		r.cpuSparkline.SetRect(x, y, colWidth, height)
		r.cpuSparkline.Draw(screen)

		r.memSparkline.SetRect(x+colWidth, y, colWidth, height)
		r.memSparkline.Draw(screen)

		r.netSparkline.SetRect(x+2*colWidth, y, colWidth, height)
		r.netSparkline.Draw(screen)

		// Last column gets remainder
		r.diskSparkline.SetRect(x+3*colWidth, y, colWidth+remainder, height)
		r.diskSparkline.Draw(screen)
	} else {
		// 2 columns: CPU, MEM
		r.cpuSparkline.SetRect(x, y, colWidth, height)
		r.cpuSparkline.Draw(screen)

		// Last column gets remainder
		r.memSparkline.SetRect(x+colWidth, y, colWidth+remainder, height)
		r.memSparkline.Draw(screen)
	}
}
