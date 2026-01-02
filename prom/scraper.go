package prom

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/prometheus/model/labels"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// KubernetesScraper implements MetricsCollector for Kubernetes components
type KubernetesScraper struct {
	config     *ScrapeConfig
	kubeConfig *rest.Config
	clientset  kubernetes.Interface
	restClient rest.Interface

	// Discovered targets
	targetsMutex sync.RWMutex
	targets      map[ComponentType][]*ScrapeTarget
}

// NewKubernetesScraper creates a new Kubernetes metrics scraper
func NewKubernetesScraper(kubeConfig *rest.Config, config *ScrapeConfig) (*KubernetesScraper, error) {
	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	// Use the CoreV1 REST client for all operations
	restClient := clientset.CoreV1().RESTClient()

	scraper := &KubernetesScraper{
		config:     config,
		kubeConfig: kubeConfig,
		clientset:  clientset,
		restClient: restClient,
		targets:    make(map[ComponentType][]*ScrapeTarget),
	}

	return scraper, nil
}

// Start begins the metrics collection process
func (ks *KubernetesScraper) Start(ctx context.Context) error {
	// Discover available targets
	if err := ks.discoverTargets(ctx); err != nil {
		return fmt.Errorf("discovering targets: %w", err)
	}

	// Start scraping goroutines for each enabled component
	ks.targetsMutex.RLock()
	for _, component := range ks.config.Components {
		targets, exists := ks.targets[component]
		if !exists || len(targets) == 0 {
			continue
		}

		go ks.scrapeComponentPeriodically(ctx, component)
	}
	ks.targetsMutex.RUnlock()

	return nil
}

// Stop gracefully stops the metrics collection
func (ks *KubernetesScraper) Stop() error {
	// Note: Actual cancellation is handled by the context passed to Start()
	return nil
}

// ScrapeComponent manually triggers a scrape for a specific component
// For node-based components (kubelet, cAdvisor), this scrapes ALL nodes
// and merges the results with proper node labels added.
func (ks *KubernetesScraper) ScrapeComponent(ctx context.Context, component ComponentType) (*ScrapedMetrics, error) {
	ks.targetsMutex.RLock()
	targets, exists := ks.targets[component]
	ks.targetsMutex.RUnlock()
	if !exists || len(targets) == 0 {
		return nil, fmt.Errorf("no targets found for component %s", component)
	}

	// For node-based components, scrape all nodes and merge results
	if component == ComponentKubelet || component == ComponentCAdvisor {
		return ks.scrapeAllTargets(ctx, targets)
	}

	// For other components, scrape the first available target
	target := targets[0]
	return ks.scrapeTarget(ctx, target)
}

// scrapeAllTargets scrapes all targets IN PARALLEL and merges results into a single ScrapedMetrics
// This is used for node-based components where we need metrics from all nodes
func (ks *KubernetesScraper) scrapeAllTargets(ctx context.Context, targets []*ScrapeTarget) (*ScrapedMetrics, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("no targets to scrape")
	}

	// Result type for collecting scrape results from goroutines
	type scrapeResult struct {
		target  *ScrapeTarget
		metrics *ScrapedMetrics
		err     error
	}

	startTime := time.Now()
	results := make(chan scrapeResult, len(targets))

	// Scrape all targets in parallel
	for _, target := range targets {
		go func(t *ScrapeTarget) {
			metrics, err := ks.scrapeTarget(ctx, t)
			results <- scrapeResult{target: t, metrics: metrics, err: err}
		}(target)
	}

	// Collect results and merge families
	mergedFamilies := make(map[string]*MetricFamily)
	var firstEndpoint string
	var totalDuration time.Duration
	var lastErr error

	for i := 0; i < len(targets); i++ {
		result := <-results
		if result.err != nil {
			lastErr = result.err
			continue // Skip failed targets but continue with others
		}

		if firstEndpoint == "" {
			firstEndpoint = result.metrics.Endpoint
		}
		totalDuration += result.metrics.ScrapeDuration

		// Merge families, adding node label to each time series
		for name, family := range result.metrics.Families {
			// Add node label to each time series in this family
			for _, ts := range family.TimeSeries {
				// Add node label if this is a node-based target
				if result.target.NodeName != "" {
					ts.Labels = append(ts.Labels, labels.Label{
						Name:  "node",
						Value: result.target.NodeName,
					})
				}
			}

			// Merge into existing family or create new
			if existing, ok := mergedFamilies[name]; ok {
				existing.TimeSeries = append(existing.TimeSeries, family.TimeSeries...)
			} else {
				mergedFamilies[name] = family
			}
		}
	}

	// Return error only if ALL targets failed
	if len(mergedFamilies) == 0 && lastErr != nil {
		return &ScrapedMetrics{
			Component:      targets[0].Component,
			Endpoint:       firstEndpoint,
			ScrapedAt:      startTime,
			ScrapeDuration: time.Since(startTime),
			Error:          lastErr,
		}, lastErr
	}

	return &ScrapedMetrics{
		Component:      targets[0].Component,
		Endpoint:       firstEndpoint + " (all nodes)",
		Families:       mergedFamilies,
		ScrapedAt:      startTime,
		ScrapeDuration: totalDuration,
	}, nil
}

// GetLastScrape returns the last scrape result for a component
func (ks *KubernetesScraper) GetLastScrape(component ComponentType) (*ScrapedMetrics, error) {
	// This would be implemented with actual state tracking
	return nil, fmt.Errorf("not implemented")
}

// GetAvailableComponents returns list of available components to scrape
func (ks *KubernetesScraper) GetAvailableComponents(ctx context.Context) ([]ComponentType, error) {
	if err := ks.discoverTargets(ctx); err != nil {
		return nil, err
	}

	ks.targetsMutex.RLock()
	defer ks.targetsMutex.RUnlock()

	var components []ComponentType
	for component, targets := range ks.targets {
		if len(targets) > 0 {
			components = append(components, component)
		}
	}

	return components, nil
}

// discoverTargets discovers available scrape targets for each component
func (ks *KubernetesScraper) discoverTargets(ctx context.Context) error {
	ks.targetsMutex.Lock()
	defer ks.targetsMutex.Unlock()

	// Clear existing targets
	ks.targets = make(map[ComponentType][]*ScrapeTarget)

	// Discover API Server (direct access)
	if err := ks.discoverAPIServerTargets(ctx); err != nil {
		// Log but don't fail - API server metrics might not be accessible
	}

	// Discover Node-based targets (kubelet, cAdvisor)
	if err := ks.discoverNodeTargets(ctx); err != nil {
		// Log but don't fail
	}

	// Discover Pod-based targets (etcd, scheduler, controller-manager, kube-proxy)
	if err := ks.discoverPodTargets(ctx); err != nil {
		// Log but don't fail
	}

	return nil
}

// discoverAPIServerTargets discovers API server metrics endpoint
func (ks *KubernetesScraper) discoverAPIServerTargets(ctx context.Context) error {
	// API server metrics are accessed directly via the root /metrics path
	target := &ScrapeTarget{
		Component: ComponentAPIServer,
		Path:      "/metrics",
		Enabled:   true,
	}

	ks.targets[ComponentAPIServer] = []*ScrapeTarget{target}
	return nil
}

// discoverNodeTargets discovers node-based metrics endpoints (kubelet, cAdvisor)
func (ks *KubernetesScraper) discoverNodeTargets(ctx context.Context) error {
	nodes, err := ks.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}

	var kubeletTargets []*ScrapeTarget
	var cadvisorTargets []*ScrapeTarget

	for _, node := range nodes.Items {
		// Kubelet metrics target - will use RESTClient with Resource("nodes").Name().SubResource("proxy").Suffix("metrics")
		kubeletTarget := &ScrapeTarget{
			Component: ComponentKubelet,
			Path:      "metrics",
			NodeName:  node.Name,
			Enabled:   true,
		}
		kubeletTargets = append(kubeletTargets, kubeletTarget)

		// cAdvisor metrics target
		cadvisorTarget := &ScrapeTarget{
			Component: ComponentCAdvisor,
			Path:      "metrics/cadvisor",
			NodeName:  node.Name,
			Enabled:   true,
		}
		cadvisorTargets = append(cadvisorTargets, cadvisorTarget)
	}

	ks.targets[ComponentKubelet] = kubeletTargets
	ks.targets[ComponentCAdvisor] = cadvisorTargets

	return nil
}

// discoverPodTargets discovers pod-based metrics endpoints for control plane components
func (ks *KubernetesScraper) discoverPodTargets(ctx context.Context) error {
	// Component to label selector mapping
	componentSelectors := map[ComponentType]string{
		ComponentEtcd:              "component=etcd",
		ComponentScheduler:         "component=kube-scheduler",
		ComponentControllerManager: "component=kube-controller-manager",
		ComponentKubeProxy:         "k8s-app=kube-proxy",
	}

	// Component to port mapping
	componentPorts := map[ComponentType]int{
		ComponentEtcd:              2381,
		ComponentScheduler:         10259,
		ComponentControllerManager: 10257,
		ComponentKubeProxy:         10249,
	}

	for component, selector := range componentSelectors {
		pods, err := ks.clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			continue // Skip this component if we can't list pods
		}

		var targets []*ScrapeTarget
		port := componentPorts[component]

		for _, pod := range pods.Items {
			if pod.Status.Phase != "Running" {
				continue
			}

			// Will use RESTClient with Namespace().Resource("pods").Name(podName:port).SubResource("proxy").Suffix("metrics")
			target := &ScrapeTarget{
				Component: component,
				Path:      "metrics",
				Port:      port,
				PodName:   pod.Name,
				Namespace: pod.Namespace,
				Enabled:   true,
			}
			targets = append(targets, target)
		}

		if len(targets) > 0 {
			ks.targets[component] = targets
		}
	}

	return nil
}

// scrapeComponentPeriodically runs periodic scraping for a component
func (ks *KubernetesScraper) scrapeComponentPeriodically(ctx context.Context, component ComponentType) {
	ticker := time.NewTicker(ks.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := ks.ScrapeComponent(ctx, component)
			if err != nil {
				// Log error but continue scraping
				continue
			}
		}
	}
}

// scrapeTarget scrapes metrics from a single target using RESTClient
func (ks *KubernetesScraper) scrapeTarget(ctx context.Context, target *ScrapeTarget) (*ScrapedMetrics, error) {
	// Add per-request timeout to prevent indefinite blocking on slow/unresponsive nodes
	reqCtx, cancel := context.WithTimeout(ctx, ks.config.Timeout)
	defer cancel()

	startTime := time.Now()

	var result rest.Result
	var endpoint string

	// Build the appropriate RESTClient request based on target type
	// Use reqCtx (with timeout) for all requests to prevent indefinite blocking
	switch target.Component {
	case ComponentAPIServer:
		// API server metrics via direct path
		endpoint = "/metrics"
		result = ks.restClient.Get().AbsPath("/metrics").Do(reqCtx)

	case ComponentKubelet, ComponentCAdvisor:
		// Node-based components via node proxy
		endpoint = fmt.Sprintf("nodes/%s/proxy/%s", target.NodeName, target.Path)
		result = ks.restClient.Get().
			Resource("nodes").
			Name(target.NodeName).
			SubResource("proxy").
			Suffix(target.Path).
			Do(reqCtx)

	case ComponentEtcd, ComponentScheduler, ComponentControllerManager, ComponentKubeProxy:
		// Pod-based components via pod proxy
		podNameWithPort := fmt.Sprintf("%s:%d", target.PodName, target.Port)
		endpoint = fmt.Sprintf("namespaces/%s/pods/%s/proxy/%s", target.Namespace, podNameWithPort, target.Path)
		result = ks.restClient.Get().
			Namespace(target.Namespace).
			Resource("pods").
			Name(podNameWithPort).
			SubResource("proxy").
			Suffix(target.Path).
			Do(reqCtx)

	default:
		return nil, fmt.Errorf("unsupported component type: %s", target.Component)
	}

	scrapeDuration := time.Since(startTime)

	// Check for errors
	if err := result.Error(); err != nil {
		return &ScrapedMetrics{
			Component:      target.Component,
			Endpoint:       endpoint,
			ScrapedAt:      startTime,
			ScrapeDuration: scrapeDuration,
			Error:          fmt.Errorf("REST request failed: %w", err),
		}, err
	}

	// Get raw response body
	rawBody, err := result.Raw()
	if err != nil {
		return &ScrapedMetrics{
			Component:      target.Component,
			Endpoint:       endpoint,
			ScrapedAt:      startTime,
			ScrapeDuration: scrapeDuration,
			Error:          fmt.Errorf("getting response body: %w", err),
		}, err
	}

	// Parse metrics
	families, err := ks.parseMetricsBody(rawBody)
	if err != nil {
		return &ScrapedMetrics{
			Component:      target.Component,
			Endpoint:       endpoint,
			ScrapedAt:      startTime,
			ScrapeDuration: scrapeDuration,
			Error:          fmt.Errorf("parsing metrics: %w", err),
		}, err
	}

	// Convert to our internal format
	metricFamilies := make(map[string]*MetricFamily)
	for name, family := range families {
		metricFamily := ks.convertMetricFamily(name, family)
		metricFamilies[name] = metricFamily
	}

	return &ScrapedMetrics{
		Component:      target.Component,
		Endpoint:       endpoint,
		Families:       metricFamilies,
		ScrapedAt:      startTime,
		ScrapeDuration: scrapeDuration,
	}, nil
}

// parseMetricsBody parses raw metrics response body into MetricFamily map
func (ks *KubernetesScraper) parseMetricsBody(body []byte) (map[string]*dto.MetricFamily, error) {
	var parser expfmt.TextParser
	return parser.TextToMetricFamilies(strings.NewReader(string(body)))
}

// convertMetricFamily converts Prometheus DTO to our internal format
func (ks *KubernetesScraper) convertMetricFamily(name string, family *dto.MetricFamily) *MetricFamily {
	metricFamily := &MetricFamily{
		Name:        name,
		Type:        family.GetType(),
		Help:        family.GetHelp(),
		LastUpdated: time.Now(),
		TimeSeries:  make([]*TimeSeries, 0),
	}

	timestamp := time.Now().UnixMilli()

	for _, metric := range family.Metric {
		// Build labels
		lbls := make(labels.Labels, 0, len(metric.Label)+1)
		lbls = append(lbls, labels.Label{Name: "__name__", Value: name})

		for _, label := range metric.Label {
			lbls = append(lbls, labels.Label{
				Name:  label.GetName(),
				Value: label.GetValue(),
			})
		}

		// Extract value based on metric type
		var value float64
		switch family.GetType() {
		case dto.MetricType_COUNTER:
			if metric.Counter != nil {
				value = metric.Counter.GetValue()
			}
		case dto.MetricType_GAUGE:
			if metric.Gauge != nil {
				value = metric.Gauge.GetValue()
			}
		case dto.MetricType_HISTOGRAM:
			if metric.Histogram != nil {
				value = float64(metric.Histogram.GetSampleCount())
			}
		case dto.MetricType_SUMMARY:
			if metric.Summary != nil {
				value = float64(metric.Summary.GetSampleCount())
			}
		case dto.MetricType_UNTYPED:
			if metric.Untyped != nil {
				value = metric.Untyped.GetValue()
			}
		}

		// Create time series with ring buffer
		samples := NewRingBuffer[MetricSample](1) // Initial capacity of 1
		samples.Add(MetricSample{
			Timestamp: timestamp,
			Value:     value,
		})
		timeSeries := &TimeSeries{
			Labels:  lbls,
			Samples: samples,
		}

		metricFamily.TimeSeries = append(metricFamily.TimeSeries, timeSeries)
	}

	return metricFamily
}
