package snapshot

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/views/model"
)

func TestBuilder_EventsRoundTripAndClone(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	in := BuildInput{
		Events: &model.EventsSnapshot{
			Summary: model.EventsSummary{
				Total:          3,
				WarningCount:   2,
				NormalCount:    1,
				TopReason:      "BackOff",
				TopReasonCount: 2,
			},
			Truncated: true,
			Events: []model.EventModel{
				{
					UID:               "uid-1",
					Namespace:         "default",
					Name:              "api-0.1",
					Type:              "Warning",
					Reason:            "BackOff",
					Message:           "Back-off restarting failed container",
					InvolvedKind:      "Pod",
					InvolvedNamespace: "default",
					InvolvedName:      "api-0",
					Source:            "kubelet",
					Count:             5,
					FirstSeen:         now.Add(-5 * time.Minute),
					LastSeen:          now,
				},
			},
		},
	}

	snap, err := NewBuilder().Build(context.Background(), in)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if snap.Events == nil || len(snap.Events.Items) != 1 || !snap.Events.Truncated {
		t.Fatalf("unexpected snap.Events: %+v", snap.Events)
	}
	if snap.Events.Summary.WarningCount != 2 || snap.Events.Summary.TopReason != "BackOff" {
		t.Fatalf("unexpected summary: %+v", snap.Events.Summary)
	}

	cloned := snap.Clone()
	cloned.Events.Items[0].Reason = "Mutated"
	if snap.Events.Items[0].Reason != "BackOff" {
		t.Fatal("Clone aliased Events.Items slice")
	}

	roundTrip := snap.Events.ToEventsSnapshot()
	if roundTrip.Summary.Total != 3 || len(roundTrip.Events) != 1 || roundTrip.Events[0].ObjectRef() != "Pod/api-0" {
		t.Fatalf("unexpected ToEventsSnapshot result: %+v", roundTrip)
	}

	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decoded Snapshot
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded.Events == nil || decoded.Events.Summary.TopReason != "BackOff" || len(decoded.Events.Items) != 1 {
		t.Fatalf("JSON round-trip lost events: %+v", decoded.Events)
	}
}
