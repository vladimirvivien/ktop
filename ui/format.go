package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/metrics"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// FormatMemory formats a memory quantity in the most appropriate unit (Mi or Gi)
// Uses Gi only when >= 10Gi to match kubectl behavior, otherwise uses Mi
// Returns fixed-width string for consistent column alignment
func FormatMemory(qty *resource.Quantity) string {
	if qty == nil {
		return "   0Mi"
	}

	bytes := qty.Value()
	if bytes == 0 {
		return "   0Mi"
	}

	// Calculate Mi (binary)
	mi := bytes / (1024 * 1024)

	// Only use Gi for very large values (>= 10240 Mi = 10 Gi)
	// This matches kubectl's behavior of preferring Mi for smaller values
	if mi >= 10240 {
		gi := bytes / (1024 * 1024 * 1024)
		return fmt.Sprintf("%4dGi", gi) // Fixed width: 4 digits + "Gi"
	}

	// Display in Mi for everything else
	return fmt.Sprintf("%4dMi", mi) // Fixed width: 4 digits + "Mi"
}

// FormatCPU formats a CPU quantity in millicores (e.g., "250m" or "n/a")
func FormatCPU(q *resource.Quantity) string {
	if q == nil {
		return "n/a"
	}
	return fmt.Sprintf("%dm", q.MilliValue())
}

// FormatDuration formats a duration into a concise human-readable string (e.g. "45s", "12m", "3h", "5d")
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// FormatEventAge returns human-readable age of an event (e.g. "5m", "2h", "3d" or "?")
func FormatEventAge(event corev1.Event) string {
	var eventTime time.Time
	if !event.LastTimestamp.IsZero() {
		eventTime = event.LastTimestamp.Time
	} else if !event.EventTime.IsZero() {
		eventTime = event.EventTime.Time
	} else if !event.FirstTimestamp.IsZero() {
		eventTime = event.FirstTimestamp.Time
	} else {
		return "?"
	}
	return FormatDuration(time.Since(eventTime))
}

// TruncateKey shortens a Kubernetes label or annotation key for compact display
func TruncateKey(key string) string {
	key = strings.TrimPrefix(key, "kubernetes.io/")
	key = strings.TrimPrefix(key, "node.kubernetes.io/")
	key = strings.TrimPrefix(key, "topology.kubernetes.io/")
	if len(key) > 25 {
		key = key[:22] + "..."
	}
	return key
}

// AddDetailRow adds a key-value row with default secondary text color to a detail table.
func AddDetailRow(table *tview.Table, row int, key, value string) {
	AddDetailRowColor(table, row, key, value, ActivePalette().DataSecondary)
}

// AddDetailRowColor adds a key-value row with custom value color to a detail table.
func AddDetailRowColor(table *tview.Table, row int, key, value string, color tcell.Color) {
	p := ActivePalette()
	keyColor := p.StatusMuted
	valColor := color
	if IsMonochrome() {
		keyColor = tcell.ColorDefault
		valColor = tcell.ColorDefault
	}
	paddedKey := fmt.Sprintf("%-10s", key)
	table.SetCell(row, 0, NewDataCell(paddedKey).SetTextColor(keyColor).SetSelectable(false))
	table.SetCell(row, 1, NewDataCell(value).SetTextColor(valColor).SetSelectable(false))
}

// AdaptiveRateMax updates a running peak rate and returns an effective maximum
// rate scaled with a minimum baseline (default 512 KB/s).
func AdaptiveRateMax(currentRate float64, peakRate *float64, minBaseline float64) float64 {
	if minBaseline <= 0 {
		minBaseline = 512 * 1024
	}
	if currentRate > *peakRate {
		*peakRate = currentRate
	}
	effectiveMax := *peakRate
	if effectiveMax < minBaseline {
		effectiveMax = minBaseline
	}
	return effectiveMax
}

// CycleFocus moves focus index forward or backward within a slice of items
func CycleFocus(currentIdx int, count int, reverse bool) int {
	if count <= 0 {
		return 0
	}
	if reverse {
		next := currentIdx - 1
		if next < 0 {
			next = count - 1
		}
		return next
	}
	return (currentIdx + 1) % count
}

// FormatBytesRate formats bytes/sec as human-readable (e.g., "1.2M/s")
// Uses K/M/G suffixes for compact display
func FormatBytesRate(bytesPerSec float64) string {
	if bytesPerSec < 0 {
		bytesPerSec = 0
	}
	if bytesPerSec < 1024 {
		return fmt.Sprintf("%.0fB/s", bytesPerSec)
	} else if bytesPerSec < 1024*1024 {
		return fmt.Sprintf("%.1fK/s", bytesPerSec/1024)
	} else if bytesPerSec < 1024*1024*1024 {
		return fmt.Sprintf("%.1fM/s", bytesPerSec/(1024*1024))
	}
	return fmt.Sprintf("%.1fG/s", bytesPerSec/(1024*1024*1024))
}

// FormatBytes formats bytes as human-readable (e.g., "1.2G")
// Uses K/M/G suffixes for compact display
func FormatBytes(bytes int64) string {
	if bytes < 0 {
		bytes = 0
	}
	if bytes < 1024 {
		return fmt.Sprintf("%dB", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1fK", float64(bytes)/1024)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1fM", float64(bytes)/(1024*1024))
	}
	return fmt.Sprintf("%.1fG", float64(bytes)/(1024*1024*1024))
}

// Truncate truncates a string to max length, adding "..." if truncated
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// FormatPSIInfo formats PSI pressure stall metrics for display in detail view info bars.
func FormatPSIInfo(psi metrics.PSIMetrics, kernelSupported bool) string {
	p := ActivePalette()
	mono := IsMonochrome()

	if !kernelSupported {
		if mono {
			return "PSI: [::b]⚠ N/A[::-]"
		}
		return fmt.Sprintf("[%s]PSI:[%s] ⚠ N/A[-]", TagFg(p.StatusMuted), TagFg(p.StatusWarning))
	}

	formatVal := func(pct float64) string {
		if mono {
			switch {
			case pct < 0.01:
				return "0%"
			case pct < 1.0:
				return fmt.Sprintf("%.2f%%", pct)
			case pct < 10.0:
				return fmt.Sprintf("%.1f%%", pct)
			case pct >= 25.0:
				return ErrorMarkup(fmt.Sprintf("%.0f%%", pct))
			default:
				return fmt.Sprintf("%.0f%%", pct)
			}
		}
		switch {
		case pct < 0.01:
			return fmt.Sprintf("[%s]0%%[-]", TagFg(p.StatusMuted))
		case pct < 1.0:
			return fmt.Sprintf("[%s]%.2f%%[-]", StallColor(pct), pct)
		case pct < 10.0:
			return fmt.Sprintf("[%s]%.1f%%[-]", StallColor(pct), pct)
		default:
			return fmt.Sprintf("[%s]%.0f%%[-]", StallColor(pct), pct)
		}
	}

	if mono {
		return fmt.Sprintf("PSI: CPU %s MEM %s IO %s",
			formatVal(psi.CPUStallPct),
			formatVal(psi.MemStallPct),
			formatVal(psi.IOStallPct),
		)
	}

	valTag := TagFg(p.DataSecondary)
	return fmt.Sprintf("[%s]PSI:[%s] CPU %s [%s]MEM %s [%s]IO %s",
		TagFg(p.StatusMuted),
		valTag, formatVal(psi.CPUStallPct),
		valTag, formatVal(psi.MemStallPct),
		valTag, formatVal(psi.IOStallPct),
	)
}

// StallColor returns the color tag for a given stall percentage.
func StallColor(pct float64) string {
	p := ActivePalette()
	switch {
	case pct < 2:
		return TagFg(p.GaugeLow)
	case pct < 10:
		return TagFg(p.GaugeMedium)
	case pct < 25:
		return TagFg(p.GaugeElevated)
	default:
		return TagFg(p.GaugeHigh)
	}
}

// FormatThrottleCell formats a CFS CPU throttling percentage for table cells.
func FormatThrottleCell(pct float64) string {
	p := ActivePalette()
	if IsMonochrome() {
		switch {
		case pct < 0.01:
			return "·"
		case pct < 1.0:
			return fmt.Sprintf("%.2f%%", pct)
		case pct < 10.0:
			return fmt.Sprintf("%.1f%%", pct)
		case pct >= 25.0:
			return ErrorMarkup(fmt.Sprintf("%.0f%%", pct))
		default:
			return fmt.Sprintf("%.0f%%", pct)
		}
	}

	switch {
	case pct < 0.01:
		return fmt.Sprintf("[%s]·[-]", TagFg(p.StatusMuted))
	case pct < 1.0:
		return fmt.Sprintf("[%s]%.2f%%[-]", StallColor(pct), pct)
	case pct < 10.0:
		return fmt.Sprintf("[%s]%.1f%%[-]", StallColor(pct), pct)
	default:
		return fmt.Sprintf("[%s]%.0f%%[-]", StallColor(pct), pct)
	}
}

// FormatHealthScore formats the 0-100 cluster health score percentage with
// threshold color coding.
func FormatHealthScore(score int) string {
	p := ActivePalette()
	if IsMonochrome() {
		if score < 50 {
			return ErrorMarkup(fmt.Sprintf("%d%%", score))
		}
		return fmt.Sprintf("%d%%", score)
	}
	c := p.GaugeHigh
	switch {
	case score >= 90:
		c = p.GaugeLow
	case score >= 70:
		c = p.GaugeMedium
	case score >= 50:
		c = p.GaugeElevated
	}
	return fmt.Sprintf("[%s]%d%%[-]", TagFg(c), score)
}

// FormatEfficiencyCell formats a workload resource efficiency percentage using
// capacity-sizing thresholds:
//
//	<25%   -> error (severely over-provisioned)
//	25-50% -> warning (over-provisioned)
//	50-90% -> ok (well-sized)
//	90-100%-> elevated/orange (at-limit risk)
//	>100%  -> error (throttled / OOM risk)
func FormatEfficiencyCell(effPct float64, hasEff bool) string {
	p := ActivePalette()
	if !hasEff {
		if IsMonochrome() {
			return "n/a"
		}
		return fmt.Sprintf("[%s]n/a[-]", TagFg(p.StatusMuted))
	}

	pctText := fmt.Sprintf("%.0f%%", effPct)
	if IsMonochrome() {
		if effPct < 25 || effPct > 100 {
			return ErrorMarkup(pctText)
		}
		if effPct < 50 || effPct >= 90 {
			return "[::b]" + pctText + "[::-]"
		}
		return pctText
	}

	var c tcell.Color
	switch {
	case effPct > 100:
		c = p.StatusError
	case effPct >= 90:
		c = p.GaugeElevated
	case effPct >= 50:
		c = p.StatusOK
	case effPct >= 25:
		c = p.StatusWarning
	default:
		c = p.StatusError
	}
	return fmt.Sprintf("[%s]%s[-]", TagFg(c), pctText)
}
