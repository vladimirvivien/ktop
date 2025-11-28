package prom

import (
	"context"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestNewKubernetesScraper(t *testing.T) {
	config := &rest.Config{
		Host: "https://test-cluster",
	}
	scrapeConfig := DefaultScrapeConfig()

	scraper, err := NewKubernetesScraper(config, scrapeConfig)
	if err != nil {
		t.Fatalf("Failed to create scraper: %v", err)
	}

	if scraper.config != scrapeConfig {
		t.Error("Scraper config not set correctly")
	}

	if scraper.kubeConfig != config {
		t.Error("Kube config not set correctly")
	}

	if scraper.targets == nil {
		t.Error("Targets map not initialized")
	}
}

func TestDiscoverAPIServerTargets(t *testing.T) {
	config := &rest.Config{Host: "https://test-cluster"}
	scrapeConfig := DefaultScrapeConfig()

	scraper, err := NewKubernetesScraper(config, scrapeConfig)
	if err != nil {
		t.Fatalf("Failed to create scraper: %v", err)
	}

	ctx := context.Background()
	err = scraper.discoverAPIServerTargets(ctx)
	if err != nil {
		t.Fatalf("Failed to discover API server targets: %v", err)
	}

	targets, exists := scraper.targets[ComponentAPIServer]
	if !exists {
		t.Fatal("API server targets not discovered")
	}

	if len(targets) != 1 {
		t.Fatalf("Expected 1 API server target, got %d", len(targets))
	}

	target := targets[0]
	if target.Component != ComponentAPIServer {
		t.Errorf("Expected component %s, got %s", ComponentAPIServer, target.Component)
	}

	if target.Path != "/metrics" {
		t.Errorf("Expected path '/metrics', got '%s'", target.Path)
	}

	if !target.Enabled {
		t.Error("Target should be enabled")
	}
}

func TestDiscoverNodeTargets(t *testing.T) {
	// Create fake Kubernetes client with test nodes
	fakeClient := fake.NewSimpleClientset()

	// Add test nodes
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Status:     corev1.NodeStatus{Phase: corev1.NodeRunning},
	}
	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
		Status:     corev1.NodeStatus{Phase: corev1.NodeRunning},
	}

	fakeClient.CoreV1().Nodes().Create(context.Background(), node1, metav1.CreateOptions{})
	fakeClient.CoreV1().Nodes().Create(context.Background(), node2, metav1.CreateOptions{})

	// Create scraper with fake client
	scraper := &KubernetesScraper{
		config:     DefaultScrapeConfig(),
		clientset:  fakeClient,
		restClient: fakeClient.CoreV1().RESTClient(),
		targets:    make(map[ComponentType][]*ScrapeTarget),
	}

	ctx := context.Background()
	err := scraper.discoverNodeTargets(ctx)
	if err != nil {
		t.Fatalf("Failed to discover node targets: %v", err)
	}

	// Check kubelet targets
	kubeletTargets, exists := scraper.targets[ComponentKubelet]
	if !exists {
		t.Fatal("Kubelet targets not discovered")
	}

	if len(kubeletTargets) != 2 {
		t.Fatalf("Expected 2 kubelet targets, got %d", len(kubeletTargets))
	}

	// Check cAdvisor targets
	cadvisorTargets, exists := scraper.targets[ComponentCAdvisor]
	if !exists {
		t.Fatal("cAdvisor targets not discovered")
	}

	if len(cadvisorTargets) != 2 {
		t.Fatalf("Expected 2 cAdvisor targets, got %d", len(cadvisorTargets))
	}

	// Verify target properties
	kubeletTarget := kubeletTargets[0]
	if kubeletTarget.Component != ComponentKubelet {
		t.Errorf("Expected component %s, got %s", ComponentKubelet, kubeletTarget.Component)
	}

	if kubeletTarget.Path != "metrics" {
		t.Errorf("Expected path 'metrics', got '%s'", kubeletTarget.Path)
	}

	if kubeletTarget.NodeName == "" {
		t.Error("NodeName should be set for kubelet target")
	}

	cadvisorTarget := cadvisorTargets[0]
	if cadvisorTarget.Path != "metrics/cadvisor" {
		t.Errorf("Expected path 'metrics/cadvisor', got '%s'", cadvisorTarget.Path)
	}
}

func TestDiscoverPodTargets(t *testing.T) {
	// Create fake Kubernetes client with test pods
	fakeClient := fake.NewSimpleClientset()

	// Add test pods for different components
	etcdPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-master",
			Namespace: "kube-system",
			Labels:    map[string]string{"component": "etcd"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	schedulerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kube-scheduler-master",
			Namespace: "kube-system",
			Labels:    map[string]string{"component": "kube-scheduler"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	fakeClient.CoreV1().Pods("kube-system").Create(context.Background(), etcdPod, metav1.CreateOptions{})
	fakeClient.CoreV1().Pods("kube-system").Create(context.Background(), schedulerPod, metav1.CreateOptions{})

	// Create scraper with fake client
	scraper := &KubernetesScraper{
		config:     DefaultScrapeConfig(),
		clientset:  fakeClient,
		restClient: fakeClient.CoreV1().RESTClient(),
		targets:    make(map[ComponentType][]*ScrapeTarget),
	}

	ctx := context.Background()
	err := scraper.discoverPodTargets(ctx)
	if err != nil {
		t.Fatalf("Failed to discover pod targets: %v", err)
	}

	// Check etcd targets
	etcdTargets, exists := scraper.targets[ComponentEtcd]
	if !exists {
		t.Fatal("Etcd targets not discovered")
	}

	if len(etcdTargets) != 1 {
		t.Fatalf("Expected 1 etcd target, got %d", len(etcdTargets))
	}

	etcdTarget := etcdTargets[0]
	if etcdTarget.Component != ComponentEtcd {
		t.Errorf("Expected component %s, got %s", ComponentEtcd, etcdTarget.Component)
	}

	if etcdTarget.Port != 2381 {
		t.Errorf("Expected port 2381, got %d", etcdTarget.Port)
	}

	if etcdTarget.PodName != "etcd-master" {
		t.Errorf("Expected pod name 'etcd-master', got '%s'", etcdTarget.PodName)
	}

	if etcdTarget.Namespace != "kube-system" {
		t.Errorf("Expected namespace 'kube-system', got '%s'", etcdTarget.Namespace)
	}

	// Check scheduler targets
	schedulerTargets, exists := scraper.targets[ComponentScheduler]
	if !exists {
		t.Fatal("Scheduler targets not discovered")
	}

	if len(schedulerTargets) != 1 {
		t.Fatalf("Expected 1 scheduler target, got %d", len(schedulerTargets))
	}

	schedulerTarget := schedulerTargets[0]
	if schedulerTarget.Port != 10259 {
		t.Errorf("Expected port 10259, got %d", schedulerTarget.Port)
	}
}

func TestParseMetricsBody(t *testing.T) {
	scraper := &KubernetesScraper{}

	// Sample Prometheus metrics data
	metricsData := `# HELP test_counter_total A test counter metric
# TYPE test_counter_total counter
test_counter_total{label1="value1",label2="value2"} 42.0

# HELP test_gauge A test gauge metric
# TYPE test_gauge gauge
test_gauge{label="test"} 3.14
`

	families, err := scraper.parseMetricsBody([]byte(metricsData))
	if err != nil {
		t.Fatalf("Failed to parse metrics: %v", err)
	}

	if len(families) != 2 {
		t.Fatalf("Expected 2 metric families, got %d", len(families))
	}

	// Check counter metric
	counterFamily, exists := families["test_counter_total"]
	if !exists {
		t.Fatal("test_counter_total metric not found")
	}

	if counterFamily.GetType() != dto.MetricType_COUNTER {
		t.Errorf("Expected counter type, got %v", counterFamily.GetType())
	}

	if len(counterFamily.Metric) != 1 {
		t.Fatalf("Expected 1 counter metric, got %d", len(counterFamily.Metric))
	}

	counterMetric := counterFamily.Metric[0]
	if counterMetric.Counter.GetValue() != 42.0 {
		t.Errorf("Expected counter value 42.0, got %f", counterMetric.Counter.GetValue())
	}

	// Check gauge metric
	gaugeFamily, exists := families["test_gauge"]
	if !exists {
		t.Fatal("test_gauge metric not found")
	}

	if gaugeFamily.GetType() != dto.MetricType_GAUGE {
		t.Errorf("Expected gauge type, got %v", gaugeFamily.GetType())
	}
}

func TestConvertMetricFamily(t *testing.T) {
	scraper := &KubernetesScraper{}

	// Create a test DTO metric family
	metricName := "test_metric"
	help := "A test metric"
	metricType := dto.MetricType_GAUGE

	dtoFamily := &dto.MetricFamily{
		Name: &metricName,
		Help: &help,
		Type: &metricType,
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{
						Name:  stringPtr("instance"),
						Value: stringPtr("localhost:8080"),
					},
					{
						Name:  stringPtr("job"),
						Value: stringPtr("test-job"),
					},
				},
				Gauge: &dto.Gauge{
					Value: float64Ptr(123.45),
				},
			},
		},
	}

	// Convert to internal format
	internalFamily := scraper.convertMetricFamily(metricName, dtoFamily)

	// Verify conversion
	if internalFamily.Name != metricName {
		t.Errorf("Expected name '%s', got '%s'", metricName, internalFamily.Name)
	}

	if internalFamily.Help != help {
		t.Errorf("Expected help '%s', got '%s'", help, internalFamily.Help)
	}

	if internalFamily.Type != metricType {
		t.Errorf("Expected type %v, got %v", metricType, internalFamily.Type)
	}

	if len(internalFamily.TimeSeries) != 1 {
		t.Fatalf("Expected 1 time series, got %d", len(internalFamily.TimeSeries))
	}

	timeSeries := internalFamily.TimeSeries[0]

	// Check labels (should include __name__ + metric labels)
	expectedLabels := 3 // __name__, instance, job
	if len(timeSeries.Labels) != expectedLabels {
		t.Errorf("Expected %d labels, got %d", expectedLabels, len(timeSeries.Labels))
	}

	// Check samples
	if timeSeries.Samples.Len() != 1 {
		t.Fatalf("Expected 1 sample, got %d", timeSeries.Samples.Len())
	}

	sample, ok := timeSeries.Samples.Last()
	if !ok {
		t.Fatal("Expected sample to be present")
	}
	if sample.Value != 123.45 {
		t.Errorf("Expected value 123.45, got %f", sample.Value)
	}

	// Verify timestamp is recent
	now := time.Now().UnixMilli()
	if sample.Timestamp < now-1000 || sample.Timestamp > now+1000 {
		t.Errorf("Sample timestamp %d seems incorrect (now: %d)", sample.Timestamp, now)
	}
}

func TestGetAvailableComponents(t *testing.T) {
	// Create fake client with test resources
	fakeClient := fake.NewSimpleClientset()

	// Add a node for kubelet/cAdvisor
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Status:     corev1.NodeStatus{Phase: corev1.NodeRunning},
	}
	fakeClient.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})

	// Add an etcd pod
	etcdPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-test",
			Namespace: "kube-system",
			Labels:    map[string]string{"component": "etcd"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	fakeClient.CoreV1().Pods("kube-system").Create(context.Background(), etcdPod, metav1.CreateOptions{})

	scraper := &KubernetesScraper{
		config:     DefaultScrapeConfig(),
		clientset:  fakeClient,
		restClient: fakeClient.CoreV1().RESTClient(),
		targets:    make(map[ComponentType][]*ScrapeTarget),
	}

	ctx := context.Background()
	components, err := scraper.GetAvailableComponents(ctx)
	if err != nil {
		t.Fatalf("Failed to get available components: %v", err)
	}

	// Should find API server, kubelet, cAdvisor, and etcd
	expectedComponents := map[ComponentType]bool{
		ComponentAPIServer: true,
		ComponentKubelet:   true,
		ComponentCAdvisor:  true,
		ComponentEtcd:      true,
	}

	if len(components) < len(expectedComponents) {
		t.Errorf("Expected at least %d components, got %d", len(expectedComponents), len(components))
	}

	for _, component := range components {
		if !expectedComponents[component] {
			t.Errorf("Unexpected component found: %s", component)
		}
	}
}

// Test helper functions
func stringPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}

// TestScrapeTargetValidation tests that scrape targets are properly validated
func TestScrapeTargetValidation(t *testing.T) {
	tests := []struct {
		name   string
		target *ScrapeTarget
		valid  bool
	}{
		{
			name: "valid kubelet target",
			target: &ScrapeTarget{
				Component: ComponentKubelet,
				Path:      "metrics",
				NodeName:  "test-node",
				Enabled:   true,
			},
			valid: true,
		},
		{
			name: "valid pod target",
			target: &ScrapeTarget{
				Component: ComponentEtcd,
				Path:      "metrics",
				Port:      2381,
				PodName:   "etcd-pod",
				Namespace: "kube-system",
				Enabled:   true,
			},
			valid: true,
		},
		{
			name: "invalid - missing node name for node component",
			target: &ScrapeTarget{
				Component: ComponentKubelet,
				Path:      "metrics",
				Enabled:   true,
			},
			valid: false,
		},
		{
			name: "invalid - missing pod name for pod component",
			target: &ScrapeTarget{
				Component: ComponentEtcd,
				Path:      "metrics",
				Port:      2381,
				Namespace: "kube-system",
				Enabled:   true,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := validateScrapeTarget(tt.target)
			if valid != tt.valid {
				t.Errorf("Expected validation result %v, got %v", tt.valid, valid)
			}
		})
	}
}

// validateScrapeTarget is a helper function to validate targets
func validateScrapeTarget(target *ScrapeTarget) bool {
	if target.Path == "" {
		return false
	}

	switch target.Component {
	case ComponentKubelet, ComponentCAdvisor:
		return target.NodeName != ""
	case ComponentEtcd, ComponentScheduler, ComponentControllerManager, ComponentKubeProxy:
		return target.PodName != "" && target.Namespace != "" && target.Port > 0
	case ComponentAPIServer:
		return true
	default:
		return false
	}
}
