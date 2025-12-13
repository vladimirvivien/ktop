package k8s

import (
	"context"
	"sort"

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

// GetEventsForNode returns events related to a specific node
func (c *Controller) GetEventsForNode(ctx context.Context, nodeName string) ([]coreV1.Event, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Events for nodes are cluster-scoped, so we list all events and filter
	allEvents, err := c.eventInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var nodeEvents []coreV1.Event
	for _, evt := range allEvents {
		if evt.InvolvedObject.Kind == "Node" && evt.InvolvedObject.Name == nodeName {
			nodeEvents = append(nodeEvents, *evt)
		}
	}

	// Sort by LastTimestamp descending (most recent first)
	sortEventsByTime(nodeEvents)
	return nodeEvents, nil
}

// GetEventsForPod returns events related to a specific pod
func (c *Controller) GetEventsForPod(ctx context.Context, namespace, podName string) ([]coreV1.Event, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Get events from the pod's namespace
	allEvents, err := c.eventInformer.Lister().Events(namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	var podEvents []coreV1.Event
	for _, evt := range allEvents {
		if evt.InvolvedObject.Kind == "Pod" && evt.InvolvedObject.Name == podName {
			podEvents = append(podEvents, *evt)
		}
	}

	// Sort by LastTimestamp descending (most recent first)
	sortEventsByTime(podEvents)
	return podEvents, nil
}

// sortEventsByTime sorts events by LastTimestamp descending (most recent first)
// Uses stable sort with secondary key (event name) for deterministic ordering
func sortEventsByTime(events []coreV1.Event) {
	sort.SliceStable(events, func(i, j int) bool {
		timeI := events[i].LastTimestamp.Time
		timeJ := events[j].LastTimestamp.Time
		// If LastTimestamp is zero, use EventTime
		if timeI.IsZero() && !events[i].EventTime.IsZero() {
			timeI = events[i].EventTime.Time
		}
		if timeJ.IsZero() && !events[j].EventTime.IsZero() {
			timeJ = events[j].EventTime.Time
		}
		// Sort descending (most recent first)
		// If times are equal, sort by name for stability
		if timeI.Equal(timeJ) {
			return events[i].Name < events[j].Name
		}
		return timeI.After(timeJ)
	})
}
