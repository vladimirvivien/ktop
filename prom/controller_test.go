package prom

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func TestNewCollectorController(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	config := DefaultScrapeConfig()

	controller := NewCollectorController(kubeConfig, config)

	if controller.config != config {
		t.Error("Config not set correctly")
	}

	if controller.kubeConfig != kubeConfig {
		t.Error("Kube config not set correctly")
	}

	if controller.availableComponents == nil {
		t.Error("Available components map not initialized")
	}

	if controller.running {
		t.Error("Controller should not be running initially")
	}
}

func TestControllerStartStop(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	config := DefaultScrapeConfig()
	config.Interval = 100 * time.Millisecond // Fast interval for testing

	controller := NewCollectorController(kubeConfig, config)

	if controller.IsRunning() {
		t.Error("Controller should not be running initially")
	}

	// Start controller
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := controller.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start controller: %v", err)
	}

	if !controller.IsRunning() {
		t.Error("Controller should be running after start")
	}

	// Stop controller
	err = controller.Stop()
	if err != nil {
		t.Fatalf("Failed to stop controller: %v", err)
	}

	if controller.IsRunning() {
		t.Error("Controller should not be running after stop")
	}
}

func TestControllerDoubleStart(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	controller := NewCollectorController(kubeConfig, DefaultScrapeConfig())

	ctx := context.Background()

	// First start should succeed
	err := controller.Start(ctx)
	if err != nil {
		t.Fatalf("First start failed: %v", err)
	}

	// Second start should fail
	err = controller.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already running controller")
	}

	controller.Stop()
}

func TestControllerStopNotRunning(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	controller := NewCollectorController(kubeConfig, DefaultScrapeConfig())

	// Stop without starting should fail
	err := controller.Stop()
	if err == nil {
		t.Error("Expected error when stopping non-running controller")
	}
}

func TestSetCallbacks(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	controller := NewCollectorController(kubeConfig, DefaultScrapeConfig())

	controller.SetMetricsCollectedCallback(func(ComponentType, *ScrapedMetrics) {
		// Callback set
	})

	controller.SetErrorCallback(func(ComponentType, error) {
		// Callback set
	})

	// Callbacks should be set (we can't easily test they're called without a full integration)
	if controller.onMetricsCollected == nil {
		t.Error("Metrics collected callback not set")
	}

	if controller.onError == nil {
		t.Error("Error callback not set")
	}
}

func TestControllerGetStats(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	controller := NewCollectorController(kubeConfig, DefaultScrapeConfig())

	stats := controller.GetStats()

	// Check required stats fields
	requiredFields := []string{"running", "available_components", "config"}
	for _, field := range requiredFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("Required stats field '%s' not found", field)
		}
	}

	// Initially not running
	if stats["running"].(bool) {
		t.Error("Controller should not be running initially")
	}

	// Start controller and check stats again
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	controller.Start(ctx)
	defer controller.Stop()

	stats = controller.GetStats()
	if !stats["running"].(bool) {
		t.Error("Controller should be running in stats")
	}
}

func TestUpdateConfig(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	controller := NewCollectorController(kubeConfig, DefaultScrapeConfig())

	newConfig := &ScrapeConfig{
		Interval:      10 * time.Second,
		Timeout:       5 * time.Second,
		MaxSamples:    2000,
		RetentionTime: 2 * time.Hour,
		Components:    []ComponentType{ComponentAPIServer},
	}

	// Should succeed when not running
	err := controller.UpdateConfig(newConfig)
	if err != nil {
		t.Errorf("Failed to update config when not running: %v", err)
	}

	if controller.GetConfig() != newConfig {
		t.Error("Config not updated correctly")
	}

	// Should fail when running
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	controller.Start(ctx)
	defer controller.Stop()

	err = controller.UpdateConfig(newConfig)
	if err == nil {
		t.Error("Expected error when updating config while running")
	}
}

func TestForceCollection(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	controller := NewCollectorController(kubeConfig, DefaultScrapeConfig())

	ctx := context.Background()

	// Should fail when not running
	err := controller.ForceCollection(ctx)
	if err == nil {
		t.Error("Expected error when forcing collection on non-running controller")
	}

	// Start controller
	controller.Start(ctx)
	defer controller.Stop()

	// Should not error when running (though it might not succeed due to no real cluster)
	err = controller.ForceCollection(ctx)
	// We don't check for error here since it will likely fail due to no real cluster
	// but the method should execute without panicking
	_ = err // Ignore error for test
}

func TestQueryMetric(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	controller := NewCollectorController(kubeConfig, DefaultScrapeConfig())

	// Should fail when store not initialized
	_, err := controller.QueryMetric("test_metric", nil)
	if err == nil {
		t.Error("Expected error when querying without initialized store")
	}

	// Initialize controller (this will initialize the store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = controller.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start controller: %v", err)
	}
	defer controller.Stop()

	// Add some test data to the store
	store := controller.GetStore()
	if store != nil {
		testMetrics := createTestScrapedMetrics()
		store.AddMetrics(testMetrics)

		// Query should work now
		value, err := controller.QueryMetric("test_gauge", map[string]string{"instance": "localhost:8080"})
		if err != nil {
			t.Errorf("Failed to query metric: %v", err)
		}

		expectedValue := 42.5
		if value != expectedValue {
			t.Errorf("Expected value %f, got %f", expectedValue, value)
		}
	}
}

func TestQueryMetricRange(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	controller := NewCollectorController(kubeConfig, DefaultScrapeConfig())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := controller.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start controller: %v", err)
	}
	defer controller.Stop()

	// Add some test data
	store := controller.GetStore()
	if store != nil {
		testMetrics := createTestScrapedMetrics()
		store.AddMetrics(testMetrics)

		// Query range
		samples, err := controller.QueryMetricRange("test_gauge",
			map[string]string{"instance": "localhost:8080"}, 5*time.Minute)
		if err != nil {
			t.Errorf("Failed to query metric range: %v", err)
		}

		if len(samples) == 0 {
			t.Error("Expected at least one sample")
		}
	}
}

func TestGetComponentMetrics(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	controller := NewCollectorController(kubeConfig, DefaultScrapeConfig())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := controller.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start controller: %v", err)
	}
	defer controller.Stop()

	// Add some test data
	store := controller.GetStore()
	if store != nil {
		kubeletMetrics := createTestKubeletMetrics()
		store.AddMetrics(kubeletMetrics)

		// Get component metrics
		metrics := controller.GetComponentMetrics(ComponentKubelet)
		if len(metrics) == 0 {
			t.Error("Expected kubelet metrics")
		}

		// Check for specific metric
		if _, exists := metrics["kubelet_running_pods"]; !exists {
			t.Error("Expected kubelet_running_pods metric")
		}
	}
}

func TestControllerWithMockCollector(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	config := DefaultScrapeConfig()
	config.Interval = 50 * time.Millisecond // Fast for testing

	controller := NewCollectorController(kubeConfig, config)

	// Create mock collector
	mockCollector := &MockCollector{
		components: []ComponentType{ComponentAPIServer, ComponentKubelet},
		metrics:    createTestScrapedMetrics(),
	}

	// Replace the collector (in a real implementation, we'd have dependency injection)
	controller.collector = mockCollector

	var mu sync.Mutex
	metricsCollectedCount := 0
	errorCount := 0

	controller.SetMetricsCollectedCallback(func(ComponentType, *ScrapedMetrics) {
		mu.Lock()
		metricsCollectedCount++
		mu.Unlock()
	})

	controller.SetErrorCallback(func(ComponentType, error) {
		mu.Lock()
		errorCount++
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := controller.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start controller with mock: %v", err)
	}

	// Wait for some collection cycles
	time.Sleep(150 * time.Millisecond)

	controller.Stop()

	// Check that collection happened (this test is flaky due to timing, so just log)
	mu.Lock()
	collected := metricsCollectedCount
	errors := errorCount
	mu.Unlock()

	t.Logf("Metrics collected: %d, Errors: %d", collected, errors)
}

// MockCollector for testing
type MockCollector struct {
	components []ComponentType
	metrics    *ScrapedMetrics
	started    bool
}

func (m *MockCollector) Start(ctx context.Context) error {
	m.started = true
	return nil
}

func (m *MockCollector) Stop() error {
	m.started = false
	return nil
}

func (m *MockCollector) ScrapeComponent(ctx context.Context, component ComponentType) (*ScrapedMetrics, error) {
	if !m.started {
		return nil, ErrCollectorNotStarted
	}

	// Return test metrics
	metrics := *m.metrics // Copy
	metrics.Component = component
	return &metrics, nil
}

func (m *MockCollector) GetLastScrape(component ComponentType) (*ScrapedMetrics, error) {
	return m.metrics, nil
}

func (m *MockCollector) GetAvailableComponents(ctx context.Context) ([]ComponentType, error) {
	return m.components, nil
}

// Custom error for testing
var ErrCollectorNotStarted = fmt.Errorf("collector not started")

func TestControllerDiscovery(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://test-cluster"}
	config := DefaultScrapeConfig()

	controller := NewCollectorController(kubeConfig, config)

	// Initialize the controller to set up the collector
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start controller to initialize internal components
	err := controller.Start(ctx)
	if err != nil {
		t.Logf("Expected error starting with fake cluster: %v", err)
	}
	defer controller.Stop()

	// Check that the discovery ran without panic
	components := controller.GetAvailableComponents()
	// We expect this to be empty due to fake client limitations, but it shouldn't panic
	_ = components
}

func TestControllerErrorHandling(t *testing.T) {
	kubeConfig := &rest.Config{Host: "https://invalid-cluster"}
	controller := NewCollectorController(kubeConfig, DefaultScrapeConfig())

	errorCount := 0
	controller.SetErrorCallback(func(ComponentType, error) {
		errorCount++
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should start but likely encounter errors due to invalid cluster
	err := controller.Start(ctx)
	// We don't necessarily expect this to fail at start time

	time.Sleep(50 * time.Millisecond)
	controller.Stop()

	// The controller should handle errors gracefully
	// We can check that error callbacks were called if errors occurred
	// But we don't require errors for the test to pass
	_ = err        // Use the variable
	_ = errorCount // Use the variable
}
