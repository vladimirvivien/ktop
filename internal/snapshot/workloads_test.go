package snapshot

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/vladimirvivien/ktop/views/model"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestBuilder_WorkloadsSnapshotRoundTripAndJSON(t *testing.T) {
	b := NewBuilder()
	ws := &model.WorkloadsSnapshot{
		HasMetrics: true,
		Deployments: []model.DeploymentModel{
			{
				Namespace: "prod",
				Name:      "api",
				Desired:   3,
				Ready:     3,
				Status:    "Ready",
				Usage: model.WorkloadUsage{
					CPU:    resource.NewMilliQuantity(450, resource.DecimalSI),
					Mem:    resource.NewQuantity(256*1024*1024, resource.BinarySI),
					HasCPU: true,
					HasMem: true,
				},
			},
		},
		Jobs: []model.JobModel{
			{Namespace: "batch", Name: "nightly", Succeeded: 1, Status: "Complete"},
		},
	}

	snap, err := b.Build(context.Background(), BuildInput{Workloads: ws})
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if snap.Workloads == nil || len(snap.Workloads.Deployments) != 1 || len(snap.Workloads.Jobs) != 1 {
		t.Fatalf("unexpected snap.Workloads: %+v", snap.Workloads)
	}
	if snap.Workloads.Deployments[0].UsageCPUMillis != 450 {
		t.Fatalf("UsageCPUMillis = %d, want 450", snap.Workloads.Deployments[0].UsageCPUMillis)
	}

	// Verify Clone deep-copies workloads slices.
	cloned := snap.Clone()
	cloned.Workloads.Deployments[0].Name = "mutated"
	if snap.Workloads.Deployments[0].Name != "api" {
		t.Fatalf("Clone() aliased Workloads.Deployments slice")
	}

	// Verify round-trip back to model.WorkloadsSnapshot and JSON serialization.
	back := snap.Workloads.ToWorkloadsSnapshot()
	if len(back.Deployments) != 1 || back.Deployments[0].Usage.CPU.MilliValue() != 450 {
		t.Fatalf("ToWorkloadsSnapshot() mismatch: %+v", back.Deployments)
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	var decoded Snapshot
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	if decoded.Workloads == nil || len(decoded.Workloads.Deployments) != 1 {
		t.Fatalf("JSON round-trip lost workloads: %+v", decoded.Workloads)
	}
}
