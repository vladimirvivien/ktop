package k8s

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vladimirvivien/ktop/views/model"
	appsV1 "k8s.io/api/apps/v1"
	batchV1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
)

var (
	GVRs = map[string]schema.GroupVersionResource{
		"nodes":                  {Group: "", Version: "v1", Resource: "nodes"},
		"namespaces":             {Group: "", Version: "v1", Resource: "namespaces"},
		"pods":                   {Group: "", Version: "v1", Resource: "pods"},
		"persistentvolumeclaims": {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
		"deployments":            {Group: appsV1.GroupName, Version: "v1", Resource: "deployments"},
		"daemonsets":             {Group: appsV1.GroupName, Version: "v1", Resource: "daemonsets"},
		"replicasets":            {Group: appsV1.GroupName, Version: "v1", Resource: "replicasets"},
		"statefulsets":           {Group: appsV1.GroupName, Version: "v1", Resource: "statefulsets"},
		"jobs":                   {Group: batchV1.GroupName, Version: "v1", Resource: "jobs"},
		"cronjobs":               {Group: batchV1.GroupName, Version: "v1", Resource: "cronjobs"},
	}
)

type RefreshNodesFunc func(ctx context.Context, items []model.NodeModel) error
type RefreshPodsFunc func(ctx context.Context, items []model.PodModel) error
type RefreshSummaryFunc func(ctx context.Context, items model.ClusterSummary) error

type Controller struct {
	client            *Client
	namespaceInformer informers.GenericInformer
	nodeInformer      informers.GenericInformer
	nodeRefreshFunc   RefreshNodesFunc
	podInformer       informers.GenericInformer
	podRefreshFunc    RefreshPodsFunc

	deploymentInformer  informers.GenericInformer
	daemonSetInformer   informers.GenericInformer
	replicaSetInformer  informers.GenericInformer
	statefulSetInformer informers.GenericInformer

	jobInformer     informers.GenericInformer
	cronJobInformer informers.GenericInformer

	pvcInformer        informers.GenericInformer
	summaryRefreshFunc RefreshSummaryFunc
}

func newController(client *Client) *Controller {
	ctrl := &Controller{client: client}
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

func (c *Controller) Start(ctx context.Context, resync time.Duration) error {
	if ctx == nil {
		return errors.New("context cannot be nil")
	}
	factory := dynamicinformer.NewDynamicSharedInformerFactory(c.client.dynaClient, resync)
	c.namespaceInformer = factory.ForResource(GVRs["namespaces"])
	c.nodeInformer = factory.ForResource(GVRs["nodes"])
	c.podInformer = factory.ForResource(GVRs["pods"])
	c.deploymentInformer = factory.ForResource(GVRs["deployments"])
	c.daemonSetInformer = factory.ForResource(GVRs["daemonsets"])
	c.replicaSetInformer = factory.ForResource(GVRs["replicasets"])
	c.statefulSetInformer = factory.ForResource(GVRs["statefulsets"])
	c.jobInformer = factory.ForResource(GVRs["jobs"])
	c.cronJobInformer = factory.ForResource(GVRs["cronjobs"])
	c.pvcInformer = factory.ForResource(GVRs["persistentvolumeclaims"])
	//c.installHandler(ctx, c.pvcInformer, c.pvcRefreshFunc)

	factory.Start(ctx.Done())
	for name, gvr := range GVRs {
		if synced := factory.WaitForCacheSync(ctx.Done()); !synced[gvr] {
			return fmt.Errorf("resource not synced: %s", name)
		}
	}
	c.setupSummaryHandler(ctx, c.summaryRefreshFunc)
	c.setupNodeHandler(ctx, c.nodeRefreshFunc)
	c.installPodsHandler(ctx, c.podRefreshFunc)

	return nil
}
