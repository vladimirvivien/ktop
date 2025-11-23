package prom

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/prom"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// PromMetricsSource implements metrics.MetricsSource using Prometheus scraping.
// This source provides enhanced metrics including network I/O, load averages, and container counts.
type PromMetricsSource struct {
	controller *prom.CollectorController
	store      prom.MetricsStore
	config     *PromConfig

	// Health tracking
	mu         sync.RWMutex
	healthy    bool
	lastError  error
	errorCount int
	lastScrape time.Time
}

// PromConfig holds configuration for the Prometheus metrics source
type PromConfig struct {
	Enabled        bool
	ScrapeInterval time.Duration
	RetentionTime  time.Duration
	MaxSamples     int
	Components     []prom.ComponentType
}

// DefaultPromConfig returns a default Prometheus configuration
func DefaultPromConfig() *PromConfig {
	return &PromConfig{
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
}

// NewPromMetricsSource creates a new Prometheus metrics source
func NewPromMetricsSource(kubeConfig *rest.Config, config *PromConfig) (*PromMetricsSource, error) {
	if config == nil {
		config = DefaultPromConfig()
	}

	// Convert PromConfig to prom.ScrapeConfig
	scrapeConfig := &prom.ScrapeConfig{
		Interval:      config.ScrapeInterval,
		Timeout:       30 * time.Second,
		MaxSamples:    config.MaxSamples,
		RetentionTime: config.RetentionTime,
		InsecureTLS:   false,
		Components:    config.Components,
	}

	// Create the collector controller
	controller := prom.NewCollectorController(kubeConfig, scrapeConfig)

	source := &PromMetricsSource{
		controller: controller,
		config:     config,
		healthy:    false, // Will be set to true after first successful scrape
	}

	// Set up callbacks for health monitoring
	controller.SetErrorCallback(source.handleError)
	controller.SetMetricsCollectedCallback(source.handleMetricsCollected)

	return source, nil
}

// Start begins the Prometheus metrics collection
func (p *PromMetricsSource) Start(ctx context.Context) error {
	if err := p.controller.Start(ctx); err != nil {
		p.recordError(err)
		return fmt.Errorf("failed to start prometheus controller: %w", err)
	}

	// Wait a moment for initialization
	time.Sleep(100 * time.Millisecond)

	// Get the store from the controller (it's created during Start)
	p.mu.Lock()
	p.store = p.controller.GetStore()
	p.mu.Unlock()

	return nil
}

// Stop halts the Prometheus metrics collection
func (p *PromMetricsSource) Stop() error {
	return p.controller.Stop()
}

// calculateCPURate calculates CPU usage rate from counter samples over a time window
// Returns CPU cores (e.g., 0.1 = 100 millicores)
// Uses silent fallback - returns error without logging on insufficient samples
func (p *PromMetricsSource) calculateCPURate(metricName string, labelMatchers map[string]string, window time.Duration) (float64, error) {
	now := time.Now()
	start := now.Add(-window)

	samples, err := p.store.QueryRange(metricName, labelMatchers, start, now)
	if err != nil {
		return 0, err
	}

	if len(samples) < 2 {
		return 0, fmt.Errorf("insufficient samples for rate calculation (need >=2, got %d)", len(samples))
	}

	// Calculate rate from first and last sample
	firstSample := samples[0]
	lastSample := samples[len(samples)-1]

	deltaValue := lastSample.Value - firstSample.Value
	deltaTimeMs := lastSample.Timestamp - firstSample.Timestamp
	deltaTimeSeconds := float64(deltaTimeMs) / 1000.0 // Convert milliseconds to seconds

	if deltaTimeSeconds <= 0 {
		return 0, fmt.Errorf("invalid time delta: %f seconds", deltaTimeSeconds)
	}

	// Rate in CPU cores
	rate := deltaValue / deltaTimeSeconds

	return rate, nil
}

// GetNodeMetrics retrieves metrics for a specific node from Prometheus
func (p *PromMetricsSource) GetNodeMetrics(ctx context.Context, nodeName string) (*metrics.NodeMetrics, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.healthy {
		return nil, fmt.Errorf("prometheus source is not healthy")
	}

	if p.store == nil {
		return nil, fmt.Errorf("metrics store not initialized")
	}

	nodeMetrics := &metrics.NodeMetrics{
		NodeName:  nodeName,
		Timestamp: time.Now(),
	}

	// Node-level metrics from cAdvisor root container (id="/")
	labelMatchers := map[string]string{"id": "/"}

	// Query CPU usage: container_cpu_usage_seconds_total (counter - needs rate calculation)
	if cpuRate, err := p.calculateCPURate("container_cpu_usage_seconds_total", labelMatchers, 40*time.Second); err == nil {
		// Convert CPU cores to millicores
		nodeMetrics.CPUUsage = resource.NewMilliQuantity(int64(cpuRate*1000), resource.DecimalSI)
	}

	// Query Memory usage: container_memory_working_set_bytes (gauge - use latest value)
	if memUsage, err := p.store.QueryLatest("container_memory_working_set_bytes", labelMatchers); err == nil {
		nodeMetrics.MemoryUsage = resource.NewQuantity(int64(memUsage), resource.BinarySI)
	}

	// Query Network RX: kubelet_node_network_receive_bytes_total
	if netRx, err := p.store.QueryLatest("kubelet_node_network_receive_bytes_total",
		map[string]string{"node": nodeName}); err == nil {
		nodeMetrics.NetworkRxBytes = resource.NewQuantity(int64(netRx), resource.BinarySI)
	}

	// Query Network TX: kubelet_node_network_transmit_bytes_total
	if netTx, err := p.store.QueryLatest("kubelet_node_network_transmit_bytes_total",
		map[string]string{"node": nodeName}); err == nil {
		nodeMetrics.NetworkTxBytes = resource.NewQuantity(int64(netTx), resource.BinarySI)
	}

	// Query Load averages: kubelet_node_load1, kubelet_node_load5, kubelet_node_load15
	if load1, err := p.store.QueryLatest("kubelet_node_load1",
		map[string]string{"node": nodeName}); err == nil {
		nodeMetrics.LoadAverage1m = load1
	}

	if load5, err := p.store.QueryLatest("kubelet_node_load5",
		map[string]string{"node": nodeName}); err == nil {
		nodeMetrics.LoadAverage5m = load5
	}

	if load15, err := p.store.QueryLatest("kubelet_node_load15",
		map[string]string{"node": nodeName}); err == nil {
		nodeMetrics.LoadAverage15m = load15
	}

	// Query Pod count: kubelet_running_pods
	if podCount, err := p.store.QueryLatest("kubelet_running_pods",
		map[string]string{"node": nodeName}); err == nil {
		nodeMetrics.PodCount = int(podCount)
	}

	// Query Container count: container_count or calculate from cadvisor
	if containerCount, err := p.store.QueryLatest("container_count",
		map[string]string{"node": nodeName}); err == nil {
		nodeMetrics.ContainerCount = int(containerCount)
	}

	return nodeMetrics, nil
}

// GetPodMetrics retrieves metrics for a specific pod by namespace and name
func (p *PromMetricsSource) GetPodMetrics(ctx context.Context, namespace, podName string) (*metrics.PodMetrics, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.healthy {
		return nil, fmt.Errorf("prometheus source is not healthy")
	}

	if p.store == nil {
		return nil, fmt.Errorf("metrics store not initialized")
	}

	podMetrics := &metrics.PodMetrics{
		PodName:   podName,
		Namespace: namespace,
		Timestamp: time.Now(),
	}

	// Query container metrics from cAdvisor
	// Include all containers (including pause containers if they emit metrics)
	labelMatchers := map[string]string{
		"pod":       podName,
		"namespace": namespace,
	}

	// Get CPU usage for containers (counter - needs rate calculation)
	containerMetrics := metrics.ContainerMetrics{
		Name: "main", // TODO: Get actual container name from labels
	}

	if cpuRate, err := p.calculateCPURate("container_cpu_usage_seconds_total", labelMatchers, 40*time.Second); err == nil {
		// Convert CPU cores to millicores
		containerMetrics.CPUUsage = resource.NewMilliQuantity(int64(cpuRate*1000), resource.DecimalSI)
	}

	// Get memory usage (gauge - use latest value)
	if memUsage, err := p.store.QueryLatest("container_memory_working_set_bytes", labelMatchers); err == nil {
		containerMetrics.MemoryUsage = resource.NewQuantity(int64(memUsage), resource.BinarySI)
	}

	// Only add container metrics if we got at least one metric
	if containerMetrics.CPUUsage != nil || containerMetrics.MemoryUsage != nil {
		podMetrics.Containers = append(podMetrics.Containers, containerMetrics)
	}

	return podMetrics, nil
}

// GetMetricsForPod retrieves metrics for a specific pod object
func (p *PromMetricsSource) GetMetricsForPod(ctx context.Context, pod metav1.Object) (*metrics.PodMetrics, error) {
	// Extract namespace and name from pod object
	// Delegate to GetPodMetrics
	return p.GetPodMetrics(ctx, pod.GetNamespace(), pod.GetName())
}

// GetAllPodMetrics retrieves metrics for all pods
func (p *PromMetricsSource) GetAllPodMetrics(ctx context.Context) ([]*metrics.PodMetrics, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.healthy {
		return nil, fmt.Errorf("prometheus source is not healthy")
	}

	if p.store == nil {
		return nil, fmt.Errorf("metrics store not initialized")
	}

	// Get all unique pod/namespace combinations from labels
	// This requires querying the store for label values
	namespaces := p.store.GetLabelValues("namespace")
	pods := p.store.GetLabelValues("pod")

	var allPodMetrics []*metrics.PodMetrics

	// This is a simplified implementation - in production would need better logic
	// to match pods with their namespaces
	for _, namespace := range namespaces {
		for _, pod := range pods {
			if podMetrics, err := p.GetPodMetrics(ctx, namespace, pod); err == nil {
				allPodMetrics = append(allPodMetrics, podMetrics)
			}
		}
	}

	return allPodMetrics, nil
}

// GetAvailableMetrics returns the list of metrics available from Prometheus
func (p *PromMetricsSource) GetAvailableMetrics() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return enhanced metrics list
	return []string{
		"cpu",
		"memory",
		"network_rx",
		"network_tx",
		"load_1m",
		"load_5m",
		"load_15m",
		"pod_count",
		"container_count",
		"disk_usage",
	}
}

// IsHealthy returns true if the Prometheus source is operational
func (p *PromMetricsSource) IsHealthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.healthy
}

// GetSourceInfo returns metadata about the Prometheus source
func (p *PromMetricsSource) GetSourceInfo() metrics.SourceInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metricCount := 0
	if p.store != nil {
		metricCount = len(p.store.GetMetricNames())
	}

	return metrics.SourceInfo{
		Type:         metrics.SourceTypePrometheus,
		Version:      "v1.0.0",
		LastScrape:   p.lastScrape,
		MetricsCount: metricCount,
		ErrorCount:   p.errorCount,
		Healthy:      p.healthy,
	}
}

// handleError is called when an error occurs during metrics collection
func (p *PromMetricsSource) handleError(component prom.ComponentType, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastError = err
	p.errorCount++
	p.healthy = false
}

// handleMetricsCollected is called when metrics are successfully collected
func (p *PromMetricsSource) handleMetricsCollected(component prom.ComponentType, metrics *prom.ScrapedMetrics) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastError = nil
	p.healthy = true
	p.lastScrape = time.Now()

	// Ensure we have a reference to the store
	if p.store == nil && p.controller != nil {
		p.store = p.controller.GetStore()
	}
}

// recordError updates health status after an error
func (p *PromMetricsSource) recordError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastError = err
	p.errorCount++
	p.healthy = false
}
