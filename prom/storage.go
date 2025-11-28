package prom

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/labels"
)

// InMemoryStore implements MetricsStore using in-memory storage
type InMemoryStore struct {
	mutex sync.RWMutex

	// Core storage: metricName -> seriesKey -> TimeSeries
	series map[string]map[string]*TimeSeries

	// Indexes for fast lookups
	metricNames map[string]bool
	labelNames  map[string]bool
	labelValues map[string]map[string]bool // labelName -> values

	// Configuration
	maxSamples    int
	retentionTime time.Duration

	// Statistics
	totalSeries  int
	totalSamples int64
	lastCleanup  time.Time
}

// NewInMemoryStore creates a new in-memory metrics store
func NewInMemoryStore(config *ScrapeConfig) *InMemoryStore {
	return &InMemoryStore{
		series:        make(map[string]map[string]*TimeSeries),
		metricNames:   make(map[string]bool),
		labelNames:    make(map[string]bool),
		labelValues:   make(map[string]map[string]bool),
		maxSamples:    config.MaxSamples,
		retentionTime: config.RetentionTime,
		lastCleanup:   time.Now(),
	}
}

// AddMetrics stores scraped metrics in the store
func (store *InMemoryStore) AddMetrics(metrics *ScrapedMetrics) error {
	if metrics.Error != nil {
		return fmt.Errorf("cannot store metrics with error: %w", metrics.Error)
	}

	store.mutex.Lock()
	defer store.mutex.Unlock()

	for metricName, family := range metrics.Families {
		// Ensure metric exists in index
		store.metricNames[metricName] = true

		// Ensure metric series map exists
		if store.series[metricName] == nil {
			store.series[metricName] = make(map[string]*TimeSeries)
		}

		// Add each time series from the family
		for _, ts := range family.TimeSeries {
			seriesKey := ts.Labels.String()

			// Update label indexes
			for _, label := range ts.Labels {
				store.labelNames[label.Name] = true
				if store.labelValues[label.Name] == nil {
					store.labelValues[label.Name] = make(map[string]bool)
				}
				store.labelValues[label.Name][label.Value] = true
			}

			// Get or create time series
			existingSeries, exists := store.series[metricName][seriesKey]
			if !exists {
				// Create new series with ring buffer
				existingSeries = &TimeSeries{
					Labels:  ts.Labels,
					Samples: NewRingBuffer[MetricSample](store.maxSamples),
				}
				store.series[metricName][seriesKey] = existingSeries
				store.totalSeries++
			}

			// Add new samples - ring buffer handles overflow automatically
			ts.Samples.Range(func(_ int, sample MetricSample) bool {
				// Track if we're overwriting (buffer was full)
				wasFull := existingSeries.Samples.IsFull()
				existingSeries.Samples.Add(sample)
				if !wasFull {
					store.totalSamples++
				}
				// If buffer was full, totalSamples stays same (overwrite)
				return true // continue iteration
			})
		}
	}

	// Perform cleanup if needed
	if time.Since(store.lastCleanup) > store.retentionTime/10 {
		store.cleanupExpiredSamples()
	}

	return nil
}

// QueryLatest returns the latest value for a metric from a single series.
// If multiple series match, returns the value from the series with the most recent timestamp.
// NOTE: For metrics that need to be summed across multiple series (e.g., container memory
// for pods with multiple containers), use QueryLatestSum instead.
func (store *InMemoryStore) QueryLatest(metricName string, labelMatchers map[string]string) (float64, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	seriesMap, exists := store.series[metricName]
	if !exists {
		return 0, fmt.Errorf("metric %s not found", metricName)
	}

	var latestValue float64
	var latestTime int64
	found := false

	for _, ts := range seriesMap {
		if !store.matchesLabels(ts.Labels, labelMatchers) {
			continue
		}

		if ts.Samples.IsEmpty() {
			continue
		}

		// Get the latest sample from ring buffer
		sample, ok := ts.Samples.Last()
		if !ok {
			continue
		}
		if !found || sample.Timestamp > latestTime {
			latestValue = sample.Value
			latestTime = sample.Timestamp
			found = true
		}
	}

	if !found {
		return 0, fmt.Errorf("no matching series found for metric %s", metricName)
	}

	return latestValue, nil
}

// QueryLatestSum returns the sum of latest values across all matching series.
// This is essential for gauge metrics like memory that need to be aggregated
// across multiple containers in a pod.
func (store *InMemoryStore) QueryLatestSum(metricName string, labelMatchers map[string]string) (float64, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	seriesMap, exists := store.series[metricName]
	if !exists {
		return 0, fmt.Errorf("metric %s not found", metricName)
	}

	var totalValue float64
	found := false

	for _, ts := range seriesMap {
		if !store.matchesLabels(ts.Labels, labelMatchers) {
			continue
		}

		if ts.Samples.IsEmpty() {
			continue
		}

		// Get the latest sample from this series and add to total
		sample, ok := ts.Samples.Last()
		if !ok {
			continue
		}
		totalValue += sample.Value
		found = true
	}

	if !found {
		return 0, fmt.Errorf("no matching series found for metric %s", metricName)
	}

	return totalValue, nil
}

// QueryRange returns metric values over a time range
// DEPRECATED: This method flattens samples from multiple series which breaks rate calculations.
// Use QueryRangePerSeries for accurate rate calculations across multiple time series.
func (store *InMemoryStore) QueryRange(metricName string, labelMatchers map[string]string, start, end time.Time) ([]*MetricSample, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	seriesMap, exists := store.series[metricName]
	if !exists {
		return nil, fmt.Errorf("metric %s not found", metricName)
	}

	var allSamples []*MetricSample
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()

	for _, ts := range seriesMap {
		if !store.matchesLabels(ts.Labels, labelMatchers) {
			continue
		}

		// Iterate over ring buffer samples
		ts.Samples.Range(func(_ int, sample MetricSample) bool {
			if sample.Timestamp >= startMs && sample.Timestamp <= endMs {
				// Create a copy to avoid mutations
				sampleCopy := &MetricSample{
					Timestamp: sample.Timestamp,
					Value:     sample.Value,
				}
				allSamples = append(allSamples, sampleCopy)
			}
			return true // continue iteration
		})
	}

	// Sort by timestamp
	sort.Slice(allSamples, func(i, j int) bool {
		return allSamples[i].Timestamp < allSamples[j].Timestamp
	})

	return allSamples, nil
}

// QueryRangePerSeries returns metric values over a time range, grouped by series key.
// Each series key maps to that series' samples, preserving per-series ordering.
// This is essential for accurate rate calculations on counter metrics.
func (store *InMemoryStore) QueryRangePerSeries(metricName string, labelMatchers map[string]string, start, end time.Time) (map[string][]*MetricSample, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	seriesMap, exists := store.series[metricName]
	if !exists {
		return nil, fmt.Errorf("metric %s not found", metricName)
	}

	result := make(map[string][]*MetricSample)
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()

	for seriesKey, ts := range seriesMap {
		if !store.matchesLabels(ts.Labels, labelMatchers) {
			continue
		}

		var seriesSamples []*MetricSample
		// Iterate over ring buffer samples
		ts.Samples.Range(func(_ int, sample MetricSample) bool {
			if sample.Timestamp >= startMs && sample.Timestamp <= endMs {
				sampleCopy := &MetricSample{
					Timestamp: sample.Timestamp,
					Value:     sample.Value,
				}
				seriesSamples = append(seriesSamples, sampleCopy)
			}
			return true // continue iteration
		})

		// Only add if we have samples in this time range
		if len(seriesSamples) > 0 {
			// Sort by timestamp within this series
			sort.Slice(seriesSamples, func(i, j int) bool {
				return seriesSamples[i].Timestamp < seriesSamples[j].Timestamp
			})
			result[seriesKey] = seriesSamples
		}
	}

	return result, nil
}

// GetMetricNames returns all available metric names
func (store *InMemoryStore) GetMetricNames() []string {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	names := make([]string, 0, len(store.metricNames))
	for name := range store.metricNames {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

// GetLabelValues returns all values for a given label name
func (store *InMemoryStore) GetLabelValues(labelName string) []string {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	values, exists := store.labelValues[labelName]
	if !exists {
		return []string{}
	}

	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}

	sort.Strings(result)
	return result
}

// Cleanup removes old metrics based on retention policy
func (store *InMemoryStore) Cleanup() error {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	store.cleanupExpiredSamples()
	return nil
}

// cleanupExpiredSamples removes stale time series (must be called with write lock).
// With ring buffers, individual samples are automatically evicted by new additions.
// This cleanup removes entire time series where even the newest sample is too old,
// indicating the series is no longer being updated.
func (store *InMemoryStore) cleanupExpiredSamples() {
	cutoffTime := time.Now().Add(-store.retentionTime).UnixMilli()

	for metricName, seriesMap := range store.series {
		for seriesKey, ts := range seriesMap {
			// Check if the series is stale (newest sample is too old)
			newest, ok := ts.Samples.Last()
			if !ok || newest.Timestamp < cutoffTime {
				// Series is empty or stale - remove it entirely
				store.totalSamples -= int64(ts.Samples.Len())
				delete(seriesMap, seriesKey)
				store.totalSeries--
			}
		}

		// Remove empty metric
		if len(seriesMap) == 0 {
			delete(store.series, metricName)
			delete(store.metricNames, metricName)
		}
	}

	store.lastCleanup = time.Now()
}

// matchesLabels checks if a time series labels match the given label matchers
func (store *InMemoryStore) matchesLabels(seriesLabels labels.Labels, labelMatchers map[string]string) bool {
	for labelName, expectedValue := range labelMatchers {
		actualValue := seriesLabels.Get(labelName)

		// Support simple pattern matching
		if strings.Contains(expectedValue, "*") {
			// Convert simple wildcard to regex-like matching
			pattern := strings.ReplaceAll(expectedValue, "*", "")
			if !strings.Contains(actualValue, pattern) {
				return false
			}
		} else {
			if actualValue != expectedValue {
				return false
			}
		}
	}
	return true
}

// GetStats returns storage statistics
func (store *InMemoryStore) GetStats() map[string]interface{} {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	return map[string]interface{}{
		"total_series":   store.totalSeries,
		"total_samples":  store.totalSamples,
		"metric_count":   len(store.metricNames),
		"label_count":    len(store.labelNames),
		"last_cleanup":   store.lastCleanup,
		"retention_time": store.retentionTime,
		"max_samples":    store.maxSamples,
	}
}

// QueryByComponent returns metrics for a specific component
func (store *InMemoryStore) QueryByComponent(component ComponentType) map[string]*TimeSeries {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	result := make(map[string]*TimeSeries)
	componentStr := string(component)

	for metricName, seriesMap := range store.series {
		for _, ts := range seriesMap {
			// Check if this series belongs to the component
			// This could be improved with better labeling
			if strings.Contains(metricName, componentStr) {
				result[metricName] = ts
			}
		}
	}

	return result
}

// GetMetricFamilyNames returns metric names grouped by component
func (store *InMemoryStore) GetMetricFamilyNames() map[ComponentType][]string {
	store.mutex.RLock()
	defer store.mutex.RUnlock()

	result := make(map[ComponentType][]string)

	for metricName := range store.metricNames {
		component := store.inferComponentFromMetricName(metricName)
		result[component] = append(result[component], metricName)
	}

	// Sort each component's metrics
	for component := range result {
		sort.Strings(result[component])
	}

	return result
}

// inferComponentFromMetricName attempts to infer component from metric name
func (store *InMemoryStore) inferComponentFromMetricName(metricName string) ComponentType {
	switch {
	case strings.HasPrefix(metricName, "apiserver_"):
		return ComponentAPIServer
	case strings.HasPrefix(metricName, "kubelet_"):
		return ComponentKubelet
	case strings.HasPrefix(metricName, "container_"):
		return ComponentCAdvisor
	case strings.HasPrefix(metricName, "etcd_"):
		return ComponentEtcd
	case strings.HasPrefix(metricName, "scheduler_"):
		return ComponentScheduler
	case strings.HasPrefix(metricName, "workqueue_"):
		return ComponentControllerManager
	case strings.HasPrefix(metricName, "kubeproxy_"):
		return ComponentKubeProxy
	default:
		return ComponentAPIServer // Default fallback
	}
}
