package context

import (
	"context"

	"github.com/vladimirvivien/ktop/client"
)

type contextKey int

const (
	keyK8sClient contextKey = iota
)

func WithK8sClient(ctx context.Context, client client.K8sClient) context.Context {
	return context.WithValue(ctx, keyK8sClient, client)
}

func K8sClient(ctx context.Context) (client.K8sClient, bool) {
	result, ok := ctx.Value(keyK8sClient).(client.K8sClient)
	return result, ok
}
