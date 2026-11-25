package snapshot

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/views/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder_StorageAndNetworkJoinCloneAndJSON(t *testing.T) {
	now := metav1.NewTime(time.Unix(1700000000, 0).UTC())
	in := BuildInput{
		Pods: []model.PodModel{
			{Namespace: "prod", Name: "db-0", Status: "Running"},
		},
		PodVolumeUsages: map[string][]PodVolumeUsageData{
			"prod/db-0": {
				{
					PVCName:       "pg-data",
					UsedBytes:     900,
					CapacityBytes: 1000,
					FillPct:       90.0,
					HasUsage:      true,
				},
			},
		},
		PodNetworkErrors: map[string]metrics.NetworkErrorRates{
			"prod/db-0": {
				RxDroppedPerSec: 1.5,
				TxDroppedPerSec: 0.5,
				RxErrorsPerSec:  0.2,
				TxErrorsPerSec:  0.3,
			},
		},
		Storage: &model.StorageSnapshot{
			Summary: model.StorageSummary{
				PVCsBound:        1,
				PVCsTotal:        1,
				PVsBound:         1,
				PVsTotal:         1,
				StorageClasses:   1,
				HighFillCount:    1,
				TotalCapacity:    1000,
				TotalUsed:        900,
				DiskReadRateBps:  512,
				DiskWriteRateBps: 1024,
			},
			PVCs: []model.PVCModel{
				{
					Namespace:      "prod",
					Name:           "pg-data",
					Status:         "Bound",
					StorageClass:   "fast-ssd",
					VolumeName:     "pv-1",
					AccessModes:    []string{"RWO"},
					CapacityBytes:  1000,
					UsedBytes:      900,
					AvailableBytes: 100,
					FillPct:        90.0,
					InodesTotal:    100,
					InodesUsed:     40,
					InodesUsedPct:  40.0,
					HasUsage:       true,
					HasInodes:      true,
					CreationTime:   now,
					Age:            "2h",
				},
			},
			PVs: []model.PVModel{
				{
					Name:          "pv-1",
					Status:        "Bound",
					CapacityBytes: 1000,
					AccessModes:   []string{"RWO"},
					ReclaimPolicy: "Retain",
					StorageClass:  "fast-ssd",
					ClaimRef:      "prod/pg-data",
					CreationTime:  now,
					Age:           "2h",
				},
			},
			StorageClasses: []model.StorageClassModel{
				{
					Name:        "fast-ssd",
					Provisioner: "ebs.csi.aws.com",
					ReclaimPol:  "Delete",
					BindingMode: "WaitForFirstConsumer",
					IsDefault:   true,
				},
			},
		},
		Network: &model.NetworkSnapshot{
			Summary: model.NetworkSummary{
				ServicesTotal:        1,
				ZeroEndpointServices: 1,
				IngressesTotal:       1,
				ClusterRxRateBps:     2048,
				ClusterTxRateBps:     4096,
				DropsPerSec:          2.0,
				ErrorsPerSec:         0.5,
				HasMetrics:           true,
			},
			Services: []model.ServiceModel{
				{
					Namespace:      "prod",
					Name:           "api",
					Type:           "ClusterIP",
					ClusterIP:      "10.0.0.10",
					Ports:          "80/TCP",
					ReadyEndpoints: 0,
					TotalEndpoints: 2,
					HasEndpoints:   true,
					CreationTime:   now,
					Age:            "1h",
				},
			},
			Ingresses: []model.IngressModel{
				{
					Namespace:    "prod",
					Name:         "edge",
					ClassName:    "nginx",
					Hosts:        []string{"api.example.com"},
					Address:      "192.0.2.10",
					Ports:        "80, 443",
					CreationTime: now,
					Age:          "1h",
				},
			},
			NodeNetwork: []model.NodeNetworkModel{
				{
					NodeName:     "node-1",
					RxRateBps:    2048,
					TxRateBps:    4096,
					RxDroppedSec: 1.5,
					TxDroppedSec: 0.5,
					RxErrorsSec:  0.2,
					TxErrorsSec:  0.3,
				},
			},
		},
	}

	snap, err := NewBuilder().Build(context.Background(), in)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if snap.Storage == nil || len(snap.Storage.PVCs) != 1 || len(snap.Storage.PVs) != 1 || len(snap.Storage.StorageClasses) != 1 {
		t.Fatalf("unexpected Storage: %+v", snap.Storage)
	}
	if snap.Network == nil || len(snap.Network.Services) != 1 || !snap.Network.Services[0].HasEndpointAlert || len(snap.Network.Ingresses) != 1 {
		t.Fatalf("unexpected Network: %+v", snap.Network)
	}
	if len(snap.Pods) != 1 || len(snap.Pods[0].VolumeUsages) != 1 || snap.Pods[0].NetworkDropsPerSec != 2.0 || snap.Pods[0].NetworkErrorsPerSec != 0.5 {
		t.Fatalf("unexpected pod storage/network enrichments: %+v", snap.Pods)
	}

	cloned := snap.Clone()
	cloned.Pods[0].VolumeUsages[0].PVCName = "mutated"
	cloned.Storage.PVCs[0].AccessModes[0] = "mutated"
	cloned.Storage.PVs[0].AccessModes[0] = "mutated"
	cloned.Storage.StorageClasses[0].Name = "mutated"
	cloned.Network.Services[0].Name = "mutated"
	cloned.Network.Ingresses[0].Hosts[0] = "mutated"
	cloned.Network.NodeNetwork[0].NodeName = "mutated"

	if snap.Pods[0].VolumeUsages[0].PVCName != "pg-data" ||
		snap.Storage.PVCs[0].AccessModes[0] != "RWO" ||
		snap.Storage.PVs[0].AccessModes[0] != "RWO" ||
		snap.Storage.StorageClasses[0].Name != "fast-ssd" ||
		snap.Network.Services[0].Name != "api" ||
		snap.Network.Ingresses[0].Hosts[0] != "api.example.com" ||
		snap.Network.NodeNetwork[0].NodeName != "node-1" {
		t.Fatal("Clone aliased Storage/Network/PodVolumeUsage slices")
	}

	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	var decoded Snapshot
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if decoded.Storage == nil || decoded.Network == nil || len(decoded.Pods[0].VolumeUsages) != 1 || decoded.Pods[0].NetworkDropsPerSec != 2.0 {
		t.Fatalf("JSON round-trip lost Storage/Network fields: %+v", decoded)
	}
}
