package k8s

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
)

func TestController_GetEventsSnapshotAndHasObject(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	ev1 := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Namespace: "default", Name: "ev-1"},
		Type:           corev1.EventTypeNormal,
		Reason:         "Scheduled",
		Message:        "Successfully assigned default/api-0",
		LastTimestamp:  metav1.NewTime(now.Add(-3 * time.Minute)),
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "api-0"},
	}
	ev2 := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Namespace: "default", Name: "ev-2"},
		Type:           corev1.EventTypeWarning,
		Reason:         "BackOff",
		Message:        "Back-off restarting failed container",
		Series:         &corev1.EventSeries{Count: 5, LastObservedTime: metav1.NewMicroTime(now)},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "api-0"},
	}
	ev3 := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Namespace: "kube-system", Name: "ev-3"},
		Type:           corev1.EventTypeWarning,
		Reason:         "FailedMount",
		Message:        "MountVolume.SetUp failed",
		LastTimestamp:  metav1.NewTime(now.Add(-1 * time.Minute)),
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "kube-system", Name: "coredns-0"},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api-0"}}
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "api"}}

	fakeClient := fake.NewSimpleClientset(ev1, ev2, ev3, pod, dep)
	factory := informers.NewSharedInformerFactory(fakeClient, 0)
	ctrl := &Controller{
		client:             &Client{namespace: AllNamespaces, kubeClient: fakeClient},
		eventInformer:      factory.Core().V1().Events(),
		podInformer:        factory.Core().V1().Pods(),
		nodeInformer:       factory.Core().V1().Nodes(),
		deploymentInformer: factory.Apps().V1().Deployments(),
	}

	// Force informer registration before Start so WaitForCacheSync populates caches.
	_ = ctrl.eventInformer.Informer()
	_ = ctrl.podInformer.Informer()
	_ = ctrl.nodeInformer.Informer()
	_ = ctrl.deploymentInformer.Informer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	factory.Start(ctx.Done())
	factory.WaitForCacheSync(ctx.Done())

	snap, err := ctrl.GetEventsSnapshot(ctx, AllNamespaces, 2)
	if err != nil {
		t.Fatalf("GetEventsSnapshot failed: %v", err)
	}
	if snap.Summary.Total != 3 || snap.Summary.WarningCount != 2 || snap.Summary.NormalCount != 1 {
		t.Fatalf("unexpected summary: %+v", snap.Summary)
	}
	if !snap.Truncated || len(snap.Events) != 2 {
		t.Fatalf("expected truncated slice of length 2, got len=%d truncated=%v", len(snap.Events), snap.Truncated)
	}
	if snap.Events[0].Name != "ev-2" || snap.Events[1].Name != "ev-3" {
		t.Fatalf("expected newest-first [ev-2, ev-3], got [%s, %s]", snap.Events[0].Name, snap.Events[1].Name)
	}

	nsSnap, err := ctrl.GetEventsSnapshot(ctx, "kube-system", 10)
	if err != nil {
		t.Fatalf("namespace GetEventsSnapshot failed: %v", err)
	}
	if nsSnap.Summary.Total != 1 || len(nsSnap.Events) != 1 || nsSnap.Events[0].Name != "ev-3" {
		t.Fatalf("unexpected kube-system snapshot: %+v", nsSnap)
	}

	if !ctrl.HasObject("Pod", "default", "api-0") {
		t.Fatal("expected HasObject true for existing Pod")
	}
	if !ctrl.HasObject("Deployment", "default", "api") {
		t.Fatal("expected HasObject true for existing Deployment")
	}
	if ctrl.HasObject("Pod", "default", "deleted-pod") {
		t.Fatal("expected HasObject false for missing Pod")
	}
	if ctrl.HasObject("ConfigMap", "default", "cm") {
		t.Fatal("expected HasObject false for unsupported kind")
	}
}
