package model

import (
	"strings"

	"github.com/vladimirvivien/ktop/metrics"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PVCModel represents a normalized PersistentVolumeClaim with optional kubelet volume usage stats.
type PVCModel struct {
	Namespace      string
	Name           string
	Status         string
	StorageClass   string
	VolumeName     string
	AccessModes    []string
	CapacityBytes  int64
	UsedBytes      int64
	AvailableBytes int64
	FillPct        float64
	InodesTotal    int64
	InodesUsed     int64
	InodesUsedPct  float64
	HasUsage       bool
	HasInodes      bool
	CreationTime   metav1.Time
	Age            string
	Raw            *corev1.PersistentVolumeClaim
}

// PVModel represents a normalized PersistentVolume.
type PVModel struct {
	Name          string
	Status        string
	CapacityBytes int64
	AccessModes   []string
	ReclaimPolicy string
	StorageClass  string
	ClaimRef      string
	CreationTime  metav1.Time
	Age           string
	Raw           *corev1.PersistentVolume
}

// StorageClassModel represents a normalized storage.k8s.io/v1 StorageClass.
type StorageClassModel struct {
	Name        string
	Provisioner string
	ReclaimPol  string
	BindingMode string
	IsDefault   bool
}

// StorageSummary aggregates cluster-wide PVC, PV, StorageClass, fill alert, and Disk I/O telemetry.
type StorageSummary struct {
	PVCsBound        int
	PVCsPending      int
	PVCsTotal        int
	PVsBound         int
	PVsAvailable     int
	PVsReleased      int
	PVsTotal         int
	StorageClasses   int
	HighFillCount    int
	TotalCapacity    int64
	TotalUsed        int64
	DiskReadRateBps  float64
	DiskWriteRateBps float64
}

// StorageSnapshot holds a complete point-in-time view of cluster storage objects and volume stats.
type StorageSnapshot struct {
	Summary        StorageSummary
	PVCs           []PVCModel
	PVs            []PVModel
	StorageClasses []StorageClassModel
}

// NewPVCModel builds a normalized PVCModel from a Kubernetes PVC and optional kubelet volume stats.
func NewPVCModel(pvc *corev1.PersistentVolumeClaim, stats *metrics.VolumeStats) PVCModel {
	if pvc == nil {
		return PVCModel{}
	}
	sc := ""
	if pvc.Spec.StorageClassName != nil {
		sc = *pvc.Spec.StorageClassName
	}
	var capBytes int64
	if q, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
		capBytes = q.Value()
	} else if pvc.Spec.Resources.Requests != nil {
		if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			capBytes = q.Value()
		}
	}

	m := PVCModel{
		Namespace:     pvc.Namespace,
		Name:          pvc.Name,
		Status:        string(pvc.Status.Phase),
		StorageClass:  sc,
		VolumeName:    pvc.Spec.VolumeName,
		AccessModes:   abbreviateAccessModes(pvc.Spec.AccessModes),
		CapacityBytes: capBytes,
		CreationTime:  pvc.CreationTimestamp,
		Age:           strings.TrimSpace(timeSince(pvc.CreationTimestamp)),
		Raw:           pvc,
	}

	if stats != nil {
		if stats.CapacityBytes > 0 {
			m.CapacityBytes = stats.CapacityBytes
		}
		if stats.HasBytes {
			m.UsedBytes = stats.UsedBytes
			m.AvailableBytes = stats.AvailableBytes
			m.FillPct = stats.UsedPct
			if m.CapacityBytes > 0 && m.FillPct == 0 && stats.UsedBytes > 0 {
				m.FillPct = (float64(stats.UsedBytes) / float64(m.CapacityBytes)) * 100
			}
			m.HasUsage = true
		}
		if stats.HasInodes {
			m.InodesTotal = stats.InodesTotal
			m.InodesUsed = stats.InodesUsed
			m.InodesUsedPct = stats.InodesUsedPct
			m.HasInodes = true
		}
	}
	return m
}

// NewPVModel builds a normalized PVModel from a Kubernetes PersistentVolume.
func NewPVModel(pv *corev1.PersistentVolume) PVModel {
	if pv == nil {
		return PVModel{}
	}
	var capBytes int64
	if q, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
		capBytes = q.Value()
	}
	claim := ""
	if pv.Spec.ClaimRef != nil {
		if pv.Spec.ClaimRef.Namespace != "" {
			claim = pv.Spec.ClaimRef.Namespace + "/" + pv.Spec.ClaimRef.Name
		} else {
			claim = pv.Spec.ClaimRef.Name
		}
	}
	return PVModel{
		Name:          pv.Name,
		Status:        string(pv.Status.Phase),
		CapacityBytes: capBytes,
		AccessModes:   abbreviateAccessModes(pv.Spec.AccessModes),
		ReclaimPolicy: string(pv.Spec.PersistentVolumeReclaimPolicy),
		StorageClass:  pv.Spec.StorageClassName,
		ClaimRef:      claim,
		CreationTime:  pv.CreationTimestamp,
		Age:           strings.TrimSpace(timeSince(pv.CreationTimestamp)),
		Raw:           pv,
	}
}

// NewStorageClassModel builds a normalized StorageClassModel from a storage/v1 StorageClass.
func NewStorageClassModel(sc *storagev1.StorageClass) StorageClassModel {
	if sc == nil {
		return StorageClassModel{}
	}
	reclaim := ""
	if sc.ReclaimPolicy != nil {
		reclaim = string(*sc.ReclaimPolicy)
	}
	binding := ""
	if sc.VolumeBindingMode != nil {
		binding = string(*sc.VolumeBindingMode)
	}
	isDefault := false
	if sc.Annotations != nil {
		isDefault = sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
			sc.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true"
	}
	return StorageClassModel{
		Name:        sc.Name,
		Provisioner: sc.Provisioner,
		ReclaimPol:  reclaim,
		BindingMode: binding,
		IsDefault:   isDefault,
	}
}

// ComputeStorageSummary aggregates storage counts, high-fill warnings, and cluster disk rates.
func ComputeStorageSummary(pvcs []PVCModel, pvs []PVModel, scs []StorageClassModel, diskReadBps, diskWriteBps float64) StorageSummary {
	summary := StorageSummary{
		PVCsTotal:        len(pvcs),
		PVsTotal:         len(pvs),
		StorageClasses:   len(scs),
		DiskReadRateBps:  diskReadBps,
		DiskWriteRateBps: diskWriteBps,
	}
	for _, pvc := range pvcs {
		switch pvc.Status {
		case string(corev1.ClaimBound):
			summary.PVCsBound++
		case string(corev1.ClaimPending):
			summary.PVCsPending++
		}
		summary.TotalCapacity += pvc.CapacityBytes
		if pvc.HasUsage {
			summary.TotalUsed += pvc.UsedBytes
			if pvc.FillPct >= 85.0 {
				summary.HighFillCount++
			}
		}
	}
	for _, pv := range pvs {
		switch pv.Status {
		case string(corev1.VolumeBound):
			summary.PVsBound++
		case string(corev1.VolumeAvailable):
			summary.PVsAvailable++
		case string(corev1.VolumeReleased):
			summary.PVsReleased++
		}
	}
	return summary
}

// SortPVCModelsBy sorts PVC models in-place by column name and direction.
func SortPVCModelsBy(items []PVCModel, column string, ascending bool) {
	sortSliceBy(items, ascending, func(a, b PVCModel) bool {
		switch column {
		case "NAME":
			return compareNamespaced(a.Name, a.Namespace, b.Name, b.Namespace)
		case "STATUS":
			if a.Status == b.Status {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.Status < b.Status
		case "CAPACITY":
			if a.CapacityBytes == b.CapacityBytes {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.CapacityBytes < b.CapacityBytes
		case "USED":
			if a.HasUsage != b.HasUsage {
				return !a.HasUsage
			}
			if a.UsedBytes == b.UsedBytes {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.UsedBytes < b.UsedBytes
		case "FILL%":
			if a.HasUsage != b.HasUsage {
				return !a.HasUsage
			}
			if a.FillPct == b.FillPct {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.FillPct < b.FillPct
		case "INODES%":
			if a.HasInodes != b.HasInodes {
				return !a.HasInodes
			}
			if a.InodesUsedPct == b.InodesUsedPct {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.InodesUsedPct < b.InodesUsedPct
		case "CLASS":
			if a.StorageClass == b.StorageClass {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.StorageClass < b.StorageClass
		case "VOLUME":
			if a.VolumeName == b.VolumeName {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.VolumeName < b.VolumeName
		case "AGE":
			if a.CreationTime.Equal(&b.CreationTime) {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.CreationTime.Before(&b.CreationTime)
		default: // NS
			return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
		}
	})
}

// SortPVModelsBy sorts PV models in-place by column name and direction.
func SortPVModelsBy(items []PVModel, column string, ascending bool) {
	sortSliceBy(items, ascending, func(a, b PVModel) bool {
		switch column {
		case "STATUS":
			if a.Status == b.Status {
				return a.Name < b.Name
			}
			return a.Status < b.Status
		case "CAPACITY":
			if a.CapacityBytes == b.CapacityBytes {
				return a.Name < b.Name
			}
			return a.CapacityBytes < b.CapacityBytes
		case "ACCESS":
			amA, amB := strings.Join(a.AccessModes, ","), strings.Join(b.AccessModes, ",")
			if amA == amB {
				return a.Name < b.Name
			}
			return amA < amB
		case "RECLAIM":
			if a.ReclaimPolicy == b.ReclaimPolicy {
				return a.Name < b.Name
			}
			return a.ReclaimPolicy < b.ReclaimPolicy
		case "CLASS":
			if a.StorageClass == b.StorageClass {
				return a.Name < b.Name
			}
			return a.StorageClass < b.StorageClass
		case "CLAIM":
			if a.ClaimRef == b.ClaimRef {
				return a.Name < b.Name
			}
			return a.ClaimRef < b.ClaimRef
		case "AGE":
			if a.CreationTime.Equal(&b.CreationTime) {
				return a.Name < b.Name
			}
			return a.CreationTime.Before(&b.CreationTime)
		default: // NAME
			return a.Name < b.Name
		}
	})
}

func abbreviateAccessModes(modes []corev1.PersistentVolumeAccessMode) []string {
	if len(modes) == 0 {
		return nil
	}
	out := make([]string, 0, len(modes))
	for _, m := range modes {
		switch m {
		case corev1.ReadWriteOnce:
			out = append(out, "RWO")
		case corev1.ReadOnlyMany:
			out = append(out, "ROX")
		case corev1.ReadWriteMany:
			out = append(out, "RWX")
		case corev1.ReadWriteOncePod:
			out = append(out, "RWOP")
		default:
			out = append(out, string(m))
		}
	}
	return out
}
