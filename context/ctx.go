package context

import (
	"context"

	"k8s.io/client-go/kubernetes"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

type contextKey int

const (
	keyK8sInterface contextKey = iota
	keyMetricsInterface
	keyIsMetricsAvailable
	keyNamespace
)

func WithK8sInterface(ctx context.Context, k8s kubernetes.Interface) context.Context {
	return context.WithValue(ctx, keyK8sInterface, k8s)
}

func WithNamespace(ctx context.Context, ns string) context.Context {
	return context.WithValue(ctx, keyNamespace, ns)
}

func WithMetricsInterface(ctx context.Context, mi metricsclient.Interface) context.Context {
	return context.WithValue(ctx, keyMetricsInterface, mi)
}

func WithIsMetricsAvailable(ctx context.Context, avail bool) context.Context {
	return context.WithValue(ctx, keyIsMetricsAvailable, avail)
}

func Namespace(ctx context.Context) (string, bool) {
	result, ok := ctx.Value(keyNamespace).(string)
	return result, ok
}

func K8sInterface(ctx context.Context) (kubernetes.Interface, bool) {
	result, ok := ctx.Value(keyK8sInterface).(kubernetes.Interface)
	return result, ok
}

func MetricsInterface(ctx context.Context) (metricsclient.Interface, bool) {
	result, ok := ctx.Value(keyMetricsInterface).(metricsclient.Interface)
	return result, ok
}

func IsMetricsAvailable(ctx context.Context) (bool, bool) {
	result, ok := ctx.Value(keyIsMetricsAvailable).(bool)
	return result, ok
}
