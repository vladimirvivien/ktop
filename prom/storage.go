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
	totalSeries   int
	totalSamples  int64
	lastCleanup   time.Time
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
				// Create new series
				existingSeries = &TimeSeries{
					Labels:  ts.Labels,
					Samples: make([]MetricSample, 0, store.maxSamples),
				}
				store.series[metricName][seriesKey] = existingSeries
				store.totalSeries++
			}
			
			// Add new samples
			for _, sample := range ts.Samples {
				existingSeries.Samples = append(existingSeries.Samples, sample)
				store.totalSamples++
			}
			
			// Trim samples if exceeded max
			if len(existingSeries.Samples) > store.maxSamples {
				excess := len(existingSeries.Samples) - store.maxSamples
				existingSeries.Samples = existingSeries.Samples[excess:]
				store.totalSamples -= int64(excess)
			}
		}
	}
	
	// Perform cleanup if needed
	if time.Since(store.lastCleanup) > store.retentionTime/10 {
		store.cleanupExpiredSamples()
	}
	
	return nil
}

// QueryLatest returns the latest value for a metric
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
		
		if len(ts.Samples) == 0 {
			continue
		}
		
		// Get the latest sample
		sample := ts.Samples[len(ts.Samples)-1]
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

// QueryRange returns metric values over a time range
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
		
		for _, sample := range ts.Samples {
			if sample.Timestamp >= startMs && sample.Timestamp <= endMs {
				// Create a copy to avoid mutations
				sampleCopy := &MetricSample{
					Timestamp: sample.Timestamp,
					Value:     sample.Value,
				}
				allSamples = append(allSamples, sampleCopy)
			}
		}
	}
	
	// Sort by timestamp
	sort.Slice(allSamples, func(i, j int) bool {
		return allSamples[i].Timestamp < allSamples[j].Timestamp
	})
	
	return allSamples, nil
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

// cleanupExpiredSamples removes samples older than retention time (must be called with write lock)
func (store *InMemoryStore) cleanupExpiredSamples() {
	cutoffTime := time.Now().Add(-store.retentionTime).UnixMilli()
	
	for metricName, seriesMap := range store.series {
		for seriesKey, ts := range seriesMap {
			// Remove expired samples
			validSamples := ts.Samples[:0] // Keep same underlying array
			for _, sample := range ts.Samples {
				if sample.Timestamp >= cutoffTime {
					validSamples = append(validSamples, sample)
				} else {
					store.totalSamples--
				}
			}
			ts.Samples = validSamples
			
			// Remove empty time series
			if len(ts.Samples) == 0 {
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
		"total_series":    store.totalSeries,
		"total_samples":   store.totalSamples,
		"metric_count":    len(store.metricNames),
		"label_count":     len(store.labelNames),
		"last_cleanup":    store.lastCleanup,
		"retention_time":  store.retentionTime,
		"max_samples":     store.maxSamples,
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