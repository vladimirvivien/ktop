package cmd

import (
	"testing"
)

func TestNewKtopCmd_Flags(t *testing.T) {
	cmd := NewKtopCmd()

	flags := []struct {
		name      string
		shorthand string
		defVal    string
	}{
		{"all-namespaces", "A", "false"},
		{"node-columns", "", ""},
		{"pod-columns", "", ""},
		{"show-all-columns", "", "false"},
		{"sort-nodes", "", ""},
		{"sort-pods", "", ""},
		{"metrics-source", "", "prometheus"},
		{"prometheus-scrape-interval", "", "5s"},
		{"prometheus-retention", "", "1h"},
		{"prometheus-max-samples", "", "10000"},
		{"log-level", "", "info"},
		{"log-format", "", "text"},
	}

	for _, f := range flags {
		flag := cmd.Flags().Lookup(f.name)
		if flag == nil {
			t.Errorf("expected flag --%s to exist", f.name)
			continue
		}
		if f.shorthand != "" && flag.Shorthand != f.shorthand {
			t.Errorf("flag --%s expected shorthand -%s, got -%s", f.name, f.shorthand, flag.Shorthand)
		}
		if flag.DefValue != f.defVal {
			t.Errorf("flag --%s expected default '%s', got '%s'", f.name, f.defVal, flag.DefValue)
		}
	}
}

func TestParseSortFlag(t *testing.T) {
	tests := []struct {
		input       string
		expectedCol string
		expectedAsc bool
	}{
		{"", "", true},
		{"+NAME", "NAME", true},
		{"-CPU", "CPU", false},
		{"-Memory", "MEMORY", false},
		{"+Pod", "POD", true},
		{"Restarts", "RST", true},
		{"-Restarts", "RST", false},
	}

	for _, tt := range tests {
		col, asc := parseSortFlag(tt.input)
		if col != tt.expectedCol || asc != tt.expectedAsc {
			t.Errorf("parseSortFlag(%q) = (%q, %v), want (%q, %v)",
				tt.input, col, asc, tt.expectedCol, tt.expectedAsc)
		}
	}
}
