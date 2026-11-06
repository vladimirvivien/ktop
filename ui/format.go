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

// AddDetailRow adds a key-value row with default white text to a detail table
func AddDetailRow(table *tview.Table, row int, key, value string) {
	AddDetailRowColor(table, row, key, value, tcell.ColorWhite)
}

// AddDetailRowColor adds a key-value row with custom value color to a detail table
func AddDetailRowColor(table *tview.Table, row int, key, value string, color tcell.Color) {
	paddedKey := fmt.Sprintf("%-10s", key)
	table.SetCell(row, 0, tview.NewTableCell(paddedKey).SetTextColor(tcell.ColorGray).SetSelectable(false))
	table.SetCell(row, 1, tview.NewTableCell(value).SetTextColor(color).SetSelectable(false))
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

// FormatPSIInfo formats PSI pressure stall metrics for display in detail view info bars
func FormatPSIInfo(psi metrics.PSIMetrics, kernelSupported bool) string {
	if !kernelSupported {
		return "[gray]PSI:[yellow] ⚠ N/A[-]"
	}

	formatVal := func(pct float64) string {
		switch {
		case pct < 0.01:
			return "[gray]0%[-]"
		case pct < 1.0:
			return fmt.Sprintf("[%s]%.2f%%[-]", StallColor(pct), pct)
		case pct < 10.0:
			return fmt.Sprintf("[%s]%.1f%%[-]", StallColor(pct), pct)
		default:
			return fmt.Sprintf("[%s]%.0f%%[-]", StallColor(pct), pct)
		}
	}

	return fmt.Sprintf("[gray]PSI:[white] CPU %s [white]MEM %s [white]IO %s",
		formatVal(psi.CPUStallPct),
		formatVal(psi.MemStallPct),
		formatVal(psi.IOStallPct),
	)
}

// StallColor returns the color name for a given stall percentage
func StallColor(pct float64) string {
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
