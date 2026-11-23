package ksm

import (
	"strings"
	"time"
)

const (
	// ModeAuto automatically discovers kube-state-metrics via the Kubernetes API
	// service proxy and degrades silently when absent or RBAC-forbidden.
	ModeAuto = "auto"
	// ModeOff disables kube-state-metrics discovery and scraping.
	ModeOff = "off"

	DefaultScrapeInterval    = 15 * time.Second
	DefaultScrapeTimeout     = 5 * time.Second
	DefaultRediscoverBackoff = 60 * time.Second
)

// Config configures the kube-state-metrics collector.
type Config struct {
	Mode           string
	Endpoint       string
	NamespaceHint  string
	ScrapeInterval time.Duration
	ScrapeTimeout  time.Duration
}

// Snapshot is an immutable point-in-time capture of allowlisted kube-state-metrics state.
type Snapshot struct {
	Available         bool
	Endpoint          string
	ScrapedAt         time.Time
	HPAs              []HPAMetrics
	PDBs              []PDBMetrics
	ResourceQuotas    []ResourceQuotaMetrics
	CronJobs          []CronJobMetrics
	PodDisruptions    map[string]string
	PodResourceClaims map[string][]ResourceClaimRef
}

// HPAMetrics captures HorizontalPodAutoscaler spec, live replica counts, and scaling conditions.
type HPAMetrics struct {
	Namespace       string
	Name            string
	TargetKind      string
	TargetName      string
	MinReplicas     int32
	MaxReplicas     int32
	CurrentReplicas int32
	DesiredReplicas int32
	AbleToScale     *bool
	ScalingActive   *bool
	ScalingLimited  *bool
}

// PDBMetrics captures PodDisruptionBudget health and disruption budget counts.
type PDBMetrics struct {
	Namespace          string
	Name               string
	CurrentHealthy     int32
	DesiredHealthy     int32
	ExpectedPods       int32
	DisruptionsAllowed int32
}

// ResourceQuotaMetrics captures a single resource's hard limit vs. used amount in a ResourceQuota.
type ResourceQuotaMetrics struct {
	Namespace string
	Name      string
	Resource  string
	Hard      float64
	Used      float64
	UsedPct   float64
}

// CronJobMetrics captures CronJob schedule and active/suspended state.
type CronJobMetrics struct {
	Namespace        string
	Name             string
	Suspended        bool
	Active           int32
	LastScheduleTime time.Time
	NextScheduleTime time.Time
}

// ResourceClaimRef captures a DRA ResourceClaim association for a Pod.
type ResourceClaimRef struct {
	Name              string
	ResourceClaimName string
	TemplateName      string
}

// HPAsForWorkload returns all HPAs targeting the specified workload kind/namespace/name.
func (s *Snapshot) HPAsForWorkload(kind, namespace, name string) []HPAMetrics {
	if s == nil || !s.Available {
		return nil
	}
	var out []HPAMetrics
	for _, h := range s.HPAs {
		if h.Namespace == namespace && h.TargetName == name && strings.EqualFold(h.TargetKind, kind) {
			out = append(out, h)
		}
	}
	return out
}

// PDBsAtRisk returns the count of PDBs allowing zero disruptions while expecting pods.
func (s *Snapshot) PDBsAtRisk() int {
	if s == nil || !s.Available {
		return 0
	}
	count := 0
	for _, p := range s.PDBs {
		if p.ExpectedPods > 0 && p.DisruptionsAllowed == 0 {
			count++
		}
	}
	return count
}

// HPAsLimited returns the count of HPAs reporting ScalingLimited=true.
func (s *Snapshot) HPAsLimited() int {
	if s == nil || !s.Available {
		return 0
	}
	count := 0
	for _, h := range s.HPAs {
		if h.ScalingLimited != nil && *h.ScalingLimited {
			count++
		}
	}
	return count
}

// QuotasNearLimit returns the count of ResourceQuota entries at or above thresholdPct utilization.
func (s *Snapshot) QuotasNearLimit(thresholdPct float64) int {
	if s == nil || !s.Available {
		return 0
	}
	count := 0
	for _, q := range s.ResourceQuotas {
		if q.Hard > 0 && q.UsedPct >= thresholdPct {
			count++
		}
	}
	return count
}
