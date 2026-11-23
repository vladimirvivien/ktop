package snapshot

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/vladimirvivien/ktop/metrics/ksm"
	"github.com/vladimirvivien/ktop/metrics/sli"
	"github.com/vladimirvivien/ktop/views/model"
)

func TestBuilder_KSMAndSLIJoinCloneAndJSON(t *testing.T) {
	limited := true
	in := BuildInput{
		Summary: model.ClusterSummary{
			SLIAvailable:           true,
			SLIUnhealthyComponents: 1,
			SLIFailingChecks:       []string{"apiserver/etcd"},
			ProbeFailingPods:       1,
			KSMAvailable:           true,
			KSMHPAsLimited:         1,
			KSMPDBsAtRisk:          1,
			KSMQuotasNearLimit:     1,
		},
		Pods: []model.PodModel{
			{Namespace: "prod", Name: "api-0", Status: "Running"},
		},
		KSM: &ksm.Snapshot{
			Available: true,
			Endpoint:  "service-proxy:monitoring/kube-state-metrics:8080",
			HPAs: []ksm.HPAMetrics{
				{Namespace: "prod", Name: "api-hpa", TargetKind: "Deployment", TargetName: "api", MinReplicas: 2, MaxReplicas: 10, CurrentReplicas: 10, DesiredReplicas: 10, ScalingLimited: &limited},
			},
			PDBs: []ksm.PDBMetrics{
				{Namespace: "prod", Name: "api-pdb", CurrentHealthy: 2, DesiredHealthy: 2, ExpectedPods: 2, DisruptionsAllowed: 0},
			},
			ResourceQuotas: []ksm.ResourceQuotaMetrics{
				{Namespace: "prod", Name: "compute", Resource: "requests.cpu", Hard: 10, Used: 9, UsedPct: 90},
			},
			PodDisruptions: map[string]string{
				"prod/api-0": "PreemptionByScheduler",
			},
			PodResourceClaims: map[string][]ksm.ResourceClaimRef{
				"prod/api-0": {{Name: "gpu", ResourceClaimName: "gpu-claim-0"}},
			},
		},
		SLI: &sli.Snapshot{
			Available:       true,
			ProbesAvailable: true,
			Components: []sli.ComponentSLI{
				{Component: "apiserver", Target: "apiserver", Healthy: false, TotalChecks: 2, FailingChecks: []sli.HealthCheck{{Name: "etcd", Type: "readyz", Healthy: false}}},
			},
			PodProbes: map[string][]sli.ProbeStats{
				"prod/api-0": {{Namespace: "prod", Pod: "api-0", Container: "app", ProbeType: "Readiness", SuccessCount: 20, FailureCount: 3, Failing: true}},
			},
		},
	}

	snap, err := NewBuilder().Build(context.Background(), in)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if snap.Governance == nil || len(snap.Governance.HPAs) != 1 || !snap.Governance.HPAs[0].ScalingLimited {
		t.Fatalf("unexpected Governance: %+v", snap.Governance)
	}
	if snap.SLIs == nil || len(snap.SLIs.Components) != 1 || snap.SLIs.Components[0].Healthy {
		t.Fatalf("unexpected SLIs: %+v", snap.SLIs)
	}
	if len(snap.Pods) != 1 || snap.Pods[0].DisruptionReason != "PreemptionByScheduler" || len(snap.Pods[0].ResourceClaims) != 1 || len(snap.Pods[0].FailingProbes) != 1 {
		t.Fatalf("unexpected enriched PodData: %+v", snap.Pods)
	}

	cloned := snap.Clone()
	cloned.Governance.HPAs[0].Name = "mutated"
	cloned.SLIs.Components[0].FailingChecks[0] = "mutated"
	cloned.Pods[0].ResourceClaims[0] = "mutated"
	if snap.Governance.HPAs[0].Name != "api-hpa" || snap.SLIs.Components[0].FailingChecks[0] != "etcd" || snap.Pods[0].ResourceClaims[0] != "gpu-claim-0" {
		t.Fatal("Clone aliased Governance/SLIs/Pod slices")
	}

	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decoded Snapshot
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded.Governance == nil || decoded.SLIs == nil || decoded.Pods[0].DisruptionReason != "PreemptionByScheduler" {
		t.Fatalf("JSON round-trip lost KSM/SLI fields: %+v", decoded)
	}
}
