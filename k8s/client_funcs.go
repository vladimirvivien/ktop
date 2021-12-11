package k8s

import (
	"context"

	appsV1 "k8s.io/api/apps/v1"
	batchV1 "k8s.io/api/batch/v1"
	coreV1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
)


func (c *Controller) GetNamespaceList(ctx context.Context) ([]coreV1.Node, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.namespaceInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var nodes []coreV1.Node
	for _, item := range items {
		unstructNode, ok := item.(runtime.Unstructured)
		if !ok {
			panic("Controller: GetDeploymentList: unexpected type for deployment")
		}
		node := new(coreV1.Node)
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructNode.UnstructuredContent(), node); err != nil {
			continue
		}
		nodes = append(nodes, *node)
	}
	return nodes, nil
}

func (c *Controller) GetDeploymentList(ctx context.Context) ([]appsV1.Deployment, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.deploymentInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var deps []appsV1.Deployment
	for _, item := range items {
		unstructDep, ok := item.(runtime.Unstructured)
		if !ok {
			panic("Controller: GetDeploymentList: unexpected type for deployment")
		}
		dep := new(appsV1.Deployment)
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructDep.UnstructuredContent(), dep); err != nil {
			continue
		}
		deps = append(deps, *dep)
	}
	return deps, nil
}

func (c *Controller) GetDaemonSetList(ctx context.Context) ([]appsV1.DaemonSet, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.daemonSetInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var daemons []appsV1.DaemonSet
	for _, item := range items {
		unstructDaemon, ok := item.(runtime.Unstructured)
		if !ok {
			panic("Controller: GetDaemonSetList: unexpected type for deployment")
		}
		daemon := new(appsV1.DaemonSet)
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructDaemon.UnstructuredContent(), daemon); err != nil {
			continue
		}
		daemons = append(daemons, *daemon)
	}
	return daemons, nil
}


func (c *Controller) GetReplicaSetList(ctx context.Context) ([]appsV1.ReplicaSet, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.replicaSetInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var replicasets []appsV1.ReplicaSet
	for _, item := range items {
		unstructDaemon, ok := item.(runtime.Unstructured)
		if !ok {
			panic("Controller: GetReplicaSetList: unexpected type for deployment")
		}
		replica := new(appsV1.ReplicaSet)
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructDaemon.UnstructuredContent(), replica); err != nil {
			continue
		}
		replicasets = append(replicasets, *replica)
	}
	return replicasets, nil
}

func (c *Controller) GetStatefulSetList(ctx context.Context) ([]appsV1.StatefulSet, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.statefulSetInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var statefulsets []appsV1.StatefulSet
	for _, item := range items {
		unstructStateful, ok := item.(runtime.Unstructured)
		if !ok {
			panic("Controller: GetStatefulSetList: unexpected type for deployment")
		}
		stateful := new(appsV1.StatefulSet)
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructStateful.UnstructuredContent(), stateful); err != nil {
			continue
		}
		statefulsets = append(statefulsets, *stateful)
	}
	return statefulsets, nil
}

func (c *Controller) GetJobList(ctx context.Context) ([]batchV1.Job, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.jobInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var jobs []batchV1.Job
	for _, item := range items {
		unstructJob, ok := item.(runtime.Unstructured)
		if !ok {
			panic("Controller: GetJobList: unexpected type for deployment")
		}
		job := new(batchV1.Job)
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructJob.UnstructuredContent(), job); err != nil {
			continue
		}
		jobs = append(jobs, *job)
	}
	return jobs, nil
}

func (c *Controller) GetCronJobList(ctx context.Context) ([]batchV1.CronJob, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	items, err := c.cronJobInformer.Lister().List(labels.Everything())
	if err != nil {
		return nil, err
	}
	var cronjobs []batchV1.CronJob
	for _, item := range items {
		unstructJob, ok := item.(runtime.Unstructured)
		if !ok {
			panic("Controller: GetCronJobList: unexpected type for deployment")
		}
		job := new(batchV1.CronJob)
		if err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructJob.UnstructuredContent(), job); err != nil {
			continue
		}
		cronjobs = append(cronjobs, *job)
	}
	return cronjobs, nil
}
