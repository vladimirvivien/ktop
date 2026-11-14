package overview

import (
	"fmt"

	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
)

// renderStallCell returns the rendered text for a STALL column cell.
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

	p := ui.ActivePalette()
	mono := ui.IsMonochrome()

	var core string
	if mono {
		switch {
		case max < 0.01:
			core = "·"
		case max < 1.0:
			core = fmt.Sprintf("%s %.2f%%", label, max)
		case max < 10.0:
			core = fmt.Sprintf("%s %.1f%%", label, max)
		case max >= 25.0:
			core = ui.ErrorMarkup(fmt.Sprintf("%s %.0f%%", label, max))
		default:
			core = fmt.Sprintf("%s %.0f%%", label, max)
		}
		if !kernelSupported {
			return "[::b]⚠ [::-]" + core
		}
		return core
	}

	switch {
	case max < 0.01:
		core = fmt.Sprintf("[%s]·[-]", ui.TagFg(p.StatusMuted))
	case max < 1.0:
		// Two decimals so 0.24% doesn't collide with 0.04% visually.
		core = fmt.Sprintf("[%s]%s %.2f%%[-]", stallColor(max), label, max)
	case max < 10.0:
		core = fmt.Sprintf("[%s]%s %.1f%%[-]", stallColor(max), label, max)
	default:
		core = fmt.Sprintf("[%s]%s %.0f%%[-]", stallColor(max), label, max)
	}

	if !kernelSupported {
		return fmt.Sprintf("[%s]⚠ [-]%s", ui.TagFg(p.StatusWarning), core)
	}
	return core
}

func stallColor(pct float64) string {
	return ui.StallColor(pct)
}
