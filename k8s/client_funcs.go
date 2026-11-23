package k8s

import (
	"context"
	"sort"

	"github.com/vladimirvivien/ktop/views/model"
	appsV1 "k8s.io/api/apps/v1"
	batchV1 "k8s.io/api/batch/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (c *Controller) GetNamespaceList(ctx context.Context) ([]*coreV1.Namespace, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	list, err := c.namespaceInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	return list, nil
}

func (c *Controller) GetDeploymentList(ctx context.Context) ([]*appsV1.Deployment, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	items, err := c.deploymentInformer.Lister().List(labels.Everything())

	if err != nil {
		return nil, err
	}

	return items, nil
}

func (c *Controller) GetDaemonSetList(ctx context.Context) ([]*appsV1.DaemonSet, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	items, err := c.daemonSetInformer.Lister().List(labels.Everything())

	if err != nil {
		return nil, err
	}

	return items, nil
}

func (c *Controller) GetReplicaSetList(ctx context.Context) ([]*appsV1.ReplicaSet, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	items, err := c.replicaSetInformer.Lister().List(labels.Everything())

	if err != nil {
		return nil, err
	}

	return items, nil
}

func (c *Controller) GetStatefulSetList(ctx context.Context) ([]*appsV1.StatefulSet, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.statefulSetInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Controller) GetJobList(ctx context.Context) ([]*batchV1.Job, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.jobInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Controller) GetCronJobList(ctx context.Context) ([]*batchV1.CronJob, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.cronJobInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	return items, nil
}

// GetDeployment returns a deep copy of a Deployment from the informer cache.
func (c *Controller) GetDeployment(ctx context.Context, namespace, name string) (*appsV1.Deployment, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	dep, err := c.deploymentInformer.Lister().Deployments(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	return dep.DeepCopy(), nil
}

// GetStatefulSet returns a deep copy of a StatefulSet from the informer cache.
func (c *Controller) GetStatefulSet(ctx context.Context, namespace, name string) (*appsV1.StatefulSet, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	sts, err := c.statefulSetInformer.Lister().StatefulSets(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	return sts.DeepCopy(), nil
}

// GetDaemonSet returns a deep copy of a DaemonSet from the informer cache.
func (c *Controller) GetDaemonSet(ctx context.Context, namespace, name string) (*appsV1.DaemonSet, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	ds, err := c.daemonSetInformer.Lister().DaemonSets(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	return ds.DeepCopy(), nil
}

// GetJob returns a deep copy of a Job from the informer cache.
func (c *Controller) GetJob(ctx context.Context, namespace, name string) (*batchV1.Job, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	job, err := c.jobInformer.Lister().Jobs(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	return job.DeepCopy(), nil
}

func (c *Controller) GetPVList(ctx context.Context) ([]*coreV1.PersistentVolume, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.pvInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (c *Controller) GetPVCList(ctx context.Context) ([]*coreV1.PersistentVolumeClaim, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.pvcInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	return items, nil
}

// GetEventsForObject returns sorted events for the given involved object kind and name.
// Pass an empty namespace for cluster-scoped objects such as Nodes.
func (c *Controller) GetEventsForObject(ctx context.Context, namespace, kind, name string) ([]coreV1.Event, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if c.eventInformer == nil {
		return nil, nil
	}

	var (
		allEvents []*coreV1.Event
		err       error
	)
	if namespace == "" {
		allEvents, err = c.eventInformer.Lister().List(labels.Everything())
	} else {
		allEvents, err = c.eventInformer.Lister().Events(namespace).List(labels.Everything())
	}
	if err != nil {
		return nil, err
	}

	var matched []coreV1.Event
	for _, evt := range allEvents {
		if evt.InvolvedObject.Kind == kind && evt.InvolvedObject.Name == name {
			matched = append(matched, *evt)
		}
	}

	sortEventsByTime(matched)
	return matched, nil
}

// GetEventsForNode returns events related to a specific node
func (c *Controller) GetEventsForNode(ctx context.Context, nodeName string) ([]coreV1.Event, error) {
	return c.GetEventsForObject(ctx, "", "Node", nodeName)
}

// GetEventsForPod returns events related to a specific pod
func (c *Controller) GetEventsForPod(ctx context.Context, namespace, podName string) ([]coreV1.Event, error) {
	return c.GetEventsForObject(ctx, namespace, "Pod", podName)
}

// sortEventsByTime sorts events by normalized last-observed timestamp descending (most recent first).
// Uses stable sort with secondary key (event name) for deterministic ordering.
func sortEventsByTime(events []coreV1.Event) {
	sort.SliceStable(events, func(i, j int) bool {
		timeI := model.EventLastTimestamp(&events[i])
		timeJ := model.EventLastTimestamp(&events[j])
		if timeI.Equal(timeJ) {
			return events[i].Name < events[j].Name
		}
		return timeI.After(timeJ)
	})
}
