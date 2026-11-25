package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/collector"
	"github.com/vladimirvivien/ktop/config"
	"github.com/vladimirvivien/ktop/internal/userdir"
	"github.com/vladimirvivien/ktop/prom"
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
		{"page", "", "overview"},
		{"metrics-source", "", "prometheus"},
		{"prometheus-scrape-interval", "", "5s"},
		{"prometheus-retention", "", "1h"},
		{"prometheus-max-samples", "", "10000"},
		{"log-level", "", "info"},
		{"log-format", "", "text"},
		{"log", "", "file"},
		{"noui", "", "false"},
		{"duration", "d", "0s"},
		{"interval", "i", "5s"},
		{"output", "o", "json"},
		{"quiet", "", "false"},
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

	t.Run("invalid ui.default_page in config file", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(userdir.EnvVar, tmpDir)
		cfgPath := filepath.Join(tmpDir, config.ConfigFileName)
		if err := os.WriteFile(cfgPath, []byte("ui:\n  default_page: unknown\n"), 0o600); err != nil {
			t.Fatalf("write config file: %v", err)
		}

		c := NewKtopCmd()
		c.SetArgs([]string{})
		err := c.Execute()
		if err == nil || !strings.Contains(err.Error(), "invalid page") {
			t.Fatalf("expected invalid page error from config file, got: %v", err)
		}
		if got := ExitCode(err); got != ExitInvalidFlags {
			t.Fatalf("ExitCode(%v) = %d, want %d", err, got, ExitInvalidFlags)
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

func TestNewKtopCmd_InvalidFlagsExitCode2(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"--does-not-exist"}},
		{name: "unexpected positional argument", args: []string{"extra-arg"}},
		{name: "invalid output format", args: []string{"--noui", "-o", "xml"}},
		{name: "negative duration", args: []string{"--noui", "-d", "-5s"}},
		{name: "zero interval", args: []string{"--noui", "-i", "0s"}},
		{name: "negative interval", args: []string{"--noui", "-i", "-1s"}},
		{name: "invalid log destination", args: []string{"--log=stdout"}},
		{name: "invalid log level", args: []string{"--log-level=trace"}},
		{name: "invalid log format", args: []string{"--log-format=yaml"}},
		{name: "invalid theme", args: []string{"--theme=solarized"}},
		{name: "invalid page", args: []string{"--page=unknown"}},
		{name: "invalid metrics source", args: []string{"--metrics-source=bogus"}},
		{name: "invalid request timeout", args: []string{"--noui", "--request-timeout=notaduration"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(userdir.EnvVar, t.TempDir())
			c := NewKtopCmd()
			c.SetArgs(tc.args)
			err := c.Execute()
			if err == nil {
				t.Fatalf("expected error for args %v", tc.args)
			}
			if got := ExitCode(err); got != ExitInvalidFlags {
				t.Errorf("ExitCode(%v) = %d, want %d (args=%v)", err, got, ExitInvalidFlags, tc.args)
			}
		})
	}
}

func TestExitCodeMapping(t *testing.T) {
	if got := ExitCode(nil); got != ExitOK {
		t.Errorf("ExitCode(nil) = %d, want %d", got, ExitOK)
	}
	if got := ExitCode(errors.New("boom")); got != ExitGeneralError {
		t.Errorf("ExitCode(generic) = %d, want %d", got, ExitGeneralError)
	}
	if got := ExitCode(collector.ErrTimeout); got != ExitCollectionTimeout {
		t.Errorf("ExitCode(ErrTimeout) = %d, want %d", got, ExitCollectionTimeout)
	}
	if got := ExitCode(context.DeadlineExceeded); got != ExitCollectionTimeout {
		t.Errorf("ExitCode(DeadlineExceeded) = %d, want %d", got, ExitCollectionTimeout)
	}
	wrapped := WrapExitError(ExitK8sConnectError, WrapExitError(ExitMetricsUnavailable, errors.New("prom down")))
	if got := ExitCode(wrapped); got != ExitMetricsUnavailable {
		t.Errorf("ExitCode(nested WrapExitError) = %d, want %d", got, ExitMetricsUnavailable)
	}
}

func TestResolveConfig_AppliesAppConfigAndCLIOverrides(t *testing.T) {
	t.Run("applies prometheus settings from config file", func(t *testing.T) {
		c := NewKtopCmd()
		if err := c.ParseFlags([]string{}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		appCfg := &config.AppConfig{
			Metrics: config.MetricsConfig{
				Source:         "prometheus",
				ScrapeInterval: "15s",
				Retention:      "10m",
				MaxSamples:     300,
				Components:     []string{"kubelet"},
			},
		}
		opts := &ktopCmdOptions{
			metricsSource:            appCfg.Metrics.Source,
			prometheusScrapeInterval: appCfg.Metrics.ScrapeInterval,
			prometheusRetention:      appCfg.Metrics.Retention,
			prometheusMaxSamples:     appCfg.Metrics.MaxSamples,
			prometheusComponents:     appCfg.Metrics.Components,
		}

		cfg, err := resolveConfig(c, opts, appCfg)
		if err != nil {
			t.Fatalf("resolveConfig failed: %v", err)
		}
		if cfg.Prometheus.ScrapeInterval != 15*time.Second {
			t.Errorf("ScrapeInterval = %v, want 15s", cfg.Prometheus.ScrapeInterval)
		}
		if cfg.Prometheus.RetentionTime != 10*time.Minute {
			t.Errorf("RetentionTime = %v, want 10m", cfg.Prometheus.RetentionTime)
		}
		if cfg.Prometheus.MaxSamples != 300 {
			t.Errorf("MaxSamples = %d, want 300", cfg.Prometheus.MaxSamples)
		}
		if len(cfg.Prometheus.Components) != 1 || cfg.Prometheus.Components[0] != prom.ComponentKubelet {
			t.Errorf("Components = %v, want [kubelet]", cfg.Prometheus.Components)
		}
	})

	t.Run("cli flags override config file settings", func(t *testing.T) {
		c := NewKtopCmd()
		if err := c.ParseFlags([]string{
			"--prometheus-scrape-interval=30s",
			"--prometheus-retention=20m",
			"--prometheus-max-samples=500",
		}); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		appCfg := &config.AppConfig{
			Metrics: config.MetricsConfig{
				Source:         "prometheus",
				ScrapeInterval: "15s",
				Retention:      "10m",
				MaxSamples:     300,
				Components:     []string{"kubelet", "cadvisor"},
			},
		}
		opts := &ktopCmdOptions{
			metricsSource:            "prometheus",
			prometheusScrapeInterval: "30s",
			prometheusRetention:      "20m",
			prometheusMaxSamples:     500,
			prometheusComponents:     appCfg.Metrics.Components,
		}

		cfg, err := resolveConfig(c, opts, appCfg)
		if err != nil {
			t.Fatalf("resolveConfig failed: %v", err)
		}
		if cfg.Prometheus.ScrapeInterval != 30*time.Second {
			t.Errorf("ScrapeInterval = %v, want 30s", cfg.Prometheus.ScrapeInterval)
		}
		if cfg.Prometheus.RetentionTime != 20*time.Minute {
			t.Errorf("RetentionTime = %v, want 20m", cfg.Prometheus.RetentionTime)
		}
		if cfg.Prometheus.MaxSamples != 500 {
			t.Errorf("MaxSamples = %d, want 500", cfg.Prometheus.MaxSamples)
		}
	})

	t.Run("invalid prometheus settings in config file fail fast", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv(userdir.EnvVar, tmpDir)
		cfgPath := filepath.Join(tmpDir, config.ConfigFileName)
		content := "metrics:\n  source: prometheus\n  max_samples: 50\n"
		if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
			t.Fatalf("write config file: %v", err)
		}

		c := NewKtopCmd()
		c.SetArgs([]string{})
		err := c.Execute()
		if err == nil || !strings.Contains(err.Error(), "prometheus-max-samples must be >= 100") {
			t.Fatalf("expected invalid max_samples error from config file, got: %v", err)
		}
		if got := ExitCode(err); got != ExitInvalidFlags {
			t.Fatalf("ExitCode(%v) = %d, want %d", err, got, ExitInvalidFlags)
		}
	})
}
