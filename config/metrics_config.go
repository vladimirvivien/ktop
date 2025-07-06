package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// MetricsConfig holds the complete metrics configuration
type MetricsConfig struct {
	Source     SourceConfig     `yaml:"source"`
	Prometheus PrometheusConfig `yaml:"prometheus"`
	Display    DisplayConfig    `yaml:"display"`
	Advanced   AdvancedConfig   `yaml:"advanced"`
}

// SourceConfig defines which metrics source to use
type SourceConfig struct {
	Type            string `yaml:"type"`               // "prometheus", "metrics-server", "hybrid"
	FallbackEnabled bool   `yaml:"fallback_enabled"`
	FallbackTimeout string `yaml:"fallback_timeout"`
}

// PrometheusConfig holds Prometheus-specific settings
type PrometheusConfig struct {
	Enabled        bool     `yaml:"enabled"`
	ScrapeInterval string   `yaml:"scrape_interval"`
	RetentionTime  string   `yaml:"retention_time"`
	MaxSamples     int      `yaml:"max_samples"`
	Components     []string `yaml:"components"`
}

// DisplayConfig controls UI display options
type DisplayConfig struct {
	EnhancedColumns bool   `yaml:"enhanced_columns"`
	ShowTrends      bool   `yaml:"show_trends"`
	ShowHealth      bool   `yaml:"show_health"`
	TimeRange       string `yaml:"time_range"`
	RefreshInterval string `yaml:"refresh_interval"`
}

// AdvancedConfig holds advanced settings
type AdvancedConfig struct {
	Debug          bool   `yaml:"debug"`
	LogLevel       string `yaml:"log_level"`
	MaxConcurrency int    `yaml:"max_concurrency"`
	CacheSize      int    `yaml:"cache_size"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *MetricsConfig {
	return &MetricsConfig{
		Source: SourceConfig{
			Type:            "auto",
			FallbackEnabled: true,
			FallbackTimeout: "5s",
		},
		Prometheus: PrometheusConfig{
			Enabled:        true,
			ScrapeInterval: "15s",
			RetentionTime:  "1h",
			MaxSamples:     10000,
			Components:     []string{"kubelet", "cadvisor", "apiserver"},
		},
		Display: DisplayConfig{
			EnhancedColumns: false,
			ShowTrends:      false,
			ShowHealth:      true,
			TimeRange:       "15m",
			RefreshInterval: "5s",
		},
		Advanced: AdvancedConfig{
			Debug:          false,
			LogLevel:       "info",
			MaxConcurrency: 10,
			CacheSize:      1000,
		},
	}
}

// GetConfigPath returns the configuration file path
// Priority: 1. Specified config file, 2. $HOME/.ktop/config.yaml
func GetConfigPath(configFile string) string {
	if configFile != "" {
		return configFile
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(homeDir, ".ktop", "config.yaml")
}

// LoadConfig loads configuration from file
func LoadConfig(configFile string) (*MetricsConfig, error) {
	configPath := GetConfigPath(configFile)
	if configPath == "" {
		return DefaultConfig(), nil
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Return default config if file doesn't exist
		return DefaultConfig(), nil
	}

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML
	config := DefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// SaveConfig saves configuration to file
func SaveConfig(config *MetricsConfig, configFile string) error {
	configPath := GetConfigPath(configFile)
	if configPath == "" {
		return fmt.Errorf("unable to determine config path")
	}

	// Create directory if it doesn't exist
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal config to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ParseDuration converts string duration to time.Duration
func (c *SourceConfig) ParseFallbackTimeout() (time.Duration, error) {
	return time.ParseDuration(c.FallbackTimeout)
}

// ParseDuration converts string duration to time.Duration
func (c *PrometheusConfig) ParseScrapeInterval() (time.Duration, error) {
	return time.ParseDuration(c.ScrapeInterval)
}

// ParseDuration converts string duration to time.Duration
func (c *PrometheusConfig) ParseRetentionTime() (time.Duration, error) {
	return time.ParseDuration(c.RetentionTime)
}

// ParseDuration converts string duration to time.Duration
func (c *DisplayConfig) ParseRefreshInterval() (time.Duration, error) {
	return time.ParseDuration(c.RefreshInterval)
}

// ParseDuration converts string duration to time.Duration
func (c *DisplayConfig) ParseTimeRange() (time.Duration, error) {
	return time.ParseDuration(c.TimeRange)
}

// CreateDefaultConfigFile creates a default config file if it doesn't exist
func CreateDefaultConfigFile(configFile string) error {
	configPath := GetConfigPath(configFile)
	if configPath == "" {
		return fmt.Errorf("unable to determine config path")
	}

	// Check if file already exists
	if _, err := os.Stat(configPath); err == nil {
		return nil // File already exists
	}

	// Create default config
	defaultConfig := DefaultConfig()
	return SaveConfig(defaultConfig, configFile)
}