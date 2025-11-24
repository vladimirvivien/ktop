package model

import (
	"fmt"
	"sort"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

type PodModel struct {
	Namespace    string
	Name         string
	Status       string
	Node         string
	IP           string
	TimeSince    string
	CreationTime metav1.Time

	PodRequestedCpuQty *resource.Quantity
	PodRequestedMemQty *resource.Quantity
	PodUsageCpuQty     *resource.Quantity
	PodUsageMemQty     *resource.Quantity

	NodeAllocatableCpuQty *resource.Quantity
	NodeAllocatableMemQty *resource.Quantity
	NodeUsageCpuQty       *resource.Quantity
	NodeUsageMemQty       *resource.Quantity

	ReadyContainers int
	TotalContainers int
	Restarts        int
	Volumes         int
	VolMounts       int
}

type PodContainerSummary struct {
	RequestedMemQty *resource.Quantity
	RequestedCpuQty *resource.Quantity
	VolMounts       int
	Ports           int
}

type ContainerStatusSummary struct {
	Ready       int
	Total       int
	Restarts    int
	Status      string
	SomeRunning bool
}

func SortPodModels(pods []PodModel) {
	SortPodModelsBy(pods, "NAMESPACE", true)
}

// SortPodModelsBy sorts pods by the specified column and direction
func SortPodModelsBy(pods []PodModel, column string, ascending bool) {
	var sortFunc func(i, j int) bool

	switch column {
	case "NAMESPACE":
		sortFunc = func(i, j int) bool {
			if pods[i].Namespace == pods[j].Namespace {
				return pods[i].Name < pods[j].Name
			}
			return pods[i].Namespace < pods[j].Namespace
		}

	case "POD":
		sortFunc = func(i, j int) bool {
			if pods[i].Name == pods[j].Name {
				return pods[i].Namespace < pods[j].Namespace
			}
			return pods[i].Name < pods[j].Name
		}

	case "READY":
		sortFunc = func(i, j int) bool {
			readyI := float64(pods[i].ReadyContainers) / float64(pods[i].TotalContainers)
			readyJ := float64(pods[j].ReadyContainers) / float64(pods[j].TotalContainers)
			if readyI == readyJ {
				return pods[i].Name < pods[j].Name
			}
			return readyI < readyJ
		}

	case "STATUS":
		sortFunc = func(i, j int) bool {
			// Running > Pending > Failed > Unknown
			statusPriority := map[string]int{
				"Running":           0,
				"Pending":           1,
				"ContainerCreating": 2,
				"CrashLoopBackOff":  3,
				"Error":             4,
				"Failed":            5,
				"Unknown":           6,
			}

			priI, okI := statusPriority[pods[i].Status]
			priJ, okJ := statusPriority[pods[j].Status]

			// Unknown statuses get highest priority (show first for investigation)
			if !okI {
				priI = 99
			}
			if !okJ {
				priJ = 99
			}

			if priI == priJ {
				return pods[i].Name < pods[j].Name
			}
			return priI < priJ
		}

	case "RESTARTS":
		sortFunc = func(i, j int) bool {
			if pods[i].Restarts == pods[j].Restarts {
				return pods[i].Name < pods[j].Name
			}
			return pods[i].Restarts < pods[j].Restarts
		}

	case "AGE":
		sortFunc = func(i, j int) bool {
			// Older pods first (earlier creation time)
			if pods[i].CreationTime.Equal(&pods[j].CreationTime) {
				return pods[i].Name < pods[j].Name
			}
			return pods[i].CreationTime.Before(&pods[j].CreationTime)
		}

	case "VOLS":
		sortFunc = func(i, j int) bool {
			if pods[i].Volumes == pods[j].Volumes {
				if pods[i].VolMounts == pods[j].VolMounts {
					return pods[i].Name < pods[j].Name
				}
				return pods[i].VolMounts < pods[j].VolMounts
			}
			return pods[i].Volumes < pods[j].Volumes
		}

	case "IP":
		sortFunc = func(i, j int) bool {
			if pods[i].IP == pods[j].IP {
				return pods[i].Name < pods[j].Name
			}
			return pods[i].IP < pods[j].IP
		}

	case "NODE":
		sortFunc = func(i, j int) bool {
			if pods[i].Node == pods[j].Node {
				return pods[i].Name < pods[j].Name
			}
			return pods[i].Node < pods[j].Node
		}

	case "CPU":
		sortFunc = func(i, j int) bool {
			cpuI := int64(0)
			cpuJ := int64(0)

			// Prefer usage metrics, fall back to requested if unavailable
			if pods[i].PodUsageCpuQty != nil && pods[i].PodUsageCpuQty.MilliValue() > 0 {
				cpuI = pods[i].PodUsageCpuQty.MilliValue()
			} else if pods[i].PodRequestedCpuQty != nil {
				cpuI = pods[i].PodRequestedCpuQty.MilliValue()
			}

			if pods[j].PodUsageCpuQty != nil && pods[j].PodUsageCpuQty.MilliValue() > 0 {
				cpuJ = pods[j].PodUsageCpuQty.MilliValue()
			} else if pods[j].PodRequestedCpuQty != nil {
				cpuJ = pods[j].PodRequestedCpuQty.MilliValue()
			}

			if cpuI == cpuJ {
				return pods[i].Name < pods[j].Name
			}
			return cpuI < cpuJ
		}

	case "MEMORY":
		sortFunc = func(i, j int) bool {
			memI := int64(0)
			memJ := int64(0)

			// Prefer usage metrics, fall back to requested if unavailable
			if pods[i].PodUsageMemQty != nil && pods[i].PodUsageMemQty.Value() > 0 {
				memI = pods[i].PodUsageMemQty.Value()
			} else if pods[i].PodRequestedMemQty != nil {
				memI = pods[i].PodRequestedMemQty.Value()
			}

			if pods[j].PodUsageMemQty != nil && pods[j].PodUsageMemQty.Value() > 0 {
				memJ = pods[j].PodUsageMemQty.Value()
			} else if pods[j].PodRequestedMemQty != nil {
				memJ = pods[j].PodRequestedMemQty.Value()
			}

			if memI == memJ {
				return pods[i].Name < pods[j].Name
			}
			return memI < memJ
		}

	default:
		// Default to NAMESPACE then NAME sorting
		sortFunc = func(i, j int) bool {
			if pods[i].Namespace == pods[j].Namespace {
				return pods[i].Name < pods[j].Name
			}
			return pods[i].Namespace < pods[j].Namespace
		}
	}

	// Apply sort
	if ascending {
		sort.Slice(pods, sortFunc)
	} else {
		// Reverse for descending
		sort.Slice(pods, func(i, j int) bool {
			return !sortFunc(i, j)
		})
	}
}

func NewPodModel(pod *v1.Pod, podMetrics *metricsV1beta1.PodMetrics, nodeMetrics *metricsV1beta1.NodeMetrics) *PodModel {
	totalCpu, totalMem := podMetricsTotals(podMetrics)
	statusSummary := getContainerStatusSummary(pod.Status.ContainerStatuses)
	if (statusSummary.Status == "" || statusSummary.Status == "Completed") && statusSummary.SomeRunning {
		if podIsReady(pod.Status.Conditions) {
			statusSummary.Status = "Running"
		} else {
			statusSummary.Status = "NotReady"
		}
	}
	containerSummary := GetPodContainerSummary(pod)
	return &PodModel{
		Namespace:          pod.GetNamespace(),
		Name:               pod.Name,
		Status:             statusSummary.Status,
		TimeSince:          timeSince(pod.CreationTimestamp),
		CreationTime:       pod.CreationTimestamp,
		IP:                 pod.Status.PodIP,
		Node:               pod.Spec.NodeName,
		Volumes:            len(pod.Spec.Volumes),
		VolMounts:          containerSummary.VolMounts,
		PodRequestedMemQty: containerSummary.RequestedMemQty,
		PodRequestedCpuQty: containerSummary.RequestedCpuQty,
		NodeUsageCpuQty:    nodeMetrics.Usage.Cpu(),
		NodeUsageMemQty:    nodeMetrics.Usage.Memory(),
		PodUsageCpuQty:     totalCpu,
		PodUsageMemQty:     totalMem,
		ReadyContainers:    statusSummary.Ready,
		TotalContainers:    statusSummary.Total,
		Restarts:           statusSummary.Restarts,
	}
}

func podMetricsTotals(metrics *metricsV1beta1.PodMetrics) (totalCpu, totalMem *resource.Quantity) {
	containers := metrics.Containers
	totalCpu = resource.NewQuantity(0, resource.DecimalSI)
	totalMem = resource.NewQuantity(0, resource.DecimalSI)
	for _, c := range containers {
		totalCpu.Add(*c.Usage.Cpu())
		totalMem.Add(*c.Usage.Memory())
	}
	return
}

func getContainerStatusSummary(containerStats []v1.ContainerStatus) ContainerStatusSummary {
	summary := ContainerStatusSummary{Total: len(containerStats)}
	for _, stat := range containerStats {
		summary.Restarts += int(stat.RestartCount)
		switch {
		case stat.Ready && stat.State.Running != nil:
			summary.Ready++
			summary.Status = "Running"
			summary.SomeRunning = true
		case stat.State.Waiting != nil:
			summary.Status = stat.State.Waiting.Reason
		case stat.State.Terminated != nil && stat.State.Terminated.Reason != "":
			summary.Status = stat.State.Terminated.Reason
		case stat.State.Terminated != nil && stat.State.Terminated.Reason == "":
			if stat.State.Terminated.Signal != 0 {
				summary.Status = fmt.Sprintf("Sig:%d", stat.State.Terminated.Signal)
			} else {
				summary.Status = fmt.Sprintf("Exit:%d", stat.State.Terminated.ExitCode)
			}
		}
	}
	return summary
}

func podIsReady(conds []v1.PodCondition) bool {
	for _, cond := range conds {
		if cond.Type == v1.PodReady && cond.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

func timeSince(ts metav1.Time) string {
	if ts.IsZero() {
		return "..."
	}
	return duration.HumanDuration(time.Since(ts.Time))
}

func GetPodContainerSummary(pod *v1.Pod) PodContainerSummary {
	mems := resource.NewQuantity(0, resource.DecimalSI)
	cpus := resource.NewQuantity(0, resource.DecimalSI)
	var ports int
	var mounts int
	for _, container := range pod.Spec.Containers {
		mems.Add(*container.Resources.Requests.Memory())
		cpus.Add(*container.Resources.Requests.Cpu())
		ports += len(container.Ports)
		mounts += len(container.VolumeMounts)
	}

	for _, container := range pod.Spec.InitContainers {
		mems.Add(*container.Resources.Requests.Memory())
		mems.Add(*container.Resources.Requests.Cpu())
		ports += len(container.Ports)
		mounts += len(container.VolumeMounts)
	}

	if pod.Spec.Overhead != nil {
		mems.Add(*pod.Spec.Overhead.Memory())
		mems.Add(*pod.Spec.Overhead.Cpu())
	}

	return PodContainerSummary{
		RequestedMemQty: mems,
		RequestedCpuQty: cpus,
		VolMounts:       mounts,
		Ports:           ports,
	}
}
