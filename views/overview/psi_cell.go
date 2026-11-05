package overview

import (
	"fmt"

	"github.com/vladimirvivien/ktop/metrics"
)

// renderStallCell returns the rendered text for a STALL column cell.
//
// The dominant axis is the largest of the three "waiting" percentages
// (CPU/MEM/IO). Values below 0.01% render as a gray dot. Color thresholds:
//
//	   < 2%   green   — healthy
//	2 – 10%   yellow  — minor pressure
//	10 – 25%  orange  — meaningful pressure
//	  >= 25%  red     — severe pressure
//
// When kernelSupported is false (node Linux kernel < 4.20), the cell is
// prefixed with "⚠ " in yellow regardless of the value, because PSI data
// from those nodes is not reliable.
func renderStallCell(psi metrics.PSIMetrics, kernelSupported bool) string {
	cpu, mem, io := psi.CPUStallPct, psi.MemStallPct, psi.IOStallPct

	max := cpu
	label := "CPU"
	if mem > max {
		max = mem
		label = "MEM"
	}
	if io > max {
		max = io
		label = "IO"
	}

	var core string
	switch {
	case max < 0.01:
		core = "[gray]·[-]"
	case max < 1.0:
		// Two decimals so 0.24% doesn't collide with 0.04% visually.
		core = fmt.Sprintf("[%s]%s %.2f%%[-]", stallColor(max), label, max)
	case max < 10.0:
		core = fmt.Sprintf("[%s]%s %.1f%%[-]", stallColor(max), label, max)
	default:
		core = fmt.Sprintf("[%s]%s %.0f%%[-]", stallColor(max), label, max)
	}

	if !kernelSupported {
		return "[yellow]⚠ [-]" + core
	}
	return core
}

func stallColor(pct float64) string {
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
