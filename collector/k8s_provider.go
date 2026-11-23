package collector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/vladimirvivien/ktop/internal/snapshot"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/views/model"
)

// WarmupWaiter is optionally implemented by metrics sources (such as Prometheus)
// that need initial scrape cycles before rate metrics are available.
type WarmupWaiter interface {
	WaitForWarmup(ctx context.Context) error
}

// K8sSnapshotProvider implements SnapshotProvider on top of *k8s.Client and
// an optional metrics.MetricsSource.
type K8sSnapshotProvider struct {
	k8sC          *k8s.Client
	metricsSource metrics.MetricsSource
	warmupTimeout time.Duration
	builder       *snapshot.Builder
}

// NewK8sSnapshotProvider constructs a synchronous snapshot provider backed by k8sC.
func NewK8sSnapshotProvider(k8sC *k8s.Client, metricsSource metrics.MetricsSource, warmupTimeout time.Duration) *K8sSnapshotProvider {
	return &K8sSnapshotProvider{
		k8sC:          k8sC,
		metricsSource: metricsSource,
		warmupTimeout: warmupTimeout,
		builder:       snapshot.NewBuilder(),
	}
}

// WaitReady starts Kubernetes informers and waits for core caches to sync, then
// waits for metrics source warmup if supported.
func (p *K8sSnapshotProvider) WaitReady(ctx context.Context) error {
	ctrl := p.k8sC.Controller()
	ctrl.SetMetricsSource(p.metricsSource)

	syncTimeout := p.warmupTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 {
			syncTimeout = remaining
		}
	}

	if err := ctrl.StartInformers(ctx, 30*time.Second, syncTimeout); err != nil {
		return err
	}

	if p.metricsSource != nil {
		if waiter, ok := p.metricsSource.(WarmupWaiter); ok {
			if err := waiter.WaitForWarmup(ctx); err != nil {
				return fmt.Errorf("waiting for metrics warmup: %w", err)
			}
		}
	}

	// Best-effort initial KSM and SLI/Probe scrape (bounded, silent on error)
	var wg sync.WaitGroup
	enrichCtx, enrichCancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer enrichCancel()
	if ksmCol := ctrl.KSMCollector(); ksmCol != nil && ksmCol.Enabled() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ksmCol.ScrapeOnce(enrichCtx)
		}()
	}
	if sliCol := ctrl.SLICollector(); sliCol != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sliCol.ScrapeOnce(enrichCtx)
		}()
	}
	wg.Wait()

	return nil
}

// Capture builds a point-in-time Snapshot from synced informers and metrics.
func (p *K8sSnapshotProvider) Capture(ctx context.Context) (*snapshot.Snapshot, error) {
	ctrl := p.k8sC.Controller()

	summary, err := ctrl.BuildClusterSummary(ctx)
	if err != nil {
		return nil, err
	}
	nodes, err := ctrl.GetNodeModels(ctx)
	if err != nil {
		return nil, err
	}
	pods, err := ctrl.GetPodModels(ctx)
	if err != nil {
		return nil, err
	}
	workloadsSnap, err := ctrl.GetWorkloadsSnapshot(ctx)
	var workloadsPtr *model.WorkloadsSnapshot
	if err == nil {
		workloadsPtr = &workloadsSnap
	}
	eventsSnap, err := ctrl.GetEventsSnapshot(ctx, k8s.AllNamespaces, k8s.DefaultSnapshotEventsLimit)
	var eventsPtr *model.EventsSnapshot
	if err == nil {
		eventsPtr = &eventsSnap
	}

	var podMetrics []*metrics.PodMetrics
	if p.metricsSource != nil {
		pm, err := p.metricsSource.GetAllPodMetrics(ctx)
		if err == nil {
			podMetrics = pm
		}
	}

	metricsSourceName := "none"
	if p.metricsSource != nil {
		if info := p.metricsSource.GetSourceInfo(); info.Type != "" {
			metricsSourceName = info.Type
		}
	}

	return p.builder.Build(ctx, snapshot.BuildInput{
		Cluster: snapshot.ClusterInfo{
			Context:           p.k8sC.ClusterContext(),
			KubernetesVersion: p.k8sC.GetServerVersion(),
			MetricsSource:     metricsSourceName,
			Ready:             true,
			PSIBeta:           !p.k8sC.PSIVersionOK(),
		},
		Summary:    summary,
		Nodes:      nodes,
		Pods:       pods,
		PodMetrics: podMetrics,
		Workloads:  workloadsPtr,
		Events:     eventsPtr,
		KSM:        ctrl.GetKSMSnapshot(),
		SLI:        ctrl.GetSLISnapshot(),
	})
}
