package ui

import (
	"fmt"
	"strings"
)

// FilterState manages text filtering for a panel
type FilterState struct {
	Active    bool   // Filter is confirmed and active
	Editing   bool   // Currently typing in filter
	Text      string // Current filter text
	TotalRows int    // Total rows before filtering
	MatchRows int    // Rows matching filter
}

// IsFiltering returns true if filter is active or being edited
func (f *FilterState) IsFiltering() bool {
	return f.Active || f.Editing
}

// Clear resets the filter state completely
func (f *FilterState) Clear() {
	f.Active = false
	f.Editing = false
	f.Text = ""
	// MatchRows will be reset on next draw when all rows match
}

// StartEditing begins filter input mode
func (f *FilterState) StartEditing() {
	f.Editing = true
}

// Confirm finalizes the filter
func (f *FilterState) Confirm() {
	if f.Text != "" {
		f.Active = true
	}
	f.Editing = false
}

// Cancel exits editing mode
func (f *FilterState) Cancel() {
	if f.Active {
		// Keep existing filter
		f.Editing = false
	} else {
		// Clear everything
		f.Clear()
	}
}

// FormatTitle formats a panel title with filter state indicator
// baseTitle: the panel name (e.g., "Nodes", "Pods")
// icon: the panel icon
// Returns formatted title string
func (f *FilterState) FormatTitle(baseTitle, icon string) string {
	if f.Editing {
		if f.Text == "" {
			return fmt.Sprintf(" %s %s [yellow][Filter: ▌][-] (%d/%d) ",
				icon, baseTitle, f.MatchRows, f.TotalRows)
		}
		return fmt.Sprintf(" %s %s [yellow][Filter: %s▌][-] (%d/%d) ",
			icon, baseTitle, f.Text, f.MatchRows, f.TotalRows)
	}
	if f.Active {
		return fmt.Sprintf(" %s %s [green][/%s][-] (%d/%d) ",
			icon, baseTitle, f.Text, f.MatchRows, f.TotalRows)
	}
	return fmt.Sprintf(" %s %s (%d) ", icon, baseTitle, f.TotalRows)
}

// FormatTitleWithScroll formats a panel title with filter state and scroll indicators
func (f *FilterState) FormatTitleWithScroll(baseTitle, icon string, firstVisible, lastVisible, totalRows int, scrollIndicator, disconnectedSuffix string) string {
	if f.Editing {
		if f.Text == "" {
			return fmt.Sprintf(" %s %s [yellow][Filter: ▌][-] (%d/%d)%s%s ",
				icon, baseTitle, f.MatchRows, f.TotalRows, scrollIndicator, disconnectedSuffix)
		}
		return fmt.Sprintf(" %s %s [yellow][Filter: %s▌][-] (%d/%d)%s%s ",
			icon, baseTitle, f.Text, f.MatchRows, f.TotalRows, scrollIndicator, disconnectedSuffix)
	}
	if f.Active && f.Text != "" {
		return fmt.Sprintf(" %s %s [green][/%s][-] (%d/%d)%s%s ",
			icon, baseTitle, f.Text, f.MatchRows, f.TotalRows, scrollIndicator, disconnectedSuffix)
	}

	// No filter - use standard format (original title)
	if totalRows == 0 || scrollIndicator == "" {
		// All content visible or no content - simple count
		return fmt.Sprintf(" %s %s (%d)%s ", icon, baseTitle, totalRows, disconnectedSuffix)
	}

	// Show scroll position
	return fmt.Sprintf(" %s %s (%d-%d/%d)%s%s ",
		icon, baseTitle, firstVisible, lastVisible, totalRows, scrollIndicator, disconnectedSuffix)
}

// MatchesRow checks if any cell in the row contains the filter text (case-insensitive)
func (f *FilterState) MatchesRow(cells []string) bool {
	if f.Text == "" {
		return true
	}
	filterLower := strings.ToLower(f.Text)
	for _, cell := range cells {
		if strings.Contains(strings.ToLower(cell), filterLower) {
			return true
		}
	}
	return false
}

// HandleBackspace removes the last character from the filter text
func (f *FilterState) HandleBackspace() bool {
	if len(f.Text) > 0 {
		f.Text = f.Text[:len(f.Text)-1]
		return true
	}
	return false
}

// AppendChar adds a character to the filter text
func (f *FilterState) AppendChar(ch rune) {
	f.Text += string(ch)
}

// HasEscapableState returns true if there's state that ESC should clear
// (either editing mode or active filter)
func (f *FilterState) HasEscapableState() bool {
	return f.Editing || f.Active
}
