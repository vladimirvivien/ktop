package config

import (
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/prom"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("Expected non-nil config")
	}

	// Verify source defaults
	if cfg.Source.Type != "metrics-server" {
		t.Errorf("Expected default source 'metrics-server', got '%s'", cfg.Source.Type)
	}

	// Verify Prometheus defaults
	if cfg.Prometheus.ScrapeInterval != 5*time.Second {
		t.Errorf("Expected default scrape interval 5s, got %v", cfg.Prometheus.ScrapeInterval)
	}

	if cfg.Prometheus.RetentionTime != 1*time.Hour {
		t.Errorf("Expected default retention time 1h, got %v", cfg.Prometheus.RetentionTime)
	}

	if cfg.Prometheus.MaxSamples != 10000 {
		t.Errorf("Expected default max samples 10000, got %d", cfg.Prometheus.MaxSamples)
	}

	// Verify default components
	expectedComponents := []prom.ComponentType{
		prom.ComponentKubelet,
		prom.ComponentCAdvisor,
	}

	if len(cfg.Prometheus.Components) != len(expectedComponents) {
		t.Errorf("Expected %d default components, got %d", len(expectedComponents), len(cfg.Prometheus.Components))
	}

	for i, expected := range expectedComponents {
		if cfg.Prometheus.Components[i] != expected {
			t.Errorf("Expected component %v at index %d, got %v", expected, i, cfg.Prometheus.Components[i])
		}
	}
}

func TestValidate_ValidMetricsServer(t *testing.T) {
	cfg := &Config{
		Source: SourceConfig{
			Type: "metrics-server",
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected valid metrics-server config, got error: %v", err)
	}
}

func TestValidate_ValidPrometheus(t *testing.T) {
	cfg := &Config{
		Source: SourceConfig{
			Type: "prometheus",
		},
		Prometheus: PrometheusConfig{
			ScrapeInterval: 15 * time.Second,
			RetentionTime:  1 * time.Hour,
			MaxSamples:     10000,
			Components: []prom.ComponentType{
				prom.ComponentKubelet,
				prom.ComponentCAdvisor,
			},
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected valid prometheus config, got error: %v", err)
	}
}

func TestValidate_InvalidSource(t *testing.T) {
	cfg := &Config{
		Source: SourceConfig{
			Type: "invalid-source",
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected error for invalid source type")
	}

	expectedMsg := "invalid metrics-source: invalid-source (must be 'metrics-server', 'prometheus', or 'none')"
	if err != nil && err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestValidate_InvalidScrapeInterval(t *testing.T) {
	cfg := &Config{
		Source: SourceConfig{
			Type: "prometheus",
		},
		Prometheus: PrometheusConfig{
			ScrapeInterval: 2 * time.Second, // Too short
			RetentionTime:  1 * time.Hour,
			MaxSamples:     10000,
			Components: []prom.ComponentType{
				prom.ComponentKubelet,
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected error for scrape interval < 5s")
	}

	if err != nil && err.Error() != "prometheus-scrape-interval must be >= 5s, got 2s" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidate_InvalidRetention(t *testing.T) {
	cfg := &Config{
		Source: SourceConfig{
			Type: "prometheus",
		},
		Prometheus: PrometheusConfig{
			ScrapeInterval: 15 * time.Second,
			RetentionTime:  2 * time.Minute, // Too short
			MaxSamples:     10000,
			Components: []prom.ComponentType{
				prom.ComponentKubelet,
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected error for retention < 5m")
	}

	if err != nil && err.Error() != "prometheus-retention must be >= 5m, got 2m0s" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidate_InvalidMaxSamples(t *testing.T) {
	cfg := &Config{
		Source: SourceConfig{
			Type: "prometheus",
		},
		Prometheus: PrometheusConfig{
			ScrapeInterval: 15 * time.Second,
			RetentionTime:  1 * time.Hour,
			MaxSamples:     50, // Too few
			Components: []prom.ComponentType{
				prom.ComponentKubelet,
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected error for max samples < 100")
	}

	if err != nil && err.Error() != "prometheus-max-samples must be >= 100, got 50" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidate_EmptyComponents(t *testing.T) {
	cfg := &Config{
		Source: SourceConfig{
			Type: "prometheus",
		},
		Prometheus: PrometheusConfig{
			ScrapeInterval: 15 * time.Second,
			RetentionTime:  1 * time.Hour,
			MaxSamples:     10000,
			Components:     []prom.ComponentType{}, // Empty
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Expected error for empty components")
	}

	if err != nil && err.Error() != "prometheus-components must not be empty" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestValidate_PrometheusFieldsIgnoredForMetricsServer(t *testing.T) {
	// When using metrics-server, prometheus config validation is skipped
	cfg := &Config{
		Source: SourceConfig{
			Type: "metrics-server",
		},
		Prometheus: PrometheusConfig{
			ScrapeInterval: 1 * time.Second, // Invalid, but should be ignored
			RetentionTime:  1 * time.Minute, // Invalid, but should be ignored
			MaxSamples:     10,              // Invalid, but should be ignored
			Components:     []prom.ComponentType{},
		},
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Expected validation to pass for metrics-server (prometheus config ignored), got error: %v", err)
	}
}

func TestParseComponents_Valid(t *testing.T) {
	testCases := []struct {
		name     string
		input    []string
		expected []prom.ComponentType
	}{
		{
			name:  "Single kubelet",
			input: []string{"kubelet"},
			expected: []prom.ComponentType{
				prom.ComponentKubelet,
			},
		},
		{
			name:  "All components",
			input: []string{"kubelet", "cadvisor", "apiserver", "etcd", "scheduler", "controller-manager", "kube-proxy"},
			expected: []prom.ComponentType{
				prom.ComponentKubelet,
				prom.ComponentCAdvisor,
				prom.ComponentAPIServer,
				prom.ComponentEtcd,
				prom.ComponentScheduler,
				prom.ComponentControllerManager,
				prom.ComponentKubeProxy,
			},
		},
		{
			name:  "Common subset",
			input: []string{"kubelet", "cadvisor", "apiserver"},
			expected: []prom.ComponentType{
				prom.ComponentKubelet,
				prom.ComponentCAdvisor,
				prom.ComponentAPIServer,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseComponents(tc.input)

			if err != nil {
				t.Fatalf("Expected no error, got: %v", err)
			}

			if len(result) != len(tc.expected) {
				t.Fatalf("Expected %d components, got %d", len(tc.expected), len(result))
			}

			for i, expected := range tc.expected {
				if result[i] != expected {
					t.Errorf("Expected component %v at index %d, got %v", expected, i, result[i])
				}
			}
		})
	}
}

func TestParseComponents_Invalid(t *testing.T) {
	testCases := []struct {
		name        string
		input       []string
		expectedErr string
	}{
		{
			name:        "Unknown component",
			input:       []string{"unknown"},
			expectedErr: "unknown component: unknown (valid: kubelet, cadvisor, apiserver, etcd, scheduler, controller-manager, kube-proxy)",
		},
		{
			name:        "Mixed valid and invalid",
			input:       []string{"kubelet", "invalid", "cadvisor"},
			expectedErr: "unknown component: invalid (valid: kubelet, cadvisor, apiserver, etcd, scheduler, controller-manager, kube-proxy)",
		},
		{
			name:        "Empty list",
			input:       []string{},
			expectedErr: "components list cannot be empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseComponents(tc.input)

			if err == nil {
				t.Fatalf("Expected error, got nil (result: %v)", result)
			}

			if err.Error() != tc.expectedErr {
				t.Errorf("Expected error '%s', got '%s'", tc.expectedErr, err.Error())
			}
		})
	}
}

func TestComponentsToStrings(t *testing.T) {
	testCases := []struct {
		name     string
		input    []prom.ComponentType
		expected []string
	}{
		{
			name: "All components",
			input: []prom.ComponentType{
				prom.ComponentKubelet,
				prom.ComponentCAdvisor,
				prom.ComponentAPIServer,
				prom.ComponentEtcd,
				prom.ComponentScheduler,
				prom.ComponentControllerManager,
				prom.ComponentKubeProxy,
			},
			expected: []string{"kubelet", "cadvisor", "apiserver", "etcd", "scheduler", "controller-manager", "kube-proxy"},
		},
		{
			name: "Subset",
			input: []prom.ComponentType{
				prom.ComponentKubelet,
				prom.ComponentCAdvisor,
			},
			expected: []string{"kubelet", "cadvisor"},
		},
		{
			name:     "Empty",
			input:    []prom.ComponentType{},
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ComponentsToStrings(tc.input)

			if len(result) != len(tc.expected) {
				t.Fatalf("Expected %d strings, got %d", len(tc.expected), len(result))
			}

			for i, expected := range tc.expected {
				if result[i] != expected {
					t.Errorf("Expected string '%s' at index %d, got '%s'", expected, i, result[i])
				}
			}
		})
	}
}

func TestParseComponents_RoundTrip(t *testing.T) {
	// Test that parsing and converting back yields the same result
	input := []string{"kubelet", "cadvisor", "apiserver"}

	components, err := ParseComponents(input)
	if err != nil {
		t.Fatalf("ParseComponents failed: %v", err)
	}

	output := ComponentsToStrings(components)

	if len(output) != len(input) {
		t.Fatalf("Expected %d strings after round trip, got %d", len(input), len(output))
	}

	for i, expected := range input {
		if output[i] != expected {
			t.Errorf("Round trip failed: expected '%s' at index %d, got '%s'", expected, i, output[i])
		}
	}
}
