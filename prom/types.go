package prom

import (
	"context"
	"sync"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prometheus/model/labels"
	"k8s.io/client-go/rest"
)

// ComponentType represents different Kubernetes components that expose metrics
type ComponentType string

const (
	ComponentAPIServer          ComponentType = "apiserver"
	ComponentKubelet            ComponentType = "kubelet"
	ComponentCAdvisor           ComponentType = "cadvisor"
	ComponentEtcd               ComponentType = "etcd"
	ComponentScheduler          ComponentType = "scheduler"
	ComponentControllerManager  ComponentType = "controller-manager"
	ComponentKubeProxy          ComponentType = "kube-proxy"
)

// MetricSample represents a single metric data point
type MetricSample struct {
	Timestamp int64
	Value     float64
}

// TimeSeries represents a time series with labels and samples
type TimeSeries struct {
	Labels  labels.Labels
	Samples []MetricSample
}

// MetricFamily represents a group of related metrics
type MetricFamily struct {
	Name        string
	Type        dto.MetricType
	Help        string
	TimeSeries  []*TimeSeries
	LastUpdated time.Time
}

// ScrapedMetrics represents metrics collected from a single component
type ScrapedMetrics struct {
	Component   ComponentType
	Endpoint    string
	Families    map[string]*MetricFamily
	ScrapedAt   time.Time
	ScrapeDuration time.Duration
	Error       error
}

// ScrapeTarget represents a component endpoint to scrape
type ScrapeTarget struct {
	Component ComponentType
	Path      string
	Port      int
	NodeName  string // For node-proxy targets
	PodName   string // For pod-proxy targets
	Namespace string // For pod-proxy targets
	Enabled   bool
}

// ScrapeConfig holds configuration for the metrics scraper
type ScrapeConfig struct {
	Interval       time.Duration
	Timeout        time.Duration
	MaxSamples     int // Maximum samples per time series
	RetentionTime  time.Duration
	InsecureTLS    bool
	Components     []ComponentType // Components to scrape
}

// MetricsCollector defines the interface for collecting metrics
type MetricsCollector interface {
	// Start begins the metrics collection process
	Start(ctx context.Context) error
	
	// Stop gracefully stops the metrics collection
	Stop() error
	
	// ScrapeComponent manually triggers a scrape for a specific component
	ScrapeComponent(ctx context.Context, component ComponentType) (*ScrapedMetrics, error)
	
	// GetLastScrape returns the last scrape result for a component
	GetLastScrape(component ComponentType) (*ScrapedMetrics, error)
	
	// GetAvailableComponents returns list of available components to scrape
	GetAvailableComponents(ctx context.Context) ([]ComponentType, error)
}

// MetricsStore defines the interface for storing and querying metrics
type MetricsStore interface {
	// AddMetrics stores scraped metrics
	AddMetrics(metrics *ScrapedMetrics) error
	
	// QueryLatest returns the latest value for a metric
	QueryLatest(metricName string, labelMatchers map[string]string) (float64, error)
	
	// QueryRange returns metric values over a time range
	QueryRange(metricName string, labelMatchers map[string]string, start, end time.Time) ([]*MetricSample, error)
	
	// GetMetricNames returns all available metric names
	GetMetricNames() []string
	
	// GetLabelValues returns all values for a given label name
	GetLabelValues(labelName string) []string
	
	// Cleanup removes old metrics based on retention policy
	Cleanup() error
}

// CollectorController manages the overall metrics collection process
type CollectorController struct {
	mutex     sync.RWMutex
	config    *ScrapeConfig
	collector MetricsCollector
	store     MetricsStore
	kubeConfig *rest.Config
	
	// State tracking
	running   bool
	lastError error
	
	// Component availability
	availableComponents map[ComponentType]bool
	
	// Event callbacks
	onMetricsCollected func(component ComponentType, metrics *ScrapedMetrics)
	onError           func(component ComponentType, err error)
}

// DefaultScrapeConfig returns a default configuration for metrics scraping
func DefaultScrapeConfig() *ScrapeConfig {
	return &ScrapeConfig{
		Interval:      30 * time.Second,
		Timeout:       10 * time.Second,
		MaxSamples:    1000,
		RetentionTime: 1 * time.Hour,
		InsecureTLS:   false,
		Components: []ComponentType{
			ComponentAPIServer,
			ComponentKubelet,
			ComponentCAdvisor,
		},
	}
}

// NewCollectorController creates a new metrics collector controller
func NewCollectorController(kubeConfig *rest.Config, config *ScrapeConfig) *CollectorController {
	if config == nil {
		config = DefaultScrapeConfig()
	}
	
	return &CollectorController{
		config:              config,
		kubeConfig:          kubeConfig,
		availableComponents: make(map[ComponentType]bool),
	}
}

// SetMetricsCollectedCallback sets a callback for when metrics are collected
func (cc *CollectorController) SetMetricsCollectedCallback(callback func(ComponentType, *ScrapedMetrics)) {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	cc.onMetricsCollected = callback
}

// SetErrorCallback sets a callback for when errors occur during collection
func (cc *CollectorController) SetErrorCallback(callback func(ComponentType, error)) {
	cc.mutex.Lock()
	defer cc.mutex.Unlock()
	cc.onError = callback
}

// IsRunning returns whether the collector is currently running
func (cc *CollectorController) IsRunning() bool {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()
	return cc.running
}

// GetLastError returns the last error that occurred during collection
func (cc *CollectorController) GetLastError() error {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()
	return cc.lastError
}

// GetAvailableComponents returns the list of available components
func (cc *CollectorController) GetAvailableComponents() []ComponentType {
	cc.mutex.RLock()
	defer cc.mutex.RUnlock()
	
	var components []ComponentType
	for component, available := range cc.availableComponents {
		if available {
			components = append(components, component)
		}
	}
	return components
}