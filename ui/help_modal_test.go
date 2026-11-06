package ui

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestHelpModal_RenderAndContext(t *testing.T) {
	tests := []struct {
		name          string
		cfg           HelpModalConfig
		expectBadges  []string
		expectContent []string
	}{
		{
			name: "overview nodes active",
			cfg: HelpModalConfig{
				CurrentPage:  "Overview",
				FocusedPanel: "nodes",
			},
			expectBadges:  []string{"(Active: Nodes)"},
			expectContent: []string{"GENERAL & NAVIGATION", "TABLE SORTING", "FILTERING & COLUMNS"},
		},
		{
			name: "overview pods active",
			cfg: HelpModalConfig{
				CurrentPage:  "Overview",
				FocusedPanel: "pods",
			},
			expectBadges: []string{"(Active: Pods)"},
		},
		{
			name: "overview header active",
			cfg: HelpModalConfig{
				CurrentPage:  "Overview",
				FocusedPanel: "header",
			},
			expectBadges: []string{"(Active: Namespace Header)"},
		},
		{
			name: "pod detail active",
			cfg: HelpModalConfig{
				CurrentPage:  "Pod Detail",
				FocusedPanel: "events",
			},
			expectBadges: []string{"(Active: Pod Detail)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modal := NewHelpModal(tt.cfg)
			if modal.GetRootView() == nil {
				t.Fatalf("expected non-nil root view")
			}
			if modal.GetFocusPrimitive() == nil {
				t.Fatalf("expected non-nil focus primitive")
			}

			text := modal.textView.GetText(true)
			for _, badge := range tt.expectBadges {
				if !strings.Contains(text, badge) {
					t.Errorf("expected text to contain badge %q, but was: %s", badge, text)
				}
			}
			for _, content := range tt.expectContent {
				if !strings.Contains(text, content) {
					t.Errorf("expected text to contain %q, but was: %s", content, text)
				}
			}
		})
	}
}

func TestHelpModal_HandleInput(t *testing.T) {
	tests := []struct {
		name        string
		event       *tcell.EventKey
		expectClose bool
	}{
		{
			name:        "escape closes",
			event:       tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone),
			expectClose: true,
		},
		{
			name:        "enter closes",
			event:       tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone),
			expectClose: true,
		},
		{
			name:        "F1 closes",
			event:       tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone),
			expectClose: true,
		},
		{
			name:        "question mark closes",
			event:       tcell.NewEventKey(tcell.KeyRune, '?', tcell.ModNone),
			expectClose: true,
		},
		{
			name:        "q closes",
			event:       tcell.NewEventKey(tcell.KeyRune, 'q', tcell.ModNone),
			expectClose: true,
		},
		{
			name:        "unhandled key does not close",
			event:       tcell.NewEventKey(tcell.KeyRune, 'x', tcell.ModNone),
			expectClose: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			closed := false
			modal := NewHelpModal(HelpModalConfig{
				OnClose: func() {
					closed = true
				},
			})

			res := modal.HandleInput(tt.event)
			if tt.expectClose {
				if !closed {
					t.Errorf("expected modal to close for event %v", tt.event)
				}
				if res != nil {
					t.Errorf("expected event to be consumed (nil), got %v", res)
				}
			} else {
				if closed {
					t.Errorf("did not expect modal to close for event %v", tt.event)
				}
				if res == nil {
					t.Errorf("expected unhandled event to pass through, got nil")
				}
			}
		})
	}
}
