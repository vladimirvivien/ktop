package ui

import (
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFormatMemory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "nil quantity", input: "", expected: "   0Mi"},
		{name: "zero bytes", input: "0", expected: "   0Mi"},
		{name: "small value in Mi", input: "56Mi", expected: "  56Mi"},
		{name: "medium value in Mi", input: "366Mi", expected: " 366Mi"},
		{name: "large value in Mi", input: "512Mi", expected: " 512Mi"},
		{name: "exactly 1Gi", input: "1Gi", expected: "1024Mi"},
		{name: "1024Mi should stay as Mi", input: "1024Mi", expected: "1024Mi"},
		{name: "1404Mi should display as Mi", input: "1404Mi", expected: "1404Mi"},
		{name: "2Gi should display as Mi", input: "2Gi", expected: "2048Mi"},
		{name: "15Gi should display as Gi", input: "15Gi", expected: "  15Gi"},
		{name: "value in Ki should convert to Mi", input: "102400Ki", expected: " 100Mi"},
		{name: "value in bytes (1Gi)", input: "1073741824", expected: "1024Mi"},
		{name: "value in bytes (20Gi)", input: "21474836480", expected: "  20Gi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var qty *resource.Quantity
			if tt.input != "" {
				q := resource.MustParse(tt.input)
				qty = &q
			}

			result := FormatMemory(qty)
			if result != tt.expected {
				t.Errorf("FormatMemory(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatCPU(t *testing.T) {
	if got := FormatCPU(nil); got != "n/a" {
		t.Errorf("FormatCPU(nil) = %s, want n/a", got)
	}

	q := resource.MustParse("250m")
	if got := FormatCPU(&q); got != "250m" {
		t.Errorf("FormatCPU(250m) = %s, want 250m", got)
	}

	q2 := resource.MustParse("2")
	if got := FormatCPU(&q2); got != "2000m" {
		t.Errorf("FormatCPU(2) = %s, want 2000m", got)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{48 * time.Hour, "2d"},
	}
	for _, tt := range tests {
		if got := FormatDuration(tt.d); got != tt.expected {
			t.Errorf("FormatDuration(%v) = %s, want %s", tt.d, got, tt.expected)
		}
	}
}

func TestFormatEventAge(t *testing.T) {
	if got := FormatEventAge(corev1.Event{}); got != "?" {
		t.Errorf("FormatEventAge(empty) = %s, want ?", got)
	}

	event := corev1.Event{
		LastTimestamp: metav1.Time{Time: time.Now().Add(-10 * time.Minute)},
	}
	got := FormatEventAge(event)
	if got != "10m" && got != "9m" {
		t.Errorf("FormatEventAge(10m ago) = %s, want ~10m", got)
	}
}

func TestTruncateKey(t *testing.T) {
	if got := TruncateKey("kubernetes.io/hostname"); got != "hostname" {
		t.Errorf("TruncateKey prefix trim failed: %s", got)
	}
	longKey := "example.com/very-long-annotation-name-exceeding-limit"
	if got := TruncateKey(longKey); len(got) > 25 {
		t.Errorf("TruncateKey failed to truncate long key: len=%d, %s", len(got), got)
	}
}

func TestAddDetailRow(t *testing.T) {
	table := tview.NewTable()
	AddDetailRow(table, 0, "TestKey", "TestVal")
	AddDetailRowColor(table, 1, "ColorKey", "ColorVal", tcell.ColorGreen)
	cell0 := table.GetCell(0, 0)
	cell1 := table.GetCell(0, 1)
	if cell0 == nil || cell1 == nil {
		t.Fatalf("expected cells to be set")
	}
	if cell1.Text != "TestVal" {
		t.Errorf("cell value = %s, want TestVal", cell1.Text)
	}
	cell1_1 := table.GetCell(1, 1)
	if cell1_1 == nil || cell1_1.Color != tcell.ColorGreen {
		t.Errorf("cell color should be green")
	}
}

func TestAdaptiveRateMax(t *testing.T) {
	peak := float64(0)
	res := AdaptiveRateMax(100*1024, &peak, 512*1024)
	if res != 512*1024 {
		t.Errorf("expected baseline 512KB/s, got %v", res)
	}
	if peak != 100*1024 {
		t.Errorf("peak should be updated to 100KB/s, got %v", peak)
	}

	res2 := AdaptiveRateMax(1024*1024, &peak, 512*1024)
	if res2 != 1024*1024 || peak != 1024*1024 {
		t.Errorf("expected 1MB/s, got %v, peak %v", res2, peak)
	}
}

func TestCycleFocus(t *testing.T) {
	if got := CycleFocus(0, 3, false); got != 1 {
		t.Errorf("CycleFocus forward from 0 in 3 = %d, want 1", got)
	}
	if got := CycleFocus(2, 3, false); got != 0 {
		t.Errorf("CycleFocus forward wrap = %d, want 0", got)
	}
	if got := CycleFocus(0, 3, true); got != 2 {
		t.Errorf("CycleFocus reverse from 0 in 3 = %d, want 2", got)
	}
}
