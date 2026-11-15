package k8s

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	coreV1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
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

func readyTestNode(name string) *coreV1.Node {
	return &coreV1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: coreV1.NodeStatus{
			Conditions: []coreV1.NodeCondition{
				{Type: coreV1.NodeReady, Status: coreV1.ConditionTrue},
			},
		},
	}
}

func TestBuildClusterSummary_OOMAndCrashLoopHealthScore(t *testing.T) {
	ns := &coreV1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	node := readyTestNode("node-1")

	oomCurrentPod := &coreV1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "oom-current", Namespace: "default"},
		Status: coreV1.PodStatus{
			Phase: coreV1.PodRunning,
			InitContainerStatuses: []coreV1.ContainerStatus{
				{
					Name:         "init-mem",
					RestartCount: 1,
					State: coreV1.ContainerState{
						Terminated: &coreV1.ContainerStateTerminated{Reason: "OOMKilled"},
					},
				},
			},
		},
	}

	crashAndLastOOMPod := &coreV1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-crash", Namespace: "default"},
		Status: coreV1.PodStatus{
			Phase: coreV1.PodRunning,
			ContainerStatuses: []coreV1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 4,
					State: coreV1.ContainerState{
						Waiting: &coreV1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					},
					LastTerminationState: coreV1.ContainerState{
						Terminated: &coreV1.ContainerStateTerminated{Reason: "OOMKilled"},
					},
				},
			},
		},
	}

	fakeClient := fake.NewSimpleClientset(ns, node, oomCurrentPod, crashAndLastOOMPod)
	client := NewWithClient(fakeClient, AllNamespaces, "test-ctx", &version.Info{Major: "1", Minor: "36"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Controller().StartInformers(ctx, time.Second, time.Second); err != nil {
		t.Fatalf("StartInformers() unexpected error: %v", err)
	}

	summary, err := client.Controller().BuildClusterSummary(ctx)
	if err != nil {
		t.Fatalf("BuildClusterSummary() unexpected error: %v", err)
	}

	if summary.OOMKillCount != 2 {
		t.Errorf("OOMKillCount = %d, want 2", summary.OOMKillCount)
	}
	if summary.CrashLoopPods != 1 {
		t.Errorf("CrashLoopPods = %d, want 1", summary.CrashLoopPods)
	}
	if summary.ContainerRestarts != 5 {
		t.Errorf("ContainerRestarts = %d, want 5", summary.ContainerRestarts)
	}
	// 100 - 15 (OOM/CrashLoop) - 5 (no metrics source) = 80
	if summary.HealthScore != 80 {
		t.Errorf("HealthScore = %d, want 80", summary.HealthScore)
	}
}

func TestStartInformers(t *testing.T) {
	t.Run("namespace-scoped succeeds even when cluster informers are forbidden", func(t *testing.T) {
		pod := &coreV1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-0", Namespace: "team-a"},
			Status:     coreV1.PodStatus{Phase: coreV1.PodRunning},
		}
		fakeClient := fake.NewSimpleClientset(pod)
		for _, res := range []string{"namespaces", "nodes", "persistentvolumes"} {
			resourceName := res
			fakeClient.PrependReactor("list", resourceName, func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: resourceName}, "", nil)
			})
		}

		client := NewWithClient(fakeClient, "team-a", "ctx", nil)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := client.Controller().StartInformers(ctx, time.Second, 200*time.Millisecond); err != nil {
			t.Fatalf("StartInformers() unexpected error: %v", err)
		}

		pods, err := client.Controller().GetPodList(ctx)
		if err != nil || len(pods) != 1 {
			t.Fatalf("GetPodList() = (%d pods, %v), want (1 pod, nil)", len(pods), err)
		}
	})

	t.Run("core sync timeout returns DeadlineExceeded", func(t *testing.T) {
		fakeClient := fake.NewSimpleClientset()
		fakeClient.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", nil)
		})

		client := NewWithClient(fakeClient, AllNamespaces, "ctx", nil)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := client.Controller().StartInformers(ctx, time.Second, 100*time.Millisecond)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("StartInformers() error = %v, want context.DeadlineExceeded", err)
		}
	})
}
