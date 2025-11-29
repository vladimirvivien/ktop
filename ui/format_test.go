package ui

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestFormatMemory(t *testing.T) {
	// FormatMemory returns fixed-width strings (4 digits + unit) for column alignment
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "nil quantity",
			input:    "",
			expected: "   0Mi",
		},
		{
			name:     "zero bytes",
			input:    "0",
			expected: "   0Mi",
		},
		{
			name:     "small value in Mi",
			input:    "56Mi",
			expected: "  56Mi",
		},
		{
			name:     "medium value in Mi",
			input:    "366Mi",
			expected: " 366Mi",
		},
		{
			name:     "large value in Mi",
			input:    "512Mi",
			expected: " 512Mi",
		},
		{
			name:     "exactly 1Gi",
			input:    "1Gi",
			expected: "1024Mi",
		},
		{
			name:     "1024Mi should stay as Mi",
			input:    "1024Mi",
			expected: "1024Mi",
		},
		{
			name:     "1404Mi should display as Mi",
			input:    "1404Mi",
			expected: "1404Mi",
		},
		{
			name:     "2Gi should display as Mi",
			input:    "2Gi",
			expected: "2048Mi",
		},
		{
			name:     "15Gi should display as Gi",
			input:    "15Gi",
			expected: "  15Gi",
		},
		{
			name:     "value in Ki should convert to Mi",
			input:    "102400Ki", // 100Mi
			expected: " 100Mi",
		},
		{
			name:     "value in bytes (1Gi)",
			input:    "1073741824", // 1Gi in bytes
			expected: "1024Mi",
		},
		{
			name:     "value in bytes (20Gi)",
			input:    "21474836480", // 20Gi in bytes
			expected: "  20Gi",
		},
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
