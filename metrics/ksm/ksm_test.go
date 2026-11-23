package ksm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const sampleKSMMetrics = `
# HELP kube_pod_info Information about pod (ignored by allowlist)
kube_pod_info{namespace="default",pod="api-0",node="worker-1"} 1
# HELP kube_horizontalpodautoscaler_info Info about HPA
kube_horizontalpodautoscaler_info{namespace="prod",horizontalpodautoscaler="api-hpa",scaletargetref_kind="Deployment",scaletargetref_name="api"} 1
kube_horizontalpodautoscaler_spec_min_replicas{namespace="prod",horizontalpodautoscaler="api-hpa"} 2
kube_horizontalpodautoscaler_spec_max_replicas{namespace="prod",horizontalpodautoscaler="api-hpa"} 10
kube_horizontalpodautoscaler_status_current_replicas{namespace="prod",horizontalpodautoscaler="api-hpa"} 10
kube_horizontalpodautoscaler_status_desired_replicas{namespace="prod",horizontalpodautoscaler="api-hpa"} 10
kube_horizontalpodautoscaler_status_condition{namespace="prod",horizontalpodautoscaler="api-hpa",condition="ScalingActive",status="true"} 1
kube_horizontalpodautoscaler_status_condition{namespace="prod",horizontalpodautoscaler="api-hpa",condition="ScalingLimited",status="true"} 1
kube_horizontalpodautoscaler_status_condition{namespace="prod",horizontalpodautoscaler="api-hpa",condition="ScalingLimited",status="false"} 0
kube_poddisruptionbudget_status_current_healthy{namespace="prod",poddisruptionbudget="api-pdb"} 2
kube_poddisruptionbudget_status_desired_healthy{namespace="prod",poddisruptionbudget="api-pdb"} 2
kube_poddisruptionbudget_status_expected_pods{namespace="prod",poddisruptionbudget="api-pdb"} 2
kube_poddisruptionbudget_status_pod_disruptions_allowed{namespace="prod",poddisruptionbudget="api-pdb"} 0
kube_resourcequota{namespace="prod",resourcequota="compute-quota",resource="requests.cpu",type="hard"} 10
kube_resourcequota{namespace="prod",resourcequota="compute-quota",resource="requests.cpu",type="used"} 9
kube_cronjob_spec_suspend{namespace="batch",cronjob="nightly"} 0
kube_cronjob_status_active{namespace="batch",cronjob="nightly"} 1
kube_cronjob_status_last_schedule_time{namespace="batch",cronjob="nightly"} 1700000000
kube_pod_status_disruption_reason{namespace="prod",pod="api-0",reason="PreemptionByScheduler"} 1
kube_pod_resourceclaim_info{namespace="ml",pod="train-0",name="gpu",resourceclaim_name="gpu-claim-0",resourceclaimtemplate_name="gpu-tpl"} 1
`

func TestParseStream_AllowlistAndSummaries(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	snap, err := ParseStream(strings.NewReader(sampleKSMMetrics), "http://ksm/metrics", now)
	if err != nil {
		t.Fatalf("ParseStream failed: %v", err)
	}
	if !snap.Available {
		t.Fatal("expected snap.Available true")
	}

	if len(snap.HPAs) != 1 {
		t.Fatalf("expected 1 HPA, got %d", len(snap.HPAs))
	}
	hpa := snap.HPAs[0]
	if hpa.TargetKind != "Deployment" || hpa.TargetName != "api" || hpa.MinReplicas != 2 || hpa.MaxReplicas != 10 || hpa.CurrentReplicas != 10 {
		t.Fatalf("unexpected HPA: %+v", hpa)
	}
	if hpa.ScalingLimited == nil || !*hpa.ScalingLimited {
		t.Fatalf("expected ScalingLimited=true, got %v", hpa.ScalingLimited)
	}
	if snap.HPAsLimited() != 1 {
		t.Fatalf("expected HPAsLimited()=1, got %d", snap.HPAsLimited())
	}
	if matches := snap.HPAsForWorkload("Deployment", "prod", "api"); len(matches) != 1 {
		t.Fatalf("expected HPAsForWorkload match, got %+v", matches)
	}

	if len(snap.PDBs) != 1 || snap.PDBs[0].DisruptionsAllowed != 0 || snap.PDBsAtRisk() != 1 {
		t.Fatalf("unexpected PDBs: %+v (atRisk=%d)", snap.PDBs, snap.PDBsAtRisk())
	}

	if len(snap.ResourceQuotas) != 1 || snap.ResourceQuotas[0].UsedPct != 90.0 || snap.QuotasNearLimit(85.0) != 1 {
		t.Fatalf("unexpected quotas: %+v", snap.ResourceQuotas)
	}

	if len(snap.CronJobs) != 1 || snap.CronJobs[0].Active != 1 || snap.CronJobs[0].LastScheduleTime.Unix() != 1700000000 {
		t.Fatalf("unexpected cronjobs: %+v", snap.CronJobs)
	}

	if got := snap.PodDisruptions["prod/api-0"]; got != "PreemptionByScheduler" {
		t.Fatalf("expected disruption reason PreemptionByScheduler, got %q", got)
	}
	if claims := snap.PodResourceClaims["ml/train-0"]; len(claims) != 1 || claims[0].ResourceClaimName != "gpu-claim-0" {
		t.Fatalf("unexpected DRA claims: %+v", claims)
	}
}

func TestCollector_DirectHTTPScrapeAndSilentDegradation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleKSMMetrics))
	}))
	defer srv.Close()

	c := NewCollector(nil, nil, Config{
		Mode:     ModeAuto,
		Endpoint: srv.URL,
	})
	snap := c.ScrapeOnce(context.Background())
	if snap == nil || !snap.Available || len(snap.HPAs) != 1 {
		t.Fatalf("expected available KSM snapshot, got %+v", snap)
	}
	if c.Latest() == nil || !c.Latest().Available {
		t.Fatal("expected Latest() populated")
	}

	off := NewCollector(nil, nil, Config{Mode: ModeOff, Endpoint: srv.URL})
	if off.Enabled() {
		t.Fatal("expected Enabled() false when ModeOff")
	}
	if got := off.ScrapeOnce(context.Background()); got != nil {
		t.Fatalf("expected nil snapshot when ModeOff, got %+v", got)
	}
}

func TestCollector_DiscoverService(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "monitoring",
			Name:      "prometheus-kube-state-metrics",
			Labels:    map[string]string{"app.kubernetes.io/name": "kube-state-metrics"},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "telemetry", Port: 8081},
				{Name: "http-metrics", Port: 8080},
			},
		},
	}
	fakeClient := fake.NewSimpleClientset(svc)
	c := NewCollector(fakeClient, nil, Config{Mode: ModeAuto})

	target := c.discoverService(context.Background())
	if target == nil {
		t.Fatal("expected KSM service discovered")
	}
	if target.namespace != "monitoring" || target.service != "prometheus-kube-state-metrics" || target.port != "8080" {
		t.Fatalf("unexpected discovered target: %+v", target)
	}
}
