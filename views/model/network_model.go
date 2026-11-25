package model

import (
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceModel represents a normalized Kubernetes Service joined with EndpointSlice readiness.
type ServiceModel struct {
	Namespace      string
	Name           string
	Type           string
	ClusterIP      string
	ExternalIP     string
	Ports          string
	ReadyEndpoints int
	TotalEndpoints int
	HasEndpoints   bool
	Selectorless   bool
	CreationTime   metav1.Time
	Age            string
	Raw            *corev1.Service
}

// IngressModel represents a normalized networking.k8s.io/v1 Ingress.
type IngressModel struct {
	Namespace    string
	Name         string
	ClassName    string
	Hosts        []string
	Address      string
	Ports        string
	CreationTime metav1.Time
	Age          string
	Raw          *networkingv1.Ingress
}

// NodeNetworkModel captures per-node throughput, packet drops, and interface errors.
type NodeNetworkModel struct {
	NodeName     string
	RxRateBps    float64
	TxRateBps    float64
	RxDroppedSec float64
	TxDroppedSec float64
	RxErrorsSec  float64
	TxErrorsSec  float64
}

// NetworkSummary aggregates cluster Service, EndpointSlice, Ingress, and network rate telemetry.
type NetworkSummary struct {
	ServicesTotal        int
	ZeroEndpointServices int
	IngressesTotal       int
	ClusterRxRateBps     float64
	ClusterTxRateBps     float64
	DropsPerSec          float64
	ErrorsPerSec         float64
	HasMetrics           bool
}

// NetworkSnapshot holds a complete point-in-time view of cluster network resources and traffic telemetry.
type NetworkSnapshot struct {
	Summary     NetworkSummary
	Services    []ServiceModel
	Ingresses   []IngressModel
	NodeNetwork []NodeNetworkModel
}

// NewServiceModel builds a normalized ServiceModel from a Kubernetes Service and EndpointSlice totals.
func NewServiceModel(svc *corev1.Service, readyEP, totalEP int, hasEP bool) ServiceModel {
	if svc == nil {
		return ServiceModel{}
	}

	extIP := extractServiceExternalIP(svc)
	ports := formatServicePorts(svc)

	return ServiceModel{
		Namespace:      svc.Namespace,
		Name:           svc.Name,
		Type:           string(svc.Spec.Type),
		ClusterIP:      svc.Spec.ClusterIP,
		ExternalIP:     extIP,
		Ports:          ports,
		ReadyEndpoints: readyEP,
		TotalEndpoints: totalEP,
		HasEndpoints:   hasEP,
		Selectorless:   len(svc.Spec.Selector) == 0,
		CreationTime:   svc.CreationTimestamp,
		Age:            strings.TrimSpace(timeSince(svc.CreationTimestamp)),
		Raw:            svc,
	}
}

func extractServiceExternalIP(svc *corev1.Service) string {
	var addrs []string
	for _, ing := range svc.Status.LoadBalancer.Ingress {
		if ing.IP != "" {
			addrs = append(addrs, ing.IP)
		} else if ing.Hostname != "" {
			addrs = append(addrs, ing.Hostname)
		}
	}
	if len(addrs) > 0 {
		return strings.Join(addrs, ",")
	}
	if len(svc.Spec.ExternalIPs) > 0 {
		return strings.Join(svc.Spec.ExternalIPs, ",")
	}
	if svc.Spec.Type == corev1.ServiceTypeExternalName && svc.Spec.ExternalName != "" {
		return svc.Spec.ExternalName
	}
	return ""
}

func formatServicePorts(svc *corev1.Service) string {
	if len(svc.Spec.Ports) == 0 {
		return ""
	}
	parts := make([]string, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		proto := string(p.Protocol)
		if proto == "" {
			proto = "TCP"
		}
		if p.NodePort > 0 {
			parts = append(parts, fmt.Sprintf("%d:%d/%s", p.Port, p.NodePort, proto))
		} else {
			parts = append(parts, fmt.Sprintf("%d/%s", p.Port, proto))
		}
	}
	return strings.Join(parts, ",")
}

// HasEndpointAlert returns true when a selector-backed Service currently has zero ready endpoints.
func (s ServiceModel) HasEndpointAlert() bool {
	return s.HasEndpoints && !s.Selectorless && s.Type != string(corev1.ServiceTypeExternalName) && s.ReadyEndpoints == 0
}

// NewIngressModel builds a normalized IngressModel from a networking/v1 Ingress.
func NewIngressModel(ing *networkingv1.Ingress) IngressModel {
	if ing == nil {
		return IngressModel{}
	}
	className := ""
	if ing.Spec.IngressClassName != nil {
		className = *ing.Spec.IngressClassName
	} else if ing.Annotations != nil {
		className = ing.Annotations["kubernetes.io/ingress.class"]
	}

	var hosts []string
	for _, rule := range ing.Spec.Rules {
		if rule.Host != "" && !slices.Contains(hosts, rule.Host) {
			hosts = append(hosts, rule.Host)
		}
	}

	var addrs []string
	for _, lb := range ing.Status.LoadBalancer.Ingress {
		if lb.IP != "" {
			addrs = append(addrs, lb.IP)
		} else if lb.Hostname != "" {
			addrs = append(addrs, lb.Hostname)
		}
	}

	ports := "80"
	if len(ing.Spec.TLS) > 0 {
		ports = "80, 443"
	}

	return IngressModel{
		Namespace:    ing.Namespace,
		Name:         ing.Name,
		ClassName:    className,
		Hosts:        hosts,
		Address:      strings.Join(addrs, ","),
		Ports:        ports,
		CreationTime: ing.CreationTimestamp,
		Age:          strings.TrimSpace(timeSince(ing.CreationTimestamp)),
		Raw:          ing,
	}
}

// ComputeNetworkSummary aggregates Service, Ingress, EndpointSlice health, and cluster network rates.
func ComputeNetworkSummary(svcs []ServiceModel, ings []IngressModel, nodes []NodeNetworkModel, hasMetrics bool) NetworkSummary {
	summary := NetworkSummary{
		ServicesTotal:  len(svcs),
		IngressesTotal: len(ings),
		HasMetrics:     hasMetrics,
	}
	for _, svc := range svcs {
		if svc.HasEndpointAlert() {
			summary.ZeroEndpointServices++
		}
	}
	for _, n := range nodes {
		summary.ClusterRxRateBps += n.RxRateBps
		summary.ClusterTxRateBps += n.TxRateBps
		summary.DropsPerSec += n.RxDroppedSec + n.TxDroppedSec
		summary.ErrorsPerSec += n.RxErrorsSec + n.TxErrorsSec
	}
	return summary
}

// SortServiceModelsBy sorts Service models in-place by column name and direction.
func SortServiceModelsBy(items []ServiceModel, column string, ascending bool) {
	sortSliceBy(items, ascending, func(a, b ServiceModel) bool {
		switch column {
		case "NAME":
			return compareNamespaced(a.Name, a.Namespace, b.Name, b.Namespace)
		case "TYPE":
			if a.Type == b.Type {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.Type < b.Type
		case "CLUSTER-IP":
			if a.ClusterIP == b.ClusterIP {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.ClusterIP < b.ClusterIP
		case "EXTERNAL-IP":
			if a.ExternalIP == b.ExternalIP {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.ExternalIP < b.ExternalIP
		case "PORTS":
			if a.Ports == b.Ports {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.Ports < b.Ports
		case "ENDPOINTS":
			if a.ReadyEndpoints == b.ReadyEndpoints {
				if a.TotalEndpoints == b.TotalEndpoints {
					return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
				}
				return a.TotalEndpoints < b.TotalEndpoints
			}
			return a.ReadyEndpoints < b.ReadyEndpoints
		case "AGE":
			if a.CreationTime.Equal(&b.CreationTime) {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.CreationTime.Before(&b.CreationTime)
		default: // NS
			return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
		}
	})
}

// SortIngressModelsBy sorts Ingress models in-place by column name and direction.
func SortIngressModelsBy(items []IngressModel, column string, ascending bool) {
	sortSliceBy(items, ascending, func(a, b IngressModel) bool {
		switch column {
		case "NAME":
			return compareNamespaced(a.Name, a.Namespace, b.Name, b.Namespace)
		case "CLASS":
			if a.ClassName == b.ClassName {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.ClassName < b.ClassName
		case "HOSTS":
			ha, hb := strings.Join(a.Hosts, ","), strings.Join(b.Hosts, ",")
			if ha == hb {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return ha < hb
		case "ADDRESS":
			if a.Address == b.Address {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.Address < b.Address
		case "PORTS":
			if a.Ports == b.Ports {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.Ports < b.Ports
		case "AGE":
			if a.CreationTime.Equal(&b.CreationTime) {
				return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
			}
			return a.CreationTime.Before(&b.CreationTime)
		default: // NS
			return compareNamespaced(a.Namespace, a.Name, b.Namespace, b.Name)
		}
	})
}
