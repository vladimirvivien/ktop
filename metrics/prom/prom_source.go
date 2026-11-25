package prom

import (
	"context"
	"fmt"
	"math"
	"sort"
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
		ScrapeInterval: 4 * time.Second,
		RetentionTime:  5 * time.Minute,
		MaxSamples:     50,
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

// WaitForWarmup waits for two scrape cycles to complete so rate-based metrics
// have populated counter deltas before an initial snapshot is captured.
func (p *PromMetricsSource) WaitForWarmup(ctx context.Context) error {
	if p == nil || p.controller == nil {
		return nil
	}
	return p.controller.WaitForCycles(ctx, 2)
}

// counterDelta computes the counter delta and elapsed seconds between the first
// and last samples in a window, handling counter resets.
func counterDelta(samples []*prom.MetricSample) (deltaVal float64, deltaSec float64) {
	if len(samples) < 2 {
		return 0, 0
	}
	first := samples[0]
	last := samples[len(samples)-1]
	deltaSec = float64(last.Timestamp-first.Timestamp) / 1000.0
	if deltaSec <= 0 {
		return 0, 0
	}
	deltaVal = last.Value - first.Value
	if deltaVal < 0 {
		deltaVal = last.Value
	}
	return deltaVal, deltaSec
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// calculateCFSThrottleByContainer computes CFS CPU throttling percentages
// per container and the maximum across workload containers for a pod.
func (p *PromMetricsSource) calculateCFSThrottleByContainer(labelMatchers map[string]string, window time.Duration) (map[string]float64, float64) {
	now := time.Now()
	start := now.Add(-window)
	perContainer := make(map[string]float64)
	var podMax float64

	throttledPeriods := make(map[string]float64)
	if tpSeries, err := p.store.QueryRangePerSeries("container_cpu_cfs_throttled_periods_total", labelMatchers, start, now); err == nil {
		for seriesKey, samples := range tpSeries {
			if !isPSIWorkloadContainer(seriesKey) || len(samples) < 2 {
				continue
			}
			cName, ok := seriesLabelValue(seriesKey, "container")
			if !ok || cName == "" {
				continue
			}
			deltaVal, deltaSec := counterDelta(samples)
			if deltaSec > 0 {
				throttledPeriods[cName] = deltaVal
			}
		}
	}

	if totalSeries, err := p.store.QueryRangePerSeries("container_cpu_cfs_periods_total", labelMatchers, start, now); err == nil {
		for seriesKey, samples := range totalSeries {
			if !isPSIWorkloadContainer(seriesKey) || len(samples) < 2 {
				continue
			}
			cName, ok := seriesLabelValue(seriesKey, "container")
			if !ok || cName == "" {
				continue
			}
			deltaTotal, deltaSec := counterDelta(samples)
			if deltaSec <= 0 || deltaTotal <= 0 {
				continue
			}
			if tp, exists := throttledPeriods[cName]; exists {
				pct := clampPct((tp / deltaTotal) * 100)
				perContainer[cName] = pct
				if pct > podMax {
					podMax = pct
				}
			}
		}
	}

	if len(perContainer) > 0 {
		return perContainer, podMax
	}

	if tsSeries, err := p.store.QueryRangePerSeries("container_cpu_cfs_throttled_seconds_total", labelMatchers, start, now); err == nil {
		for seriesKey, samples := range tsSeries {
			if !isPSIWorkloadContainer(seriesKey) || len(samples) < 2 {
				continue
			}
			cName, ok := seriesLabelValue(seriesKey, "container")
			if !ok || cName == "" {
				continue
			}
			deltaVal, deltaSec := counterDelta(samples)
			if deltaSec <= 0 {
				continue
			}
			pct := clampPct((deltaVal / deltaSec) * 100)
			perContainer[cName] = pct
			if pct > podMax {
				podMax = pct
			}
		}
	}

	return perContainer, podMax
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

const extendedRateLookback = 3 * time.Minute
const maxSeriesSampleAgeMs = int64(90 * 1000)

// selectNodeCPUSeries returns a series filter for container_cpu_usage_seconds_total
// at node scope (id="/"). If any series reports cpu="total" or omits the cpu
// label, only those aggregate series are selected to prevent double-counting
// with per-core cpu="0".. series. If only per-core series exist (e.g., cgroups
// v1), all series are selected so their rates sum across cores.
func selectNodeCPUSeries(seriesMap map[string][]*prom.MetricSample) func(string) bool {
	hasAggregate := false
	for seriesKey := range seriesMap {
		cpuVal, hasCPU := seriesLabelValue(seriesKey, "cpu")
		if !hasCPU || cpuVal == "total" {
			hasAggregate = true
			break
		}
	}
	if !hasAggregate {
		return nil
	}
	return func(seriesKey string) bool {
		cpuVal, hasCPU := seriesLabelValue(seriesKey, "cpu")
		return !hasCPU || cpuVal == "total"
	}
}

// querySeriesForRate queries per-series samples over window and falls back to an
// extended lookback for series that have fewer than 2 samples in window due to
// dynamic cAdvisor housekeeping intervals exceeding window (up to 60s).
func querySeriesForRate(store prom.MetricsStore, metricName string, labelMatchers map[string]string, window time.Duration, filterFn func(string) bool) (map[string][]*prom.MetricSample, error) {
	now := time.Now()
	seriesSamples, err := store.QueryRangePerSeries(metricName, labelMatchers, now.Add(-window), now)
	if err != nil {
		return nil, err
	}

	if window >= extendedRateLookback {
		return seriesSamples, nil
	}

	needsExtended := len(seriesSamples) == 0
	if !needsExtended {
		for k, samples := range seriesSamples {
			if filterFn != nil && !filterFn(k) {
				continue
			}
			if len(samples) < 2 {
				needsExtended = true
				break
			}
		}
	}
	if !needsExtended {
		return seriesSamples, nil
	}

	extSeries, extErr := store.QueryRangePerSeries(metricName, labelMatchers, now.Add(-extendedRateLookback), now)
	if extErr != nil || len(extSeries) == 0 {
		return seriesSamples, nil
	}

	cutoffMs := now.UnixMilli() - maxSeriesSampleAgeMs
	if seriesSamples == nil {
		seriesSamples = make(map[string][]*prom.MetricSample)
	}
	for k, ext := range extSeries {
		if filterFn != nil && !filterFn(k) {
			continue
		}
		if len(seriesSamples[k]) >= 2 {
			continue
		}
		if len(ext) >= 2 && ext[len(ext)-1].Timestamp >= cutoffMs {
			seriesSamples[k] = ext[len(ext)-2:]
		}
	}
	return seriesSamples, nil
}

// sumSeriesRates calculates counter rate for each matching series and sums them.
func sumSeriesRates(seriesSamples map[string][]*prom.MetricSample, filterFn func(string) bool) (float64, error) {
	if len(seriesSamples) == 0 {
		return 0, fmt.Errorf("no matching series found for rate calculation")
	}

	var totalRate float64
	validSeriesCount := 0

	for seriesKey, samples := range seriesSamples {
		if filterFn != nil && !filterFn(seriesKey) {
			continue
		}
		if len(samples) < 2 {
			continue
		}

		deltaValue, deltaTimeSeconds := counterDelta(samples)
		if deltaTimeSeconds <= 0 {
			continue
		}

		totalRate += deltaValue / deltaTimeSeconds
		validSeriesCount++
	}

	if validSeriesCount == 0 {
		return 0, fmt.Errorf("insufficient samples for rate calculation in any series")
	}

	return totalRate, nil
}

// calculateCPURateWithFilter calculates CPU rate with optional series filter function
func (p *PromMetricsSource) calculateCPURateWithFilter(metricName string, labelMatchers map[string]string, window time.Duration, filterFn func(seriesKey string) bool) (float64, error) {
	seriesSamples, err := querySeriesForRate(p.store, metricName, labelMatchers, window, filterFn)
	if err != nil {
		return 0, err
	}
	return sumSeriesRates(seriesSamples, filterFn)
}

// seriesLabelValue extracts the exact value of labelName from a Prometheus series key
// formatted as {k1="v1", k2="v2"} without heap allocations.
// It checks that the label name is preceded by a delimiter ('{', ',', or ' ') so a
// suffix of another label name cannot false-match.
func seriesLabelValue(seriesKey, labelName string) (string, bool) {
	search := labelName + `="`
	offset := 0
	for offset < len(seriesKey) {
		idx := strings.Index(seriesKey[offset:], search)
		if idx < 0 {
			return "", false
		}
		pos := offset + idx
		if pos == 0 || seriesKey[pos-1] == '{' || seriesKey[pos-1] == ',' || seriesKey[pos-1] == ' ' {
			valStart := pos + len(search)
			valEnd := strings.IndexByte(seriesKey[valStart:], '"')
			if valEnd < 0 {
				return "", false
			}
			return seriesKey[valStart : valStart+valEnd], true
		}
		offset = pos + len(search)
	}
	return "", false
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
	if container, ok := seriesLabelValue(seriesKey, "container"); ok && container == "POD" {
		return false
	}

	if cpu, ok := seriesLabelValue(seriesKey, "cpu"); ok && cpu != "total" {
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
	if container, ok := seriesLabelValue(seriesKey, "container"); ok && (container == "POD" || container == "") {
		return false
	}

	return true
}

// isPodAggregateMemory returns true if the seriesKey represents a pod-level
// aggregate metric (container=""). Used as fallback for pods that don't expose
// individual container metrics (e.g., static pods like kube-apiserver).
func isPodAggregateMemory(seriesKey string) bool {
	container, ok := seriesLabelValue(seriesKey, "container")
	return ok && container == ""
}

// isPSIWorkloadContainer returns true if the seriesKey represents a workload
// container's PSI series. Excludes the pause container (container="POD") and
// the pod-level aggregate (container="") — PSI is meaningful per workload
// container, and the pod-level signal is computed as max across them.
func isPSIWorkloadContainer(seriesKey string) bool {
	if container, ok := seriesLabelValue(seriesKey, "container"); ok && (container == "POD" || container == "") {
		return false
	}
	return true
}

// calculatePSIMaxRate returns the maximum per-series stall percentage across
// all matching series, in [0..100].
//
// PSI percentages from sibling containers are independent — a pod with one
// 50%-stalled container and one idle container is not 25% stalled, it has a
// 50%-stalled container. Use MAX, not SUM or MEAN.
func (p *PromMetricsSource) calculatePSIMaxRate(metricName string, labelMatchers map[string]string, window time.Duration, filterFn func(seriesKey string) bool) (float64, error) {
	seriesSamples, err := querySeriesForRate(p.store, metricName, labelMatchers, window, filterFn)
	if err != nil {
		return 0, err
	}
	if len(seriesSamples) == 0 {
		return 0, fmt.Errorf("no matching series found for psi rate calculation")
	}

	var maxRate float64
	validSeriesCount := 0

	for seriesKey, samples := range seriesSamples {
		if filterFn != nil && !filterFn(seriesKey) {
			continue
		}
		if len(samples) < 2 {
			continue
		}

		first := samples[0]
		last := samples[len(samples)-1]

		deltaValue := last.Value - first.Value
		deltaTimeSec := float64(last.Timestamp-first.Timestamp) / 1000.0
		if deltaTimeSec <= 0 {
			continue
		}
		// Counter reset (container restart): treat last value as the rate basis.
		if deltaValue < 0 {
			deltaValue = last.Value
		}

		rate := deltaValue / deltaTimeSec
		if rate > maxRate {
			maxRate = rate
		}
		validSeriesCount++
	}

	if validSeriesCount == 0 {
		return 0, fmt.Errorf("insufficient samples for psi rate calculation")
	}
	return maxRate * 100, nil
}

// latestPerSeriesQuerier is an optional fast path implemented by prom.InMemoryStore
// to fetch only the latest sample per matching series without allocating full
// time-range slices.
type latestPerSeriesQuerier interface {
	QueryLatestPerSeries(metricName string, labelMatchers map[string]string, since time.Time) (map[string]prom.MetricSample, error)
}

// queryLatestSumFiltered queries latest values and sums them, applying a filter function
// This is used for memory metrics where we need to sum across workload containers only
func (p *PromMetricsSource) queryLatestSumFiltered(metricName string, labelMatchers map[string]string, filterFn func(seriesKey string) bool) (float64, error) {
	// Query with a recent time range to get all matching series
	now := time.Now()
	start := now.Add(-5 * time.Minute) // Look back 5 minutes for latest data

	if lq, ok := p.store.(latestPerSeriesQuerier); ok {
		latestSeries, err := lq.QueryLatestPerSeries(metricName, labelMatchers, start)
		if err != nil {
			return 0, err
		}
		if len(latestSeries) == 0 {
			return 0, fmt.Errorf("no matching series found")
		}
		var totalValue float64
		found := false
		for seriesKey, sample := range latestSeries {
			if filterFn != nil && !filterFn(seriesKey) {
				continue
			}
			totalValue += sample.Value
			found = true
		}
		if !found {
			return 0, fmt.Errorf("no matching series found after filtering")
		}
		return totalValue, nil
	}

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
	if cpuSeries, err := querySeriesForRate(p.store, "container_cpu_usage_seconds_total", labelMatchers, 40*time.Second, nil); err == nil {
		if cpuRate, err := sumSeriesRates(cpuSeries, selectNodeCPUSeries(cpuSeries)); err == nil {
			// Convert CPU cores to millicores
			// Use Ceil to avoid truncating sub-millicores to 0
			nodeMetrics.CPUUsage = resource.NewMilliQuantity(int64(math.Ceil(cpuRate*1000)), resource.DecimalSI)
		}
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

	// Pressure Stall Information for the node's root cgroup (id="/").
	// Rate of a seconds counter over the window is fraction-stalled in [0..1];
	// percent values stored on PSIMetrics multiply by 100.
	const psiWindow = 40 * time.Second
	if v, err := p.calculateCPURate("container_pressure_cpu_waiting_seconds_total", labelMatchers, psiWindow); err == nil {
		nodeMetrics.PSI.CPUStallPct = v * 100
	}
	if v, err := p.calculateCPURate("container_pressure_cpu_stalled_seconds_total", labelMatchers, psiWindow); err == nil {
		nodeMetrics.PSI.CPUStalledPct = v * 100
	}
	if v, err := p.calculateCPURate("container_pressure_memory_waiting_seconds_total", labelMatchers, psiWindow); err == nil {
		nodeMetrics.PSI.MemStallPct = v * 100
	}
	if v, err := p.calculateCPURate("container_pressure_memory_stalled_seconds_total", labelMatchers, psiWindow); err == nil {
		nodeMetrics.PSI.MemStalledPct = v * 100
	}
	if v, err := p.calculateCPURate("container_pressure_io_waiting_seconds_total", labelMatchers, psiWindow); err == nil {
		nodeMetrics.PSI.IOStallPct = v * 100
	}
	if v, err := p.calculateCPURate("container_pressure_io_stalled_seconds_total", labelMatchers, psiWindow); err == nil {
		nodeMetrics.PSI.IOStalledPct = v * 100
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

	// CFS CPU throttling percentage per container and pod-level max.
	const cfsWindow = 40 * time.Second
	perContainerThrottle, podMaxThrottle := p.calculateCFSThrottleByContainer(labelMatchers, cfsWindow)
	podMetrics.CPUThrottledPct = podMaxThrottle
	if len(podMetrics.Containers) > 0 {
		podMetrics.Containers[0].CPUThrottled = podMaxThrottle
	}
	for cName, pct := range perContainerThrottle {
		podMetrics.Containers = append(podMetrics.Containers, metrics.ContainerMetrics{
			Name:         cName,
			CPUThrottled: pct,
		})
	}

	// Pressure Stall Information at pod scope. PSI percentages don't sum
	// meaningfully across containers; pod-level signal is the worst-affected
	// container per axis.
	const psiWindow = 40 * time.Second
	if v, err := p.calculatePSIMaxRate("container_pressure_cpu_waiting_seconds_total", labelMatchers, psiWindow, isPSIWorkloadContainer); err == nil {
		podMetrics.PSI.CPUStallPct = v
	}
	if v, err := p.calculatePSIMaxRate("container_pressure_cpu_stalled_seconds_total", labelMatchers, psiWindow, isPSIWorkloadContainer); err == nil {
		podMetrics.PSI.CPUStalledPct = v
	}
	if v, err := p.calculatePSIMaxRate("container_pressure_memory_waiting_seconds_total", labelMatchers, psiWindow, isPSIWorkloadContainer); err == nil {
		podMetrics.PSI.MemStallPct = v
	}
	if v, err := p.calculatePSIMaxRate("container_pressure_memory_stalled_seconds_total", labelMatchers, psiWindow, isPSIWorkloadContainer); err == nil {
		podMetrics.PSI.MemStalledPct = v
	}
	if v, err := p.calculatePSIMaxRate("container_pressure_io_waiting_seconds_total", labelMatchers, psiWindow, isPSIWorkloadContainer); err == nil {
		podMetrics.PSI.IOStallPct = v
	}
	if v, err := p.calculatePSIMaxRate("container_pressure_io_stalled_seconds_total", labelMatchers, psiWindow, isPSIWorkloadContainer); err == nil {
		podMetrics.PSI.IOStalledPct = v
	}

	return podMetrics, nil
}

// GetPodNetworkDiskMetrics retrieves network and disk I/O rates for a specific pod.
// Network metrics are at the pod level (shared network namespace via pause container).
// Disk metrics are aggregated across containers in the pod.
// Returns zero values if metrics are not available.
// Note: Pod-level network metrics are not available on containerd-based clusters.
func (p *PromMetricsSource) GetPodNetworkDiskMetrics(ctx context.Context, namespace, podName string) (netRx, netTx, diskRead, diskWrite float64, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.isHealthyLocked() {
		return 0, 0, 0, 0, fmt.Errorf("prometheus source is not healthy")
	}

	if p.store == nil {
		return 0, 0, 0, 0, nil
	}

	// Pod-level metrics: filter by pod and namespace
	labelMatchers := map[string]string{
		"pod":       podName,
		"namespace": namespace,
	}

	// Query Disk Read rate from cAdvisor
	if rate, calcErr := p.calculateCPURate("container_fs_reads_bytes_total", labelMatchers, 40*time.Second); calcErr == nil {
		diskRead = rate
	}

	// Query Disk Write rate from cAdvisor
	if rate, calcErr := p.calculateCPURate("container_fs_writes_bytes_total", labelMatchers, 40*time.Second); calcErr == nil {
		diskWrite = rate
	}

	// Note: netRx and netTx remain 0 - pod-level network metrics are not available
	// on containerd-based Kubernetes clusters (cAdvisor only exports node-level network)
	return netRx, netTx, diskRead, diskWrite, nil
}

// GetMetricsForPod retrieves metrics for a specific pod object
func (p *PromMetricsSource) GetMetricsForPod(ctx context.Context, pod metav1.Object) (*metrics.PodMetrics, error) {
	// Extract namespace and name from pod object
	// Delegate to GetPodMetrics
	return p.GetPodMetrics(ctx, pod.GetNamespace(), pod.GetName())
}

// GetAllPodMetrics retrieves metrics for all pods using a single pass per metric
// instead of a Cartesian namespaces × pods loop.
func (p *PromMetricsSource) GetAllPodMetrics(ctx context.Context) ([]*metrics.PodMetrics, error) {
	p.mu.RLock()
	if !p.isHealthyLocked() {
		p.mu.RUnlock()
		return nil, fmt.Errorf("prometheus source is not healthy")
	}
	if p.store == nil {
		p.mu.RUnlock()
		return nil, fmt.Errorf("metrics store not initialized")
	}
	store := p.store
	p.mu.RUnlock()

	type podKey struct {
		namespace string
		pod       string
	}
	type podAgg struct {
		cpuRate         float64
		hasCPU          bool
		workloadMem     float64
		hasWorkloadMem  bool
		aggMem          float64
		hasAggMem       bool
		psi             metrics.PSIMetrics
		cpuThrottledPct float64
	}

	now := time.Now()
	rateStart := now.Add(-40 * time.Second)
	memStart := now.Add(-5 * time.Minute)

	pods := make(map[podKey]*podAgg)
	getOrCreate := func(ns, pod string) *podAgg {
		k := podKey{namespace: ns, pod: pod}
		agg, ok := pods[k]
		if !ok {
			agg = &podAgg{}
			pods[k] = agg
		}
		return agg
	}

	// 1. Single pass over CPU usage series
	if cpuSeries, err := querySeriesForRate(store, "container_cpu_usage_seconds_total", nil, 40*time.Second, isWorkloadContainerCPUTotal); err == nil {
		for seriesKey, samples := range cpuSeries {
			ns, ok1 := seriesLabelValue(seriesKey, "namespace")
			pod, ok2 := seriesLabelValue(seriesKey, "pod")
			if !ok1 || !ok2 || ns == "" || pod == "" || !isWorkloadContainerCPUTotal(seriesKey) || len(samples) < 2 {
				continue
			}
			first := samples[0]
			last := samples[len(samples)-1]
			deltaSec := float64(last.Timestamp-first.Timestamp) / 1000.0
			if deltaSec <= 0 {
				continue
			}
			deltaVal := last.Value - first.Value
			if deltaVal < 0 {
				deltaVal = last.Value
			}
			agg := getOrCreate(ns, pod)
			agg.cpuRate += deltaVal / deltaSec
			agg.hasCPU = true
		}
	}

	// 2. Single pass over memory working set series
	recordMem := func(seriesKey string, latestVal float64) {
		ns, ok1 := seriesLabelValue(seriesKey, "namespace")
		pod, ok2 := seriesLabelValue(seriesKey, "pod")
		if !ok1 || !ok2 || ns == "" || pod == "" {
			return
		}
		if isWorkloadContainerMemory(seriesKey) {
			agg := getOrCreate(ns, pod)
			agg.workloadMem += latestVal
			agg.hasWorkloadMem = true
		} else if isPodAggregateMemory(seriesKey) {
			agg := getOrCreate(ns, pod)
			agg.aggMem += latestVal
			agg.hasAggMem = true
		}
	}
	if lq, ok := store.(latestPerSeriesQuerier); ok {
		if memSeries, err := lq.QueryLatestPerSeries("container_memory_working_set_bytes", nil, memStart); err == nil {
			for seriesKey, sample := range memSeries {
				recordMem(seriesKey, sample.Value)
			}
		}
	} else if memSeries, err := store.QueryRangePerSeries("container_memory_working_set_bytes", nil, memStart, now); err == nil {
		for seriesKey, samples := range memSeries {
			if len(samples) == 0 {
				continue
			}
			recordMem(seriesKey, samples[len(samples)-1].Value)
		}
	}

	// 3. Single pass per PSI metric axis
	collectPSI := func(metricName string, assign func(psi *metrics.PSIMetrics, pct float64)) {
		seriesMap, err := store.QueryRangePerSeries(metricName, nil, rateStart, now)
		if err != nil {
			return
		}
		for seriesKey, samples := range seriesMap {
			if !isPSIWorkloadContainer(seriesKey) || len(samples) < 2 {
				continue
			}
			ns, ok1 := seriesLabelValue(seriesKey, "namespace")
			pod, ok2 := seriesLabelValue(seriesKey, "pod")
			if !ok1 || !ok2 || ns == "" || pod == "" {
				continue
			}
			first := samples[0]
			last := samples[len(samples)-1]
			deltaSec := float64(last.Timestamp-first.Timestamp) / 1000.0
			if deltaSec <= 0 {
				continue
			}
			deltaVal := last.Value - first.Value
			if deltaVal < 0 {
				deltaVal = last.Value
			}
			pct := (deltaVal / deltaSec) * 100
			agg := getOrCreate(ns, pod)
			assign(&agg.psi, pct)
		}
	}

	collectPSI("container_pressure_cpu_waiting_seconds_total", func(psi *metrics.PSIMetrics, pct float64) {
		if pct > psi.CPUStallPct {
			psi.CPUStallPct = pct
		}
	})
	collectPSI("container_pressure_cpu_stalled_seconds_total", func(psi *metrics.PSIMetrics, pct float64) {
		if pct > psi.CPUStalledPct {
			psi.CPUStalledPct = pct
		}
	})
	collectPSI("container_pressure_memory_waiting_seconds_total", func(psi *metrics.PSIMetrics, pct float64) {
		if pct > psi.MemStallPct {
			psi.MemStallPct = pct
		}
	})
	collectPSI("container_pressure_memory_stalled_seconds_total", func(psi *metrics.PSIMetrics, pct float64) {
		if pct > psi.MemStalledPct {
			psi.MemStalledPct = pct
		}
	})
	collectPSI("container_pressure_io_waiting_seconds_total", func(psi *metrics.PSIMetrics, pct float64) {
		if pct > psi.IOStallPct {
			psi.IOStallPct = pct
		}
	})
	collectPSI("container_pressure_io_stalled_seconds_total", func(psi *metrics.PSIMetrics, pct float64) {
		if pct > psi.IOStalledPct {
			psi.IOStalledPct = pct
		}
	})

	// 4. Single pass for CFS CPU throttling percentage (periods ratio, fallback to throttled seconds rate)
	type containerKey struct {
		ns        string
		pod       string
		container string
	}
	throttledPeriods := make(map[containerKey]float64)
	if tpSeries, err := store.QueryRangePerSeries("container_cpu_cfs_throttled_periods_total", nil, rateStart, now); err == nil {
		for seriesKey, samples := range tpSeries {
			if !isPSIWorkloadContainer(seriesKey) || len(samples) < 2 {
				continue
			}
			ns, ok1 := seriesLabelValue(seriesKey, "namespace")
			pod, ok2 := seriesLabelValue(seriesKey, "pod")
			cName, ok3 := seriesLabelValue(seriesKey, "container")
			if !ok1 || !ok2 || !ok3 || ns == "" || pod == "" {
				continue
			}
			deltaVal, _ := counterDelta(samples)
			if deltaVal >= 0 {
				throttledPeriods[containerKey{ns: ns, pod: pod, container: cName}] = deltaVal
			}
		}
	}
	hasPeriodRatio := false
	if totalSeries, err := store.QueryRangePerSeries("container_cpu_cfs_periods_total", nil, rateStart, now); err == nil {
		for seriesKey, samples := range totalSeries {
			if !isPSIWorkloadContainer(seriesKey) || len(samples) < 2 {
				continue
			}
			ns, ok1 := seriesLabelValue(seriesKey, "namespace")
			pod, ok2 := seriesLabelValue(seriesKey, "pod")
			cName, ok3 := seriesLabelValue(seriesKey, "container")
			if !ok1 || !ok2 || !ok3 || ns == "" || pod == "" {
				continue
			}
			deltaTotal, _ := counterDelta(samples)
			if deltaTotal <= 0 {
				continue
			}
			if tp, ok := throttledPeriods[containerKey{ns: ns, pod: pod, container: cName}]; ok {
				hasPeriodRatio = true
				pct := clampPct((tp / deltaTotal) * 100)
				agg := getOrCreate(ns, pod)
				if pct > agg.cpuThrottledPct {
					agg.cpuThrottledPct = pct
				}
			}
		}
	}
	if !hasPeriodRatio {
		if tsSeries, err := store.QueryRangePerSeries("container_cpu_cfs_throttled_seconds_total", nil, rateStart, now); err == nil {
			for seriesKey, samples := range tsSeries {
				if !isPSIWorkloadContainer(seriesKey) || len(samples) < 2 {
					continue
				}
				ns, ok1 := seriesLabelValue(seriesKey, "namespace")
				pod, ok2 := seriesLabelValue(seriesKey, "pod")
				if !ok1 || !ok2 || ns == "" || pod == "" {
					continue
				}
				deltaVal, deltaSec := counterDelta(samples)
				if deltaSec <= 0 {
					continue
				}
				pct := clampPct((deltaVal / deltaSec) * 100)
				agg := getOrCreate(ns, pod)
				if pct > agg.cpuThrottledPct {
					agg.cpuThrottledPct = pct
				}
			}
		}
	}

	allPodMetrics := make([]*metrics.PodMetrics, 0, len(pods))
	for k, agg := range pods {
		pm := &metrics.PodMetrics{
			PodName:         k.pod,
			Namespace:       k.namespace,
			Timestamp:       now,
			PSI:             agg.psi,
			CPUThrottledPct: agg.cpuThrottledPct,
		}
		cm := metrics.ContainerMetrics{Name: "main"}
		if agg.hasCPU {
			cm.CPUUsage = resource.NewMilliQuantity(int64(math.Ceil(agg.cpuRate*1000)), resource.DecimalSI)
		}
		if agg.hasWorkloadMem {
			cm.MemoryUsage = resource.NewQuantity(int64(agg.workloadMem), resource.BinarySI)
		} else if agg.hasAggMem {
			cm.MemoryUsage = resource.NewQuantity(int64(agg.aggMem), resource.BinarySI)
		}
		if cm.CPUUsage != nil || cm.MemoryUsage != nil {
			pm.Containers = append(pm.Containers, cm)
		}
		allPodMetrics = append(allPodMetrics, pm)
	}

	return allPodMetrics, nil
}

// GetAllVolumeStats returns capacity, used, available, and inode statistics per
// "namespace/pvcName" from kubelet_volume_stats_* metrics.
func (p *PromMetricsSource) GetAllVolumeStats(ctx context.Context) (map[string]metrics.VolumeStats, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	p.mu.RLock()
	if !p.isHealthyLocked() {
		p.mu.RUnlock()
		return nil, fmt.Errorf("prometheus source is not healthy")
	}
	if p.store == nil {
		p.mu.RUnlock()
		return nil, fmt.Errorf("metrics store not initialized")
	}
	store := p.store
	p.mu.RUnlock()

	now := time.Now()
	since := now.Add(-5 * time.Minute)
	out := make(map[string]metrics.VolumeStats)

	getOrCreate := func(ns, pvc string) *metrics.VolumeStats {
		key := ns + "/" + pvc
		st, ok := out[key]
		if !ok {
			st = metrics.VolumeStats{Namespace: ns, PVCName: pvc}
		}
		return &st
	}
	commit := func(st *metrics.VolumeStats) {
		out[st.Namespace+"/"+st.PVCName] = *st
	}

	visitLatest := func(metricName string, assign func(st *metrics.VolumeStats, val float64)) {
		if lq, ok := store.(latestPerSeriesQuerier); ok {
			series, err := lq.QueryLatestPerSeries(metricName, nil, since)
			if err != nil {
				return
			}
			for seriesKey, sample := range series {
				ns, ok1 := seriesLabelValue(seriesKey, "namespace")
				pvc, ok2 := seriesLabelValue(seriesKey, "persistentvolumeclaim")
				if !ok1 || !ok2 || ns == "" || pvc == "" {
					continue
				}
				st := getOrCreate(ns, pvc)
				assign(st, sample.Value)
				commit(st)
			}
			return
		}
		seriesMap, err := store.QueryRangePerSeries(metricName, nil, since, now)
		if err != nil {
			return
		}
		for seriesKey, samples := range seriesMap {
			if len(samples) == 0 {
				continue
			}
			ns, ok1 := seriesLabelValue(seriesKey, "namespace")
			pvc, ok2 := seriesLabelValue(seriesKey, "persistentvolumeclaim")
			if !ok1 || !ok2 || ns == "" || pvc == "" {
				continue
			}
			st := getOrCreate(ns, pvc)
			assign(st, samples[len(samples)-1].Value)
			commit(st)
		}
	}

	visitLatest("kubelet_volume_stats_capacity_bytes", func(st *metrics.VolumeStats, val float64) {
		st.CapacityBytes = int64(val)
		st.HasBytes = true
	})
	visitLatest("kubelet_volume_stats_used_bytes", func(st *metrics.VolumeStats, val float64) {
		st.UsedBytes = int64(val)
		st.HasBytes = true
	})
	visitLatest("kubelet_volume_stats_available_bytes", func(st *metrics.VolumeStats, val float64) {
		st.AvailableBytes = int64(val)
		st.HasBytes = true
	})
	visitLatest("kubelet_volume_stats_inodes", func(st *metrics.VolumeStats, val float64) {
		st.InodesTotal = int64(val)
		st.HasInodes = true
	})
	visitLatest("kubelet_volume_stats_inodes_used", func(st *metrics.VolumeStats, val float64) {
		st.InodesUsed = int64(val)
		st.HasInodes = true
	})

	for k, st := range out {
		if st.CapacityBytes > 0 {
			st.UsedPct = clampPct((float64(st.UsedBytes) / float64(st.CapacityBytes)) * 100)
		}
		if st.InodesTotal > 0 {
			st.InodesUsedPct = clampPct((float64(st.InodesUsed) / float64(st.InodesTotal)) * 100)
		}
		out[k] = st
	}

	return out, nil
}

// GetClusterNetworkErrorRates returns cluster-total and per-node packet drop
// and error rates across node root cgroups (id="/").
func (p *PromMetricsSource) GetClusterNetworkErrorRates(ctx context.Context) (metrics.NetworkErrorRates, map[string]metrics.NetworkErrorRates, error) {
	if ctx != nil && ctx.Err() != nil {
		return metrics.NetworkErrorRates{}, nil, ctx.Err()
	}
	p.mu.RLock()
	if !p.isHealthyLocked() {
		p.mu.RUnlock()
		return metrics.NetworkErrorRates{}, nil, fmt.Errorf("prometheus source is not healthy")
	}
	if p.store == nil {
		p.mu.RUnlock()
		return metrics.NetworkErrorRates{}, nil, fmt.Errorf("metrics store not initialized")
	}
	store := p.store
	p.mu.RUnlock()

	const rateWindow = 40 * time.Second
	matchers := map[string]string{"id": "/"}
	var total metrics.NetworkErrorRates
	perNode := make(map[string]metrics.NetworkErrorRates)

	collectMetric := func(metricName string, assign func(r *metrics.NetworkErrorRates, rate float64)) {
		seriesMap, err := querySeriesForRate(store, metricName, matchers, rateWindow, nil)
		if err != nil {
			return
		}
		for seriesKey, samples := range seriesMap {
			if len(samples) < 2 {
				continue
			}
			deltaVal, deltaSec := counterDelta(samples)
			if deltaSec <= 0 {
				continue
			}
			rate := deltaVal / deltaSec
			assign(&total, rate)
			if nodeName, ok := seriesLabelValue(seriesKey, "node"); ok && nodeName != "" {
				nr := perNode[nodeName]
				assign(&nr, rate)
				perNode[nodeName] = nr
			}
		}
	}

	collectMetric("container_network_receive_packets_dropped_total", func(r *metrics.NetworkErrorRates, v float64) {
		r.RxDroppedPerSec += v
	})
	collectMetric("container_network_transmit_packets_dropped_total", func(r *metrics.NetworkErrorRates, v float64) {
		r.TxDroppedPerSec += v
	})
	collectMetric("container_network_receive_errors_total", func(r *metrics.NetworkErrorRates, v float64) {
		r.RxErrorsPerSec += v
	})
	collectMetric("container_network_transmit_errors_total", func(r *metrics.NetworkErrorRates, v float64) {
		r.TxErrorsPerSec += v
	})

	return total, perNode, nil
}

// GetPodNetworkErrorRates returns packet drop and error rates for a specific pod.
func (p *PromMetricsSource) GetPodNetworkErrorRates(ctx context.Context, namespace, podName string) (metrics.NetworkErrorRates, error) {
	if ctx != nil && ctx.Err() != nil {
		return metrics.NetworkErrorRates{}, ctx.Err()
	}
	p.mu.RLock()
	if !p.isHealthyLocked() {
		p.mu.RUnlock()
		return metrics.NetworkErrorRates{}, fmt.Errorf("prometheus source is not healthy")
	}
	if p.store == nil {
		p.mu.RUnlock()
		return metrics.NetworkErrorRates{}, fmt.Errorf("metrics store not initialized")
	}
	p.mu.RUnlock()

	const rateWindow = 40 * time.Second
	matchers := map[string]string{
		"namespace": namespace,
		"pod":       podName,
	}

	var out metrics.NetworkErrorRates
	if rate, err := p.calculateCPURate("container_network_receive_packets_dropped_total", matchers, rateWindow); err == nil {
		out.RxDroppedPerSec = rate
	}
	if rate, err := p.calculateCPURate("container_network_transmit_packets_dropped_total", matchers, rateWindow); err == nil {
		out.TxDroppedPerSec = rate
	}
	if rate, err := p.calculateCPURate("container_network_receive_errors_total", matchers, rateWindow); err == nil {
		out.RxErrorsPerSec = rate
	}
	if rate, err := p.calculateCPURate("container_network_transmit_errors_total", matchers, rateWindow); err == nil {
		out.TxErrorsPerSec = rate
	}
	return out, nil
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
// metricCount must be obtained before acquiring lock to avoid nested locking.
func (p *PromMetricsSource) buildSourceInfoLocked(healthy bool, metricCount int) metrics.SourceInfo {
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
	// Get metric count BEFORE acquiring lock to avoid nested locking
	metricCount := 0
	if p.store != nil {
		metricCount = len(p.store.GetMetricNames())
	}

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
		info = p.buildSourceInfoLocked(false, metricCount)
	}
	p.mu.Unlock()

	// Notify outside the lock to avoid deadlock
	if shouldNotify && p.healthCallback != nil {
		p.healthCallback(false, info)
	}
}

// handleMetricsCollected is called when metrics are successfully collected
func (p *PromMetricsSource) handleMetricsCollected(component prom.ComponentType, collectedMetrics *prom.ScrapedMetrics) {
	// Get metric count BEFORE acquiring lock to avoid nested locking
	metricCount := 0
	if p.store != nil {
		metricCount = len(p.store.GetMetricNames())
	}

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
		info = p.buildSourceInfoLocked(true, metricCount)
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

type tsSample struct {
	ts  int64
	val float64
}

func seriesCounterMilliRates(samples []*prom.MetricSample) []tsSample {
	if len(samples) < 2 {
		return nil
	}
	pts := make([]tsSample, 0, len(samples)-1)
	for i := 1; i < len(samples); i++ {
		prev := samples[i-1]
		curr := samples[i]
		deltaTimeSeconds := float64(curr.Timestamp-prev.Timestamp) / 1000.0
		if deltaTimeSeconds <= 0 {
			continue
		}
		deltaValue := curr.Value - prev.Value
		if deltaValue < 0 {
			deltaValue = curr.Value
		}
		pts = append(pts, tsSample{
			ts:  curr.Timestamp,
			val: (deltaValue / deltaTimeSeconds) * 1000,
		})
	}
	return pts
}

func seriesGaugePoints(samples []*prom.MetricSample) []tsSample {
	if len(samples) == 0 {
		return nil
	}
	pts := make([]tsSample, 0, len(samples))
	for _, s := range samples {
		pts = append(pts, tsSample{ts: s.Timestamp, val: s.Value})
	}
	return pts
}

// aggregateSeriesHistory combines per-series point slices chronologically.
// For a single series it preserves timestamps directly; for multiple series
// it sweeps sorted unique timestamps carrying forward each series' latest
// sample within maxSeriesSampleAgeMs so staggered cAdvisor updates do not
// produce transient dips.
func aggregateSeriesHistory(perSeries [][]tsSample) []metrics.HistoryDataPoint {
	if len(perSeries) == 0 {
		return []metrics.HistoryDataPoint{}
	}
	if len(perSeries) == 1 {
		out := make([]metrics.HistoryDataPoint, len(perSeries[0]))
		for i, pt := range perSeries[0] {
			out[i] = metrics.HistoryDataPoint{
				Timestamp: time.UnixMilli(pt.ts),
				Value:     pt.val,
			}
		}
		return out
	}

	tsSet := make(map[int64]struct{})
	for _, s := range perSeries {
		for _, pt := range s {
			tsSet[pt.ts] = struct{}{}
		}
	}
	if len(tsSet) == 0 {
		return []metrics.HistoryDataPoint{}
	}

	timestamps := make([]int64, 0, len(tsSet))
	for ts := range tsSet {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	cursors := make([]int, len(perSeries))
	lastVal := make([]float64, len(perSeries))
	lastTs := make([]int64, len(perSeries))
	seen := make([]bool, len(perSeries))

	out := make([]metrics.HistoryDataPoint, 0, len(timestamps))
	for _, ts := range timestamps {
		var sum float64
		for i, s := range perSeries {
			for cursors[i] < len(s) && s[cursors[i]].ts <= ts {
				lastVal[i] = s[cursors[i]].val
				lastTs[i] = s[cursors[i]].ts
				seen[i] = true
				cursors[i]++
			}
			if seen[i] && ts-lastTs[i] <= maxSeriesSampleAgeMs {
				sum += lastVal[i]
			}
		}
		out = append(out, metrics.HistoryDataPoint{
			Timestamp: time.UnixMilli(ts),
			Value:     sum,
		})
	}
	return out
}

func finalizeHistory(resource metrics.ResourceType, points []metrics.HistoryDataPoint, maxPoints int) *metrics.ResourceHistory {
	history := &metrics.ResourceHistory{
		Resource:   resource,
		DataPoints: points,
		MinValue:   math.MaxFloat64,
		MaxValue:   -math.MaxFloat64,
	}

	if maxPoints > 0 && len(history.DataPoints) > maxPoints {
		history.DataPoints = downsampleDataPoints(history.DataPoints, maxPoints)
	}

	if len(history.DataPoints) == 0 {
		history.MinValue = 0
		history.MaxValue = 0
		return history
	}

	for _, dp := range history.DataPoints {
		if dp.Value < history.MinValue {
			history.MinValue = dp.Value
		}
		if dp.Value > history.MaxValue {
			history.MaxValue = dp.Value
		}
	}
	return history
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

	seriesSamples, err := p.store.QueryRangePerSeries(metricName, labelMatchers, start, now)
	if err != nil {
		return nil, err
	}

	var cpuFilter func(string) bool
	if query.Resource == metrics.ResourceCPU {
		cpuFilter = selectNodeCPUSeries(seriesSamples)
	}

	perSeries := make([][]tsSample, 0, len(seriesSamples))
	for seriesKey, samples := range seriesSamples {
		if query.Resource == metrics.ResourceCPU {
			if cpuFilter != nil && !cpuFilter(seriesKey) {
				continue
			}
			if pts := seriesCounterMilliRates(samples); len(pts) > 0 {
				perSeries = append(perSeries, pts)
			}
		} else {
			if pts := seriesGaugePoints(samples); len(pts) > 0 {
				perSeries = append(perSeries, pts)
			}
		}
	}

	points := aggregateSeriesHistory(perSeries)
	return finalizeHistory(query.Resource, points, query.MaxPoints), nil
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

	perSeries := make([][]tsSample, 0, len(seriesSamples))
	for seriesKey, samples := range seriesSamples {
		if query.Resource == metrics.ResourceCPU {
			if !isWorkloadContainerCPUTotal(seriesKey) {
				continue
			}
			if pts := seriesCounterMilliRates(samples); len(pts) > 0 {
				perSeries = append(perSeries, pts)
			}
		} else {
			if !memoryFilter(seriesKey) {
				continue
			}
			if pts := seriesGaugePoints(samples); len(pts) > 0 {
				perSeries = append(perSeries, pts)
			}
		}
	}

	points := aggregateSeriesHistory(perSeries)
	return finalizeHistory(query.Resource, points, query.MaxPoints), nil
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
