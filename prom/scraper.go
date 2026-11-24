package prom

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
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

const maxConcurrentScrapes = 6

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

// Start discovers available scrape targets. Periodic scraping and storage are
// driven by CollectorController.runCollector.
func (ks *KubernetesScraper) Start(ctx context.Context) error {
	if err := ks.discoverTargets(ctx); err != nil {
		return fmt.Errorf("discovering targets: %w", err)
	}

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
	sem := make(chan struct{}, maxConcurrentScrapes)

	// Scrape targets with bounded concurrency to cap peak heap usage on multi-node clusters.
	for _, target := range targets {
		go func(t *ScrapeTarget) {
			sem <- struct{}{}
			defer func() { <-sem }()
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

// scrapeTarget scrapes metrics from a single target using RESTClient
func (ks *KubernetesScraper) scrapeTarget(ctx context.Context, target *ScrapeTarget) (*ScrapedMetrics, error) {
	// Add per-request timeout to prevent indefinite blocking on slow/unresponsive nodes
	reqCtx, cancel := context.WithTimeout(ctx, ks.config.Timeout)
	defer cancel()

	startTime := time.Now()

	var req *rest.Request
	var endpoint string

	// Build the appropriate RESTClient request based on target type
	// Use reqCtx (with timeout) for all requests to prevent indefinite blocking
	switch target.Component {
	case ComponentAPIServer:
		// API server metrics via direct path
		endpoint = "/metrics"
		req = ks.restClient.Get().AbsPath("/metrics")

	case ComponentKubelet, ComponentCAdvisor:
		// Node-based components via node proxy
		endpoint = fmt.Sprintf("nodes/%s/proxy/%s", target.NodeName, target.Path)
		req = ks.restClient.Get().
			Resource("nodes").
			Name(target.NodeName).
			SubResource("proxy").
			Suffix(target.Path)

	case ComponentEtcd, ComponentScheduler, ComponentControllerManager, ComponentKubeProxy:
		// Pod-based components via pod proxy
		podNameWithPort := fmt.Sprintf("%s:%d", target.PodName, target.Port)
		endpoint = fmt.Sprintf("namespaces/%s/pods/%s/proxy/%s", target.Namespace, podNameWithPort, target.Path)
		req = ks.restClient.Get().
			Namespace(target.Namespace).
			Resource("pods").
			Name(podNameWithPort).
			SubResource("proxy").
			Suffix(target.Path)

	default:
		return nil, fmt.Errorf("unsupported component type: %s", target.Component)
	}

	body, err := req.Stream(reqCtx)
	scrapeDuration := time.Since(startTime)
	if err != nil {
		return &ScrapedMetrics{
			Component:      target.Component,
			Endpoint:       endpoint,
			ScrapedAt:      startTime,
			ScrapeDuration: scrapeDuration,
			Error:          fmt.Errorf("REST request failed: %w", err),
		}, err
	}
	defer body.Close()

	// Stream-filter to allowlisted metrics before parsing so non-allowlisted
	// families never allocate protobuf DTO trees on the heap.
	families, err := parseAllowedMetrics(body, DefaultMetricAllowlist)
	if err != nil {
		return &ScrapedMetrics{
			Component:      target.Component,
			Endpoint:       endpoint,
			ScrapedAt:      startTime,
			ScrapeDuration: scrapeDuration,
			Error:          fmt.Errorf("parsing metrics: %w", err),
		}, err
	}

	// Convert to our internal format, filtering to only allowed metrics.
	// Anchor upstream sample timestamps into local host clock space using the
	// freshest allowed sample timestamp in this payload so clock skew between
	// the node and ktop host does not skew range queries.
	scrapeTimestamp := time.Now().UnixMilli()
	maxUpstreamTs := maxAllowedUpstreamTimestamp(families)
	var clockOffsetMs int64
	if maxUpstreamTs > 0 {
		clockOffsetMs = scrapeTimestamp - maxUpstreamTs
	}

	filterCAdvisor := target.Component == ComponentCAdvisor
	metricFamilies := make(map[string]*MetricFamily, len(families))
	for name, family := range families {
		if !DefaultMetricAllowlist[name] {
			continue
		}
		metricFamily := ks.convertMetricFamilyFiltered(name, family, scrapeTimestamp, clockOffsetMs, filterCAdvisor)
		if len(metricFamily.TimeSeries) > 0 {
			metricFamilies[name] = metricFamily
		}
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
	return parser.TextToMetricFamilies(bytes.NewReader(body))
}

// filterAllowedMetrics reads a Prometheus text exposition stream line-by-line
// and retains only metadata (# HELP / # TYPE) and sample lines whose metric
// name is present in allowlist.
func filterAllowedMetrics(r io.Reader, allowlist map[string]bool) (*bytes.Buffer, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	var out bytes.Buffer

	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimLeft(line, " \t")
			if len(trimmed) > 0 {
				var name string
				if trimmed[0] == '#' {
					name = extractHeaderMetricName(trimmed)
				} else {
					name = extractSampleMetricName(trimmed)
				}
				if name != "" && allowlist[name] {
					out.Write(line)
					if line[len(line)-1] != '\n' {
						out.WriteByte('\n')
					}
				}
			}
		}
		if err == io.EOF {
			return &out, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func extractHeaderMetricName(line []byte) string {
	rest := bytes.TrimLeft(line[1:], " \t")
	switch {
	case bytes.HasPrefix(rest, []byte("HELP ")), bytes.HasPrefix(rest, []byte("HELP\t")):
		rest = bytes.TrimLeft(rest[4:], " \t")
	case bytes.HasPrefix(rest, []byte("TYPE ")), bytes.HasPrefix(rest, []byte("TYPE\t")):
		rest = bytes.TrimLeft(rest[4:], " \t")
	default:
		return ""
	}
	end := bytes.IndexAny(rest, " \t\r\n")
	if end < 0 {
		return string(rest)
	}
	return string(rest[:end])
}

func extractSampleMetricName(line []byte) string {
	end := bytes.IndexAny(line, "{ \t\r\n")
	if end <= 0 {
		return ""
	}
	return string(line[:end])
}

func parseAllowedMetrics(r io.Reader, allowlist map[string]bool) (map[string]*dto.MetricFamily, error) {
	filtered, err := filterAllowedMetrics(r, allowlist)
	if err != nil {
		return nil, err
	}
	if filtered.Len() == 0 {
		return map[string]*dto.MetricFamily{}, nil
	}
	var parser expfmt.TextParser
	return parser.TextToMetricFamilies(filtered)
}

// shouldKeepCAdvisorSeries returns true if a cAdvisor series is queried by ktop.
// Dropping unqueried systemd/slice cgroups, pause containers, and per-core pod
// CPU series cuts stored series cardinality significantly without affecting any
// node or pod metric calculation.
func shouldKeepCAdvisorSeries(metricName string, metric *dto.Metric) bool {
	if metricName == "container_count" || !strings.HasPrefix(metricName, "container_") {
		return true
	}

	var id, pod, namespace, container, cpu string
	for _, l := range metric.Label {
		switch l.GetName() {
		case "id":
			id = l.GetValue()
		case "pod":
			pod = l.GetValue()
		case "namespace":
			namespace = l.GetValue()
		case "container":
			container = l.GetValue()
		case "cpu":
			cpu = l.GetValue()
		}
	}

	isNodeRoot := id == "/"
	isPod := pod != "" && namespace != ""
	if !isNodeRoot && !isPod {
		return false
	}

	switch metricName {
	case "container_network_receive_bytes_total", "container_network_transmit_bytes_total":
		return isNodeRoot
	case "container_cpu_cfs_throttled_seconds_total",
		"container_cpu_cfs_throttled_periods_total",
		"container_cpu_cfs_periods_total":
		return isPod && container != "" && container != "POD"
	case "container_pressure_cpu_waiting_seconds_total",
		"container_pressure_cpu_stalled_seconds_total",
		"container_pressure_memory_waiting_seconds_total",
		"container_pressure_memory_stalled_seconds_total",
		"container_pressure_io_waiting_seconds_total",
		"container_pressure_io_stalled_seconds_total":
		return isNodeRoot || (isPod && container != "" && container != "POD")
	case "container_cpu_usage_seconds_total":
		if isNodeRoot {
			return true
		}
		if container == "POD" {
			return false
		}
		return cpu == "" || cpu == "total"
	case "container_memory_working_set_bytes":
		if isNodeRoot {
			return true
		}
		return container != "POD"
	default:
		return isNodeRoot || isPod
	}
}

// maxAllowedUpstreamTimestamp returns the maximum explicit sample timestamp
// across allowed metric families, or 0 if no samples carry upstream timestamps.
func maxAllowedUpstreamTimestamp(families map[string]*dto.MetricFamily) int64 {
	var maxTs int64
	for name, family := range families {
		if !DefaultMetricAllowlist[name] || family == nil {
			continue
		}
		for _, m := range family.Metric {
			if ts := m.GetTimestampMs(); ts > maxTs {
				maxTs = ts
			}
		}
	}
	return maxTs
}

// convertMetricFamily converts Prometheus DTO to our internal format.
func (ks *KubernetesScraper) convertMetricFamily(name string, family *dto.MetricFamily) *MetricFamily {
	return ks.convertMetricFamilyWithTimestamp(name, family, time.Now().UnixMilli(), 0)
}

// convertMetricFamilyWithTimestamp converts Prometheus DTO to our internal format,
// preserving upstream metric timestamps anchored by clockOffsetMs when present.
func (ks *KubernetesScraper) convertMetricFamilyWithTimestamp(name string, family *dto.MetricFamily, scrapeTimestamp, clockOffsetMs int64) *MetricFamily {
	return ks.convertMetricFamilyFiltered(name, family, scrapeTimestamp, clockOffsetMs, false)
}

func (ks *KubernetesScraper) convertMetricFamilyFiltered(name string, family *dto.MetricFamily, scrapeTimestamp, clockOffsetMs int64, filterCAdvisor bool) *MetricFamily {
	metricFamily := &MetricFamily{
		Name:        name,
		Type:        family.GetType(),
		Help:        family.GetHelp(),
		LastUpdated: time.Now(),
		TimeSeries:  make([]*TimeSeries, 0),
	}

	for _, metric := range family.Metric {
		if filterCAdvisor && !shouldKeepCAdvisorSeries(name, metric) {
			continue
		}
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

		sampleTimestamp := scrapeTimestamp
		upstreamTimestamp := metric.GetTimestampMs()
		if upstreamTimestamp > 0 {
			sampleTimestamp = upstreamTimestamp + clockOffsetMs
		}

		// Create time series with ring buffer
		samples := NewRingBuffer[MetricSample](1) // Initial capacity of 1
		samples.Add(MetricSample{
			Timestamp:         sampleTimestamp,
			UpstreamTimestamp: upstreamTimestamp,
			Value:             value,
		})
		timeSeries := &TimeSeries{
			Labels:  lbls,
			Samples: samples,
		}

		metricFamily.TimeSeries = append(metricFamily.TimeSeries, timeSeries)
	}

	return metricFamily
}
