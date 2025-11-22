package k8s

import (
	"context"

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
