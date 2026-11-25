package model

import (
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewPVCModelAndSummary(t *testing.T) {
	scName := "fast-ssd"
	now := metav1.NewTime(time.Now().Add(-2 * time.Hour))

	pvc1 := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "pg-data", CreationTimestamp: now},
		Spec: corev1.PersistentVolumeClaimSpec{
			StorageClassName: &scName,
			VolumeName:       "pv-1",
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce, corev1.ReadOnlyMany},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase:    corev1.ClaimBound,
			Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
		},
	}
	pvc2 := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "staging", Name: "cache-pvc", CreationTimestamp: now},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
	}

	stats := &metrics.VolumeStats{
		Namespace:     "prod",
		PVCName:       "pg-data",
		CapacityBytes: 1000,
		UsedBytes:     900,
		UsedPct:       90.0,
		InodesTotal:   1000,
		InodesUsed:    400,
		InodesUsedPct: 40.0,
		HasBytes:      true,
		HasInodes:     true,
	}

	m1 := NewPVCModel(pvc1, stats)
	if !m1.HasUsage || m1.FillPct != 90.0 || m1.InodesUsedPct != 40.0 {
		t.Fatalf("unexpected m1 usage: %+v", m1)
	}
	if len(m1.AccessModes) != 2 || m1.AccessModes[0] != "RWO" || m1.AccessModes[1] != "ROX" {
		t.Fatalf("unexpected access modes: %v", m1.AccessModes)
	}

	m2 := NewPVCModel(pvc2, nil)
	wantCap := resource.MustParse("5Gi")
	if m2.HasUsage || m2.CapacityBytes != wantCap.Value() {
		t.Fatalf("unexpected pending m2: %+v", m2)
	}

	pv1 := NewPVModel(&corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv-1", CreationTimestamp: now},
		Spec: corev1.PersistentVolumeSpec{
			Capacity:                      corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOncePod},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			StorageClassName:              scName,
			ClaimRef:                      &corev1.ObjectReference{Namespace: "prod", Name: "pg-data"},
		},
		Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
	})
	if pv1.ClaimRef != "prod/pg-data" || pv1.AccessModes[0] != "RWOP" {
		t.Fatalf("unexpected PV model: %+v", pv1)
	}

	sc := NewStorageClassModel(&storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: scName, Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"}},
		Provisioner: "kubernetes.io/csi",
	})
	if !sc.IsDefault {
		t.Error("expected default StorageClass")
	}

	summary := ComputeStorageSummary([]PVCModel{m1, m2}, []PVModel{pv1}, []StorageClassModel{sc}, 1024, 2048)
	if summary.PVCsBound != 1 || summary.PVCsPending != 1 || summary.PVCsTotal != 2 {
		t.Errorf("unexpected PVC counts: %+v", summary)
	}
	if summary.HighFillCount != 1 || summary.StorageClasses != 1 || summary.DiskReadRateBps != 1024 {
		t.Errorf("unexpected summary fields: %+v", summary)
	}

	items := []PVCModel{m2, m1}
	SortPVCModelsBy(items, "FILL%", false)
	if items[0].Name != "pg-data" {
		t.Errorf("expected pg-data first by FILL%% desc, got %s", items[0].Name)
	}
}
