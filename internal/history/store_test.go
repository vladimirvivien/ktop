package history

import (
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/internal/snapshot"
)

func TestStore_PushWindowAndExtract(t *testing.T) {
	store := NewStore(5)
	base := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 7; i++ {
		store.PushSnapshot(&snapshot.Snapshot{
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			Summary: snapshot.SummaryData{
				AllocatableCPUMillis:   4000,
				UsageCPUMillis:         int64((i + 1) * 400),
				AllocatableMemoryBytes: 8000,
				UsageMemoryBytes:       int64((i + 1) * 500),
				NetworkRxRateBytesPerS: float64(i * 100),
			},
			Nodes: []snapshot.NodeData{
				{Name: "worker-1", AllocatableCPUMillis: 2000, UsageCPUMillis: int64((i + 1) * 200)},
				{Name: "worker-2", AllocatableCPUMillis: 2000, UsageCPUMillis: int64((i + 1) * 100)},
			},
		})
	}

	if store.Len() != 5 {
		t.Fatalf("Len() = %d, want 5 (ring eviction)", store.Len())
	}

	series := store.Extract(2*time.Minute, 2)
	if len(series.CPU) != 3 {
		t.Fatalf("expected 3 samples in trailing 2m window, got %d", len(series.CPU))
	}
	if series.PeakCPU < 69.9 || series.PeakCPU > 70.1 {
		t.Fatalf("PeakCPU = %.2f, want 70.0", series.PeakCPU)
	}
	if len(series.TopNodes) != 2 || series.TopNodes[0] != "worker-1" {
		t.Fatalf("unexpected TopNodes: %v", series.TopNodes)
	}
}
