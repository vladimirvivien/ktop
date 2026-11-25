package k8s

import (
	"context"
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type fakeStorageMetricsSource struct {
	fakeWorkloadsMetricsSource
	volStats   map[string]metrics.VolumeStats
	nodeMetric map[string]*metrics.NodeMetrics
}

func (f *fakeStorageMetricsSource) GetNodeMetrics(_ context.Context, name string) (*metrics.NodeMetrics, error) {
	return f.nodeMetric[name], nil
}

func (f *fakeStorageMetricsSource) GetAllVolumeStats(context.Context) (map[string]metrics.VolumeStats, error) {
	return f.volStats, nil
}

func TestController_GetStorageSnapshotAndRBACFallback(t *testing.T) {
	scName := "gp3"
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "db-data"},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &scName,
			VolumeName:       "pv-1",
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase:    corev1.ClaimBound,
			Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
		},
	}
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-1"},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:         corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			StorageClassName: scName,
			ClaimRef:         &corev1.ObjectReference{Namespace: "prod", Name: "db-data"},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	}
	sc := &storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: scName},
		Provisioner: "ebs.csi.aws.com",
	}
	node := readyTestNode("node-1")

	fakeClient := fake.NewSimpleClientset(pvc, pv, sc, node)
	client := NewWithClient(fakeClient, AllNamespaces, "ctx", &version.Info{GitVersion: "v1.35.0"})
	ctrl := client.Controller()
	ctrl.SetMetricsSource(&fakeStorageMetricsSource{
		volStats: map[string]metrics.VolumeStats{
			"prod/db-data": {
				Namespace:     "prod",
				PVCName:       "db-data",
				CapacityBytes: 1000,
				UsedBytes:     900,
				UsedPct:       90.0,
				InodesTotal:   1000,
				InodesUsed:    250,
				InodesUsedPct: 25.0,
				HasBytes:      true,
				HasInodes:     true,
			},
		},
		nodeMetric: map[string]*metrics.NodeMetrics{
			"node-1": {NodeName: "node-1", DiskReadRate: 2048, DiskWriteRate: 4096},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ctrl.StartInformers(ctx, time.Second, time.Second); err != nil {
		t.Fatalf("StartInformers: %v", err)
	}

	snap, err := ctrl.GetStorageSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetStorageSnapshot: %v", err)
	}
	if len(snap.PVCs) != 1 || len(snap.PVs) != 1 || len(snap.StorageClasses) != 1 {
		t.Fatalf("unexpected counts: pvcs=%d pvs=%d scs=%d", len(snap.PVCs), len(snap.PVs), len(snap.StorageClasses))
	}
	if !snap.PVCs[0].HasUsage || snap.PVCs[0].FillPct != 90.0 || snap.Summary.HighFillCount != 1 {
		t.Fatalf("expected joined high-fill volume stats, got pvc=%+v summary=%+v", snap.PVCs[0], snap.Summary)
	}
	if snap.Summary.DiskReadRateBps != 2048 || snap.Summary.DiskWriteRateBps != 4096 {
		t.Fatalf("unexpected disk rates: %+v", snap.Summary)
	}

	// Verify 403 Forbidden on StorageClasses degrades silently without error.
	forbidClient := fake.NewSimpleClientset(pvc, pv, node)
	forbidClient.PrependReactor("list", "storageclasses", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "storage.k8s.io", Resource: "storageclasses"}, "", nil)
	})
	c2 := NewWithClient(forbidClient, AllNamespaces, "ctx", nil)
	if err := c2.Controller().StartInformers(ctx, time.Second, time.Second); err != nil {
		t.Fatalf("StartInformers with forbidden StorageClass should succeed, got: %v", err)
	}
	snap2, err := c2.Controller().GetStorageSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetStorageSnapshot with nil StorageClass informer failed: %v", err)
	}
	if len(snap2.PVCs) != 1 || len(snap2.StorageClasses) != 0 {
		t.Fatalf("unexpected degraded snapshot: %+v", snap2)
	}
}
