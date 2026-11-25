package k8s

import (
	"context"
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/metrics"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type fakeNetworkMetricsSource struct {
	fakeWorkloadsMetricsSource
	nodeMetric map[string]*metrics.NodeMetrics
	clusterErr metrics.NetworkErrorRates
	perNodeErr map[string]metrics.NetworkErrorRates
}

func (f *fakeNetworkMetricsSource) GetNodeMetrics(_ context.Context, name string) (*metrics.NodeMetrics, error) {
	return f.nodeMetric[name], nil
}

func (f *fakeNetworkMetricsSource) GetClusterNetworkErrorRates(context.Context) (metrics.NetworkErrorRates, map[string]metrics.NetworkErrorRates, error) {
	return f.clusterErr, f.perNodeErr, nil
}

func (f *fakeNetworkMetricsSource) GetPodNetworkErrorRates(context.Context, string, string) (metrics.NetworkErrorRates, error) {
	return metrics.NetworkErrorRates{}, nil
}

func TestController_GetNetworkSnapshotAndRBACFallback(t *testing.T) {
	readyTrue := true
	readyFalse := false

	svcOK := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "web"},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.96.0.10",
			Selector:  map[string]string{"app": "web"},
			Ports:     []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}},
		},
	}
	svcDown := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "orphaned"},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.96.0.20",
			Selector:  map[string]string{"app": "gone"},
			Ports:     []corev1.ServicePort{{Port: 8080, Protocol: corev1.ProtocolTCP}},
		},
	}
	es := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "prod",
			Name:      "web-abc",
			Labels:    map[string]string{discoveryv1.LabelServiceName: "web"},
		},
		Endpoints: []discoveryv1.Endpoint{
			{Addresses: []string{"10.0.0.1"}, Conditions: discoveryv1.EndpointConditions{Ready: &readyTrue}},
			{Addresses: []string{"10.0.0.2"}, Conditions: discoveryv1.EndpointConditions{Ready: &readyFalse}},
		},
	}
	ingClass := "nginx"
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "edge"},
		Spec: networkingv1.IngressSpec{
			IngressClassName: &ingClass,
			Rules:            []networkingv1.IngressRule{{Host: "app.example.com"}},
		},
	}
	node := readyTestNode("node-1")

	fakeClient := fake.NewSimpleClientset(svcOK, svcDown, es, ing, node)
	client := NewWithClient(fakeClient, AllNamespaces, "ctx", nil)
	ctrl := client.Controller()
	ctrl.SetMetricsSource(&fakeNetworkMetricsSource{
		nodeMetric: map[string]*metrics.NodeMetrics{
			"node-1": {NodeName: "node-1", NetworkRxRate: 5000, NetworkTxRate: 3000},
		},
		perNodeErr: map[string]metrics.NetworkErrorRates{
			"node-1": {RxDroppedPerSec: 1.5, TxErrorsPerSec: 0.25},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ctrl.StartInformers(ctx, time.Second, time.Second); err != nil {
		t.Fatalf("StartInformers: %v", err)
	}

	snap, err := ctrl.GetNetworkSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetNetworkSnapshot: %v", err)
	}
	if snap.Summary.ServicesTotal != 2 || snap.Summary.ZeroEndpointServices != 1 || snap.Summary.IngressesTotal != 1 {
		t.Fatalf("unexpected network summary counts: %+v", snap.Summary)
	}
	if snap.Summary.ClusterRxRateBps != 5000 || snap.Summary.ClusterTxRateBps != 3000 || snap.Summary.DropsPerSec != 1.5 || snap.Summary.ErrorsPerSec != 0.25 {
		t.Fatalf("unexpected network summary rates: %+v", snap.Summary)
	}
	if snap.Services[0].Name != "orphaned" || snap.Services[1].ReadyEndpoints != 1 || snap.Services[1].TotalEndpoints != 2 {
		t.Fatalf("unexpected services order/endpoints: %+v", snap.Services)
	}

	// Verify 403 Forbidden on Ingresses / EndpointSlices degrades silently.
	forbidClient := fake.NewSimpleClientset(svcOK, node)
	forbidClient.PrependReactor("list", "ingresses", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "networking.k8s.io", Resource: "ingresses"}, "", nil)
	})
	forbidClient.PrependReactor("list", "endpointslices", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "discovery.k8s.io", Resource: "endpointslices"}, "", nil)
	})
	c2 := NewWithClient(forbidClient, AllNamespaces, "ctx", nil)
	if err := c2.Controller().StartInformers(ctx, time.Second, time.Second); err != nil {
		t.Fatalf("StartInformers with forbidden optional network resources failed: %v", err)
	}
	snap2, err := c2.Controller().GetNetworkSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetNetworkSnapshot with degraded informers failed: %v", err)
	}
	if len(snap2.Services) != 1 || snap2.Services[0].HasEndpoints || snap2.Summary.ZeroEndpointServices != 0 {
		t.Fatalf("expected HasEndpoints=false when EndpointSlices forbidden, got %+v", snap2)
	}
}
