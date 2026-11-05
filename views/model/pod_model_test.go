package model

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestGetPodContainerSummary(t *testing.T) {
	tests := []struct {
		name          string
		pod           *v1.Pod
		wantMemBytes  int64
		wantCPUMilli  int64
		wantPorts     int
		wantVolMounts int
	}{
		{
			name: "regular containers only",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "app",
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("128Mi"),
									v1.ResourceCPU:    resource.MustParse("250m"),
								},
							},
							Ports:        []v1.ContainerPort{{ContainerPort: 8080}, {ContainerPort: 9090}},
							VolumeMounts: []v1.VolumeMount{{Name: "config"}},
						},
					},
				},
			},
			wantMemBytes:  128 * 1024 * 1024,
			wantCPUMilli:  250,
			wantPorts:     2,
			wantVolMounts: 1,
		},
		{
			name: "containers + initContainers + overhead keeps CPU and memory separate",
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "app",
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("256Mi"),
									v1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
							Ports:        []v1.ContainerPort{{ContainerPort: 8080}},
							VolumeMounts: []v1.VolumeMount{{Name: "data"}},
						},
					},
					InitContainers: []v1.Container{
						{
							Name: "init-db",
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceMemory: resource.MustParse("64Mi"),
									v1.ResourceCPU:    resource.MustParse("200m"),
								},
							},
							Ports:        []v1.ContainerPort{{ContainerPort: 5432}},
							VolumeMounts: []v1.VolumeMount{{Name: "init-vol"}},
						},
					},
					Overhead: v1.ResourceList{
						v1.ResourceMemory: resource.MustParse("32Mi"),
						v1.ResourceCPU:    resource.MustParse("100m"),
					},
				},
			},
			wantMemBytes:  (256 + 64 + 32) * 1024 * 1024,
			wantCPUMilli:  500 + 200 + 100,
			wantPorts:     2,
			wantVolMounts: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GetPodContainerSummary(tc.pod)
			if got.RequestedMemQty.Value() != tc.wantMemBytes {
				t.Errorf("RequestedMemQty.Value() = %d, want %d", got.RequestedMemQty.Value(), tc.wantMemBytes)
			}
			if got.RequestedCpuQty.MilliValue() != tc.wantCPUMilli {
				t.Errorf("RequestedCpuQty.MilliValue() = %d, want %d", got.RequestedCpuQty.MilliValue(), tc.wantCPUMilli)
			}
			if got.Ports != tc.wantPorts {
				t.Errorf("Ports = %d, want %d", got.Ports, tc.wantPorts)
			}
			if got.VolMounts != tc.wantVolMounts {
				t.Errorf("VolMounts = %d, want %d", got.VolMounts, tc.wantVolMounts)
			}
		})
	}
}
