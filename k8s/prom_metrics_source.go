package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vladimirvivien/ktop/prom"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/rest"
)

// PromConfig holds configuration for Prometheus metrics source
type PromConfig struct {
	Enabled        bool
	ScrapeInterval time.Duration
	RetentionTime  time.Duration
	MaxSamples     int
	Components     []prom.ComponentType
}

// PromMetricsSource implements MetricsSource using Prometheus scrapers
type PromMetricsSource struct {
	controller *prom.CollectorController
	store      prom.MetricsStore
	config     *PromConfig
	healthy    bool
	mu         sync.RWMutex
	sourceInfo SourceInfo
}

// NewPromMetricsSource creates a new Prometheus metrics source
func NewPromMetricsSource(kubeConfig *rest.Config, config *PromConfig) (*PromMetricsSource, error) {
	scrapeConfig := &prom.ScrapeConfig{
		Interval:      config.ScrapeInterval,
		Timeout:       30 * time.Second,
		MaxSamples:    config.MaxSamples,
		RetentionTime: config.RetentionTime,
		Components:    config.Components,
	}

	controller := prom.NewCollectorController(kubeConfig, scrapeConfig)

	source := &PromMetricsSource{
		controller: controller,
		store:      nil, // Will be set after Start()
		config:     config,
		healthy:    false,
		sourceInfo: SourceInfo{
			Type:    "prometheus",
			Version: "1.0.0",
			State:   SourceStateInitializing,
		},
	}

	// Set up callbacks
	controller.SetErrorCallback(source.handleError)
	controller.SetMetricsCollectedCallback(source.handleMetricsCollected)

	return source, nil
}

// Start begins metrics collection
func (p *PromMetricsSource) Start(ctx context.Context) error {
	if err := p.controller.Start(ctx); err != nil {
		return fmt.Errorf("starting prometheus controller: %w", err)
	}
	
	// Get the store after controller is initialized
	p.mu.Lock()
	p.store = p.controller.GetStore()
	p.sourceInfo.State = SourceStateCollecting
	p.mu.Unlock()
	
	// Trigger immediate collection for all components
	go func() {
		// Give the controller a moment to initialize
		time.Sleep(100 * time.Millisecond)
		p.controller.ForceCollection(ctx)
	}()
	
	return nil
}

// Stop halts metrics collection
func (p *PromMetricsSource) Stop() error {
	return p.controller.Stop()
}

// GetNodeMetrics retrieves metrics for a specific node
func (p *PromMetricsSource) GetNodeMetrics(ctx context.Context, nodeName string) (*NodeMetrics, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.healthy {
		return nil, fmt.Errorf("prometheus source unhealthy")
	}

	if p.store == nil {
		return nil, fmt.Errorf("prometheus store not initialized")
	}

	metrics := &NodeMetrics{
		NodeName:  nodeName,
		Timestamp: time.Now(),
	}

	// Initialize with zero values
	metrics.CPUUsage = resource.NewMilliQuantity(0, resource.DecimalSI)
	metrics.MemoryUsage = resource.NewQuantity(0, resource.BinarySI)

	// CPU Usage from kubelet
	if cpuUsage, err := p.store.QueryLatest("kubelet_node_cpu_usage_seconds_total",
		map[string]string{"node": nodeName}); err == nil && cpuUsage > 0 {
		metrics.CPUUsage = resource.NewMilliQuantity(int64(cpuUsage*1000), resource.DecimalSI)
	}

	// Memory Usage from kubelet
	if memUsage, err := p.store.QueryLatest("kubelet_node_memory_working_set_bytes",
		map[string]string{"node": nodeName}); err == nil && memUsage > 0 {
		metrics.MemoryUsage = resource.NewQuantity(int64(memUsage), resource.BinarySI)
	}

	// Network metrics from kubelet
	if netRx, err := p.store.QueryLatest("kubelet_node_network_receive_bytes_total",
		map[string]string{"node": nodeName}); err == nil {
		metrics.NetworkRxBytes = resource.NewQuantity(int64(netRx), resource.BinarySI)
	}

	if netTx, err := p.store.QueryLatest("kubelet_node_network_transmit_bytes_total",
		map[string]string{"node": nodeName}); err == nil {
		metrics.NetworkTxBytes = resource.NewQuantity(int64(netTx), resource.BinarySI)
	}

	// Load average from kubelet
	if load1, err := p.store.QueryLatest("kubelet_node_load1",
		map[string]string{"node": nodeName}); err == nil {
		metrics.LoadAverage1m = load1
	}

	if load5, err := p.store.QueryLatest("kubelet_node_load5",
		map[string]string{"node": nodeName}); err == nil {
		metrics.LoadAverage5m = load5
	}

	if load15, err := p.store.QueryLatest("kubelet_node_load15",
		map[string]string{"node": nodeName}); err == nil {
		metrics.LoadAverage15m = load15
	}

	// Pod count from kubelet
	if podCount, err := p.store.QueryLatest("kubelet_running_pods",
		map[string]string{"node": nodeName}); err == nil {
		metrics.PodCount = int(podCount)
	}

	// Container count from cAdvisor
	if containerCount, err := p.store.QueryLatest("container_count",
		map[string]string{"node": nodeName}); err == nil {
		metrics.ContainerCount = int(containerCount)
	}

	return metrics, nil
}

// GetPodMetrics retrieves metrics for a specific pod
func (p *PromMetricsSource) GetPodMetrics(ctx context.Context, namespace, podName string) (*PodMetrics, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.healthy {
		return nil, fmt.Errorf("prometheus source unhealthy")
	}

	if p.store == nil {
		return nil, fmt.Errorf("prometheus store not initialized")
	}

	metrics := &PodMetrics{
		PodName:   podName,
		Namespace: namespace,
		Timestamp: time.Now(),
	}

	// Get container metrics from cAdvisor
	containerCPU, err := p.store.QueryLatest("container_cpu_usage_seconds_total",
		map[string]string{"pod": podName, "namespace": namespace})
	if err == nil {
		containerMem, _ := p.store.QueryLatest("container_memory_working_set_bytes",
			map[string]string{"pod": podName, "namespace": namespace})

		containerThrottled, _ := p.store.QueryLatest("container_cpu_cfs_throttled_periods_total",
			map[string]string{"pod": podName, "namespace": namespace})

		metrics.ContainerMetrics = []ContainerMetrics{
			{
				Name:         "main", // Would need to query for actual container names
				CPUUsage:     resource.NewMilliQuantity(int64(containerCPU*1000), resource.DecimalSI),
				MemoryUsage:  resource.NewQuantity(int64(containerMem), resource.BinarySI),
				CPUThrottled: containerThrottled,
			},
		}
	}

	return metrics, nil
}

// GetAllPodMetrics retrieves metrics for all pods
func (p *PromMetricsSource) GetAllPodMetrics(ctx context.Context) ([]*PodMetrics, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.healthy {
		return nil, fmt.Errorf("prometheus source unhealthy")
	}

	// This would need to be implemented by querying the store for all pods
	// For now, return empty slice
	return []*PodMetrics{}, nil
}

// GetAvailableMetrics returns list of available metrics
func (p *PromMetricsSource) GetAvailableMetrics() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.store == nil {
		return []string{}
	}

	return p.store.GetMetricNames()
}

// IsHealthy returns the health status of the metrics source
func (p *PromMetricsSource) IsHealthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.healthy
}

// GetSourceInfo returns information about the metrics source
func (p *PromMetricsSource) GetSourceInfo() SourceInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.sourceInfo
}

// handleError is called when the collector encounters an error
func (p *PromMetricsSource) handleError(component prom.ComponentType, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.healthy = false
	p.sourceInfo.Errors++
	p.sourceInfo.State = SourceStateUnhealthy
}

// handleMetricsCollected is called when metrics are successfully collected
func (p *PromMetricsSource) handleMetricsCollected(component prom.ComponentType, metrics *prom.ScrapedMetrics) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.healthy = true
	p.sourceInfo.LastScrape = time.Now()
	p.sourceInfo.State = SourceStateHealthy
	if metrics != nil && metrics.Families != nil {
		p.sourceInfo.MetricsCount = len(metrics.Families)
	}
}