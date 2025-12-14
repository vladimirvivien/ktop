package prom

import (
	"context"
	"fmt"
	"math"
	"strings"
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

	// Health tracking - per-component to handle concurrent scrapes correctly
	mu               sync.RWMutex
	componentHealthy map[prom.ComponentType]bool // Track health per component
	lastError        error
	errorCount       int
	lastScrape       time.Time
	healthCallback   func(healthy bool, info metrics.SourceInfo)
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
		ScrapeInterval: 5 * time.Second,
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
		controller:       controller,
		config:           config,
		componentHealthy: make(map[prom.ComponentType]bool), // Track per-component health
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

// TestConnection performs a test scrape to verify connectivity and permissions.
// Returns nil if the prometheus endpoints are accessible.
func (p *PromMetricsSource) TestConnection(ctx context.Context) error {
	return p.controller.TestScrape(ctx)
}

// calculateCPURate calculates CPU usage rate from counter samples over a time window
// Returns CPU cores (e.g., 0.1 = 100 millicores)
// Uses silent fallback - returns error without logging on insufficient samples
//
// This function handles multiple time series correctly by:
// 1. Querying samples per-series (not flattening across series)
// 2. Calculating rate for each series individually
// 3. Summing the rates across all matching series
//
// This is essential for pods with multiple containers, where each container
// has its own container_cpu_usage_seconds_total time series.
func (p *PromMetricsSource) calculateCPURate(metricName string, labelMatchers map[string]string, window time.Duration) (float64, error) {
	return p.calculateCPURateWithFilter(metricName, labelMatchers, window, nil)
}

// calculateCPURateWithFilter calculates CPU rate with optional series filter function
func (p *PromMetricsSource) calculateCPURateWithFilter(metricName string, labelMatchers map[string]string, window time.Duration, filterFn func(seriesKey string) bool) (float64, error) {
	now := time.Now()
	start := now.Add(-window)

	// Use QueryRangePerSeries to get samples grouped by series
	// This prevents mixing samples from different containers
	seriesSamples, err := p.store.QueryRangePerSeries(metricName, labelMatchers, start, now)
	if err != nil {
		return 0, err
	}

	if len(seriesSamples) == 0 {
		return 0, fmt.Errorf("no matching series found for rate calculation")
	}

	// Calculate rate for each series and sum them
	var totalRate float64
	validSeriesCount := 0

	for seriesKey, samples := range seriesSamples {
		// Apply filter if provided
		if filterFn != nil && !filterFn(seriesKey) {
			continue
		}

		if len(samples) < 2 {
			// Skip series with insufficient samples
			continue
		}

		// Calculate rate from first and last sample within this series
		firstSample := samples[0]
		lastSample := samples[len(samples)-1]

		deltaValue := lastSample.Value - firstSample.Value
		deltaTimeMs := lastSample.Timestamp - firstSample.Timestamp
		deltaTimeSeconds := float64(deltaTimeMs) / 1000.0 // Convert milliseconds to seconds

		if deltaTimeSeconds <= 0 {
			// Skip series with invalid time delta
			continue
		}

		// Handle counter resets (value went down)
		if deltaValue < 0 {
			// Counter reset detected - use last value as the rate approximation
			// This is a simplified approach; a more accurate method would detect
			// the reset point and calculate rate from there
			deltaValue = lastSample.Value
		}

		// Rate in CPU cores for this series
		seriesRate := deltaValue / deltaTimeSeconds
		totalRate += seriesRate
		validSeriesCount++
	}

	if validSeriesCount == 0 {
		return 0, fmt.Errorf("insufficient samples for rate calculation in any series")
	}

	return totalRate, nil
}

// isWorkloadContainerCPUTotal returns true if the seriesKey represents:
// 1. A valid pod metric (container="" is the pod aggregate, which is what we want)
// 2. The total CPU metric (not per-CPU breakdown)
//
// In containerd runtime (modern k8s), cAdvisor only exports pod-level aggregates
// with container="". Individual container metrics don't exist.
// We include container="" because it's the pod aggregate we need.
// We exclude container="POD" because that's the pause container.
//
// cAdvisor emits metrics with cpu="total" plus per-CPU metrics (cpu="0", cpu="1", etc.)
// We only want cpu="total" to avoid double-counting.
//
// The series key format from labels.Labels.String() is:
// {__name__="metric", container="", cpu="total", namespace="ns", pod="pod-name", ...}
func isWorkloadContainerCPUTotal(seriesKey string) bool {
	// Exclude POD (pause) container - not useful for pod metrics
	if strings.Contains(seriesKey, `container="POD"`) {
		return false
	}

	// For CPU metrics, only include cpu="total", not per-CPU breakdowns
	// If cpu label exists but is not "total", exclude it
	if strings.Contains(seriesKey, `cpu="`) && !strings.Contains(seriesKey, `cpu="total"`) {
		return false
	}

	return true
}

// isWorkloadContainerMemory returns true if the seriesKey represents a valid
// individual container metric for memory. This matches metrics-server behavior:
// - Exclude container="" (pod-level aggregate to avoid double counting)
// - Exclude container="POD" (pause container)
// Only sum individual container metrics like metrics-server does.
func isWorkloadContainerMemory(seriesKey string) bool {
	// Exclude POD (pause) container - not useful for pod metrics
	if strings.Contains(seriesKey, `container="POD"`) {
		return false
	}

	// Exclude pod-level aggregate (container="") to avoid double counting
	// when individual container metrics also exist.
	// Metrics-server uses container!="" to get only individual containers.
	if strings.Contains(seriesKey, `container=""`) {
		return false
	}

	return true
}

// isPodAggregateMemory returns true if the seriesKey represents a pod-level
// aggregate metric (container=""). Used as fallback for pods that don't expose
// individual container metrics (e.g., static pods like kube-apiserver).
func isPodAggregateMemory(seriesKey string) bool {
	// Only accept container="" (pod aggregate)
	// Exclude container="POD" (pause container)
	if strings.Contains(seriesKey, `container="POD"`) {
		return false
	}
	return strings.Contains(seriesKey, `container=""`)
}

// queryLatestSumFiltered queries latest values and sums them, applying a filter function
// This is used for memory metrics where we need to sum across workload containers only
func (p *PromMetricsSource) queryLatestSumFiltered(metricName string, labelMatchers map[string]string, filterFn func(seriesKey string) bool) (float64, error) {
	// Query with a recent time range to get all matching series
	now := time.Now()
	start := now.Add(-5 * time.Minute) // Look back 5 minutes for latest data

	seriesSamples, err := p.store.QueryRangePerSeries(metricName, labelMatchers, start, now)
	if err != nil {
		return 0, err
	}

	if len(seriesSamples) == 0 {
		return 0, fmt.Errorf("no matching series found")
	}

	var totalValue float64
	found := false

	for seriesKey, samples := range seriesSamples {
		// Apply filter if provided
		if filterFn != nil && !filterFn(seriesKey) {
			continue
		}

		if len(samples) == 0 {
			continue
		}

		// Get the latest sample from this series
		latestSample := samples[len(samples)-1]
		totalValue += latestSample.Value
		found = true
	}

	if !found {
		return 0, fmt.Errorf("no matching series found after filtering")
	}

	return totalValue, nil
}

// GetNodeMetrics retrieves metrics for a specific node from Prometheus
func (p *PromMetricsSource) GetNodeMetrics(ctx context.Context, nodeName string) (*metrics.NodeMetrics, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.isHealthyLocked() {
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
	// IMPORTANT: Must include "node" label to get metrics for the specific node
	// The "node" label is added by the scraper when collecting from each node
	labelMatchers := map[string]string{"id": "/", "node": nodeName}

	// Query CPU usage: container_cpu_usage_seconds_total (counter - needs rate calculation)
	if cpuRate, err := p.calculateCPURate("container_cpu_usage_seconds_total", labelMatchers, 40*time.Second); err == nil {
		// Convert CPU cores to millicores
		// Use Ceil to avoid truncating sub-millicores to 0
		nodeMetrics.CPUUsage = resource.NewMilliQuantity(int64(math.Ceil(cpuRate*1000)), resource.DecimalSI)
	}

	// Query Memory usage: container_memory_working_set_bytes (gauge - use latest value)
	if memUsage, err := p.store.QueryLatest("container_memory_working_set_bytes", labelMatchers); err == nil {
		nodeMetrics.MemoryUsage = resource.NewQuantity(int64(memUsage), resource.BinarySI)
	}

	// Query Network RX rate: container_network_receive_bytes_total (counter - needs rate calculation)
	// Use cAdvisor root container metrics aggregated across all interfaces
	if netRxRate, err := p.calculateCPURate("container_network_receive_bytes_total", labelMatchers, 40*time.Second); err == nil {
		nodeMetrics.NetworkRxRate = netRxRate // bytes/sec
		nodeMetrics.NetworkRxBytes = resource.NewQuantity(int64(netRxRate), resource.BinarySI)
	}

	// Query Network TX rate: container_network_transmit_bytes_total (counter - needs rate calculation)
	if netTxRate, err := p.calculateCPURate("container_network_transmit_bytes_total", labelMatchers, 40*time.Second); err == nil {
		nodeMetrics.NetworkTxRate = netTxRate // bytes/sec
		nodeMetrics.NetworkTxBytes = resource.NewQuantity(int64(netTxRate), resource.BinarySI)
	}

	// Query Disk Read rate: container_fs_reads_bytes_total (counter - needs rate calculation)
	if diskReadRate, err := p.calculateCPURate("container_fs_reads_bytes_total", labelMatchers, 40*time.Second); err == nil {
		nodeMetrics.DiskReadRate = diskReadRate // bytes/sec
	}

	// Query Disk Write rate: container_fs_writes_bytes_total (counter - needs rate calculation)
	if diskWriteRate, err := p.calculateCPURate("container_fs_writes_bytes_total", labelMatchers, 40*time.Second); err == nil {
		nodeMetrics.DiskWriteRate = diskWriteRate // bytes/sec
	}

	// Note: Load averages (node_load1/5/15) are not exposed by kubelet/cAdvisor
	// They require node_exporter. Values will remain 0.

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

	if !p.isHealthyLocked() {
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
	// Filter to only include actual workload containers by excluding:
	// - container="" (pod-level aggregate that would cause double-counting)
	// - container="POD" (pause container)
	labelMatchers := map[string]string{
		"pod":       podName,
		"namespace": namespace,
	}

	containerMetrics := metrics.ContainerMetrics{
		Name: "main", // TODO: Get actual container name from labels
	}

	// Get CPU usage for containers (counter - needs rate calculation)
	// Filter to: workload containers only + cpu="total" only (not per-CPU breakdown)
	if cpuRate, err := p.calculateCPURateWithFilter("container_cpu_usage_seconds_total", labelMatchers, 40*time.Second, isWorkloadContainerCPUTotal); err == nil {
		// Use Ceil to avoid truncating sub-millicores to 0 (e.g., 0.7m → 1m, not 0m)
		containerMetrics.CPUUsage = resource.NewMilliQuantity(int64(math.Ceil(cpuRate*1000)), resource.DecimalSI)
	}

	// Get memory usage (gauge - sum across workload containers only)
	// Memory metrics don't have cpu label, so use simpler filter
	// Strategy: Try individual containers first, fall back to pod aggregate for static pods
	if memUsage, err := p.queryLatestSumFiltered("container_memory_working_set_bytes", labelMatchers, isWorkloadContainerMemory); err == nil {
		containerMetrics.MemoryUsage = resource.NewQuantity(int64(memUsage), resource.BinarySI)
	} else {
		// Fallback: Some pods (static pods like kube-apiserver) only have aggregate metrics
		if memUsage, err := p.queryLatestSumFiltered("container_memory_working_set_bytes", labelMatchers, isPodAggregateMemory); err == nil {
			containerMetrics.MemoryUsage = resource.NewQuantity(int64(memUsage), resource.BinarySI)
		}
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

	if !p.isHealthyLocked() {
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
	// Healthy if ANY component is healthy (handles concurrent scrapes correctly)
	for _, healthy := range p.componentHealthy {
		if healthy {
			return true
		}
	}
	return false
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
		Healthy:      p.isHealthyLocked(),
	}
}

// SetHealthCallback registers a callback for health state changes.
// The callback is invoked whenever IsHealthy() would return a different value.
// Note: The callback is NOT invoked immediately - callers should check IsHealthy()
// for the initial state and use the callback only for subsequent changes.
func (p *PromMetricsSource) SetHealthCallback(callback func(healthy bool, info metrics.SourceInfo)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthCallback = callback
}

// buildSourceInfoLocked builds SourceInfo while holding the lock.
// Must be called with p.mu held.
func (p *PromMetricsSource) buildSourceInfoLocked(healthy bool) metrics.SourceInfo {
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
		Healthy:      healthy,
	}
}

// handleError is called when an error occurs during metrics collection
func (p *PromMetricsSource) handleError(component prom.ComponentType, err error) {
	var shouldNotify bool
	var info metrics.SourceInfo

	p.mu.Lock()
	p.lastError = err
	p.errorCount++

	// Track health per-component to handle concurrent scrapes correctly
	wasHealthy := p.isHealthyLocked()
	p.componentHealthy[component] = false
	nowHealthy := p.isHealthyLocked()

	// Check if we need to notify (overall state changed from healthy to unhealthy)
	if wasHealthy && !nowHealthy {
		shouldNotify = true
		info = p.buildSourceInfoLocked(false)
	}
	p.mu.Unlock()

	// Notify outside the lock to avoid deadlock
	if shouldNotify && p.healthCallback != nil {
		p.healthCallback(false, info)
	}
}

// handleMetricsCollected is called when metrics are successfully collected
func (p *PromMetricsSource) handleMetricsCollected(component prom.ComponentType, collectedMetrics *prom.ScrapedMetrics) {
	var shouldNotify bool
	var info metrics.SourceInfo

	p.mu.Lock()
	p.lastError = nil
	p.lastScrape = time.Now()

	// Ensure we have a reference to the store
	if p.store == nil && p.controller != nil {
		p.store = p.controller.GetStore()
	}

	// Track health per-component to handle concurrent scrapes correctly
	wasHealthy := p.isHealthyLocked()
	p.componentHealthy[component] = true
	nowHealthy := p.isHealthyLocked()

	// Check if we need to notify (overall state changed from unhealthy to healthy)
	if !wasHealthy && nowHealthy {
		shouldNotify = true
		info = p.buildSourceInfoLocked(true)
	}
	p.mu.Unlock()

	// Notify outside the lock to avoid deadlock
	if shouldNotify && p.healthCallback != nil {
		p.healthCallback(true, info)
	}
}

// recordError updates health status after an error (for non-component errors like Start failure)
func (p *PromMetricsSource) recordError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastError = err
	p.errorCount++
	// Don't change component health - this is for controller-level errors
}

// isHealthyLocked checks if any component is healthy (must be called with lock held)
func (p *PromMetricsSource) isHealthyLocked() bool {
	for _, healthy := range p.componentHealthy {
		if healthy {
			return true
		}
	}
	return false
}

// setHealthyForTesting sets health state for testing purposes (not thread-safe, use only in tests)
func (p *PromMetricsSource) setHealthyForTesting(healthy bool) {
	if healthy {
		// Set at least one component as healthy
		p.componentHealthy[prom.ComponentKubelet] = true
	} else {
		// Clear all component health
		p.componentHealthy = make(map[prom.ComponentType]bool)
	}
}

// GetNodeHistory retrieves historical data for a specific resource on a node.
// For Prometheus, this queries the stored time series data.
func (p *PromMetricsSource) GetNodeHistory(ctx context.Context, nodeName string, query metrics.HistoryQuery) (*metrics.ResourceHistory, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.isHealthyLocked() {
		return nil, fmt.Errorf("prometheus source is not healthy")
	}

	if p.store == nil {
		return nil, fmt.Errorf("metrics store not initialized")
	}

	var metricName string
	labelMatchers := map[string]string{"id": "/", "node": nodeName}

	switch query.Resource {
	case metrics.ResourceCPU:
		metricName = "container_cpu_usage_seconds_total"
	case metrics.ResourceMemory:
		metricName = "container_memory_working_set_bytes"
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", query.Resource)
	}

	now := time.Now()
	start := now.Add(-query.Duration)

	samples, err := p.store.QueryRange(metricName, labelMatchers, start, now)
	if err != nil {
		return nil, err
	}

	history := &metrics.ResourceHistory{
		Resource:   query.Resource,
		DataPoints: make([]metrics.HistoryDataPoint, 0, len(samples)),
		MinValue:   math.MaxFloat64,
		MaxValue:   -math.MaxFloat64,
	}

	// Convert samples to history data points
	// For CPU counters, we need to calculate rates between consecutive points
	if query.Resource == metrics.ResourceCPU && len(samples) >= 2 {
		for i := 1; i < len(samples); i++ {
			prev := samples[i-1]
			curr := samples[i]

			deltaValue := curr.Value - prev.Value
			deltaTimeMs := curr.Timestamp - prev.Timestamp
			deltaTimeSeconds := float64(deltaTimeMs) / 1000.0

			if deltaTimeSeconds <= 0 {
				continue
			}

			// Handle counter reset
			if deltaValue < 0 {
				deltaValue = curr.Value
			}

			// Rate in millicores
			rate := (deltaValue / deltaTimeSeconds) * 1000

			dp := metrics.HistoryDataPoint{
				Timestamp: time.UnixMilli(curr.Timestamp),
				Value:     rate,
			}

			history.DataPoints = append(history.DataPoints, dp)

			if rate < history.MinValue {
				history.MinValue = rate
			}
			if rate > history.MaxValue {
				history.MaxValue = rate
			}
		}
	} else {
		// For gauges (memory), use raw values
		for _, sample := range samples {
			dp := metrics.HistoryDataPoint{
				Timestamp: time.UnixMilli(sample.Timestamp),
				Value:     sample.Value,
			}

			history.DataPoints = append(history.DataPoints, dp)

			if sample.Value < history.MinValue {
				history.MinValue = sample.Value
			}
			if sample.Value > history.MaxValue {
				history.MaxValue = sample.Value
			}
		}
	}

	// Apply MaxPoints limit if specified
	if query.MaxPoints > 0 && len(history.DataPoints) > query.MaxPoints {
		history.DataPoints = downsampleDataPoints(history.DataPoints, query.MaxPoints)
	}

	// Reset min/max if no data points
	if len(history.DataPoints) == 0 {
		history.MinValue = 0
		history.MaxValue = 0
	}

	return history, nil
}

// GetPodHistory retrieves historical data for a specific resource on a pod.
// For Prometheus, this queries the stored time series data.
func (p *PromMetricsSource) GetPodHistory(ctx context.Context, namespace, podName string, query metrics.HistoryQuery) (*metrics.ResourceHistory, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.isHealthyLocked() {
		return nil, fmt.Errorf("prometheus source is not healthy")
	}

	if p.store == nil {
		return nil, fmt.Errorf("metrics store not initialized")
	}

	var metricName string
	labelMatchers := map[string]string{
		"pod":       podName,
		"namespace": namespace,
	}

	switch query.Resource {
	case metrics.ResourceCPU:
		metricName = "container_cpu_usage_seconds_total"
	case metrics.ResourceMemory:
		metricName = "container_memory_working_set_bytes"
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", query.Resource)
	}

	now := time.Now()
	start := now.Add(-query.Duration)

	// Get samples per series to handle multiple containers correctly
	seriesSamples, err := p.store.QueryRangePerSeries(metricName, labelMatchers, start, now)
	if err != nil {
		return nil, err
	}

	history := &metrics.ResourceHistory{
		Resource:   query.Resource,
		DataPoints: make([]metrics.HistoryDataPoint, 0),
		MinValue:   math.MaxFloat64,
		MaxValue:   -math.MaxFloat64,
	}

	// For pods, we need to aggregate across containers at each timestamp
	// Build a map of timestamp -> aggregated value
	timestampValues := make(map[int64]float64)

	// For memory, determine the right filter based on available data
	// Some pods (static pods) only have aggregate metrics (container="")
	memoryFilter := isWorkloadContainerMemory
	if query.Resource == metrics.ResourceMemory {
		hasIndividualContainers := false
		for seriesKey := range seriesSamples {
			if isWorkloadContainerMemory(seriesKey) {
				hasIndividualContainers = true
				break
			}
		}
		if !hasIndividualContainers {
			// Fallback to aggregate for static pods
			memoryFilter = isPodAggregateMemory
		}
	}

	for seriesKey, samples := range seriesSamples {
		// Apply filter for CPU (only workload containers, cpu="total")
		if query.Resource == metrics.ResourceCPU && !isWorkloadContainerCPUTotal(seriesKey) {
			continue
		}
		// Apply filter for memory (using determined filter)
		if query.Resource == metrics.ResourceMemory && !memoryFilter(seriesKey) {
			continue
		}

		if query.Resource == metrics.ResourceCPU && len(samples) >= 2 {
			// Calculate rates for CPU counter
			for i := 1; i < len(samples); i++ {
				prev := samples[i-1]
				curr := samples[i]

				deltaValue := curr.Value - prev.Value
				deltaTimeMs := curr.Timestamp - prev.Timestamp
				deltaTimeSeconds := float64(deltaTimeMs) / 1000.0

				if deltaTimeSeconds <= 0 {
					continue
				}

				if deltaValue < 0 {
					deltaValue = curr.Value
				}

				rate := (deltaValue / deltaTimeSeconds) * 1000
				timestampValues[curr.Timestamp] += rate
			}
		} else {
			// For memory gauges, sum across containers at each timestamp
			for _, sample := range samples {
				timestampValues[sample.Timestamp] += sample.Value
			}
		}
	}

	// Convert map to sorted slice
	timestamps := make([]int64, 0, len(timestampValues))
	for ts := range timestampValues {
		timestamps = append(timestamps, ts)
	}

	// Sort timestamps
	for i := 0; i < len(timestamps)-1; i++ {
		for j := i + 1; j < len(timestamps); j++ {
			if timestamps[i] > timestamps[j] {
				timestamps[i], timestamps[j] = timestamps[j], timestamps[i]
			}
		}
	}

	for _, ts := range timestamps {
		value := timestampValues[ts]
		dp := metrics.HistoryDataPoint{
			Timestamp: time.UnixMilli(ts),
			Value:     value,
		}
		history.DataPoints = append(history.DataPoints, dp)

		if value < history.MinValue {
			history.MinValue = value
		}
		if value > history.MaxValue {
			history.MaxValue = value
		}
	}

	// Apply MaxPoints limit if specified
	if query.MaxPoints > 0 && len(history.DataPoints) > query.MaxPoints {
		history.DataPoints = downsampleDataPoints(history.DataPoints, query.MaxPoints)
	}

	// Reset min/max if no data points
	if len(history.DataPoints) == 0 {
		history.MinValue = 0
		history.MaxValue = 0
	}

	return history, nil
}

// SupportsHistory returns true since Prometheus has historical data
func (p *PromMetricsSource) SupportsHistory() bool {
	return true
}

// downsampleDataPoints reduces the number of data points by averaging
func downsampleDataPoints(points []metrics.HistoryDataPoint, maxPoints int) []metrics.HistoryDataPoint {
	if len(points) <= maxPoints {
		return points
	}

	result := make([]metrics.HistoryDataPoint, maxPoints)
	bucketSize := float64(len(points)) / float64(maxPoints)

	for i := 0; i < maxPoints; i++ {
		startIdx := int(float64(i) * bucketSize)
		endIdx := int(float64(i+1) * bucketSize)
		if endIdx > len(points) {
			endIdx = len(points)
		}

		// Average the values in this bucket
		var sum float64
		var count int
		var lastTimestamp time.Time
		for j := startIdx; j < endIdx; j++ {
			sum += points[j].Value
			lastTimestamp = points[j].Timestamp
			count++
		}

		if count > 0 {
			result[i] = metrics.HistoryDataPoint{
				Timestamp: lastTimestamp,
				Value:     sum / float64(count),
			}
		}
	}

	return result
}
