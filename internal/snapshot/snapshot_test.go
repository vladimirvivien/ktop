package snapshot

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSnapshotJSONSerializationAndProjections(t *testing.T) {
	ts := time.Date(2026, 11, 13, 20, 0, 0, 0, time.UTC)
	snap := &Snapshot{
		Timestamp: ts,
		Cluster: ClusterInfo{
			Context:       "prod-east",
			MetricsSource: "prometheus",
			Ready:         true,
		},
		Summary: SummaryData{
			HealthScore: 95,
			NodesReady:  2,
			NodesCount:  2,
		},
		Nodes: []NodeData{
			{
				Name:               "node-1",
				Roles:              []string{"control-plane"},
				Status:             "Ready",
				PSIKernelSupported: true,
			},
		},
		Pods: []PodData{
			{
				Namespace:       "default",
				Name:            "api-0",
				Node:            "node-1",
				Status:          "Running",
				CPUThrottledPct: 4.5,
				Containers: []ContainerData{
					{Name: "api", UsageCPUMillis: 250, CPUThrottledPct: 4.5},
				},
			},
			{
				Namespace: "kube-system",
				Name:      "coredns-0",
				Node:      "node-2",
				Status:    "Running",
			},
		},
	}

	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	for _, wantKey := range []string{`"health_score":95`, `"metrics_source":"prometheus"`, `"cpu_throttled_pct":4.5`} {
		if !strings.Contains(string(raw), wantKey) {
			t.Errorf("JSON missing %s: %s", wantKey, string(raw))
		}
	}

	cloned := snap.Clone()
	cloned.Nodes[0].Roles[0] = "mutated"
	cloned.Pods[0].Containers[0].Name = "mutated"
	if snap.Nodes[0].Roles[0] != "control-plane" {
		t.Errorf("Clone aliased Nodes slice")
	}
	if snap.Pods[0].Containers[0].Name != "api" {
		t.Errorf("Clone aliased Containers slice")
	}

	if n, ok := snap.NodeByName("node-1"); !ok || n.Status != "Ready" {
		t.Errorf("NodeByName(node-1) = %+v, %v", n, ok)
	}
	if p, ok := snap.PodByKey("default", "api-0"); !ok || p.CPUThrottledPct != 4.5 {
		t.Errorf("PodByKey(default, api-0) = %+v, %v", p, ok)
	}
	if onNode := snap.PodsOnNode("node-1"); len(onNode) != 1 || onNode[0].Name != "api-0" {
		t.Errorf("PodsOnNode(node-1) = %+v", onNode)
	}
}
