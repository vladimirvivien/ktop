package ksm

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type serviceTarget struct {
	namespace string
	service   string
	port      string
	directURL string
}

func (t serviceTarget) label() string {
	if t.directURL != "" {
		return t.directURL
	}
	return fmt.Sprintf("service-proxy:%s/%s:%s", t.namespace, t.service, t.port)
}

// Collector auto-discovers and periodically scrapes kube-state-metrics, storing
// the latest immutable Snapshot for lock-free reads.
type Collector struct {
	cfg        Config
	clientset  kubernetes.Interface
	restClient rest.Interface
	httpClient *http.Client

	targetMu       sync.RWMutex
	target         *serviceTarget
	lastDiscoverAt time.Time

	latest atomic.Pointer[Snapshot]
}

// NewCollector builds a kube-state-metrics Collector backed by kubeClient/restConfig.
// If cfg.Mode is ModeOff, the collector stays inactive and degrades silently.
func NewCollector(kubeClient kubernetes.Interface, restConfig *rest.Config, cfg Config) *Collector {
	if cfg.Mode == "" {
		cfg.Mode = ModeAuto
	}
	if cfg.ScrapeInterval <= 0 {
		cfg.ScrapeInterval = DefaultScrapeInterval
	}
	if cfg.ScrapeTimeout <= 0 {
		cfg.ScrapeTimeout = DefaultScrapeTimeout
	}

	var rc rest.Interface
	if kubeClient != nil {
		rc = kubeClient.CoreV1().RESTClient()
	} else if restConfig != nil {
		if cs, err := kubernetes.NewForConfig(restConfig); err == nil {
			kubeClient = cs
			rc = cs.CoreV1().RESTClient()
		}
	}

	return &Collector{
		cfg:        cfg,
		clientset:  kubeClient,
		restClient: rc,
		httpClient: &http.Client{Timeout: cfg.ScrapeTimeout},
	}
}

// Enabled reports whether KSM collection is enabled by configuration.
func (c *Collector) Enabled() bool {
	return c != nil && strings.ToLower(strings.TrimSpace(c.cfg.Mode)) != ModeOff
}

// Latest returns the most recently scraped KSM Snapshot, or nil when unavailable.
func (c *Collector) Latest() *Snapshot {
	if c == nil {
		return nil
	}
	return c.latest.Load()
}

// Start launches background KSM discovery and periodic scraping until ctx is cancelled.
func (c *Collector) Start(ctx context.Context) {
	if !c.Enabled() || ctx == nil {
		return
	}
	go func() {
		c.ScrapeOnce(ctx)
		ticker := time.NewTicker(c.cfg.ScrapeInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.ScrapeOnce(ctx)
			}
		}
	}()
}

// ScrapeOnce resolves the KSM target (if not already resolved) and performs one
// bounded scrape. Failures are logged at debug level and never returned as fatal errors.
func (c *Collector) ScrapeOnce(ctx context.Context) *Snapshot {
	if !c.Enabled() {
		return nil
	}
	scrapeCtx, cancel := context.WithTimeout(ctx, c.cfg.ScrapeTimeout)
	defer cancel()

	target := c.resolveTarget(scrapeCtx)
	if target == nil {
		c.latest.Store(&Snapshot{Available: false})
		return nil
	}

	stream, err := c.openStream(scrapeCtx, *target)
	if err != nil {
		slog.Debug("kube-state-metrics scrape skipped", "target", target.label(), "error", err)
		c.clearDiscoveredTargetIfDynamic()
		c.latest.Store(&Snapshot{Available: false, Endpoint: target.label()})
		return nil
	}
	defer stream.Close()

	snap, err := ParseStream(stream, target.label(), time.Now())
	if err != nil {
		slog.Debug("kube-state-metrics parse failed", "target", target.label(), "error", err)
		c.latest.Store(&Snapshot{Available: false, Endpoint: target.label()})
		return nil
	}

	c.latest.Store(snap)
	return snap
}

func (c *Collector) resolveTarget(ctx context.Context) *serviceTarget {
	c.targetMu.RLock()
	existing := c.target
	lastTry := c.lastDiscoverAt
	c.targetMu.RUnlock()
	if existing != nil {
		return existing
	}

	// Explicit --ksm-endpoint takes precedence over auto-discovery.
	if ep := strings.TrimSpace(c.cfg.Endpoint); ep != "" {
		t := parseExplicitEndpoint(ep)
		c.targetMu.Lock()
		c.target = t
		c.targetMu.Unlock()
		return t
	}

	if !lastTry.IsZero() && time.Since(lastTry) < DefaultRediscoverBackoff {
		return nil
	}

	discovered := c.discoverService(ctx)
	c.targetMu.Lock()
	c.lastDiscoverAt = time.Now()
	c.target = discovered
	c.targetMu.Unlock()
	if discovered != nil {
		slog.Info("discovered kube-state-metrics target", "target", discovered.label())
	}
	return discovered
}

func (c *Collector) clearDiscoveredTargetIfDynamic() {
	if strings.TrimSpace(c.cfg.Endpoint) != "" {
		return
	}
	c.targetMu.Lock()
	c.target = nil
	c.targetMu.Unlock()
}

func parseExplicitEndpoint(ep string) *serviceTarget {
	if strings.HasPrefix(ep, "http://") || strings.HasPrefix(ep, "https://") {
		return &serviceTarget{directURL: ep}
	}
	parts := strings.SplitN(ep, "/", 2)
	if len(parts) != 2 {
		return &serviceTarget{namespace: "kube-system", service: ep, port: "8080"}
	}
	ns := strings.TrimSpace(parts[0])
	svcPort := strings.SplitN(strings.TrimSpace(parts[1]), ":", 2)
	svc := svcPort[0]
	port := "8080"
	if len(svcPort) == 2 && svcPort[1] != "" {
		port = svcPort[1]
	}
	return &serviceTarget{namespace: ns, service: svc, port: port}
}

func (c *Collector) discoverService(ctx context.Context) *serviceTarget {
	if c.clientset == nil {
		return nil
	}

	candidateNS := []string{"kube-system", "monitoring", "observability", "prometheus"}
	if hint := strings.TrimSpace(c.cfg.NamespaceHint); hint != "" {
		found := false
		for _, ns := range candidateNS {
			if ns == hint {
				found = true
				break
			}
		}
		if !found {
			candidateNS = append([]string{hint}, candidateNS...)
		}
	}

	for _, ns := range candidateNS {
		svcs, err := c.clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		if t := matchKSMService(ns, svcs.Items); t != nil {
			return t
		}
	}
	return nil
}

func matchKSMService(namespace string, items []corev1.Service) *serviceTarget {
	for _, svc := range items {
		nameLower := strings.ToLower(svc.Name)
		isKSM := strings.Contains(nameLower, "kube-state-metrics") ||
			strings.EqualFold(svc.Labels["app.kubernetes.io/name"], "kube-state-metrics") ||
			strings.EqualFold(svc.Labels["app"], "kube-state-metrics")
		if !isKSM {
			continue
		}
		port := pickKSMPort(svc.Spec.Ports)
		return &serviceTarget{
			namespace: namespace,
			service:   svc.Name,
			port:      port,
		}
	}
	return nil
}

func pickKSMPort(ports []corev1.ServicePort) string {
	for _, p := range ports {
		n := strings.ToLower(p.Name)
		if n == "http-metrics" || n == "metrics" || n == "http" {
			return fmt.Sprintf("%d", p.Port)
		}
	}
	for _, p := range ports {
		if p.Port == 8080 {
			return "8080"
		}
	}
	if len(ports) > 0 && ports[0].Port > 0 {
		return fmt.Sprintf("%d", ports[0].Port)
	}
	return "8080"
}

func (c *Collector) openStream(ctx context.Context, target serviceTarget) (io.ReadCloser, error) {
	if target.directURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.directURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, target.directURL)
		}
		return resp.Body, nil
	}

	if c.restClient == nil {
		return nil, fmt.Errorf("kubernetes REST client unavailable")
	}

	serviceRef := fmt.Sprintf("http:%s:%s", target.service, target.port)
	return c.restClient.Get().
		Namespace(target.namespace).
		Resource("services").
		Name(serviceRef).
		SubResource("proxy").
		Suffix("metrics").
		Stream(ctx)
}
