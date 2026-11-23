package sli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const sampleAPIServerSLIs = `
# HELP kubernetes_healthcheck [ALPHA] This metric records the result of a single healthcheck.
kubernetes_healthcheck{name="etcd",type="readyz"} 1
kubernetes_healthcheck{name="informer-sync",type="readyz"} 0
kubernetes_healthchecks_total{name="etcd",status="success",type="readyz"} 120
kubernetes_healthchecks_total{name="informer-sync",status="error",type="readyz"} 4
`

const sampleKubeletSLIs = `
kubernetes_healthcheck{name="syncloop",type="healthz"} 1
kubernetes_healthchecks_total{name="syncloop",status="success",type="healthz"} 50
`

const sampleKubeletProbes = `
prober_probe_total{container="app",namespace="prod",pod="api-0",probe_type="Readiness",result="successful"} 98
prober_probe_total{container="app",namespace="prod",pod="api-0",probe_type="Readiness",result="failed"} 3
prober_probe_total{container="app",namespace="prod",pod="api-0",probe_type="Liveness",result="successful"} 100
prober_probe_total{container="sidecar",namespace="default",pod="web-0",probe_type="Readiness",result="successful"} 50
`

func TestParseSLIAndProbesStreams(t *testing.T) {
	comp, ok, err := ParseSLIStream(strings.NewReader(sampleAPIServerSLIs), "apiserver", "apiserver")
	if err != nil || !ok {
		t.Fatalf("ParseSLIStream failed: ok=%v err=%v", ok, err)
	}
	if comp.Healthy || comp.TotalChecks != 2 || len(comp.FailingChecks) != 1 || comp.FailingChecks[0].Name != "informer-sync" {
		t.Fatalf("unexpected ComponentSLI: %+v", comp)
	}

	probes, ok, err := ParseProbesStream(strings.NewReader(sampleKubeletProbes))
	if err != nil || !ok {
		t.Fatalf("ParseProbesStream failed: ok=%v err=%v", ok, err)
	}
	apiProbes := probes["prod/api-0"]
	if len(apiProbes) != 2 {
		t.Fatalf("expected 2 probe entries for prod/api-0, got %+v", apiProbes)
	}
	var foundFailing bool
	for _, p := range apiProbes {
		if p.ProbeType == "Readiness" && p.Failing && p.FailureCount == 3 && p.SuccessCount == 98 {
			foundFailing = true
		}
	}
	if !foundFailing {
		t.Fatalf("expected failing Readiness probe on prod/api-0, got %+v", apiProbes)
	}
}

func TestCollector_ScrapesAPIServerAndKubeletSLIsAndProbes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metrics/slis":
			_, _ = w.Write([]byte(sampleAPIServerSLIs))
		case "/api/v1/nodes/worker-1/proxy/metrics/slis":
			_, _ = w.Write([]byte(sampleKubeletSLIs))
		case "/api/v1/nodes/worker-1/proxy/metrics/probes":
			_, _ = w.Write([]byte(sampleKubeletProbes))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("NewForConfig failed: %v", err)
	}

	c := NewCollector(cs, nil, func(context.Context) ([]string, error) {
		return []string{"worker-1"}, nil
	}, Config{})

	snap := c.ScrapeOnce(context.Background())
	if snap == nil || !snap.Available || !snap.ProbesAvailable {
		t.Fatalf("expected both SLIs and probes available, got %+v", snap)
	}
	if snap.UnhealthyComponentCount() != 1 {
		t.Fatalf("expected 1 unhealthy component (apiserver), got %d", snap.UnhealthyComponentCount())
	}
	failingNames := snap.FailingCheckNames()
	if len(failingNames) != 1 || failingNames[0] != "apiserver/informer-sync" {
		t.Fatalf("unexpected failing checks: %+v", failingNames)
	}
	if snap.FailingProbePods() != 1 {
		t.Fatalf("expected 1 failing probe pod, got %d", snap.FailingProbePods())
	}
}
