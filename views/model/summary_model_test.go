package model

import "testing"

func TestCalculateHealthScore(t *testing.T) {
	tests := []struct {
		name       string
		summary    ClusterSummary
		hasMetrics bool
		want       int
	}{
		{
			name: "healthy cluster with metrics",
			summary: ClusterSummary{
				NodesCount: 3,
				NodesReady: 3,
			},
			hasMetrics: true,
			want:       100,
		},
		{
			name: "missing metrics deducts 5",
			summary: ClusterSummary{
				NodesCount: 3,
				NodesReady: 3,
			},
			hasMetrics: false,
			want:       95,
		},
		{
			name: "one unready node deducts 30",
			summary: ClusterSummary{
				NodesCount: 3,
				NodesReady: 2,
			},
			hasMetrics: true,
			want:       70,
		},
		{
			name: "failed pods deduct 15",
			summary: ClusterSummary{
				NodesCount: 2,
				NodesReady: 2,
				FailedPods: 2,
			},
			hasMetrics: true,
			want:       85,
		},
		{
			name: "psi saturation stall over 20pct deducts 10",
			summary: ClusterSummary{
				NodesCount:        2,
				NodesReady:        2,
				AvgNodeIOStallPct: 24.5,
			},
			hasMetrics: true,
			want:       90,
		},
		{
			name: "node pressure deducts 5 per node",
			summary: ClusterSummary{
				NodesCount:        3,
				NodesReady:        3,
				NodePressureCount: 2,
			},
			hasMetrics: true,
			want:       90,
		},
		{
			name: "severely degraded cluster clamps at 0",
			summary: ClusterSummary{
				NodesCount:        4,
				NodesReady:        0,
				FailedPods:        3,
				NodePressureCount: 2,
				AvgNodeCPUStallPct: 30,
			},
			hasMetrics: false,
			want:       0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CalculateHealthScore(tc.summary, tc.hasMetrics)
			if got != tc.want {
				t.Errorf("CalculateHealthScore() = %d, want %d", got, tc.want)
			}
		})
	}
}
