package overview

import (
	"strings"
	"testing"

	"github.com/vladimirvivien/ktop/metrics"
)

func TestRenderStallCell(t *testing.T) {
	cases := []struct {
		name            string
		psi             metrics.PSIMetrics
		kernelSupported bool
		wantContains    []string
		wantNotContain  []string
	}{
		{
			name:            "zero PSI renders gray dot",
			psi:             metrics.PSIMetrics{},
			kernelSupported: true,
			wantContains:    []string{"[gray]·[-]"},
			wantNotContain:  []string{"⚠", "CPU", "MEM", "IO"},
		},
		{
			name:            "round-to-zero (< 0.01%) renders gray dot",
			psi:             metrics.PSIMetrics{CPUStallPct: 0.001},
			kernelSupported: true,
			wantContains:    []string{"[gray]·[-]"},
		},
		{
			name:            "sub-1% stall keeps 2 decimals",
			psi:             metrics.PSIMetrics{IOStallPct: 0.24},
			kernelSupported: true,
			wantContains:    []string{"[green]IO 0.24%"},
		},
		{
			name:            "CPU dominant ~1.5% → green, 1 decimal",
			psi:             metrics.PSIMetrics{CPUStallPct: 1.5, MemStallPct: 0.5},
			kernelSupported: true,
			wantContains:    []string{"[green]CPU 1.5%"},
		},
		{
			name:            "MEM dominant 7% → yellow",
			psi:             metrics.PSIMetrics{CPUStallPct: 1, MemStallPct: 7, IOStallPct: 3},
			kernelSupported: true,
			wantContains:    []string{"[yellow]MEM 7.0%"},
		},
		{
			name:            "IO dominant 18% → orange, integer",
			psi:             metrics.PSIMetrics{CPUStallPct: 5, IOStallPct: 18},
			kernelSupported: true,
			wantContains:    []string{"[orange]IO 18%"},
		},
		{
			name:            "severe pressure → red",
			psi:             metrics.PSIMetrics{CPUStallPct: 45},
			kernelSupported: true,
			wantContains:    []string{"[red]CPU 45%"},
		},
		{
			name:            "unsupported kernel adds warning glyph",
			psi:             metrics.PSIMetrics{CPUStallPct: 5},
			kernelSupported: false,
			wantContains:    []string{"[yellow]⚠ [-]", "CPU 5.0%"},
		},
		{
			name:            "unsupported kernel with zero PSI still warns",
			psi:             metrics.PSIMetrics{},
			kernelSupported: false,
			wantContains:    []string{"[yellow]⚠ [-]", "[gray]·[-]"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderStallCell(tc.psi, tc.kernelSupported)
			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("got %q, want to contain %q", got, want)
				}
			}
			for _, want := range tc.wantNotContain {
				if strings.Contains(got, want) {
					t.Errorf("got %q, should NOT contain %q", got, want)
				}
			}
		})
	}
}

func TestStallColorThresholds(t *testing.T) {
	cases := []struct {
		pct  float64
		want string
	}{
		{0.5, "green"},
		{1.99, "green"},
		{2.0, "yellow"},
		{9.99, "yellow"},
		{10.0, "orange"},
		{24.99, "orange"},
		{25.0, "red"},
		{99.0, "red"},
	}
	for _, tc := range cases {
		if got := stallColor(tc.pct); got != tc.want {
			t.Errorf("stallColor(%v) = %q, want %q", tc.pct, got, tc.want)
		}
	}
}
