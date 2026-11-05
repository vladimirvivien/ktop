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
