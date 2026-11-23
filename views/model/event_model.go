package model

import (
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// EventModel is the normalized view model for a Kubernetes Event, unifying
// legacy core/v1 fields and converted events.k8s.io/v1 series/reporting fields.
type EventModel struct {
	UID               string
	Namespace         string
	Name              string
	Type              string
	Reason            string
	Message           string
	InvolvedKind      string
	InvolvedNamespace string
	InvolvedName      string
	Source            string
	Count             int32
	FirstSeen         time.Time
	LastSeen          time.Time
	Raw               *corev1.Event
}

// EventsSummary aggregates event counts and the most frequent reason across a stream.
type EventsSummary struct {
	Total          int
	WarningCount   int
	NormalCount    int
	TopReason      string
	TopReasonCount int
}

// EventsSnapshot holds summary counts and a bounded, sorted slice of recent events.
type EventsSnapshot struct {
	Summary   EventsSummary
	Events    []EventModel
	Truncated bool
}

// ObjectRef returns a compact "Kind/Name" identifier for the involved object.
func (e EventModel) ObjectRef() string {
	switch {
	case e.InvolvedKind != "" && e.InvolvedName != "":
		return e.InvolvedKind + "/" + e.InvolvedName
	case e.InvolvedName != "":
		return e.InvolvedName
	default:
		return e.InvolvedKind
	}
}

// IsWarning returns true when the event has Type "Warning".
func (e EventModel) IsWarning() bool {
	return strings.EqualFold(e.Type, corev1.EventTypeWarning)
}

// EffectiveNamespace returns the involved object's namespace when present,
// falling back to the event object's own namespace.
func (e EventModel) EffectiveNamespace() string {
	if e.InvolvedNamespace != "" {
		return e.InvolvedNamespace
	}
	return e.Namespace
}

// EventLastTimestamp returns the most accurate last-observed timestamp for an Event,
// preferring events.k8s.io/v1 Series.LastObservedTime, then LastTimestamp, EventTime,
// and finally CreationTimestamp.
func EventLastTimestamp(evt *corev1.Event) time.Time {
	if evt == nil {
		return time.Time{}
	}
	if evt.Series != nil && !evt.Series.LastObservedTime.IsZero() {
		return evt.Series.LastObservedTime.Time
	}
	if !evt.LastTimestamp.IsZero() {
		return evt.LastTimestamp.Time
	}
	if !evt.EventTime.IsZero() {
		return evt.EventTime.Time
	}
	return evt.CreationTimestamp.Time
}

// EventFirstTimestamp returns the earliest observed timestamp for an Event.
func EventFirstTimestamp(evt *corev1.Event) time.Time {
	if evt == nil {
		return time.Time{}
	}
	if !evt.FirstTimestamp.IsZero() {
		return evt.FirstTimestamp.Time
	}
	if !evt.EventTime.IsZero() {
		return evt.EventTime.Time
	}
	return evt.CreationTimestamp.Time
}

// EventCount returns the normalized occurrence count for an Event (minimum 1).
func EventCount(evt *corev1.Event) int32 {
	if evt == nil {
		return 0
	}
	if evt.Series != nil && evt.Series.Count > 0 {
		return evt.Series.Count
	}
	if evt.Count > 0 {
		return evt.Count
	}
	return 1
}

// EventSource returns a readable reporting controller/component label.
func EventSource(evt *corev1.Event) string {
	if evt == nil {
		return ""
	}
	comp := strings.TrimSpace(evt.Source.Component)
	host := strings.TrimSpace(evt.Source.Host)
	if comp == "" {
		comp = strings.TrimSpace(evt.ReportingController)
	}
	if host == "" {
		host = strings.TrimSpace(evt.ReportingInstance)
	}
	switch {
	case comp != "" && host != "" && host != comp:
		return fmt.Sprintf("%s (%s)", comp, host)
	case comp != "":
		return comp
	default:
		return host
	}
}

// NewEventModel constructs a normalized EventModel from a core/v1 Event.
func NewEventModel(evt *corev1.Event) EventModel {
	if evt == nil {
		return EventModel{}
	}
	evType := evt.Type
	if evType == "" {
		evType = corev1.EventTypeNormal
	}
	return EventModel{
		UID:               string(evt.UID),
		Namespace:         evt.Namespace,
		Name:              evt.Name,
		Type:              evType,
		Reason:            evt.Reason,
		Message:           strings.TrimSpace(evt.Message),
		InvolvedKind:      evt.InvolvedObject.Kind,
		InvolvedNamespace: evt.InvolvedObject.Namespace,
		InvolvedName:      evt.InvolvedObject.Name,
		Source:            EventSource(evt),
		Count:             EventCount(evt),
		FirstSeen:         EventFirstTimestamp(evt),
		LastSeen:          EventLastTimestamp(evt),
		Raw:               evt.DeepCopy(),
	}
}

// ComputeEventsSummary aggregates totals and the top firing reason across events.
// When warnings exist, TopReason reports the most frequent Warning reason; otherwise
// it reports the most frequent reason overall.
func ComputeEventsSummary(events []EventModel) EventsSummary {
	var summary EventsSummary
	summary.Total = len(events)
	allReasons := make(map[string]int)
	warnReasons := make(map[string]int)

	for _, e := range events {
		if e.IsWarning() {
			summary.WarningCount++
			if e.Reason != "" {
				warnReasons[e.Reason]++
			}
		} else {
			summary.NormalCount++
		}
		if e.Reason != "" {
			allReasons[e.Reason]++
		}
	}

	sourceMap := allReasons
	if len(warnReasons) > 0 {
		sourceMap = warnReasons
	}
	for reason, count := range sourceMap {
		if count > summary.TopReasonCount || (count == summary.TopReasonCount && reason < summary.TopReason) {
			summary.TopReason = reason
			summary.TopReasonCount = count
		}
	}
	return summary
}

// SortEventModelsBy sorts events stably by the specified column header name.
func SortEventModelsBy(items []EventModel, col string, asc bool) {
	col = strings.ToUpper(strings.TrimSpace(col))
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		var less bool
		var eq bool

		switch col {
		case "TYPE":
			less, eq = a.Type < b.Type, a.Type == b.Type
		case "REASON":
			less, eq = a.Reason < b.Reason, a.Reason == b.Reason
		case "OBJECT":
			oa, ob := a.ObjectRef(), b.ObjectRef()
			less, eq = oa < ob, oa == ob
		case "NS", "NAMESPACE":
			na, nb := a.EffectiveNamespace(), b.EffectiveNamespace()
			less, eq = na < nb, na == nb
		case "COUNT":
			less, eq = a.Count < b.Count, a.Count == b.Count
		case "MESSAGE":
			less, eq = a.Message < b.Message, a.Message == b.Message
		default: // "LAST SEEN"
			less = a.LastSeen.Before(b.LastSeen)
			eq = a.LastSeen.Equal(b.LastSeen)
		}

		if eq {
			if !a.LastSeen.Equal(b.LastSeen) {
				return a.LastSeen.After(b.LastSeen)
			}
			if a.Namespace != b.Namespace {
				return a.Namespace < b.Namespace
			}
			return a.Name < b.Name
		}
		if asc {
			return less
		}
		return !less
	})
}
