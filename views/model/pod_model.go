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

	PodCPUValue  int64
	PodMemValue  int64
	NodeCPUValue int64
	NodeMemValue int64

	ReadyContainers int
	TotalContainers int
	Restarts        int
	Volumes         int
}

type ContainerSummary struct {
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
	containerSummary := getContainerSummary(pod.Status.ContainerStatuses)
	if (containerSummary.Status == "" || containerSummary.Status == "Completed") && containerSummary.SomeRunning {
		if podIsReady(pod.Status.Conditions) {
			containerSummary.Status = "Running"
		} else {
			containerSummary.Status = "NotReady"
		}
	}
	return &PodModel{
		Namespace:       pod.GetNamespace(),
		Name:            pod.Name,
		Status:          containerSummary.Status,
		TimeSince:       timeSince(pod.CreationTimestamp),
		IP:              pod.Status.PodIP,
		Node:            pod.Spec.NodeName,
		Volumes:         len(pod.Spec.Volumes),
		NodeCPUValue:    nodeMetrics.Usage.Cpu().MilliValue(),
		NodeMemValue:    nodeMetrics.Usage.Memory().MilliValue(),
		PodCPUValue:     totalCpu.MilliValue(),
		PodMemValue:     totalMem.MilliValue(),
		ReadyContainers: containerSummary.Ready,
		TotalContainers: containerSummary.Total,
		Restarts:        containerSummary.Restarts,
	}
}

func podMetricsTotals(metrics *metricsV1beta1.PodMetrics) (totalCpu, totalMem resource.Quantity) {
	containers := metrics.Containers
	for _, c := range containers {
		totalCpu.Add(*c.Usage.Cpu())
		totalMem.Add(*c.Usage.Memory())
	}
	return
}

func getContainerSummary(containerStats []v1.ContainerStatus) ContainerSummary {
	summary := ContainerSummary{Total: len(containerStats)}
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

func GetPodResourceRequests(pod *v1.Pod) (cpus *resource.Quantity, mems *resource.Quantity) {
	mems = resource.NewQuantity(0, resource.DecimalSI)
	cpus = resource.NewQuantity(0, resource.DecimalSI)
	for _, container := range pod.Spec.Containers {
		mems.Add(*container.Resources.Requests.Memory())
		cpus.Add(*container.Resources.Requests.Cpu())
	}

	for _, container := range pod.Spec.InitContainers {
		mems.Add(*container.Resources.Requests.Memory())
		mems.Add(*container.Resources.Requests.Cpu())
	}

	if pod.Spec.Overhead != nil {
		mems.Add(*pod.Spec.Overhead.Memory())
		mems.Add(*pod.Spec.Overhead.Cpu())
	}
	return
}

func GetPodContainerLimits(pod *v1.Pod) v1.ResourceList {
	limits := make(v1.ResourceList)
	for _, container := range pod.Spec.Containers {
		limits.Memory().Add(*container.Resources.Limits.Memory())
		limits.Cpu().Add(*container.Resources.Limits.Cpu())
	}
	for _, container := range pod.Spec.InitContainers {
		limits.Memory().Add(*container.Resources.Limits.Memory())
		limits.Cpu().Add(*container.Resources.Limits.Cpu())
	}
	return limits
}