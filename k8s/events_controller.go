package k8s

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/vladimirvivien/ktop/views/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

const (
	// DefaultTUIEventsLimit bounds the number of events returned for TUI rendering.
	DefaultTUIEventsLimit = 500
	// DefaultSnapshotEventsLimit bounds the number of events embedded in --noui snapshots.
	DefaultSnapshotEventsLimit = 200
)

func (c *Controller) notifyEventsChanged() {
	if c.eventsNotifyCh == nil {
		return
	}
	select {
	case c.eventsNotifyCh <- struct{}{}:
	default:
	}
}

func (c *Controller) setupEventsHandler(ctx context.Context, handlerFunc RefreshEventsFunc) {
	if handlerFunc == nil {
		return
	}

	if c.eventInformer != nil {
		_, _ = c.eventInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    func(any) { c.notifyEventsChanged() },
			UpdateFunc: func(any, any) { c.notifyEventsChanged() },
			DeleteFunc: func(any) { c.notifyEventsChanged() },
		})
	}

	refresh := func() {
		snap, err := c.GetEventsSnapshot(ctx, AllNamespaces, DefaultTUIEventsLimit)
		if err != nil {
			if ctx.Err() == nil {
				slog.Debug("events refresh skipped", "error", err)
			}
			return
		}
		if err := handlerFunc(ctx, snap); err != nil && ctx.Err() == nil {
			slog.Debug("events refresh handler failed", "error", err)
		}
	}

	go func() {
		refresh()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh()
			case <-c.eventsNotifyCh:
				refresh()
			}
		}
	}()
}

// GetEventsSnapshot reads recent Kubernetes Events from the cached informer lister,
// computes summary metrics across all matched events, sorts newest-first, and bounds
// the returned list to limit (when limit > 0).
func (c *Controller) GetEventsSnapshot(ctx context.Context, namespace string, limit int) (model.EventsSnapshot, error) {
	if ctx != nil && ctx.Err() != nil {
		return model.EventsSnapshot{}, ctx.Err()
	}
	if c.eventInformer == nil {
		return model.EventsSnapshot{}, nil
	}

	var (
		raw []*corev1.Event
		err error
	)
	if namespace == "" || namespace == AllNamespaces {
		raw, err = c.eventInformer.Lister().List(labels.Everything())
	} else {
		raw, err = c.eventInformer.Lister().Events(namespace).List(labels.Everything())
	}
	if err != nil {
		return model.EventsSnapshot{}, err
	}

	items := make([]model.EventModel, 0, len(raw))
	for _, evt := range raw {
		if evt == nil {
			continue
		}
		items = append(items, model.NewEventModel(evt))
	}

	summary := model.ComputeEventsSummary(items)
	model.SortEventModelsBy(items, "LAST SEEN", false)

	truncated := false
	if limit > 0 && len(items) > limit {
		items = items[:limit]
		truncated = true
	}

	return model.EventsSnapshot{
		Summary:   summary,
		Events:    items,
		Truncated: truncated,
	}, nil
}

// HasObject reports whether an inspectable Kubernetes object currently exists in
// the controller's cached informers.
func (c *Controller) HasObject(kind, namespace, name string) bool {
	if name == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "pod":
		if c.podInformer == nil {
			return false
		}
		obj, err := c.podInformer.Lister().Pods(namespace).Get(name)
		return err == nil && obj != nil
	case "node":
		if c.nodeInformer == nil {
			return false
		}
		obj, err := c.nodeInformer.Lister().Get(name)
		return err == nil && obj != nil
	case "deployment":
		if c.deploymentInformer == nil {
			return false
		}
		obj, err := c.deploymentInformer.Lister().Deployments(namespace).Get(name)
		return err == nil && obj != nil
	case "statefulset":
		if c.statefulSetInformer == nil {
			return false
		}
		obj, err := c.statefulSetInformer.Lister().StatefulSets(namespace).Get(name)
		return err == nil && obj != nil
	case "daemonset":
		if c.daemonSetInformer == nil {
			return false
		}
		obj, err := c.daemonSetInformer.Lister().DaemonSets(namespace).Get(name)
		return err == nil && obj != nil
	case "job":
		if c.jobInformer == nil {
			return false
		}
		obj, err := c.jobInformer.Lister().Jobs(namespace).Get(name)
		return err == nil && obj != nil
	default:
		return false
	}
}
