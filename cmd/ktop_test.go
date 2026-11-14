package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vladimirvivien/ktop/config"
	"github.com/vladimirvivien/ktop/internal/userdir"
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
		{"theme", "", "auto"},
		{"color-mode", "", "auto"},
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

func TestNewKtopCmd_ThemeValidationFailsFast(t *testing.T) {
	t.Run("invalid theme flag", func(t *testing.T) {
		t.Setenv(userdir.EnvVar, t.TempDir())
		c := NewKtopCmd()
		c.SetArgs([]string{"--theme=solarized"})
		err := c.Execute()
		if err == nil || !strings.Contains(err.Error(), "invalid theme") {
			t.Fatalf("expected invalid theme error, got: %v", err)
		}
	})

	t.Run("invalid color-mode flag", func(t *testing.T) {
		t.Setenv(userdir.EnvVar, t.TempDir())
		c := NewKtopCmd()
		c.SetArgs([]string{"--color-mode=8bit"})
		err := c.Execute()
		if err == nil || !strings.Contains(err.Error(), "invalid color mode") {
			t.Fatalf("expected invalid color mode error, got: %v", err)
		}
	})

	t.Run("invalid ui.theme in config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(userdir.EnvVar, tmpDir)
		cfgPath := filepath.Join(tmpDir, config.ConfigFileName)
		if err := os.WriteFile(cfgPath, []byte("ui:\n  theme: neon\n"), 0o600); err != nil {
			t.Fatalf("write config file: %v", err)
		}

		c := NewKtopCmd()
		c.SetArgs([]string{})
		err := c.Execute()
		if err == nil || !strings.Contains(err.Error(), "invalid theme") {
			t.Fatalf("expected invalid theme error from config file, got: %v", err)
		}
	})
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
