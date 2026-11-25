package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func sampleStorageSnapshot(now time.Time) model.StorageSnapshot {
	scName := "fast-ssd"
	pvcs := []model.PVCModel{
		model.NewPVCModel(&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "pg-data", CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour))},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: &scName,
				VolumeName:       "pv-prod-1",
				Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}},
			},
			Status: corev1.PersistentVolumeClaimStatus{
				Phase:    corev1.ClaimBound,
				Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
			},
		}, &metrics.VolumeStats{
			Namespace:     "prod",
			PVCName:       "pg-data",
			CapacityBytes: 1000,
			UsedBytes:     900,
			UsedPct:       90.0,
			InodesTotal:   100,
			InodesUsed:    50,
			InodesUsedPct: 50.0,
			HasBytes:      true,
			HasInodes:     true,
		}),
		model.NewPVCModel(&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Namespace: "staging", Name: "cache-pvc", CreationTimestamp: metav1.NewTime(now.Add(-30 * time.Minute))},
			Spec: corev1.PersistentVolumeClaimSpec{
				StorageClassName: &scName,
				Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("5Gi")}},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
		}, nil),
	}
	pvs := []model.PVModel{
		model.NewPVModel(&corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "pv-prod-1", CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour))},
			Spec: corev1.PersistentVolumeSpec{
				Capacity:                      corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")},
				AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
				StorageClassName:              scName,
				ClaimRef:                      &corev1.ObjectReference{Namespace: "prod", Name: "pg-data"},
			},
			Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeBound},
		}),
	}
	scs := []model.StorageClassModel{
		{Name: "fast-ssd", Provisioner: "ebs.csi.aws.com", IsDefault: true},
	}
	return model.StorageSnapshot{
		Summary:        model.ComputeStorageSummary(pvcs, pvs, scs, 1024, 2048),
		PVCs:           pvcs,
		PVs:            pvs,
		StorageClasses: scs,
	}
}

func TestStoragePage_RenderFilterAndYAMLModal(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	app := application.New(nil, nil)
	page := New(app, "Storage")
	app.RegisterPage(application.PageDescriptor{
		ID:         "storage",
		Title:      "Storage",
		Key:        '1',
		RootType:   application.PageStorage,
		Panel:      page,
		ChildNames: []string{"pvcs", "pvs"},
	})
	if err := app.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	snap := sampleStorageSnapshot(time.Now().UTC())
	page.applySnapshot(snap)

	ts, err := ui.NewTestScreen(160, 40)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(app.RootPrimitive())
	text := ts.Text()
	for _, want := range []string{
		"PVCs: 1/2 bound",
		"1 pending",
		"1 >=85%",
		"PVs: 1/1 bound",
		"Classes: 1",
		"pg-data",
		"90%",
		"cache-pvc",
		"n/a",
		"pv-prod-1",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q on Storage screen, got:\n%s", want, text)
		}
	}

	// Verify namespace filter scopes both PVCs and claim-bound PVs.
	page.SetNamespaceFilter("staging")
	if len(page.pvcs.displayed) != 1 || page.pvcs.displayed[0].Name != "cache-pvc" {
		t.Fatalf("expected staging PVC only, got %+v", page.pvcs.displayed)
	}
	if len(page.pvs.displayed) != 0 {
		t.Fatalf("expected 0 PVs matching staging claim, got %+v", page.pvs.displayed)
	}
	page.SetNamespaceFilter("")

	// Verify '/' row filter scopes PVCs table.
	dispatchKey := func(p tview.Primitive, ev *tcell.EventKey) {
		if p == nil {
			return
		}
		if h := p.InputHandler(); h != nil {
			h(ev, func(next tview.Primitive) {
				if next != nil {
					app.Focus(next)
				}
			})
		}
	}
	app.Focus(page.pvcs.table)
	dispatchKey(page.pvcs.table, tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone))
	for _, ch := range "pg-data" {
		dispatchKey(page.pvcs.table, tcell.NewEventKey(tcell.KeyRune, ch, tcell.ModNone))
	}
	dispatchKey(page.pvcs.table, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if len(page.pvcs.displayed) != 1 || page.pvcs.displayed[0].Name != "pg-data" {
		t.Fatalf("expected row filter 'pg-data' to leave 1 PVC, got %+v", page.pvcs.displayed)
	}
	page.HandleEscape()

	// Verify Enter opens YAML modal for selected PVC and Esc closes it returning focus.
	page.pvcs.table.Select(1, 0)
	dispatchKey(page.pvcs.table, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	ts.Draw(app.RootPrimitive())
	if !strings.Contains(ts.Text(), "PVC YAML: prod/pg-data") {
		t.Fatalf("expected PVC YAML modal to open, got:\n%s", ts.Text())
	}
	dispatchKey(app.FocusedPrimitive(), tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	ts.Draw(app.RootPrimitive())
	if strings.Contains(ts.Text(), "PVC YAML: prod/pg-data") {
		t.Fatalf("expected PVC YAML modal closed on Esc, got:\n%s", ts.Text())
	}
}
