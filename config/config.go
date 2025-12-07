package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/vladimirvivien/ktop/prom"
)

// ValidMetricsSources maps input values to canonical names
var ValidMetricsSources = map[string]string{
	"prom":           "prometheus",
	"prometheus":     "prometheus",
	"metrics-server": "metrics-server",
	"none":           "none",
}

// NormalizeMetricsSource converts aliases to canonical names
func NormalizeMetricsSource(source string) (string, error) {
	normalized, ok := ValidMetricsSources[strings.ToLower(source)]
	if !ok {
		return "", fmt.Errorf("invalid metrics source: %s (valid: prom, prometheus, metrics-server, none)", source)
	}
	return normalized, nil
}

// Config holds the complete application configuration
type Config struct {
	Source     SourceConfig
	Prometheus PrometheusConfig
}

// SourceConfig defines which metrics source to use
type SourceConfig struct {
	Type string // "metrics-server" | "prometheus" | "none"
}

// PrometheusConfig holds Prometheus-specific settings
type PrometheusConfig struct {
	ScrapeInterval time.Duration
	RetentionTime  time.Duration
	MaxSamples     int
	Components     []prom.ComponentType
}

// DefaultConfig returns the default configuration
// Default source is prometheus with automatic fallback to metrics-server then none
func DefaultConfig() *Config {
	return &Config{
		Source: SourceConfig{
			Type: "prometheus",
		},
		Prometheus: PrometheusConfig{
			ScrapeInterval: 5 * time.Second,
			RetentionTime:  1 * time.Hour,
			MaxSamples:     10000,
			Components: []prom.ComponentType{
				prom.ComponentKubelet,
				prom.ComponentCAdvisor,
			},
		},
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate and normalize source type
	normalized, err := NormalizeMetricsSource(c.Source.Type)
	if err != nil {
		return err
	}
	c.Source.Type = normalized

	// Validate Prometheus config if prometheus source is selected
	if c.Source.Type == "prometheus" {
		if c.Prometheus.ScrapeInterval < 5*time.Second {
			return fmt.Errorf("prometheus-scrape-interval must be >= 5s, got %v", c.Prometheus.ScrapeInterval)
		}

		if c.Prometheus.RetentionTime < 5*time.Minute {
			return fmt.Errorf("prometheus-retention must be >= 5m, got %v", c.Prometheus.RetentionTime)
		}

		if c.Prometheus.MaxSamples < 100 {
			return fmt.Errorf("prometheus-max-samples must be >= 100, got %d", c.Prometheus.MaxSamples)
		}

		if len(c.Prometheus.Components) == 0 {
			return fmt.Errorf("prometheus-components must not be empty")
		}
	}

	return nil
}

// ParseComponents converts a string slice to ComponentType slice
// Returns an error if any component name is invalid
func ParseComponents(components []string) ([]prom.ComponentType, error) {
	if len(components) == 0 {
		return nil, fmt.Errorf("components list cannot be empty")
	}

	result := make([]prom.ComponentType, 0, len(components))
	for _, comp := range components {
		switch comp {
		case "kubelet":
			result = append(result, prom.ComponentKubelet)
		case "cadvisor":
			result = append(result, prom.ComponentCAdvisor)
		case "apiserver":
			result = append(result, prom.ComponentAPIServer)
		case "etcd":
			result = append(result, prom.ComponentEtcd)
		case "scheduler":
			result = append(result, prom.ComponentScheduler)
		case "controller-manager":
			result = append(result, prom.ComponentControllerManager)
		case "kube-proxy":
			result = append(result, prom.ComponentKubeProxy)
		default:
			return nil, fmt.Errorf("unknown component: %s (valid: kubelet, cadvisor, apiserver, etcd, scheduler, controller-manager, kube-proxy)", comp)
		}
	}

	return result, nil
}

// ComponentsToStrings converts ComponentType slice to string slice
// Useful for displaying or serializing configuration
func ComponentsToStrings(components []prom.ComponentType) []string {
	result := make([]string, 0, len(components))
	for _, comp := range components {
		switch comp {
		case prom.ComponentKubelet:
			result = append(result, "kubelet")
		case prom.ComponentCAdvisor:
			result = append(result, "cadvisor")
		case prom.ComponentAPIServer:
			result = append(result, "apiserver")
		case prom.ComponentEtcd:
			result = append(result, "etcd")
		case prom.ComponentScheduler:
			result = append(result, "scheduler")
		case prom.ComponentControllerManager:
			result = append(result, "controller-manager")
		case prom.ComponentKubeProxy:
			result = append(result, "kube-proxy")
		default:
			result = append(result, "unknown")
		}
	}
	return result
}
