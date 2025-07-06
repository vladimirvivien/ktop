package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/vladimirvivien/ktop/prom"
	"k8s.io/client-go/rest"
)

// HybridConfig holds configuration for the hybrid metrics controller
type HybridConfig struct {
	PreferredSource     string        // "prometheus", "metrics-server", "auto"
	FallbackEnabled     bool
	FallbackTimeout     time.Duration
	HealthCheckInterval time.Duration
}

// HybridMetricsController manages multiple metrics sources with fallback capability
type HybridMetricsController struct {
	promSource          *PromMetricsSource
	metricsServerSource *MetricsServerSource
	config              *HybridConfig
	preferredSource     string
}

// MetricsServerSource wraps the existing metrics server client to implement MetricsSource interface
type MetricsServerSource struct {
	client *Client
}

// NewMetricsServerSource creates a new metrics server source
func NewMetricsServerSource(kubeConfig *rest.Config) (*MetricsServerSource, error) {
	// For now, we'll need to pass in the k8s client from the caller
	// This is a placeholder implementation
	return &MetricsServerSource{}, nil
}

// GetNodeMetrics retrieves node metrics from metrics server
func (m *MetricsServerSource) GetNodeMetrics(ctx context.Context, nodeName string) (*NodeMetrics, error) {
	// This is a placeholder implementation
	// In the real implementation, we would query the metrics server API
	return &NodeMetrics{
		NodeName:  nodeName,
		Timestamp: time.Now(),
	}, nil
}

// GetPodMetrics retrieves pod metrics from metrics server
func (m *MetricsServerSource) GetPodMetrics(ctx context.Context, namespace, podName string) (*PodMetrics, error) {
	// This is a placeholder implementation
	// In the real implementation, we would query the metrics server API
	return &PodMetrics{
		PodName:   podName,
		Namespace: namespace,
		Timestamp: time.Now(),
	}, nil
}

// GetAllPodMetrics retrieves all pod metrics from metrics server
func (m *MetricsServerSource) GetAllPodMetrics(ctx context.Context) ([]*PodMetrics, error) {
	// This would need to iterate through namespaces and pods
	// For now, return empty slice
	return []*PodMetrics{}, nil
}

// GetAvailableMetrics returns list of available metrics from metrics server
func (m *MetricsServerSource) GetAvailableMetrics() []string {
	return []string{"cpu", "memory"}
}

// IsHealthy checks if metrics server is healthy
func (m *MetricsServerSource) IsHealthy() bool {
	// Could perform actual health check
	return true
}

// GetSourceInfo returns information about the metrics server source
func (m *MetricsServerSource) GetSourceInfo() SourceInfo {
	return SourceInfo{
		Type:         "metrics-server",
		Version:      "v0.6.0",
		LastScrape:   time.Now(),
		MetricsCount: 2, // CPU and Memory
		Errors:       0,
		State:        SourceStateHealthy, // Metrics server is always ready when available
	}
}

// NewHybridMetricsController creates a new hybrid metrics controller
func NewHybridMetricsController(kubeConfig *rest.Config, config *HybridConfig) (*HybridMetricsController, error) {
	controller := &HybridMetricsController{
		config: config,
	}

	// Initialize Prometheus source
	promConfig := &PromConfig{
		Enabled:        true,
		ScrapeInterval: 15 * time.Second,
		RetentionTime:  1 * time.Hour,
		MaxSamples:     10000,
		Components: []prom.ComponentType{
			prom.ComponentKubelet,
			prom.ComponentCAdvisor,
			prom.ComponentAPIServer,
		},
	}

	promSource, err := NewPromMetricsSource(kubeConfig, promConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus source: %w", err)
	}
	controller.promSource = promSource

	// Initialize traditional metrics server source
	metricsServerSource, err := NewMetricsServerSource(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics server source: %w", err)
	}
	controller.metricsServerSource = metricsServerSource

	controller.preferredSource = config.PreferredSource

	return controller, nil
}

// Start begins metrics collection for all sources
func (h *HybridMetricsController) Start(ctx context.Context) error {
	// Start Prometheus source
	if err := h.promSource.Start(ctx); err != nil {
		return fmt.Errorf("failed to start prometheus source: %w", err)
	}

	// Metrics server doesn't need explicit start
	return nil
}

// Stop halts metrics collection for all sources
func (h *HybridMetricsController) Stop() error {
	if h.promSource != nil {
		return h.promSource.Stop()
	}
	return nil
}

// GetNodeMetrics retrieves node metrics using configured strategy
func (h *HybridMetricsController) GetNodeMetrics(ctx context.Context, nodeName string) (*NodeMetrics, error) {
	switch h.config.PreferredSource {
	case "prometheus":
		if metrics, err := h.promSource.GetNodeMetrics(ctx, nodeName); err == nil {
			return metrics, nil
		}
		if h.config.FallbackEnabled {
			return h.metricsServerSource.GetNodeMetrics(ctx, nodeName)
		}
		return nil, fmt.Errorf("prometheus source failed and fallback disabled")

	case "metrics-server":
		if metrics, err := h.metricsServerSource.GetNodeMetrics(ctx, nodeName); err == nil {
			return metrics, nil
		}
		if h.config.FallbackEnabled {
			return h.promSource.GetNodeMetrics(ctx, nodeName)
		}
		return nil, fmt.Errorf("metrics server failed and fallback disabled")

	case "auto":
		// Try prometheus first (richer metrics), fallback to metrics server
		if h.promSource.IsHealthy() {
			if metrics, err := h.promSource.GetNodeMetrics(ctx, nodeName); err == nil {
				return metrics, nil
			}
		}
		return h.metricsServerSource.GetNodeMetrics(ctx, nodeName)

	default:
		return nil, fmt.Errorf("unknown preferred source: %s", h.config.PreferredSource)
	}
}

// GetPodMetrics retrieves pod metrics using configured strategy
func (h *HybridMetricsController) GetPodMetrics(ctx context.Context, namespace, podName string) (*PodMetrics, error) {
	switch h.config.PreferredSource {
	case "prometheus":
		if metrics, err := h.promSource.GetPodMetrics(ctx, namespace, podName); err == nil {
			return metrics, nil
		}
		if h.config.FallbackEnabled {
			return h.metricsServerSource.GetPodMetrics(ctx, namespace, podName)
		}
		return nil, fmt.Errorf("prometheus source failed and fallback disabled")

	case "metrics-server":
		if metrics, err := h.metricsServerSource.GetPodMetrics(ctx, namespace, podName); err == nil {
			return metrics, nil
		}
		if h.config.FallbackEnabled {
			return h.promSource.GetPodMetrics(ctx, namespace, podName)
		}
		return nil, fmt.Errorf("metrics server failed and fallback disabled")

	case "auto":
		// Try prometheus first (richer metrics), fallback to metrics server
		if h.promSource.IsHealthy() {
			if metrics, err := h.promSource.GetPodMetrics(ctx, namespace, podName); err == nil {
				return metrics, nil
			}
		}
		return h.metricsServerSource.GetPodMetrics(ctx, namespace, podName)

	default:
		return nil, fmt.Errorf("unknown preferred source: %s", h.config.PreferredSource)
	}
}

// GetAllPodMetrics retrieves all pod metrics using configured strategy
func (h *HybridMetricsController) GetAllPodMetrics(ctx context.Context) ([]*PodMetrics, error) {
	// Use the same strategy as other methods
	var source MetricsSource
	switch h.config.PreferredSource {
	case "prometheus":
		source = h.promSource
	case "metrics-server":
		source = h.metricsServerSource
	case "auto":
		if h.promSource.IsHealthy() {
			source = h.promSource
		} else {
			source = h.metricsServerSource
		}
	default:
		return nil, fmt.Errorf("unknown preferred source: %s", h.config.PreferredSource)
	}

	return source.GetAllPodMetrics(ctx)
}

// GetAvailableMetrics returns combined list of available metrics
func (h *HybridMetricsController) GetAvailableMetrics() []string {
	metricsMap := make(map[string]bool)

	// Add metrics from both sources
	for _, metric := range h.promSource.GetAvailableMetrics() {
		metricsMap[metric] = true
	}
	for _, metric := range h.metricsServerSource.GetAvailableMetrics() {
		metricsMap[metric] = true
	}

	// Convert map to slice
	var metrics []string
	for metric := range metricsMap {
		metrics = append(metrics, metric)
	}

	return metrics
}

// IsHealthy returns true if at least one source is healthy
func (h *HybridMetricsController) IsHealthy() bool {
	return h.promSource.IsHealthy() || h.metricsServerSource.IsHealthy()
}

// GetSourceInfo returns information about the active metrics source
func (h *HybridMetricsController) GetSourceInfo() SourceInfo {
	// If user explicitly selected a source, show that in the info
	// even if we're falling back to another source
	if h.config.PreferredSource == "prometheus" {
		info := h.promSource.GetSourceInfo()
		if !h.promSource.IsHealthy() && h.config.FallbackEnabled {
			info.Type = "prometheus (fallback: metrics-server)"
		}
		return info
	} else if h.config.PreferredSource == "metrics-server" {
		info := h.metricsServerSource.GetSourceInfo()
		if !h.metricsServerSource.IsHealthy() && h.config.FallbackEnabled {
			info.Type = "metrics-server (fallback: prometheus)"
		}
		return info
	}

	// For auto mode, return info from the active source
	if h.promSource.IsHealthy() {
		return h.promSource.GetSourceInfo()
	}
	return h.metricsServerSource.GetSourceInfo()
}