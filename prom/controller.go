package prom

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Start initializes and starts the metrics collection controller
func (cc *CollectorController) Start(ctx context.Context) error {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	
	if cc.running {
		return fmt.Errorf("controller is already running")
	}
	
	// Initialize components
	if err := cc.initialize(); err != nil {
		return fmt.Errorf("initializing controller: %w", err)
	}
	
	// Start background processes
	cc.running = true
	
	// Start the metrics collector
	go cc.runCollector(ctx)
	
	// Start periodic cleanup
	go cc.runPeriodicCleanup(ctx)
	
	// Start component discovery
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
	discoveryInterval := 5 * time.Minute
	
	// Run initial discovery
	cc.discoverAvailableComponents(ctx)
	
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
		"running":             running,
		"available_components": availableComponents,
		"config":              cc.config,
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