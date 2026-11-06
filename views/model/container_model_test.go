package model

import (
	"testing"
)

func TestSortContainerInfosBy(t *testing.T) {
	containers := []ContainerInfo{
		{
			Name:         "sidecar",
			Image:        "docker.io/envoyproxy/envoy:v1.28",
			State:        "Running",
			Ready:        true,
			RestartCount: 2,
			CPUUsage:     "150m",
			MemoryUsage:  "256Mi",
		},
		{
			Name:         "app",
			Image:        "quay.io/myorg/api:v2.1.0",
			State:        "Running",
			Ready:        true,
			RestartCount: 0,
			CPUUsage:     "500m",
			MemoryUsage:  "1Gi",
		},
		{
			Name:         "init",
			Image:        "busybox:latest",
			State:        "Terminated",
			Ready:        false,
			RestartCount: 5,
			CPUUsage:     "10m",
			MemoryUsage:  "64Mi",
		},
	}

	t.Run("sort by Name ascending", func(t *testing.T) {
		sorted := make([]ContainerInfo, len(containers))
		copy(sorted, containers)
		SortContainerInfosBy(sorted, "NAME", true)
		if sorted[0].Name != "app" || sorted[1].Name != "init" || sorted[2].Name != "sidecar" {
			t.Fatalf("unexpected order: %v, %v, %v", sorted[0].Name, sorted[1].Name, sorted[2].Name)
		}
	})

	t.Run("sort by Name descending", func(t *testing.T) {
		sorted := make([]ContainerInfo, len(containers))
		copy(sorted, containers)
		SortContainerInfosBy(sorted, "NAME", false)
		if sorted[0].Name != "sidecar" || sorted[1].Name != "init" || sorted[2].Name != "app" {
			t.Fatalf("unexpected order: %v, %v, %v", sorted[0].Name, sorted[1].Name, sorted[2].Name)
		}
	})

	t.Run("sort by Restarts ascending", func(t *testing.T) {
		sorted := make([]ContainerInfo, len(containers))
		copy(sorted, containers)
		SortContainerInfosBy(sorted, "RESTARTS", true)
		if sorted[0].Name != "app" || sorted[1].Name != "sidecar" || sorted[2].Name != "init" {
			t.Fatalf("unexpected order: %v, %v, %v", sorted[0].Name, sorted[1].Name, sorted[2].Name)
		}
	})

	t.Run("sort by CPU descending", func(t *testing.T) {
		sorted := make([]ContainerInfo, len(containers))
		copy(sorted, containers)
		SortContainerInfosBy(sorted, "CPU", false)
		if sorted[0].Name != "app" || sorted[1].Name != "sidecar" || sorted[2].Name != "init" {
			t.Fatalf("unexpected order: %v, %v, %v", sorted[0].Name, sorted[1].Name, sorted[2].Name)
		}
	})

	t.Run("sort by Memory descending", func(t *testing.T) {
		sorted := make([]ContainerInfo, len(containers))
		copy(sorted, containers)
		SortContainerInfosBy(sorted, "MEM", false)
		if sorted[0].Name != "app" || sorted[1].Name != "sidecar" || sorted[2].Name != "init" {
			t.Fatalf("unexpected order: %v, %v, %v", sorted[0].Name, sorted[1].Name, sorted[2].Name)
		}
	})
}
