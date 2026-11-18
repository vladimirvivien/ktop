package k8s

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/vladimirvivien/ktop/health"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/views/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	appsV1Informers "k8s.io/client-go/informers/apps/v1"
	batchV1Informers "k8s.io/client-go/informers/batch/v1"
	coreV1Informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type RefreshNodesFunc func(ctx context.Context, items []model.NodeModel) error
type RefreshPodsFunc func(ctx context.Context, items []model.PodModel) error
type RefreshSummaryFunc func(ctx context.Context, items model.ClusterSummary) error
type RefreshWorkloadsFunc func(ctx context.Context, snap model.WorkloadsSnapshot) error

type Controller struct {
	client        *Client
	metricsSource metrics.MetricsSource // NEW: for cluster summary metrics

	namespaceInformer coreV1Informers.NamespaceInformer
	nodeInformer      coreV1Informers.NodeInformer
	podInformer       coreV1Informers.PodInformer
	pvInformer        coreV1Informers.PersistentVolumeInformer
	pvcInformer       coreV1Informers.PersistentVolumeClaimInformer
	eventInformer     coreV1Informers.EventInformer

	jobInformer     batchV1Informers.JobInformer
	cronJobInformer batchV1Informers.CronJobInformer

	deploymentInformer  appsV1Informers.DeploymentInformer
	daemonSetInformer   appsV1Informers.DaemonSetInformer
	replicaSetInformer  appsV1Informers.ReplicaSetInformer
	statefulSetInformer appsV1Informers.StatefulSetInformer

	nodeRefreshFunc      RefreshNodesFunc
	podRefreshFunc       RefreshPodsFunc
	summaryRefreshFunc   RefreshSummaryFunc
	workloadsRefreshFunc RefreshWorkloadsFunc
	workloadsNotifyCh    chan struct{}

	// API health tracking
	healthTracker *health.APIHealthTracker

	startMu          sync.Mutex
	started          bool
	startCtx         context.Context
	workloadsStarted bool
}

func newController(client *Client) *Controller {
	ctrl := &Controller{
		client:            client,
		workloadsNotifyCh: make(chan struct{}, 1),
	}
	return ctrl
}

func (c *Controller) SetNodeRefreshFunc(fn RefreshNodesFunc) *Controller {
	c.nodeRefreshFunc = fn
	return c
}
func (c *Controller) SetPodRefreshFunc(fn RefreshPodsFunc) *Controller {
	c.podRefreshFunc = fn
	return c
}

func (c *Controller) SetClusterSummaryRefreshFunc(fn RefreshSummaryFunc) *Controller {
	c.summaryRefreshFunc = fn
	return c
}

func (c *Controller) SetWorkloadsRefreshFunc(fn RefreshWorkloadsFunc) *Controller {
	c.startMu.Lock()
	c.workloadsRefreshFunc = fn
	shouldStart := c.started && !c.workloadsStarted && fn != nil && c.startCtx != nil
	if shouldStart {
		c.workloadsStarted = true
	}
	ctx := c.startCtx
	c.startMu.Unlock()

	if shouldStart {
		c.setupWorkloadsHandler(ctx, fn)
	}
	return c
}

func (c *Controller) SetMetricsSource(source metrics.MetricsSource) *Controller {
	c.metricsSource = source
	return c
}

func (c *Controller) SetHealthTracker(tracker *health.APIHealthTracker) *Controller {
	c.healthTracker = tracker
	return c
}

// reportSuccess reports a successful API operation to the health tracker
func (c *Controller) reportSuccess() {
	if c.healthTracker != nil {
		c.healthTracker.ReportSuccess()
	}
}

// reportError reports an API error to the health tracker
func (c *Controller) reportError(err error) {
	if c.healthTracker != nil {
		c.healthTracker.ReportError(err)
	}
}

func (c *Controller) GetClient() *Client {
	return c.client
}

func (c *Controller) initInformers(resync time.Duration) (informers.SharedInformerFactory, []cache.InformerSynced, []cache.InformerSynced) {
	var factory informers.SharedInformerFactory
	if c.client.namespace == AllNamespaces {
		factory = informers.NewSharedInformerFactory(c.client.kubeClient, resync)
	} else {
		factory = informers.NewSharedInformerFactoryWithOptions(c.client.kubeClient, resync, informers.WithNamespace(c.client.namespace))
	}

	// Register every informer before calling factory.Start so caches sync.
	coreInformers := factory.Core().V1()
	c.namespaceInformer = coreInformers.Namespaces()
	namespaceHasSynced := c.namespaceInformer.Informer().HasSynced
	c.nodeInformer = coreInformers.Nodes()
	nodeHasSynced := c.nodeInformer.Informer().HasSynced
	c.podInformer = coreInformers.Pods()
	podHasSynced := c.podInformer.Informer().HasSynced
	c.pvInformer = coreInformers.PersistentVolumes()
	pvHasSynced := c.pvInformer.Informer().HasSynced
	c.pvcInformer = coreInformers.PersistentVolumeClaims()
	pvcHasSynced := c.pvcInformer.Informer().HasSynced
	c.eventInformer = coreInformers.Events()
	eventHasSynced := c.eventInformer.Informer().HasSynced

	appsInformers := factory.Apps().V1()
	c.deploymentInformer = appsInformers.Deployments()
	deploymentHasSynced := c.deploymentInformer.Informer().HasSynced
	c.daemonSetInformer = appsInformers.DaemonSets()
	daemonsetHasSynced := c.daemonSetInformer.Informer().HasSynced
	c.replicaSetInformer = appsInformers.ReplicaSets()
	replicasetHasSynced := c.replicaSetInformer.Informer().HasSynced
	c.statefulSetInformer = appsInformers.StatefulSets()
	statefulsetHasSynced := c.statefulSetInformer.Informer().HasSynced

	batchInformers := factory.Batch().V1()
	c.jobInformer = batchInformers.Jobs()
	jobHasSynced := c.jobInformer.Informer().HasSynced
	c.cronJobInformer = batchInformers.CronJobs()
	cronJobHasSynced := c.cronJobInformer.Informer().HasSynced

	if c.client.namespace == AllNamespaces {
		coreSynced := []cache.InformerSynced{namespaceHasSynced, nodeHasSynced, podHasSynced}
		extraSynced := []cache.InformerSynced{
			pvHasSynced, pvcHasSynced, eventHasSynced,
			deploymentHasSynced, daemonsetHasSynced, replicasetHasSynced,
			statefulsetHasSynced, jobHasSynced, cronJobHasSynced,
		}
		return factory, coreSynced, extraSynced
	}

	coreSynced := []cache.InformerSynced{podHasSynced}
	extraSynced := []cache.InformerSynced{
		namespaceHasSynced, nodeHasSynced,
		pvHasSynced, pvcHasSynced, eventHasSynced,
		deploymentHasSynced, daemonsetHasSynced, replicasetHasSynced,
		statefulsetHasSynced, jobHasSynced, cronJobHasSynced,
	}
	return factory, coreSynced, extraSynced
}

func (c *Controller) Start(ctx context.Context, resync time.Duration) error {
	if ctx == nil {
		return errors.New("context cannot be nil")
	}

	c.startMu.Lock()
	if c.started {
		c.startMu.Unlock()
		return nil
	}
	c.started = true
	c.startCtx = ctx
	c.startMu.Unlock()

	factory, coreSynced, extraSynced := c.initInformers(resync)
	factory.Start(ctx.Done())

	// Wait for core resources to sync with a short timeout to prevent hanging
	// This gives informers a chance to populate initial data before UI renders
	syncCtx, syncCancel := context.WithTimeout(ctx, 2*time.Second)
	defer syncCancel()

	// Preserve TUI startup sync expectations across namespace, node, and pod informers.
	tuiCoreSynced := []cache.InformerSynced{
		c.namespaceInformer.Informer().HasSynced,
		c.nodeInformer.Informer().HasSynced,
		c.podInformer.Informer().HasSynced,
	}
	_ = coreSynced

	if ok := cache.WaitForCacheSync(syncCtx.Done(), tuiCoreSynced...); !ok {
		// Timeout or error - continue anyway with graceful degradation
		// UI will render with empty/partial data initially
		slog.Warn("initial informer cache sync incomplete; continuing with partial data")
	}

	// defer waiting for non-core resources to sync
	go func() {
		ok := cache.WaitForCacheSync(ctx.Done(), extraSynced...)
		if !ok {
			slog.Warn("some non-core informers failed to sync", "namespace", c.client.namespace)
		}
		if c.workloadsNotifyCh != nil {
			select {
			case c.workloadsNotifyCh <- struct{}{}:
			default:
			}
		}
	}()

	c.setupSummaryHandler(ctx, c.summaryRefreshFunc)
	c.setupNodeHandler(ctx, c.nodeRefreshFunc)
	c.installPodsHandler(ctx, c.podRefreshFunc)

	c.startMu.Lock()
	wlFn := c.workloadsRefreshFunc
	startWL := !c.workloadsStarted && wlFn != nil
	if startWL {
		c.workloadsStarted = true
	}
	c.startMu.Unlock()
	if startWL {
		c.setupWorkloadsHandler(ctx, wlFn)
	}

	// Wire up reconnect callback to trigger immediate health check when user presses Retry
	if c.healthTracker != nil {
		c.healthTracker.SetOnTryReconnect(func() {
			c.checkAPIHealth(ctx)
		})
	}

	// Start API health monitor - makes periodic live API calls to detect connection loss
	c.startAPIHealthMonitor(ctx)

	return nil
}

// StartInformers starts Kubernetes resource informers and waits for core
// caches to synchronize within syncTimeout without launching UI refresh tickers.
func (c *Controller) StartInformers(ctx context.Context, resync, syncTimeout time.Duration) error {
	if ctx == nil {
		return errors.New("context cannot be nil")
	}
	if syncTimeout <= 0 {
		syncTimeout = 15 * time.Second
	}

	factory, coreSynced, extraSynced := c.initInformers(resync)
	factory.Start(ctx.Done())

	coreCtx, coreCancel := context.WithTimeout(ctx, syncTimeout)
	defer coreCancel()

	if ok := cache.WaitForCacheSync(coreCtx.Done(), coreSynced...); !ok {
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("informer cache sync timed out: %w", context.DeadlineExceeded)
	}

	extraTimeout := 1 * time.Second
	if syncTimeout < extraTimeout {
		extraTimeout = syncTimeout
	}
	extraCtx, extraCancel := context.WithTimeout(ctx, extraTimeout)
	defer extraCancel()
	if ok := cache.WaitForCacheSync(extraCtx.Done(), extraSynced...); !ok {
		slog.Warn("some non-core informers failed to sync", "namespace", c.client.namespace)
	}

	return nil
}

// startAPIHealthMonitor periodically checks API server connectivity
// This is needed because informers cache data and don't report connection loss
func (c *Controller) startAPIHealthMonitor(ctx context.Context) {
	if c.healthTracker == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.checkAPIHealth(ctx)
			}
		}
	}()
}

// checkAPIHealth makes a lightweight API call to verify connectivity
// This bypasses all caching to get an accurate health status
func (c *Controller) checkAPIHealth(ctx context.Context) {
	if c.healthTracker == nil {
		return
	}

	// Use a short timeout for the health check
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Invalidate discovery cache to ensure fresh API call
	if c.client.discoClient != nil {
		c.client.discoClient.Invalidate()
	}

	// Make a direct API call that cannot be cached - list pods in the configured namespace
	// so both cluster-wide and namespace-scoped RBAC users can verify connectivity
	_, err := c.client.kubeClient.CoreV1().Pods(c.client.namespace).List(checkCtx, metav1.ListOptions{
		Limit: 1,
	})
	if err != nil {
		c.reportError(err)
		return
	}

	c.reportSuccess()
}
