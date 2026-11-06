package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vladimirvivien/ktop/internal/userdir"
	"github.com/vladimirvivien/ktop/prom"
	"sigs.k8s.io/yaml"
)

// ConfigFileName is the standard configuration file name stored in ~/.ktop
const ConfigFileName = "config.yaml"

// Available Node and Pod columns in canonical display order
var (
	AllNodeColumns = []string{
		"NAME", "STATUS", "RST", "PODS", "TAINTS", "PRESSURE", "IP", "VOLS", "DISK", "CPU", "MEM", "STALL",
	}

	AllPodColumns = []string{
		"NAMESPACE", "POD", "READY", "STATUS", "RST", "AGE", "VOLS", "IP", "NODE", "CPU", "MEMORY", "STALL",
	}

	DefaultNodeColumns = []string{
		"NAME", "STATUS", "RST", "PODS", "DISK", "CPU", "MEM",
	}

	DefaultPodColumns = []string{
		"NAMESPACE", "POD", "READY", "STATUS", "RST", "AGE", "CPU", "MEMORY",
	}

	ColumnDescriptions = map[string]string{
		// Node columns
		"NAME":     "Node name",
		"STATUS":   "Ready/NotReady status",
		"RST":      "Total restarts across pods",
		"PODS":     "Allocated pods / capacity",
		"TAINTS":   "Configured node taints",
		"PRESSURE": "Node resource pressure",
		"IP":       "Internal node IP address",
		"VOLS":     "Attached volume count",
		"DISK":     "Ephemeral storage root usage",
		"CPU":      "CPU usage graph and ratio",
		"MEM":      "Memory usage graph and ratio",
		"STALL":    "PSI resource pressure stall",

		// Pod columns
		"NAMESPACE": "Pod namespace",
		"POD":       "Pod name",
		"READY":     "Ready containers count",
		"AGE":       "Time since creation",
		"NODE":      "Assigned worker node",
		"MEMORY":    "Memory usage graph and bytes",
	}
)

// NormalizeColumns converts column names to uppercase and trims whitespace.
func NormalizeColumns(cols []string) []string {
	result := make([]string, 0, len(cols))
	for _, c := range cols {
		trimmed := strings.ToUpper(strings.TrimSpace(c))
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// AppConfig represents the root schema stored in ~/.ktop/config.yaml.
type AppConfig struct {
	UI      UIConfig      `json:"ui" yaml:"ui"`
	Metrics MetricsConfig `json:"metrics" yaml:"metrics"`
	Logging LoggingConfig `json:"logging" yaml:"logging"`
}

// UIConfig holds UI preferences.
type UIConfig struct {
	Overview OverviewUIConfig `json:"overview" yaml:"overview"`
}

// OverviewUIConfig holds column selections for overview tables.
type OverviewUIConfig struct {
	NodeCols []string `json:"node_cols,omitempty" yaml:"node_cols,omitempty"`
	PodCols  []string `json:"pod_cols,omitempty" yaml:"pod_cols,omitempty"`
}

// MetricsConfig holds metrics collector configuration.
type MetricsConfig struct {
	Source         string   `json:"source" yaml:"source"`
	ScrapeInterval string   `json:"scrape_interval" yaml:"scrape_interval"`
	Retention      string   `json:"retention" yaml:"retention"`
	MaxSamples     int      `json:"max_samples" yaml:"max_samples"`
	Components     []string `json:"components" yaml:"components"`
}

// LoggingConfig holds application logging configuration.
type LoggingConfig struct {
	Level  string `json:"level" yaml:"level"`
	Format string `json:"format" yaml:"format"`
}

// DefaultAppConfig returns an AppConfig populated with sensible resolved defaults.
func DefaultAppConfig() *AppConfig {
	return &AppConfig{
		UI: UIConfig{
			Overview: OverviewUIConfig{
				NodeCols: slicesClone(DefaultNodeColumns),
				PodCols:  slicesClone(DefaultPodColumns),
			},
		},
		Metrics: MetricsConfig{
			Source:         "prometheus",
			ScrapeInterval: "5s",
			Retention:      "1h",
			MaxSamples:     10000,
			Components:     []string{"kubelet", "cadvisor"},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

func slicesClone(s []string) []string {
	res := make([]string, len(s))
	copy(res, s)
	return res
}

// ConfigPath returns the resolved path to ~/.ktop/config.yaml.
func ConfigPath() (string, error) {
	dir, err := userdir.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFileName), nil
}

// LoadAppConfig reads ~/.ktop/config.yaml.
// If the file does not exist, it returns (nil, nil) indicating default fallback.
func LoadAppConfig() (*AppConfig, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config %s: %w", path, err)
	}

	cfg.UI.Overview.NodeCols = NormalizeColumns(cfg.UI.Overview.NodeCols)
	cfg.UI.Overview.PodCols = NormalizeColumns(cfg.UI.Overview.PodCols)

	return &cfg, nil
}

// SaveAppConfig writes the given AppConfig to ~/.ktop/config.yaml (mode 0600).
func SaveAppConfig(cfg *AppConfig) error {
	dir, err := userdir.Ensure()
	if err != nil {
		return fmt.Errorf("ensure ktop directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	header := "# ktop configuration file\n"
	content := append([]byte(header), data...)

	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// SaveOverviewColumns updates or creates ~/.ktop/config.yaml with specified node and pod columns.
// If nodeCols or podCols is empty, existing configured columns or defaults are preserved.
func SaveOverviewColumns(nodeCols, podCols []string) error {
	cfg, err := LoadAppConfig()
	if err != nil || cfg == nil {
		cfg = DefaultAppConfig()
	}

	if len(nodeCols) > 0 {
		cfg.UI.Overview.NodeCols = NormalizeColumns(nodeCols)
	}
	if len(podCols) > 0 {
		cfg.UI.Overview.PodCols = NormalizeColumns(podCols)
	}

	return SaveAppConfig(cfg)
}

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
