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
	"github.com/vladimirvivien/ktop/metrics/ksm"
	"github.com/vladimirvivien/ktop/metrics/sli"
	"github.com/vladimirvivien/ktop/views/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	appsV1Informers "k8s.io/client-go/informers/apps/v1"
	batchV1Informers "k8s.io/client-go/informers/batch/v1"
	coreV1Informers "k8s.io/client-go/informers/core/v1"
	discoveryV1Informers "k8s.io/client-go/informers/discovery/v1"
	networkingV1Informers "k8s.io/client-go/informers/networking/v1"
	storageV1Informers "k8s.io/client-go/informers/storage/v1"
	"k8s.io/client-go/tools/cache"
)

type RefreshNodesFunc func(ctx context.Context, items []model.NodeModel) error
type RefreshPodsFunc func(ctx context.Context, items []model.PodModel) error
type RefreshSummaryFunc func(ctx context.Context, items model.ClusterSummary) error
type RefreshWorkloadsFunc func(ctx context.Context, snap model.WorkloadsSnapshot) error
type RefreshEventsFunc func(ctx context.Context, snap model.EventsSnapshot) error
type RefreshStorageFunc func(ctx context.Context, snap model.StorageSnapshot) error
type RefreshNetworkFunc func(ctx context.Context, snap model.NetworkSnapshot) error

type Controller struct {
	client        *Client
	metricsSource metrics.MetricsSource // NEW: for cluster summary metrics
	ksmCollector  *ksm.Collector
	sliCollector  *sli.Collector

	namespaceInformer     coreV1Informers.NamespaceInformer
	nodeInformer          coreV1Informers.NodeInformer
	podInformer           coreV1Informers.PodInformer
	pvInformer            coreV1Informers.PersistentVolumeInformer
	pvcInformer           coreV1Informers.PersistentVolumeClaimInformer
	eventInformer         coreV1Informers.EventInformer
	serviceInformer       coreV1Informers.ServiceInformer
	storageClassInformer  storageV1Informers.StorageClassInformer
	endpointSliceInformer discoveryV1Informers.EndpointSliceInformer
	ingressInformer       networkingV1Informers.IngressInformer

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
	eventsRefreshFunc    RefreshEventsFunc
	eventsNotifyCh       chan struct{}
	storageRefreshFunc   RefreshStorageFunc
	storageNotifyCh      chan struct{}
	networkRefreshFunc   RefreshNetworkFunc
	networkNotifyCh      chan struct{}

	// API health tracking
	healthTracker *health.APIHealthTracker

	startMu          sync.Mutex
	started          bool
	startCtx         context.Context
	workloadsStarted bool
	eventsStarted    bool
	storageStarted   bool
	networkStarted   bool
}

func newController(client *Client) *Controller {
	ctrl := &Controller{
		client:            client,
		workloadsNotifyCh: make(chan struct{}, 1),
		eventsNotifyCh:    make(chan struct{}, 1),
		storageNotifyCh:   make(chan struct{}, 1),
		networkNotifyCh:   make(chan struct{}, 1),
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

func (c *Controller) SetEventsRefreshFunc(fn RefreshEventsFunc) *Controller {
	c.startMu.Lock()
	c.eventsRefreshFunc = fn
	shouldStart := c.started && !c.eventsStarted && fn != nil && c.startCtx != nil
	if shouldStart {
		c.eventsStarted = true
	}
	ctx := c.startCtx
	c.startMu.Unlock()

	if shouldStart {
		c.setupEventsHandler(ctx, fn)
	}
	return c
}

// SetStorageRefreshFunc registers the callback invoked when storage informers or metrics refresh.
func (c *Controller) SetStorageRefreshFunc(fn RefreshStorageFunc) *Controller {
	c.startMu.Lock()
	c.storageRefreshFunc = fn
	shouldStart := c.started && !c.storageStarted && fn != nil && c.startCtx != nil
	if shouldStart {
		c.storageStarted = true
	}
	ctx := c.startCtx
	c.startMu.Unlock()

	if shouldStart {
		c.setupStorageHandler(ctx, fn)
	}
	return c
}

// SetNetworkRefreshFunc registers the callback invoked when network informers or metrics refresh.
func (c *Controller) SetNetworkRefreshFunc(fn RefreshNetworkFunc) *Controller {
	c.startMu.Lock()
	c.networkRefreshFunc = fn
	shouldStart := c.started && !c.networkStarted && fn != nil && c.startCtx != nil
	if shouldStart {
		c.networkStarted = true
	}
	ctx := c.startCtx
	c.startMu.Unlock()

	if shouldStart {
		c.setupNetworkHandler(ctx, fn)
	}
	return c
}

func (c *Controller) SetMetricsSource(source metrics.MetricsSource) *Controller {
	c.metricsSource = source
	return c
}

// SetKSMCollector attaches an optional kube-state-metrics collector.
func (c *Controller) SetKSMCollector(col *ksm.Collector) *Controller {
	c.ksmCollector = col
	return c
}

// SetSLICollector attaches an optional Component SLI & Kubelet Probe collector.
func (c *Controller) SetSLICollector(col *sli.Collector) *Controller {
	c.sliCollector = col
	return c
}

// KSMCollector returns the attached kube-state-metrics collector, if any.
func (c *Controller) KSMCollector() *ksm.Collector {
	return c.ksmCollector
}

// SLICollector returns the attached Component SLI & Probe collector, if any.
func (c *Controller) SLICollector() *sli.Collector {
	return c.sliCollector
}

// GetKSMSnapshot returns the latest lock-free KSM snapshot, or nil if unavailable.
func (c *Controller) GetKSMSnapshot() *ksm.Snapshot {
	if c == nil || c.ksmCollector == nil {
		return nil
	}
	return c.ksmCollector.Latest()
}

// GetSLISnapshot returns the latest lock-free Component SLI & Probe snapshot, or nil if unavailable.
func (c *Controller) GetSLISnapshot() *sli.Snapshot {
	if c == nil || c.sliCollector == nil {
		return nil
	}
	return c.sliCollector.Latest()
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

func canListResource(ctx context.Context, resource string, listFn func(context.Context) error) bool {
	if listFn == nil {
		return true
	}
	probeCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	err := listFn(probeCtx)
	if err == nil {
		return true
	}
	if apierrors.IsForbidden(err) || apierrors.IsNotFound(err) {
		slog.Debug("optional informer disabled due to cluster capability or RBAC", "resource", resource, "error", err)
		return false
	}
	slog.Debug("optional informer probe failed; registering informer", "resource", resource, "error", err)
	return true
}

func (c *Controller) initInformers(ctx context.Context, resync time.Duration) (informers.SharedInformerFactory, []cache.InformerSynced, []cache.InformerSynced) {
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

	var allowSC, allowSvc, allowEPS, allowIng bool
	if c.client != nil && c.client.kubeClient != nil {
		var wg sync.WaitGroup
		wg.Add(4)
		go func() {
			defer wg.Done()
			allowSC = canListResource(ctx, "storageclasses", func(pctx context.Context) error {
				_, err := c.client.kubeClient.StorageV1().StorageClasses().List(pctx, metav1.ListOptions{Limit: 1})
				return err
			})
		}()
		go func() {
			defer wg.Done()
			allowSvc = canListResource(ctx, "services", func(pctx context.Context) error {
				_, err := c.client.kubeClient.CoreV1().Services(c.client.namespace).List(pctx, metav1.ListOptions{Limit: 1})
				return err
			})
		}()
		go func() {
			defer wg.Done()
			allowEPS = canListResource(ctx, "endpointslices", func(pctx context.Context) error {
				_, err := c.client.kubeClient.DiscoveryV1().EndpointSlices(c.client.namespace).List(pctx, metav1.ListOptions{Limit: 1})
				return err
			})
		}()
		go func() {
			defer wg.Done()
			allowIng = canListResource(ctx, "ingresses", func(pctx context.Context) error {
				_, err := c.client.kubeClient.NetworkingV1().Ingresses(c.client.namespace).List(pctx, metav1.ListOptions{Limit: 1})
				return err
			})
		}()
		wg.Wait()
	}

	var optionalSynced []cache.InformerSynced
	if allowSC {
		c.storageClassInformer = factory.Storage().V1().StorageClasses()
		optionalSynced = append(optionalSynced, c.storageClassInformer.Informer().HasSynced)
	}
	if allowSvc {
		c.serviceInformer = coreInformers.Services()
		optionalSynced = append(optionalSynced, c.serviceInformer.Informer().HasSynced)
	}
	if allowEPS {
		c.endpointSliceInformer = factory.Discovery().V1().EndpointSlices()
		optionalSynced = append(optionalSynced, c.endpointSliceInformer.Informer().HasSynced)
	}
	if allowIng {
		c.ingressInformer = factory.Networking().V1().Ingresses()
		optionalSynced = append(optionalSynced, c.ingressInformer.Informer().HasSynced)
	}

	if c.client.namespace == AllNamespaces {
		coreSynced := []cache.InformerSynced{namespaceHasSynced, nodeHasSynced, podHasSynced}
		extraSynced := append([]cache.InformerSynced{
			pvHasSynced, pvcHasSynced, eventHasSynced,
			deploymentHasSynced, daemonsetHasSynced, replicasetHasSynced,
			statefulsetHasSynced, jobHasSynced, cronJobHasSynced,
		}, optionalSynced...)
		return factory, coreSynced, extraSynced
	}

	coreSynced := []cache.InformerSynced{podHasSynced}
	extraSynced := append([]cache.InformerSynced{
		namespaceHasSynced, nodeHasSynced,
		pvHasSynced, pvcHasSynced, eventHasSynced,
		deploymentHasSynced, daemonsetHasSynced, replicasetHasSynced,
		statefulsetHasSynced, jobHasSynced, cronJobHasSynced,
	}, optionalSynced...)
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

	factory, coreSynced, extraSynced := c.initInformers(ctx, resync)
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
		c.notifyEventsChanged()
		c.notifyStorageChanged()
		c.notifyNetworkChanged()
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
	evFn := c.eventsRefreshFunc
	startEV := !c.eventsStarted && evFn != nil
	if startEV {
		c.eventsStarted = true
	}
	stFn := c.storageRefreshFunc
	startST := !c.storageStarted && stFn != nil
	if startST {
		c.storageStarted = true
	}
	nwFn := c.networkRefreshFunc
	startNW := !c.networkStarted && nwFn != nil
	if startNW {
		c.networkStarted = true
	}
	c.startMu.Unlock()
	if startWL {
		c.setupWorkloadsHandler(ctx, wlFn)
	}
	if startEV {
		c.setupEventsHandler(ctx, evFn)
	}
	if startST {
		c.setupStorageHandler(ctx, stFn)
	}
	if startNW {
		c.setupNetworkHandler(ctx, nwFn)
	}

	// Wire up reconnect callback to trigger immediate health check when user presses Retry
	if c.healthTracker != nil {
		c.healthTracker.SetOnTryReconnect(func() {
			c.checkAPIHealth(ctx)
		})
	}

	// Start optional background KSM and SLI/Probe collectors (degrade silently if unavailable)
	if c.ksmCollector != nil && c.ksmCollector.Enabled() {
		c.ksmCollector.Start(ctx)
	}
	if c.sliCollector != nil {
		c.sliCollector.Start(ctx)
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

	factory, coreSynced, extraSynced := c.initInformers(ctx, resync)
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
