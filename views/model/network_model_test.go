package model

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNetworkModelsAndSummary(t *testing.T) {
	now := metav1.NewTime(time.Now().Add(-30 * time.Minute))

	svcHealthy := NewServiceModel(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "web", CreationTimestamp: now},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeLoadBalancer,
			ClusterIP: "10.96.0.10",
			Selector:  map[string]string{"app": "web"},
			Ports: []corev1.ServicePort{
				{Port: 80, NodePort: 30080, Protocol: corev1.ProtocolTCP},
			},
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}},
			},
		},
	}, 2, 2, true)
	if svcHealthy.ExternalIP != "203.0.113.10" || svcHealthy.Ports != "80:30080/TCP" || svcHealthy.HasEndpointAlert() {
		t.Fatalf("unexpected healthy service model: %+v", svcHealthy)
	}

	svcBroken := NewServiceModel(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "orphaned", CreationTimestamp: now},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.96.0.20",
			Selector:  map[string]string{"app": "gone"},
			Ports:     []corev1.ServicePort{{Port: 8080}},
		},
	}, 0, 1, true)
	if !svcBroken.HasEndpointAlert() || svcBroken.Ports != "8080/TCP" {
		t.Fatalf("expected endpoint alert on selector-backed 0-ready service: %+v", svcBroken)
	}

	svcExternal := NewServiceModel(&corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "ext-db", CreationTimestamp: now},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: "db.example.com",
		},
	}, 0, 0, true)
	if svcExternal.HasEndpointAlert() || svcExternal.ExternalIP != "db.example.com" {
		t.Fatalf("unexpected ExternalName service alert/external IP: %+v", svcExternal)
	}

	ingClass := "nginx"
	ing := NewIngressModel(&networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "edge", CreationTimestamp: now},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingClass,
			TLS:              []networkingv1.IngressTLS{{Hosts: []string{"app.example.com"}}},
			Rules: []networkingv1.IngressRule{
				{Host: "app.example.com"},
				{Host: "app.example.com"},
				{Host: "api.example.com"},
			},
		},
		Status: networkingv1.IngressStatus{
			LoadBalancer: networkingv1.IngressLoadBalancerStatus{
				Ingress: []networkingv1.IngressLoadBalancerIngress{{Hostname: "lb.example.com"}},
			},
		},
	})
	if ing.ClassName != "nginx" || ing.Ports != "80, 443" || len(ing.Hosts) != 2 || ing.Address != "lb.example.com" {
		t.Fatalf("unexpected ingress model: %+v", ing)
	}

	summary := ComputeNetworkSummary(
		[]ServiceModel{svcHealthy, svcBroken, svcExternal},
		[]IngressModel{ing},
		[]NodeNetworkModel{{
			NodeName:     "n1",
			RxRateBps:    1000,
			TxRateBps:    2000,
			RxDroppedSec: 1.5,
			TxDroppedSec: 0.5,
			RxErrorsSec:  0.2,
			TxErrorsSec:  0.3,
		}},
		true,
	)
	if summary.ServicesTotal != 3 || summary.ZeroEndpointServices != 1 || summary.IngressesTotal != 1 {
		t.Errorf("unexpected summary counts: %+v", summary)
	}
	if summary.ClusterRxRateBps != 1000 || summary.ClusterTxRateBps != 2000 || summary.DropsPerSec != 2.0 || summary.ErrorsPerSec != 0.5 {
		t.Errorf("unexpected summary rates: %+v", summary)
	}

	svcs := []ServiceModel{svcHealthy, svcBroken}
	SortServiceModelsBy(svcs, "ENDPOINTS", true)
	if svcs[0].Name != "orphaned" {
		t.Errorf("expected orphaned first by ENDPOINTS asc, got %s", svcs[0].Name)
	}
}
