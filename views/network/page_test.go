package network

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/model"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func sampleNetworkSnapshot(now time.Time) model.NetworkSnapshot {
	svcs := []model.ServiceModel{
		model.NewServiceModel(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "api-svc", CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour))},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeClusterIP,
				ClusterIP: "10.96.0.10",
				Selector:  map[string]string{"app": "api"},
				Ports:     []corev1.ServicePort{{Port: 443, Protocol: corev1.ProtocolTCP}},
			},
		}, 2, 2, true),
		model.NewServiceModel(&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Namespace: "staging", Name: "broken-svc", CreationTimestamp: metav1.NewTime(now.Add(-15 * time.Minute))},
			Spec: corev1.ServiceSpec{
				Type:      corev1.ServiceTypeLoadBalancer,
				ClusterIP: "10.96.0.20",
				Selector:  map[string]string{"app": "broken"},
				Ports:     []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}},
			},
		}, 0, 2, true),
	}
	className := "nginx"
	ings := []model.IngressModel{
		model.NewIngressModel(&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "edge", CreationTimestamp: metav1.NewTime(now.Add(-1 * time.Hour))},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &className,
				TLS:              []networkingv1.IngressTLS{{Hosts: []string{"api.example.com"}}},
				Rules:            []networkingv1.IngressRule{{Host: "api.example.com"}},
			},
			Status: networkingv1.IngressStatus{
				LoadBalancer: networkingv1.IngressLoadBalancerStatus{
					Ingress: []networkingv1.IngressLoadBalancerIngress{{IP: "192.0.2.50"}},
				},
			},
		}),
	}
	nodes := []model.NodeNetworkModel{
		{
			NodeName:     "worker-1",
			RxRateBps:    2048,
			TxRateBps:    4096,
			RxDroppedSec: 1.5,
			RxErrorsSec:  0.5,
		},
	}
	return model.NetworkSnapshot{
		Summary:     model.ComputeNetworkSummary(svcs, ings, nodes, true),
		Services:    svcs,
		Ingresses:   ings,
		NodeNetwork: nodes,
	}
}

func TestNetworkPage_RenderFilterAndYAMLModal(t *testing.T) {
	t.Cleanup(func() {
		_, _ = ui.ResolveAndApplyTheme("dark", "truecolor")
	})

	app := application.New(nil, nil)
	page := New(app, "Network")
	app.RegisterPage(application.PageDescriptor{
		ID:         "network",
		Title:      "Network",
		Key:        '1',
		RootType:   application.PageNetwork,
		Panel:      page,
		ChildNames: []string{"services", "ingresses"},
	})
	if err := app.Setup(context.Background()); err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	snap := sampleNetworkSnapshot(time.Now().UTC())
	page.applySnapshot(snap)

	ts, err := ui.NewTestScreen(160, 40)
	if err != nil {
		t.Fatalf("NewTestScreen failed: %v", err)
	}
	t.Cleanup(ts.Close)

	ts.Draw(app.RootPrimitive())
	text := ts.Text()
	for _, want := range []string{
		"Services: 2",
		"1 zero-ep",
		"Ingresses: 1",
		"Drops: 1.5/s",
		"Errs: 0.5/s",
		"api-svc",
		"2/2",
		"broken-svc",
		"0/2",
		"edge",
		"api.example.com",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q on Network screen, got:\n%s", want, text)
		}
	}

	// Verify namespace filter scopes Services and Ingresses.
	page.SetNamespaceFilter("prod")
	if len(page.services.displayed) != 1 || page.services.displayed[0].Name != "api-svc" {
		t.Fatalf("expected prod Service only, got %+v", page.services.displayed)
	}
	if len(page.ingresses.displayed) != 1 || page.ingresses.displayed[0].Name != "edge" {
		t.Fatalf("expected prod Ingress only, got %+v", page.ingresses.displayed)
	}
	page.SetNamespaceFilter("")

	// Verify '/' row filter on Services table.
	dispatchKey := func(p tview.Primitive, ev *tcell.EventKey) {
		if p == nil {
			return
		}
		if h := p.InputHandler(); h != nil {
			h(ev, func(next tview.Primitive) {
				if next != nil {
					app.Focus(next)
				}
			})
		}
	}
	app.Focus(page.services.table)
	dispatchKey(page.services.table, tcell.NewEventKey(tcell.KeyRune, '/', tcell.ModNone))
	for _, ch := range "broken" {
		dispatchKey(page.services.table, tcell.NewEventKey(tcell.KeyRune, ch, tcell.ModNone))
	}
	dispatchKey(page.services.table, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if len(page.services.displayed) != 1 || page.services.displayed[0].Name != "broken-svc" {
		t.Fatalf("expected row filter 'broken' to leave 1 Service, got %+v", page.services.displayed)
	}
	page.HandleEscape()

	// Verify Enter opens YAML modal for selected Service and Esc closes it.
	page.services.table.Select(1, 0)
	dispatchKey(page.services.table, tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	ts.Draw(app.RootPrimitive())
	if !strings.Contains(ts.Text(), "Service YAML: prod/api-svc") {
		t.Fatalf("expected Service YAML modal to open, got:\n%s", ts.Text())
	}
	dispatchKey(app.FocusedPrimitive(), tcell.NewEventKey(tcell.KeyEscape, 0, tcell.ModNone))
	ts.Draw(app.RootPrimitive())
	if strings.Contains(ts.Text(), "Service YAML: prod/api-svc") {
		t.Fatalf("expected Service YAML modal closed on Esc, got:\n%s", ts.Text())
	}
}
