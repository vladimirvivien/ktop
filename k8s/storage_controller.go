package k8s

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

func (c *Controller) notifyStorageChanged() {
	if c.storageNotifyCh == nil {
		return
	}
	select {
	case c.storageNotifyCh <- struct{}{}:
	default:
	}
}

func (c *Controller) setupStorageHandler(ctx context.Context, handlerFunc RefreshStorageFunc) {
	if handlerFunc == nil {
		return
	}

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { c.notifyStorageChanged() },
		UpdateFunc: func(any, any) { c.notifyStorageChanged() },
		DeleteFunc: func(any) { c.notifyStorageChanged() },
	}
	if c.pvcInformer != nil {
		_, _ = c.pvcInformer.Informer().AddEventHandler(handlers)
	}
	if c.pvInformer != nil {
		_, _ = c.pvInformer.Informer().AddEventHandler(handlers)
	}
	if c.storageClassInformer != nil {
		_, _ = c.storageClassInformer.Informer().AddEventHandler(handlers)
	}

	refresh := func() {
		snap, err := c.GetStorageSnapshot(ctx)
		if err != nil {
			if ctx.Err() == nil {
				slog.Debug("storage refresh skipped", "error", err)
			}
			return
		}
		if err := handlerFunc(ctx, snap); err != nil && ctx.Err() == nil {
			slog.Debug("storage refresh handler failed", "error", err)
		}
	}

	go func() {
		refresh()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			case <-c.storageNotifyCh:
				refresh()
			}
		}
	}()
}

// GetStorageSnapshot reads PVCs, PVs, and StorageClasses from informer caches
// and joins live kubelet volume usage statistics and cluster disk I/O rates when available.
func (c *Controller) GetStorageSnapshot(ctx context.Context) (model.StorageSnapshot, error) {
	if ctx != nil && ctx.Err() != nil {
		return model.StorageSnapshot{}, ctx.Err()
	}

	var volStats map[string]metrics.VolumeStats
	if vp, ok := c.metricsSource.(metrics.VolumeStatsProvider); ok {
		if stats, err := vp.GetAllVolumeStats(ctx); err == nil {
			volStats = stats
		}
	}

	var pvcs []model.PVCModel
	if c.pvcInformer != nil {
		rawPVCs, err := c.pvcInformer.Lister().List(labels.Everything())
		if err != nil {
			return model.StorageSnapshot{}, err
		}
		pvcs = make([]model.PVCModel, 0, len(rawPVCs))
		for _, pvc := range rawPVCs {
			if pvc == nil {
				continue
			}
			var stPtr *metrics.VolumeStats
			if volStats != nil {
				if st, ok := volStats[pvc.Namespace+"/"+pvc.Name]; ok {
					stCopy := st
					stPtr = &stCopy
				}
			}
			pvcs = append(pvcs, model.NewPVCModel(pvc, stPtr))
		}
	}
	model.SortPVCModelsBy(pvcs, "NS", true)

	var pvs []model.PVModel
	if c.pvInformer != nil {
		rawPVs, err := c.pvInformer.Lister().List(labels.Everything())
		if err == nil {
			pvs = make([]model.PVModel, 0, len(rawPVs))
			for _, pv := range rawPVs {
				if pv == nil {
					continue
				}
				pvs = append(pvs, model.NewPVModel(pv))
			}
		}
	}
	model.SortPVModelsBy(pvs, "NAME", true)

	var scs []model.StorageClassModel
	if c.storageClassInformer != nil {
		rawSCs, err := c.storageClassInformer.Lister().List(labels.Everything())
		if err == nil {
			scs = make([]model.StorageClassModel, 0, len(rawSCs))
			for _, sc := range rawSCs {
				if sc == nil {
					continue
				}
				scs = append(scs, model.NewStorageClassModel(sc))
			}
		}
	}
	slices.SortFunc(scs, func(a, b model.StorageClassModel) int {
		return strings.Compare(a.Name, b.Name)
	})

	var diskReadBps, diskWriteBps float64
	if c.metricsSource != nil && c.nodeInformer != nil {
		if nodes, err := c.nodeInformer.Lister().List(labels.Everything()); err == nil {
			for _, node := range nodes {
				if node == nil {
					continue
				}
				if nm, mErr := c.metricsSource.GetNodeMetrics(ctx, node.Name); mErr == nil && nm != nil {
					diskReadBps += nm.DiskReadRate
					diskWriteBps += nm.DiskWriteRate
				}
			}
		}
	}

	summary := model.ComputeStorageSummary(pvcs, pvs, scs, diskReadBps, diskWriteBps)
	return model.StorageSnapshot{
		Summary:        summary,
		PVCs:           pvcs,
		PVs:            pvs,
		StorageClasses: scs,
	}, nil
}
