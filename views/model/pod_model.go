package model

import (
	"fmt"
	"slices"
	"time"

	ktopmetrics "github.com/vladimirvivien/ktop/metrics"
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

	// PSI carries pressure stall percentages for this pod (max across its
	// containers, per axis). Zero when the metrics source does not provide PSI.
	PSI ktopmetrics.PSIMetrics

	// NodePSIKernelSupported mirrors NodeModel.PSIKernelSupported for the pod's
	// node, denormalized here to avoid a per-row node lookup at render time.
	NodePSIKernelSupported bool
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

func podLess(a, b PodModel, column string) bool {
	switch column {
	case "NAMESPACE":
		if a.Namespace == b.Namespace {
			return a.Name < b.Name
		}
		return a.Namespace < b.Namespace

	case "POD", "NAME":
		if a.Name == b.Name {
			return a.Namespace < b.Namespace
		}
		return a.Name < b.Name

	case "READY":
		readyA := float64(a.ReadyContainers) / float64(a.TotalContainers)
		readyB := float64(b.ReadyContainers) / float64(b.TotalContainers)
		if readyA == readyB {
			return a.Name < b.Name
		}
		return readyA < readyB

	case "STATUS":
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

		priA, okA := statusPriority[a.Status]
		priB, okB := statusPriority[b.Status]

		// Unknown statuses get highest priority (show first for investigation)
		if !okA {
			priA = 99
		}
		if !okB {
			priB = 99
		}

		if priA == priB {
			return a.Name < b.Name
		}
		return priA < priB

	case "RST", "RESTARTS":
		if a.Restarts == b.Restarts {
			return a.Name < b.Name
		}
		return a.Restarts < b.Restarts

	case "AGE":
		// Older pods first (earlier creation time)
		if a.CreationTime.Equal(&b.CreationTime) {
			return a.Name < b.Name
		}
		return a.CreationTime.Before(&b.CreationTime)

	case "VOLS":
		if a.Volumes == b.Volumes {
			if a.VolMounts == b.VolMounts {
				return a.Name < b.Name
			}
			return a.VolMounts < b.VolMounts
		}
		return a.Volumes < b.Volumes

	case "IP":
		if a.IP == b.IP {
			return a.Name < b.Name
		}
		return a.IP < b.IP

	case "NODE":
		if a.Node == b.Node {
			return a.Name < b.Name
		}
		return a.Node < b.Node

	case "CPU":
		cpuA := int64(0)
		cpuB := int64(0)

		// Prefer usage metrics, fall back to requested if unavailable
		if a.PodUsageCpuQty != nil && a.PodUsageCpuQty.MilliValue() > 0 {
			cpuA = a.PodUsageCpuQty.MilliValue()
		} else if a.PodRequestedCpuQty != nil {
			cpuA = a.PodRequestedCpuQty.MilliValue()
		}

		if b.PodUsageCpuQty != nil && b.PodUsageCpuQty.MilliValue() > 0 {
			cpuB = b.PodUsageCpuQty.MilliValue()
		} else if b.PodRequestedCpuQty != nil {
			cpuB = b.PodRequestedCpuQty.MilliValue()
		}

		if cpuA == cpuB {
			return a.Name < b.Name
		}
		return cpuA < cpuB

	case "MEMORY", "MEM":
		memA := int64(0)
		memB := int64(0)

		// Prefer usage metrics, fall back to requested if unavailable
		if a.PodUsageMemQty != nil && a.PodUsageMemQty.Value() > 0 {
			memA = a.PodUsageMemQty.Value()
		} else if a.PodRequestedMemQty != nil {
			memA = a.PodRequestedMemQty.Value()
		}

		if b.PodUsageMemQty != nil && b.PodUsageMemQty.Value() > 0 {
			memB = b.PodUsageMemQty.Value()
		} else if b.PodRequestedMemQty != nil {
			memB = b.PodRequestedMemQty.Value()
		}

		if memA == memB {
			return a.Name < b.Name
		}
		return memA < memB

	case "STALL":
		// Sort by the dominant stall axis (max of CPU/MEM/IO waiting %).
		dom := func(p PodModel) float64 {
			m := p.PSI.CPUStallPct
			if p.PSI.MemStallPct > m {
				m = p.PSI.MemStallPct
			}
			if p.PSI.IOStallPct > m {
				m = p.PSI.IOStallPct
			}
			return m
		}
		sa, sb := dom(a), dom(b)
		if sa == sb {
			return a.Name < b.Name
		}
		return sa < sb

	default:
		// Default to NAMESPACE then NAME sorting
		if a.Namespace == b.Namespace {
			return a.Name < b.Name
		}
		return a.Namespace < b.Namespace
	}
}

// SortPodModelsBy sorts pods by the specified column and direction
func SortPodModelsBy(pods []PodModel, column string, ascending bool) {
	if len(pods) <= 1 {
		return
	}
	slices.SortFunc(pods, func(a, b PodModel) int {
		if podLess(a, b, column) {
			if ascending {
				return -1
			}
			return 1
		}
		if podLess(b, a, column) {
			if ascending {
				return 1
			}
			return -1
		}
		return 0
	})
}

// SortPodModelPointersBy sorts a slice of *PodModel by the specified column and direction
func SortPodModelPointersBy(pods []*PodModel, column string, ascending bool) {
	if len(pods) <= 1 {
		return
	}
	slices.SortFunc(pods, func(a, b *PodModel) int {
		if a == nil && b == nil {
			return 0
		}
		if a == nil {
			return 1
		}
		if b == nil {
			return -1
		}
		if podLess(*a, *b, column) {
			if ascending {
				return -1
			}
			return 1
		}
		if podLess(*b, *a, column) {
			if ascending {
				return 1
			}
			return -1
		}
		return 0
	})
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
		return "...   "
	}
	d := duration.HumanDuration(time.Since(ts.Time))
	// Right-align with 6-character width for values like "2m40s" or "120d"
	return fmt.Sprintf("%-6s", d)
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
		cpus.Add(*container.Resources.Requests.Cpu())
		ports += len(container.Ports)
		mounts += len(container.VolumeMounts)
	}

	if pod.Spec.Overhead != nil {
		mems.Add(*pod.Spec.Overhead.Memory())
		cpus.Add(*pod.Spec.Overhead.Cpu())
	}

	return PodContainerSummary{
		RequestedMemQty: mems,
		RequestedCpuQty: cpus,
		VolMounts:       mounts,
		Ports:           ports,
	}
}
