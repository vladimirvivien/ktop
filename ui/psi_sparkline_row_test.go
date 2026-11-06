package ui

import (
	"strings"
	"testing"

	"github.com/vladimirvivien/ktop/metrics"
)

func TestPSISparklineRow_Update(t *testing.T) {
	t.Run("supported kernel with valid stall percentages", func(t *testing.T) {
		row := NewPSISparklineRow()
		psi := metrics.PSIMetrics{
			CPUStallPct: 1.5,
			MemStallPct: 7.2,
			IOStallPct:  18.0,
		}

		row.Update(psi, true)

		cpuTitle := row.GetCPUSparkline().GetTitle()
		memTitle := row.GetMEMSparkline().GetTitle()
		ioTitle := row.GetIOSparkline().GetTitle()

		if !strings.Contains(cpuTitle, "CPU Stall") || !strings.Contains(cpuTitle, "1.5%") {
			t.Errorf("expected CPU Stall title with 1.5%%, got: %s", cpuTitle)
		}
		if !strings.Contains(memTitle, "MEM Stall") || !strings.Contains(memTitle, "7.2%") {
			t.Errorf("expected MEM Stall title with 7.2%%, got: %s", memTitle)
		}
		if !strings.Contains(ioTitle, "IO Stall") || !strings.Contains(ioTitle, "18%") {
			t.Errorf("expected IO Stall title with 18%%, got: %s", ioTitle)
		}
	})

	t.Run("unsupported kernel displays warning N/A", func(t *testing.T) {
		row := NewPSISparklineRow()
		psi := metrics.PSIMetrics{
			CPUStallPct: 1.5,
			MemStallPct: 7.2,
			IOStallPct:  18.0,
		}

		row.Update(psi, false)

		cpuTitle := row.GetCPUSparkline().GetTitle()
		memTitle := row.GetMEMSparkline().GetTitle()
		ioTitle := row.GetIOSparkline().GetTitle()

		for _, title := range []string{cpuTitle, memTitle, ioTitle} {
			if !strings.Contains(title, "⚠ N/A") {
				t.Errorf("expected title to contain '⚠ N/A', got: %s", title)
			}
		}
	})

	t.Run("reset clears sparkline data", func(t *testing.T) {
		row := NewPSISparklineRow()
		row.Update(metrics.PSIMetrics{CPUStallPct: 5.0}, true)
		row.Reset()

		for _, v := range row.GetCPUSparkline().state.Values() {
			if v != 0 {
				t.Errorf("expected CPU sparkline values to be 0 after reset, got %f", v)
			}
		}
	})
}
