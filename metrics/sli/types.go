package sli

import (
	"fmt"
	"sort"
	"time"
)

const (
	DefaultScrapeInterval = 15 * time.Second
	DefaultScrapeTimeout  = 5 * time.Second
)

// Config configures the Component SLI (/metrics/slis) and Kubelet Probe (/metrics/probes) collector.
type Config struct {
	ScrapeInterval time.Duration
	ScrapeTimeout  time.Duration
}

// HealthCheck represents a single kubernetes_healthcheck SLI probe.
type HealthCheck struct {
	Name         string
	Type         string // healthz, livez, readyz
	Healthy      bool
	SuccessCount float64
	ErrorCount   float64
}

// ComponentSLI aggregates /metrics/slis healthcheck status for one component target
// (for example, "apiserver" or a specific node's "kubelet").
type ComponentSLI struct {
	Component     string
	Target        string
	Healthy       bool
	TotalChecks   int
	FailingChecks []HealthCheck
	Checks        []HealthCheck
}

// ProbeStats captures cumulative kubelet prober_probe_total results for one pod container probe.
type ProbeStats struct {
	Namespace    string
	Pod          string
	Container    string
	ProbeType    string // Readiness, Liveness, Startup
	SuccessCount float64
	FailureCount float64
	UnknownCount float64
	Failing      bool
}

// Snapshot is an immutable point-in-time capture of Component SLIs and Kubelet probe counters.
type Snapshot struct {
	Available       bool
	ProbesAvailable bool
	ScrapedAt       time.Time
	Components      []ComponentSLI
	PodProbes       map[string][]ProbeStats
}

// UnhealthyComponentCount returns how many scraped components explicitly reported
// at least one failing kubernetes_healthcheck (value == 0). Returns 0 when SLIs are unavailable.
func (s *Snapshot) UnhealthyComponentCount() int {
	if s == nil || !s.Available {
		return 0
	}
	n := 0
	for _, c := range s.Components {
		if !c.Healthy {
			n++
		}
	}
	return n
}

// FailingCheckNames returns compact "target/check" labels for all currently failing SLI checks.
func (s *Snapshot) FailingCheckNames() []string {
	if s == nil || !s.Available {
		return nil
	}
	var out []string
	for _, c := range s.Components {
		for _, chk := range c.FailingChecks {
			label := c.Target
			if label == "" {
				label = c.Component
			}
			out = append(out, fmt.Sprintf("%s/%s", label, chk.Name))
		}
	}
	sort.Strings(out)
	return out
}

// FailingProbePods returns the number of distinct pods with non-zero probe failures.
func (s *Snapshot) FailingProbePods() int {
	if s == nil || !s.ProbesAvailable {
		return 0
	}
	count := 0
	for _, probes := range s.PodProbes {
		for _, p := range probes {
			if p.Failing {
				count++
				break
			}
		}
	}
	return count
}

// ProbesForPod returns sorted probe stats for the specified pod, if any.
func (s *Snapshot) ProbesForPod(namespace, name string) []ProbeStats {
	if s == nil || !s.ProbesAvailable {
		return nil
	}
	return s.PodProbes[namespace+"/"+name]
}
