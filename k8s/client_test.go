package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestAssertCoreAuthz(t *testing.T) {
	tests := []struct {
		name             string
		namespace        string
		forbidNamespaces bool
		forbidNodes      bool
		forbidPods       bool
		wantErrContains  string
	}{
		{
			name:      "all resources authorized in cluster-wide mode",
			namespace: AllNamespaces,
		},
		{
			name:             "cluster-wide mode fails when namespaces forbidden",
			namespace:        AllNamespaces,
			forbidNamespaces: true,
			wantErrContains:  "user not authorized to list namespaces",
		},
		{
			name:            "cluster-wide mode fails when nodes forbidden",
			namespace:       AllNamespaces,
			forbidNodes:     true,
			wantErrContains: "user not authorized to list nodes",
		},
		{
			name:             "namespace-scoped mode succeeds when namespaces and nodes are forbidden but pods allowed",
			namespace:        "foobar",
			forbidNamespaces: true,
			forbidNodes:      true,
		},
		{
			name:             "namespace-scoped mode fails when pods in namespace are forbidden",
			namespace:        "foobar",
			forbidNamespaces: true,
			forbidNodes:      true,
			forbidPods:       true,
			wantErrContains:  "user not authorized to list pods",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := fake.NewSimpleClientset()

			if tc.forbidNamespaces {
				fakeClient.PrependReactor("list", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "namespaces"}, "", nil)
				})
			}
			if tc.forbidNodes {
				fakeClient.PrependReactor("list", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "nodes"}, "", nil)
				})
			}
			if tc.forbidPods {
				fakeClient.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", nil)
				})
			}

			client := &Client{
				namespace:  tc.namespace,
				kubeClient: fakeClient,
			}
			client.controller = newController(client)

			err := client.AssertCoreAuthz(context.Background())
			if tc.wantErrContains == "" {
				if err != nil {
					t.Fatalf("AssertCoreAuthz() unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Fatalf("AssertCoreAuthz() error = %v, want substring %q", err, tc.wantErrContains)
				}
			}
		})
	}
}

func TestControllerStart_NoPanicOnForbiddenNonCoreInformers(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	// Simulate 403 Forbidden on cluster-scoped PersistentVolumes and Deployments
	for _, res := range []string{"persistentvolumes", "namespaces", "nodes", "deployments"} {
		resourceName := res
		fakeClient.PrependReactor("list", resourceName, func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: resourceName}, "", nil)
		})
	}

	client := &Client{
		namespace:  "foobar",
		kubeClient: fakeClient,
	}
	client.controller = newController(client)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if err := client.Controller().Start(ctx, time.Second); err != nil {
		t.Fatalf("Controller.Start() unexpected error: %v", err)
	}

	// Wait for context expiration to ensure the background non-core WaitForCacheSync
	// goroutine exits cleanly without panicking.
	<-ctx.Done()
	time.Sleep(50 * time.Millisecond)
}
