package model

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

// SortContainerInfosBy sorts a slice of ContainerInfo by the specified column and direction.
func SortContainerInfosBy(containers []ContainerInfo, column string, ascending bool) {
	if len(containers) <= 1 {
		return
	}

	col := strings.ToUpper(column)
	slices.SortFunc(containers, func(a, b ContainerInfo) int {
		if containerLess(a, b, col) {
			if ascending {
				return -1
			}
			return 1
		}
		if containerLess(b, a, col) {
			if ascending {
				return 1
			}
			return -1
		}
		// Secondary tie-breaker by container Name
		if a.Name < b.Name {
			if ascending {
				return -1
			}
			return 1
		}
		if a.Name > b.Name {
			if ascending {
				return 1
			}
			return -1
		}
		return 0
	})
}

func containerLess(a, b ContainerInfo, column string) bool {
	switch column {
	case "NAME", "CONTAINER":
		return a.Name < b.Name
	case "IMAGE":
		return a.Image < b.Image
	case "STATE":
		if a.State != b.State {
			return a.State < b.State
		}
		return a.Name < b.Name
	case "READY":
		if a.Ready != b.Ready {
			return !a.Ready && b.Ready
		}
		return a.Name < b.Name
	case "RESTARTS":
		if a.RestartCount != b.RestartCount {
			return a.RestartCount < b.RestartCount
		}
		return a.Name < b.Name
	case "CPU":
		aQty := containerCPUQty(a)
		bQty := containerCPUQty(b)
		if aQty != bQty {
			return aQty < bQty
		}
		return a.Name < b.Name
	case "MEM", "MEMORY":
		aQty := containerMemQty(a)
		bQty := containerMemQty(b)
		if aQty != bQty {
			return aQty < bQty
		}
		return a.Name < b.Name
	default:
		return a.Name < b.Name
	}
}

func containerCPUQty(c ContainerInfo) int64 {
	val := c.CPUUsage
	if val == "" || val == "-" {
		val = c.CPURequest
	}
	if val == "" || val == "-" {
		val = c.CPULimit
	}
	if val == "" || val == "-" {
		return 0
	}
	q, err := resource.ParseQuantity(val)
	if err != nil {
		return 0
	}
	return q.MilliValue()
}

func containerMemQty(c ContainerInfo) int64 {
	val := c.MemoryUsage
	if val == "" || val == "-" {
		val = c.MemoryRequest
	}
	if val == "" || val == "-" {
		val = c.MemoryLimit
	}
	if val == "" || val == "-" {
		return 0
	}
	q, err := resource.ParseQuantity(val)
	if err != nil {
		return 0
	}
	return q.Value()
}
