package model

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewEventModel_LegacyAndSeriesNormalization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	legacy := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "pod-a.123",
			UID:       "uid-1",
		},
		Type:           corev1.EventTypeWarning,
		Reason:         "FailedScheduling",
		Message:        "  0/3 nodes are available  ",
		Count:          4,
		FirstTimestamp: metav1.NewTime(now.Add(-10 * time.Minute)),
		LastTimestamp:  metav1.NewTime(now.Add(-1 * time.Minute)),
		Source:         corev1.EventSource{Component: "default-scheduler"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: "default", Name: "api-0"},
	}

	gotLegacy := NewEventModel(legacy)
	if !gotLegacy.IsWarning() || gotLegacy.Count != 4 || gotLegacy.ObjectRef() != "Pod/api-0" {
		t.Fatalf("unexpected legacy model: %+v", gotLegacy)
	}
	if gotLegacy.Message != "0/3 nodes are available" {
		t.Fatalf("expected trimmed message, got %q", gotLegacy.Message)
	}
	if gotLegacy.Source != "default-scheduler" {
		t.Fatalf("expected source default-scheduler, got %q", gotLegacy.Source)
	}
	if !gotLegacy.LastSeen.Equal(now.Add(-1 * time.Minute)) {
		t.Fatalf("unexpected LastSeen: %v", gotLegacy.LastSeen)
	}

	seriesEvt := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "prod",
			Name:              "pod-b.456",
			CreationTimestamp: metav1.NewTime(now.Add(-5 * time.Minute)),
		},
		Type:                corev1.EventTypeWarning,
		Reason:              "BackOff",
		Message:             "Back-off restarting failed container",
		EventTime:           metav1.NewMicroTime(now.Add(-4 * time.Minute)),
		Series:              &corev1.EventSeries{Count: 12, LastObservedTime: metav1.NewMicroTime(now)},
		ReportingController: "kubelet",
		ReportingInstance:   "worker-1",
		InvolvedObject:      corev1.ObjectReference{Kind: "Pod", Namespace: "prod", Name: "worker-0"},
	}

	gotSeries := NewEventModel(seriesEvt)
	if gotSeries.Count != 12 {
		t.Fatalf("expected series count 12, got %d", gotSeries.Count)
	}
	if !gotSeries.LastSeen.Equal(now) {
		t.Fatalf("expected Series.LastObservedTime %v, got %v", now, gotSeries.LastSeen)
	}
	if !gotSeries.FirstSeen.Equal(now.Add(-4 * time.Minute)) {
		t.Fatalf("expected EventTime as FirstSeen, got %v", gotSeries.FirstSeen)
	}
	if gotSeries.Source != "kubelet (worker-1)" {
		t.Fatalf("expected reporting controller/instance, got %q", gotSeries.Source)
	}
}

func TestComputeEventsSummary_PrefersWarningReason(t *testing.T) {
	items := []EventModel{
		{Type: corev1.EventTypeNormal, Reason: "Scheduled"},
		{Type: corev1.EventTypeNormal, Reason: "Scheduled"},
		{Type: corev1.EventTypeNormal, Reason: "Scheduled"},
		{Type: corev1.EventTypeWarning, Reason: "BackOff"},
		{Type: corev1.EventTypeWarning, Reason: "BackOff"},
		{Type: corev1.EventTypeWarning, Reason: "Unhealthy"},
	}

	summary := ComputeEventsSummary(items)
	if summary.Total != 6 || summary.WarningCount != 3 || summary.NormalCount != 3 {
		t.Fatalf("unexpected totals: %+v", summary)
	}
	if summary.TopReason != "BackOff" || summary.TopReasonCount != 2 {
		t.Fatalf("expected top warning reason BackOff (2), got %q (%d)", summary.TopReason, summary.TopReasonCount)
	}
}

func TestSortEventModelsBy(t *testing.T) {
	now := time.Now().UTC()
	items := []EventModel{
		{Name: "e1", Type: "Normal", Reason: "Pulled", Count: 1, LastSeen: now.Add(-5 * time.Minute)},
		{Name: "e2", Type: "Warning", Reason: "BackOff", Count: 9, LastSeen: now.Add(-1 * time.Minute)},
		{Name: "e3", Type: "Warning", Reason: "Failed", Count: 3, LastSeen: now.Add(-3 * time.Minute)},
	}

	SortEventModelsBy(items, "LAST SEEN", false)
	if items[0].Name != "e2" || items[1].Name != "e3" || items[2].Name != "e1" {
		t.Fatalf("expected newest-first order [e2 e3 e1], got [%s %s %s]", items[0].Name, items[1].Name, items[2].Name)
	}

	SortEventModelsBy(items, "COUNT", false)
	if items[0].Name != "e2" || items[1].Name != "e3" || items[2].Name != "e1" {
		t.Fatalf("expected count-desc order [e2 e3 e1], got [%s %s %s]", items[0].Name, items[1].Name, items[2].Name)
	}

	SortEventModelsBy(items, "REASON", true)
	if items[0].Reason != "BackOff" || items[1].Reason != "Failed" || items[2].Reason != "Pulled" {
		t.Fatalf("expected reason-asc order, got [%s %s %s]", items[0].Reason, items[1].Reason, items[2].Reason)
	}
}
