package ui

import (
	"testing"
)

func TestFilterState_MatchesRow(t *testing.T) {
	tests := []struct {
		name     string
		filter   string
		cells    []string
		expected bool
	}{
		{
			name:     "empty filter matches all",
			filter:   "",
			cells:    []string{"nginx", "Running", "10.0.0.1"},
			expected: true,
		},
		{
			name:     "exact match",
			filter:   "nginx",
			cells:    []string{"nginx-deployment-abc123", "Running", "10.0.0.1"},
			expected: true,
		},
		{
			name:     "case insensitive match",
			filter:   "NGINX",
			cells:    []string{"nginx-deployment-abc123", "Running", "10.0.0.1"},
			expected: true,
		},
		{
			name:     "partial match",
			filter:   "dep",
			cells:    []string{"nginx-deployment-abc123", "Running", "10.0.0.1"},
			expected: true,
		},
		{
			name:     "no match",
			filter:   "redis",
			cells:    []string{"nginx-deployment-abc123", "Running", "10.0.0.1"},
			expected: false,
		},
		{
			name:     "match in status column",
			filter:   "run",
			cells:    []string{"nginx", "Running", "10.0.0.1"},
			expected: true,
		},
		{
			name:     "match IP address",
			filter:   "10.0",
			cells:    []string{"nginx", "Running", "10.0.0.1"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &FilterState{Text: tt.filter}
			result := f.MatchesRow(tt.cells)
			if result != tt.expected {
				t.Errorf("MatchesRow() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilterState_StateTransitions(t *testing.T) {
	f := &FilterState{}

	// Initial state
	if f.Active || f.Editing || f.Text != "" {
		t.Error("Initial state should be inactive")
	}

	// Start editing
	f.StartEditing()
	if !f.Editing || f.Active {
		t.Error("After StartEditing, should be editing but not active")
	}

	// Add text
	f.AppendChar('n')
	f.AppendChar('g')
	f.AppendChar('i')
	f.AppendChar('n')
	f.AppendChar('x')
	if f.Text != "nginx" {
		t.Errorf("Text = %q, want 'nginx'", f.Text)
	}

	// Backspace
	f.HandleBackspace()
	if f.Text != "ngin" {
		t.Errorf("After backspace, Text = %q, want 'ngin'", f.Text)
	}

	// Confirm
	f.AppendChar('x')
	f.Confirm()
	if !f.Active || f.Editing {
		t.Error("After Confirm, should be active but not editing")
	}

	// Clear
	f.Clear()
	if f.Active || f.Editing || f.Text != "" {
		t.Error("After Clear, should be fully reset")
	}
}

func TestFilterState_CancelBehavior(t *testing.T) {
	t.Run("cancel without active filter clears", func(t *testing.T) {
		f := &FilterState{}
		f.StartEditing()
		f.AppendChar('t')
		f.AppendChar('e')
		f.AppendChar('s')
		f.AppendChar('t')
		f.Cancel()

		if f.Active || f.Editing || f.Text != "" {
			t.Error("Cancel without active filter should clear everything")
		}
	})

	t.Run("cancel with active filter keeps it", func(t *testing.T) {
		f := &FilterState{}
		f.StartEditing()
		f.AppendChar('t')
		f.AppendChar('e')
		f.AppendChar('s')
		f.AppendChar('t')
		f.Confirm()

		// Now edit again and cancel
		f.StartEditing()
		f.AppendChar('2')
		f.Cancel()

		// Should keep the confirmed filter
		if !f.Active || f.Editing {
			t.Error("After Cancel with active filter, should still be active")
		}
	})
}

func TestFilterState_FormatTitle(t *testing.T) {
	tests := []struct {
		name     string
		state    FilterState
		base     string
		icon     string
		contains []string
	}{
		{
			name:     "no filter",
			state:    FilterState{TotalRows: 10, MatchRows: 10},
			base:     "Pods",
			icon:     "📦",
			contains: []string{"Pods", "(10)"},
		},
		{
			name:     "editing empty",
			state:    FilterState{Editing: true, TotalRows: 10, MatchRows: 10},
			base:     "Pods",
			icon:     "📦",
			contains: []string{"Filter:", "▌"},
		},
		{
			name:     "editing with text",
			state:    FilterState{Editing: true, Text: "nginx", TotalRows: 10, MatchRows: 3},
			base:     "Pods",
			icon:     "📦",
			contains: []string{"Filter:", "nginx", "3/10"},
		},
		{
			name:     "active filter",
			state:    FilterState{Active: true, Text: "nginx", TotalRows: 10, MatchRows: 3},
			base:     "Pods",
			icon:     "📦",
			contains: []string{"/nginx", "3/10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.state.FormatTitle(tt.base, tt.icon)
			for _, s := range tt.contains {
				if !containsString(result, s) {
					t.Errorf("FormatTitle() = %q, should contain %q", result, s)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
