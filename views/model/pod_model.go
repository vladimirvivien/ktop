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
	Namespace string
	Name      string
	Status    string
	Node      string
	IP        string
	TimeSince string

	PodRequestedCpuQty *resource.Quantity
	PodRequestedMemQty *resource.Quantity
	PodLimitCpuQty     *resource.Quantity
	PodLimitMemQty     *resource.Quantity
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
	LimitMemQty     *resource.Quantity
	LimitCpuQty     *resource.Quantity
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
	sort.Slice(pods, func(i, j int) bool {
		if pods[i].Namespace != pods[j].Namespace {
			return pods[i].Namespace < pods[j].Namespace
		}
		return pods[i].Name < pods[j].Name
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
		IP:                 pod.Status.PodIP,
		Node:               pod.Spec.NodeName,
		Volumes:            len(pod.Spec.Volumes),
		VolMounts:          containerSummary.VolMounts,
		PodRequestedMemQty: containerSummary.RequestedMemQty,
		PodRequestedCpuQty: containerSummary.RequestedCpuQty,
		PodLimitMemQty:     containerSummary.LimitMemQty,
		PodLimitCpuQty:     containerSummary.LimitCpuQty,
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
	requestedMems := resource.NewQuantity(0, resource.DecimalSI)
	requestedCpus := resource.NewQuantity(0, resource.DecimalSI)
	limitMems := resource.NewQuantity(0, resource.DecimalSI)
	limitCpus := resource.NewQuantity(0, resource.DecimalSI)
	var ports int
	var mounts int
	
	for _, container := range pod.Spec.Containers {
		// Handle requests
		if reqMem := container.Resources.Requests.Memory(); reqMem != nil {
			requestedMems.Add(*reqMem)
		}
		if reqCpu := container.Resources.Requests.Cpu(); reqCpu != nil {
			requestedCpus.Add(*reqCpu)
		}
		
		// Handle limits
		if limMem := container.Resources.Limits.Memory(); limMem != nil {
			limitMems.Add(*limMem)
		}
		if limCpu := container.Resources.Limits.Cpu(); limCpu != nil {
			limitCpus.Add(*limCpu)
		}
		
		ports += len(container.Ports)
		mounts += len(container.VolumeMounts)
	}

	for _, container := range pod.Spec.InitContainers {
		// Handle requests
		if reqMem := container.Resources.Requests.Memory(); reqMem != nil {
			requestedMems.Add(*reqMem)
		}
		if reqCpu := container.Resources.Requests.Cpu(); reqCpu != nil {
			requestedCpus.Add(*reqCpu)
		}
		
		// Handle limits
		if limMem := container.Resources.Limits.Memory(); limMem != nil {
			limitMems.Add(*limMem)
		}
		if limCpu := container.Resources.Limits.Cpu(); limCpu != nil {
			limitCpus.Add(*limCpu)
		}
		
		ports += len(container.Ports)
		mounts += len(container.VolumeMounts)
	}

	if pod.Spec.Overhead != nil {
		if ovhMem := pod.Spec.Overhead.Memory(); ovhMem != nil {
			requestedMems.Add(*ovhMem)
			limitMems.Add(*ovhMem)
		}
		if ovhCpu := pod.Spec.Overhead.Cpu(); ovhCpu != nil {
			requestedCpus.Add(*ovhCpu)
			limitCpus.Add(*ovhCpu)
		}
	}

	return PodContainerSummary{
		RequestedMemQty: requestedMems,
		RequestedCpuQty: requestedCpus,
		LimitMemQty:     limitMems,
		LimitCpuQty:     limitCpus,
		VolMounts:       mounts,
		Ports:           ports,
	}
}