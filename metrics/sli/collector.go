package sli

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// NodeLister optionally supplies cached node names to avoid extra List API calls.
type NodeLister func(ctx context.Context) ([]string, error)

// Collector periodically scrapes Kubernetes Component SLIs (/metrics/slis on apiserver
// and kubelets) and Kubelet probe counters (/metrics/probes), storing an immutable
// Snapshot for lock-free reads. All endpoints degrade silently on RBAC or connectivity errors.
type Collector struct {
	cfg        Config
	clientset  kubernetes.Interface
	restClient rest.Interface
	nodeLister NodeLister
	latest     atomic.Pointer[Snapshot]
}

// NewCollector builds an SLI and probe Collector backed by kubeClient/restConfig.
func NewCollector(kubeClient kubernetes.Interface, restConfig *rest.Config, nodeLister NodeLister, cfg Config) *Collector {
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
		nodeLister: nodeLister,
	}
}

// Latest returns the most recently scraped SLI/probe Snapshot, or nil if none yet.
func (c *Collector) Latest() *Snapshot {
	if c == nil {
		return nil
	}
	return c.latest.Load()
}

// Start launches background SLI and probe scraping until ctx is cancelled.
func (c *Collector) Start(ctx context.Context) {
	if c == nil || ctx == nil {
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

// ScrapeOnce scrapes /metrics/slis (apiserver + kubelets) and /metrics/probes (kubelets)
// once with bounded concurrency and stores the resulting Snapshot.
func (c *Collector) ScrapeOnce(ctx context.Context) *Snapshot {
	if c == nil || c.restClient == nil {
		return nil
	}

	scrapeCtx, cancel := context.WithTimeout(ctx, c.cfg.ScrapeTimeout)
	defer cancel()

	var (
		mu              sync.Mutex
		components      []ComponentSLI
		podProbes       = make(map[string][]ProbeStats)
		sliAvailable    bool
		probesAvailable bool
		wg              sync.WaitGroup
	)

	// 1. API Server /metrics/slis
	wg.Add(1)
	go func() {
		defer wg.Done()
		stream, err := c.restClient.Get().AbsPath("/metrics/slis").Stream(scrapeCtx)
		if err != nil {
			slog.Debug("apiserver /metrics/slis unavailable", "error", err)
			return
		}
		defer stream.Close()

		comp, ok, err := ParseSLIStream(stream, "apiserver", "apiserver")
		if err != nil || !ok {
			return
		}
		mu.Lock()
		components = append(components, comp)
		sliAvailable = true
		mu.Unlock()
	}()

	// 2. Kubelet /metrics/slis and /metrics/probes via nodes/proxy (bounded concurrency)
	nodes := c.listNodes(scrapeCtx)
	sem := make(chan struct{}, 6)
	for _, nodeName := range nodes {
		if nodeName == "" {
			continue
		}
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-scrapeCtx.Done():
				return
			}

			if stream, err := c.restClient.Get().
				Resource("nodes").
				Name(node).
				SubResource("proxy").
				Suffix("metrics/slis").
				Stream(scrapeCtx); err == nil {
				comp, ok, parseErr := ParseSLIStream(stream, "kubelet", node)
				stream.Close()
				if parseErr == nil && ok {
					mu.Lock()
					components = append(components, comp)
					sliAvailable = true
					mu.Unlock()
				}
			}

			if stream, err := c.restClient.Get().
				Resource("nodes").
				Name(node).
				SubResource("proxy").
				Suffix("metrics/probes").
				Stream(scrapeCtx); err == nil {
				probes, ok, parseErr := ParseProbesStream(stream)
				stream.Close()
				if parseErr == nil && ok {
					mu.Lock()
					probesAvailable = true
					for podKey, list := range probes {
						podProbes[podKey] = append(podProbes[podKey], list...)
					}
					mu.Unlock()
				}
			}
		}(nodeName)
	}

	wg.Wait()

	sort.Slice(components, func(i, j int) bool {
		if components[i].Component != components[j].Component {
			return components[i].Component < components[j].Component
		}
		return components[i].Target < components[j].Target
	})

	snap := &Snapshot{
		Available:       sliAvailable,
		ProbesAvailable: probesAvailable,
		ScrapedAt:       time.Now().UTC(),
		Components:      components,
		PodProbes:       podProbes,
	}
	c.latest.Store(snap)
	return snap
}

func (c *Collector) listNodes(ctx context.Context) []string {
	if c.nodeLister != nil {
		if names, err := c.nodeLister(ctx); err == nil {
			return names
		}
	}
	if c.clientset == nil {
		return nil
	}
	list, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(list.Items))
	for _, n := range list.Items {
		names = append(names, n.Name)
	}
	return names
}
