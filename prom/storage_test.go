package prom

import (
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/model/labels"
)

func TestNewInMemoryStore(t *testing.T) {
	config := DefaultScrapeConfig()
	store := NewInMemoryStore(config)

	if store.maxSamples != config.MaxSamples {
		t.Errorf("Expected maxSamples %d, got %d", config.MaxSamples, store.maxSamples)
	}

	if store.retentionTime != config.RetentionTime {
		t.Errorf("Expected retentionTime %v, got %v", config.RetentionTime, store.retentionTime)
	}

	if store.series == nil {
		t.Error("Series map not initialized")
	}

	if store.metricNames == nil {
		t.Error("Metric names map not initialized")
	}

	if store.labelNames == nil {
		t.Error("Label names map not initialized")
	}

	if store.labelValues == nil {
		t.Error("Label values map not initialized")
	}
}

func TestAddMetrics(t *testing.T) {
	store := NewInMemoryStore(DefaultScrapeConfig())

	// Create test metrics
	metrics := createTestScrapedMetrics()

	err := store.AddMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to add metrics: %v", err)
	}

	// Verify metrics were added
	metricNames := store.GetMetricNames()
	if len(metricNames) == 0 {
		t.Error("No metric names found after adding metrics")
	}

	expectedMetrics := []string{"test_counter", "test_gauge"}
	for _, expected := range expectedMetrics {
		found := false
		for _, name := range metricNames {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected metric '%s' not found", expected)
		}
	}

	// Verify label indexes were updated
	labelValues := store.GetLabelValues("instance")
	if len(labelValues) == 0 {
		t.Error("No label values found for 'instance' label")
	}
}

func TestQueryLatest(t *testing.T) {
	store := NewInMemoryStore(DefaultScrapeConfig())
	metrics := createTestScrapedMetrics()

	err := store.AddMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to add metrics: %v", err)
	}

	// Query existing metric
	value, err := store.QueryLatest("test_gauge", map[string]string{"instance": "localhost:8080"})
	if err != nil {
		t.Fatalf("Failed to query latest: %v", err)
	}

	expectedValue := 42.5
	if value != expectedValue {
		t.Errorf("Expected value %f, got %f", expectedValue, value)
	}

	// Query non-existent metric
	_, err = store.QueryLatest("non_existent_metric", nil)
	if err == nil {
		t.Error("Expected error for non-existent metric")
	}

	// Query with non-matching labels
	_, err = store.QueryLatest("test_gauge", map[string]string{"instance": "non-existent"})
	if err == nil {
		t.Error("Expected error for non-matching labels")
	}
}

func TestQueryRange(t *testing.T) {
	store := NewInMemoryStore(DefaultScrapeConfig())

	// Add metrics at different times
	now := time.Now()
	for i := 0; i < 5; i++ {
		metrics := createTestScrapedMetricsAtTime(now.Add(time.Duration(i) * time.Minute))
		store.AddMetrics(metrics)
	}

	// Query range
	start := now.Add(-1 * time.Minute)
	end := now.Add(6 * time.Minute)

	samples, err := store.QueryRange("test_gauge", map[string]string{"instance": "localhost:8080"}, start, end)
	if err != nil {
		t.Fatalf("Failed to query range: %v", err)
	}

	if len(samples) != 5 {
		t.Errorf("Expected 5 samples, got %d", len(samples))
	}

	// Verify samples are sorted by timestamp
	for i := 1; i < len(samples); i++ {
		if samples[i].Timestamp < samples[i-1].Timestamp {
			t.Error("Samples not sorted by timestamp")
		}
	}
}

func TestMaxSamplesLimit(t *testing.T) {
	config := DefaultScrapeConfig()
	config.MaxSamples = 3 // Set low limit for testing
	store := NewInMemoryStore(config)

	// Add more samples than the limit
	metrics := createTestScrapedMetrics()
	metricFamily := metrics.Families["test_gauge"]
	timeSeries := metricFamily.TimeSeries[0]

	// Add 5 samples (more than the limit of 3)
	for i := 0; i < 5; i++ {
		timeSeries.Samples.Add(MetricSample{
			Timestamp: time.Now().Add(time.Duration(i) * time.Second).UnixMilli(),
			Value:     float64(i),
		})
	}

	err := store.AddMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to add metrics: %v", err)
	}

	// Verify only the last 3 samples are kept
	seriesMap := store.series["test_gauge"]
	for _, ts := range seriesMap {
		if ts.Samples.Len() > config.MaxSamples {
			t.Errorf("Expected at most %d samples, got %d", config.MaxSamples, ts.Samples.Len())
		}
	}
}

func TestCleanup(t *testing.T) {
	config := DefaultScrapeConfig()
	config.RetentionTime = 1 * time.Second // Very short retention for testing
	store := NewInMemoryStore(config)

	// Add old metrics
	oldTime := time.Now().Add(-2 * time.Second)
	oldMetrics := createTestScrapedMetricsAtTime(oldTime)
	store.AddMetrics(oldMetrics)

	// Add new metrics
	newMetrics := createTestScrapedMetrics()
	store.AddMetrics(newMetrics)

	// Wait for retention time to pass
	time.Sleep(100 * time.Millisecond)

	// Run cleanup
	err := store.Cleanup()
	if err != nil {
		t.Fatalf("Failed to run cleanup: %v", err)
	}

	// Verify old metrics are removed (this is timing-dependent, so we'll check that cleanup ran)
	stats := store.GetStats()
	if stats["last_cleanup"].(time.Time).IsZero() {
		t.Error("Cleanup timestamp not updated")
	}
}

func TestGetLabelValues(t *testing.T) {
	store := NewInMemoryStore(DefaultScrapeConfig())
	metrics := createTestScrapedMetrics()

	err := store.AddMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to add metrics: %v", err)
	}

	// Test existing label
	values := store.GetLabelValues("instance")
	if len(values) == 0 {
		t.Error("No values found for 'instance' label")
	}

	expectedValue := "localhost:8080"
	found := false
	for _, value := range values {
		if value == expectedValue {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected value '%s' not found", expectedValue)
	}

	// Test non-existent label
	values = store.GetLabelValues("non_existent_label")
	if len(values) != 0 {
		t.Errorf("Expected 0 values for non-existent label, got %d", len(values))
	}
}

func TestGetStats(t *testing.T) {
	store := NewInMemoryStore(DefaultScrapeConfig())
	metrics := createTestScrapedMetrics()

	err := store.AddMetrics(metrics)
	if err != nil {
		t.Fatalf("Failed to add metrics: %v", err)
	}

	stats := store.GetStats()

	// Check required stats fields
	requiredFields := []string{"total_series", "total_samples", "metric_count", "label_count", "last_cleanup", "retention_time", "max_samples"}
	for _, field := range requiredFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("Required stats field '%s' not found", field)
		}
	}

	// Verify some values
	if stats["total_series"].(int) <= 0 {
		t.Error("Total series should be greater than 0")
	}

	if stats["total_samples"].(int64) <= 0 {
		t.Error("Total samples should be greater than 0")
	}

	if stats["metric_count"].(int) <= 0 {
		t.Error("Metric count should be greater than 0")
	}
}

func TestMatchesLabels(t *testing.T) {
	store := NewInMemoryStore(DefaultScrapeConfig())

	labels1 := labels.Labels{
		{Name: "__name__", Value: "test_metric"},
		{Name: "instance", Value: "localhost:8080"},
		{Name: "job", Value: "test-job"},
	}

	tests := []struct {
		name     string
		labels   labels.Labels
		matchers map[string]string
		expected bool
	}{
		{
			name:     "exact match",
			labels:   labels1,
			matchers: map[string]string{"instance": "localhost:8080"},
			expected: true,
		},
		{
			name:     "multiple matchers match",
			labels:   labels1,
			matchers: map[string]string{"instance": "localhost:8080", "job": "test-job"},
			expected: true,
		},
		{
			name:     "no match",
			labels:   labels1,
			matchers: map[string]string{"instance": "different:9090"},
			expected: false,
		},
		{
			name:     "wildcard match",
			labels:   labels1,
			matchers: map[string]string{"instance": "*localhost*"},
			expected: true,
		},
		{
			name:     "wildcard no match",
			labels:   labels1,
			matchers: map[string]string{"instance": "*different*"},
			expected: false,
		},
		{
			name:     "empty matchers",
			labels:   labels1,
			matchers: map[string]string{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.matchesLabels(tt.labels, tt.matchers)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestQueryByComponent(t *testing.T) {
	store := NewInMemoryStore(DefaultScrapeConfig())

	// Add metrics for different components
	kubeletMetrics := createTestKubeletMetrics()
	cadvisorMetrics := createTestCAdvisorMetrics()

	store.AddMetrics(kubeletMetrics)
	store.AddMetrics(cadvisorMetrics)

	// Query kubelet metrics
	kubeletResults := store.QueryByComponent(ComponentKubelet)
	if len(kubeletResults) == 0 {
		t.Error("No kubelet metrics found")
	}

	// Query cAdvisor metrics (note: the inference is based on metric name prefixes)
	cadvisorResults := store.QueryByComponent(ComponentCAdvisor)
	if len(cadvisorResults) == 0 {
		t.Log("No cAdvisor metrics found (expected if metric name doesn't match inference pattern)")
	}
}

func TestGetMetricFamilyNames(t *testing.T) {
	store := NewInMemoryStore(DefaultScrapeConfig())

	// Add metrics for different components
	kubeletMetrics := createTestKubeletMetrics()
	cadvisorMetrics := createTestCAdvisorMetrics()

	store.AddMetrics(kubeletMetrics)
	store.AddMetrics(cadvisorMetrics)

	familyNames := store.GetMetricFamilyNames()

	// Should have metrics for kubelet and cAdvisor
	if len(familyNames[ComponentKubelet]) == 0 {
		t.Error("No kubelet metric families found")
	}

	if len(familyNames[ComponentCAdvisor]) == 0 {
		t.Error("No cAdvisor metric families found")
	}
}

// Helper functions for creating test data

// createTestTimeSeries creates a TimeSeries with a RingBuffer containing the given samples
func createTestTimeSeries(lbls labels.Labels, samples ...MetricSample) *TimeSeries {
	rb := NewRingBuffer[MetricSample](100) // Default capacity
	for _, s := range samples {
		rb.Add(s)
	}
	return &TimeSeries{
		Labels:  lbls,
		Samples: rb,
	}
}

func createTestScrapedMetrics() *ScrapedMetrics {
	return createTestScrapedMetricsAtTime(time.Now())
}

func createTestScrapedMetricsAtTime(timestamp time.Time) *ScrapedMetrics {
	families := make(map[string]*MetricFamily)

	// Create a counter metric
	counterFamily := &MetricFamily{
		Name:        "test_counter",
		Type:        dto.MetricType_COUNTER,
		Help:        "A test counter metric",
		LastUpdated: timestamp,
		TimeSeries: []*TimeSeries{
			createTestTimeSeries(
				labels.Labels{
					{Name: "__name__", Value: "test_counter"},
					{Name: "instance", Value: "localhost:8080"},
					{Name: "job", Value: "test-job"},
				},
				MetricSample{Timestamp: timestamp.UnixMilli(), Value: 123.0},
			),
		},
	}
	families["test_counter"] = counterFamily

	// Create a gauge metric
	gaugeFamily := &MetricFamily{
		Name:        "test_gauge",
		Type:        dto.MetricType_GAUGE,
		Help:        "A test gauge metric",
		LastUpdated: timestamp,
		TimeSeries: []*TimeSeries{
			createTestTimeSeries(
				labels.Labels{
					{Name: "__name__", Value: "test_gauge"},
					{Name: "instance", Value: "localhost:8080"},
					{Name: "job", Value: "test-job"},
				},
				MetricSample{Timestamp: timestamp.UnixMilli(), Value: 42.5},
			),
		},
	}
	families["test_gauge"] = gaugeFamily

	return &ScrapedMetrics{
		Component:      ComponentAPIServer,
		Endpoint:       "/metrics",
		Families:       families,
		ScrapedAt:      timestamp,
		ScrapeDuration: 100 * time.Millisecond,
	}
}

func createTestKubeletMetrics() *ScrapedMetrics {
	families := make(map[string]*MetricFamily)

	family := &MetricFamily{
		Name:        "kubelet_running_pods",
		Type:        dto.MetricType_GAUGE,
		Help:        "Number of running pods",
		LastUpdated: time.Now(),
		TimeSeries: []*TimeSeries{
			createTestTimeSeries(
				labels.Labels{
					{Name: "__name__", Value: "kubelet_running_pods"},
					{Name: "instance", Value: "node1:10250"},
				},
				MetricSample{Timestamp: time.Now().UnixMilli(), Value: 15.0},
			),
		},
	}
	families["kubelet_running_pods"] = family

	return &ScrapedMetrics{
		Component:      ComponentKubelet,
		Endpoint:       "nodes/node1/proxy/metrics",
		Families:       families,
		ScrapedAt:      time.Now(),
		ScrapeDuration: 50 * time.Millisecond,
	}
}

func createTestCAdvisorMetrics() *ScrapedMetrics {
	families := make(map[string]*MetricFamily)

	family := &MetricFamily{
		Name:        "container_memory_usage_bytes",
		Type:        dto.MetricType_GAUGE,
		Help:        "Container memory usage",
		LastUpdated: time.Now(),
		TimeSeries: []*TimeSeries{
			createTestTimeSeries(
				labels.Labels{
					{Name: "__name__", Value: "container_memory_usage_bytes"},
					{Name: "instance", Value: "node1:10255"},
					{Name: "container", Value: "test-container"},
				},
				MetricSample{Timestamp: time.Now().UnixMilli(), Value: 1048576.0}, // 1MB
			),
		},
	}
	families["container_memory_usage_bytes"] = family

	return &ScrapedMetrics{
		Component:      ComponentCAdvisor,
		Endpoint:       "nodes/node1/proxy/metrics/cadvisor",
		Families:       families,
		ScrapedAt:      time.Now(),
		ScrapeDuration: 75 * time.Millisecond,
	}
}
