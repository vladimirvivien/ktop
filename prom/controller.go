package prom

import (
	"context"
	"fmt"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Start initializes and starts the metrics collection controller
func (cc *CollectorController) Start(ctx context.Context) error {
	// First phase: check and initialize under lock
	cc.mutex.Lock()
	if cc.running {
		cc.mutex.Unlock()
		return fmt.Errorf("controller is already running")
	}

	// Initialize components (still under lock)
	if err := cc.initialize(); err != nil {
		cc.mutex.Unlock()
		return fmt.Errorf("initializing controller: %w", err)
	}
	cc.running = true
	cc.mutex.Unlock()

	// Run initial component discovery BEFORE starting collector
	// This ensures components are available for the first collection
	// Use a short timeout to avoid blocking startup if cluster is slow
	// Note: Lock is released, discoverAvailableComponents will acquire its own
	discoveryCtx, discoveryCancel := context.WithTimeout(ctx, cc.config.Timeout)
	cc.discoverAvailableComponents(discoveryCtx)
	discoveryCancel()

	// Start the metrics collector
	go cc.runCollector(ctx)

	// Start periodic cleanup
	go cc.runPeriodicCleanup(ctx)

	// Start component discovery (for periodic re-discovery)
	go cc.runComponentDiscovery(ctx)

	return nil
}

// Stop gracefully stops the metrics collection controller
func (cc *CollectorController) Stop() error {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()

	if !cc.running {
		return fmt.Errorf("controller is not running")
	}

	cc.running = false

	// Cleanup would be handled by context cancellation
	return nil
}

// TestScrape performs a quick test to verify connectivity to prometheus endpoints.
// It makes a direct API call to test RBAC permissions for nodes/proxy.
// Returns nil if the metrics endpoints are accessible.
func (cc *CollectorController) TestScrape(ctx context.Context) error {
	cc.mutex.RLock()
	if !cc.running {
		cc.mutex.RUnlock()
		return fmt.Errorf("controller is not running")
	}
	kubeConfig := cc.kubeConfig
	cc.mutex.RUnlock()

	// Create a quick client to test connectivity
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("creating test client: %w", err)
	}

	// List nodes to get a node name for testing
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return fmt.Errorf("no nodes found in cluster")
	}

	nodeName := nodes.Items[0].Name

	// Test access to node/proxy metrics endpoint (required for prometheus scraping)
	req := clientset.CoreV1().RESTClient().Get().
		Resource("nodes").
		Name(nodeName).
		SubResource("proxy").
		Suffix("metrics")

	_, err = req.DoRaw(ctx)
	if err != nil {
		return fmt.Errorf("cannot access node metrics (check RBAC for nodes/proxy): %w", err)
	}

	return nil
}

// initialize sets up the collector and store components
func (cc *CollectorController) initialize() error {
	// Create metrics store
	cc.store = NewInMemoryStore(cc.config)

	// Create metrics collector
	scraper, err := NewKubernetesScraper(cc.kubeConfig, cc.config)
	if err != nil {
		return fmt.Errorf("creating scraper: %w", err)
	}
	cc.collector = scraper

	return nil
}

// runCollector manages the metrics collection process
func (cc *CollectorController) runCollector(ctx context.Context) {
	if err := cc.collector.Start(ctx); err != nil {
		cc.setLastError(err)
		return
	}

	// Run immediate first collection (don't wait for ticker)
	cc.collectFromAllComponents(ctx)

	ticker := time.NewTicker(cc.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cc.collectFromAllComponents(ctx)
		}
	}
}

// collectFromAllComponents collects metrics from all enabled components
func (cc *CollectorController) collectFromAllComponents(ctx context.Context) {
	var wg sync.WaitGroup

	for _, component := range cc.config.Components {
		if !cc.isComponentAvailable(component) {
			continue
		}

		wg.Add(1)
		go func(comp ComponentType) {
			defer wg.Done()
			cc.collectFromComponent(ctx, comp)
		}(component)
	}

	wg.Wait()
}

// collectFromComponent collects metrics from a single component
func (cc *CollectorController) collectFromComponent(ctx context.Context, component ComponentType) {
	metrics, err := cc.collector.ScrapeComponent(ctx, component)
	if err != nil {
		cc.setLastError(err)
		if cc.onError != nil {
			cc.onError(component, err)
		}
		return
	}

	// Store the metrics
	if err := cc.store.AddMetrics(metrics); err != nil {
		cc.setLastError(err)
		if cc.onError != nil {
			cc.onError(component, err)
		}
		return
	}

	// Notify callback if set
	if cc.onMetricsCollected != nil {
		cc.onMetricsCollected(component, metrics)
	}
}

// runPeriodicCleanup manages periodic cleanup of old metrics
func (cc *CollectorController) runPeriodicCleanup(ctx context.Context) {
	// Run cleanup every 1/4 of retention time
	cleanupInterval := cc.config.RetentionTime / 4
	if cleanupInterval < time.Minute {
		cleanupInterval = time.Minute
	}

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := cc.store.Cleanup(); err != nil {
				cc.setLastError(err)
			}
		}
	}
}

// runComponentDiscovery periodically discovers available components
func (cc *CollectorController) runComponentDiscovery(ctx context.Context) {
	// Discovery every 5 minutes
	// Note: Initial discovery is done in Start() before this goroutine starts
	discoveryInterval := 5 * time.Minute

	ticker := time.NewTicker(discoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cc.discoverAvailableComponents(ctx)
		}
	}
}

// discoverAvailableComponents updates the list of available components
func (cc *CollectorController) discoverAvailableComponents(ctx context.Context) {
	components, err := cc.collector.GetAvailableComponents(ctx)
	if err != nil {
		cc.setLastError(err)
		return
	}

	cc.mutex.Lock()
	defer cc.mutex.Unlock()

	// Reset availability
	for component := range cc.availableComponents {
		cc.availableComponents[component] = false
	}

	// Mark discovered components as available
	for _, component := range components {
		cc.availableComponents[component] = true
	}
}

// isComponentAvailable checks if a component is available for scraping
func (cc *CollectorController) isComponentAvailable(component ComponentType) bool {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()
	return cc.availableComponents[component]
}

// setLastError safely sets the last error
func (cc *CollectorController) setLastError(err error) {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	cc.lastError = err
}

// GetStore returns the metrics store for querying
func (cc *CollectorController) GetStore() MetricsStore {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()
	return cc.store
}

// GetConfig returns the current configuration
func (cc *CollectorController) GetConfig() *ScrapeConfig {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()
	return cc.config
}

// UpdateConfig updates the scrape configuration
func (cc *CollectorController) UpdateConfig(config *ScrapeConfig) error {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()

	if cc.running {
		return fmt.Errorf("cannot update config while controller is running")
	}

	cc.config = config
	return nil
}

// GetStats returns statistics about the controller and store
func (cc *CollectorController) GetStats() map[string]interface{} {
	cc.mutex.RLock()
	running := cc.running
	lastError := cc.lastError
	availableComponents := make([]ComponentType, 0, len(cc.availableComponents))
	for component, available := range cc.availableComponents {
		if available {
			availableComponents = append(availableComponents, component)
		}
	}
	cc.mutex.RUnlock()

	stats := map[string]interface{}{
		"running":              running,
		"available_components": availableComponents,
		"config":               cc.config,
	}

	if lastError != nil {
		stats["last_error"] = lastError.Error()
	}

	if cc.store != nil {
		if memStore, ok := cc.store.(*InMemoryStore); ok {
			stats["store"] = memStore.GetStats()
		}
	}

	return stats
}

// ForceCollection manually triggers collection from all available components
func (cc *CollectorController) ForceCollection(ctx context.Context) error {
	if !cc.IsRunning() {
		return fmt.Errorf("controller is not running")
	}

	cc.collectFromAllComponents(ctx)
	return nil
}

// GetComponentMetrics returns metrics for a specific component
func (cc *CollectorController) GetComponentMetrics(component ComponentType) map[string]*TimeSeries {
	if cc.store == nil {
		return nil
	}

	if memStore, ok := cc.store.(*InMemoryStore); ok {
		return memStore.QueryByComponent(component)
	}

	return nil
}

// QueryMetric provides a simple interface to query the latest value of a metric
func (cc *CollectorController) QueryMetric(metricName string, labelMatchers map[string]string) (float64, error) {
	if cc.store == nil {
		return 0, fmt.Errorf("store not initialized")
	}

	return cc.store.QueryLatest(metricName, labelMatchers)
}

// QueryMetricRange provides a simple interface to query metrics over a time range
func (cc *CollectorController) QueryMetricRange(metricName string, labelMatchers map[string]string, duration time.Duration) ([]*MetricSample, error) {
	if cc.store == nil {
		return nil, fmt.Errorf("store not initialized")
	}

	end := time.Now()
	start := end.Add(-duration)

	return cc.store.QueryRange(metricName, labelMatchers, start, end)
}
