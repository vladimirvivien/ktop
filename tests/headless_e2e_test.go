package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/cmd"
	"github.com/vladimirvivien/ktop/collector"
	"github.com/vladimirvivien/ktop/internal/snapshot"
	"github.com/vladimirvivien/ktop/internal/userdir"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type mockK8sEnv struct {
	server *httptest.Server

	mu              sync.Mutex
	pods            []coreV1.Pod
	forbidPods      bool
	forbidProm      bool
	blockPodsList   bool
	scrapeCount     atomic.Int64
	podWatchChans   map[chan watch.Event]struct{}
	resourceVersion atomic.Int64
}

func newMockK8sEnv(t *testing.T, initialPods []coreV1.Pod) *mockK8sEnv {
	t.Helper()
	env := &mockK8sEnv{
		pods:          append([]coreV1.Pod(nil), initialPods...),
		podWatchChans: make(map[chan watch.Event]struct{}),
	}
	env.resourceVersion.Store(1)

	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"major":      "1",
			"minor":      "36",
			"gitVersion": "v1.36.0",
		})
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"kind":     "APIVersions",
			"versions": []string{"v1"},
		})
	})
	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"kind":       "APIGroupList",
			"apiVersion": "v1",
			"groups":     []any{},
		})
	})

	// Core v1 resources
	mux.HandleFunc("/api/v1/namespaces", env.handleListOrWatch(func() any {
		return map[string]any{
			"apiVersion": "v1",
			"kind":       "NamespaceList",
			"metadata":   map[string]string{"resourceVersion": "1"},
			"items": []coreV1.Namespace{
				{ObjectMeta: metav1.ObjectMeta{Name: "default", ResourceVersion: "1"}},
			},
		}
	}, nil))

	mux.HandleFunc("/api/v1/nodes", env.handleListOrWatch(func() any {
		return map[string]any{
			"apiVersion": "v1",
			"kind":       "NodeList",
			"metadata":   map[string]string{"resourceVersion": "1"},
			"items": []coreV1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "worker-1", ResourceVersion: "1"},
					Status: coreV1.NodeStatus{
						Conditions: []coreV1.NodeCondition{
							{Type: coreV1.NodeReady, Status: coreV1.ConditionTrue},
						},
					},
				},
			},
		}
	}, nil))

	// Prometheus node proxy routes (/api/v1/nodes/<node>/proxy/...)
	mux.HandleFunc("/api/v1/nodes/", env.handleNodeSubpath)

	mux.HandleFunc("/api/v1/pods", env.handlePods)

	// Empty synced lists for remaining core, apps/v1, and batch/v1 informers
	for _, route := range []struct {
		path       string
		apiVersion string
		kind       string
	}{
		{"/api/v1/persistentvolumes", "v1", "PersistentVolumeList"},
		{"/api/v1/persistentvolumeclaims", "v1", "PersistentVolumeClaimList"},
		{"/api/v1/events", "v1", "EventList"},
		{"/apis/apps/v1/deployments", "apps/v1", "DeploymentList"},
		{"/apis/apps/v1/daemonsets", "apps/v1", "DaemonSetList"},
		{"/apis/apps/v1/replicasets", "apps/v1", "ReplicaSetList"},
		{"/apis/apps/v1/statefulsets", "apps/v1", "StatefulSetList"},
		{"/apis/batch/v1/jobs", "batch/v1", "JobList"},
		{"/apis/batch/v1/cronjobs", "batch/v1", "CronJobList"},
	} {
		rt := route
		mux.HandleFunc(rt.path, env.handleListOrWatch(func() any {
			return map[string]any{
				"apiVersion": rt.apiVersion,
				"kind":       rt.kind,
				"metadata":   map[string]string{"resourceVersion": "1"},
				"items":      []any{},
			}
		}, nil))
	}

	env.server = httptest.NewServer(mux)
	t.Cleanup(env.server.Close)
	return env
}

func (e *mockK8sEnv) kubeconfig(t *testing.T) string {
	t.Helper()
	kcPath := filepath.Join(t.TempDir(), "kubeconfig")
	kc := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
  name: e2e-cluster
contexts:
- context:
    cluster: e2e-cluster
    user: e2e-user
  name: e2e-ctx
current-context: e2e-ctx
users:
- name: e2e-user
  user: {}
`, e.server.URL)
	if err := os.WriteFile(kcPath, []byte(kc), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return kcPath
}

func (e *mockK8sEnv) emitPodModified(pod coreV1.Pod) {
	rv := fmt.Sprintf("%d", e.resourceVersion.Add(1))
	pod.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
	pod.ResourceVersion = rv
	cp := pod.DeepCopy()

	e.mu.Lock()
	replaced := false
	for i := range e.pods {
		if e.pods[i].Namespace == pod.Namespace && e.pods[i].Name == pod.Name {
			e.pods[i] = *cp
			replaced = true
			break
		}
	}
	if !replaced {
		e.pods = append(e.pods, *cp)
	}
	var chans []chan watch.Event
	for ch := range e.podWatchChans {
		chans = append(chans, ch)
	}
	e.mu.Unlock()

	evt := watch.Event{Type: watch.Modified, Object: cp}
	for _, ch := range chans {
		select {
		case ch <- evt:
		default:
		}
	}
}

func (e *mockK8sEnv) handlePods(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	forbid := e.forbidPods
	block := e.blockPodsList
	e.mu.Unlock()

	if forbid {
		writeJSON(w, http.StatusForbidden, metav1.Status{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
			Status:   metav1.StatusFailure,
			Message:  "pods is forbidden",
			Reason:   metav1.StatusReasonForbidden,
			Code:     http.StatusForbidden,
		})
		return
	}
	if block {
		<-r.Context().Done()
		return
	}

	if r.URL.Query().Get("watch") == "true" {
		ch := make(chan watch.Event, 8)
		e.mu.Lock()
		e.podWatchChans[ch] = struct{}{}
		podsCopy := append([]coreV1.Pod(nil), e.pods...)
		rv := fmt.Sprintf("%d", e.resourceVersion.Load())
		e.mu.Unlock()
		defer func() {
			e.mu.Lock()
			delete(e.podWatchChans, ch)
			e.mu.Unlock()
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		enc := json.NewEncoder(w)

		if r.URL.Query().Get("sendInitialEvents") == "true" {
			for i := range podsCopy {
				p := podsCopy[i]
				p.TypeMeta = metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"}
				_ = enc.Encode(map[string]any{
					"type":   watch.Added,
					"object": &p,
				})
			}
			_ = enc.Encode(map[string]any{
				"type": watch.Bookmark,
				"object": map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"resourceVersion": rv,
						"annotations": map[string]string{
							metav1.InitialEventsAnnotationKey: "true",
						},
					},
				},
			})
		}

		if flusher != nil {
			flusher.Flush()
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case evt := <-ch:
				_ = enc.Encode(map[string]any{
					"type":   evt.Type,
					"object": evt.Object,
				})
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	}

	e.mu.Lock()
	podsCopy := append([]coreV1.Pod(nil), e.pods...)
	rv := fmt.Sprintf("%d", e.resourceVersion.Load())
	e.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"apiVersion": "v1",
		"kind":       "PodList",
		"metadata":   map[string]string{"resourceVersion": rv},
		"items":      podsCopy,
	})
}

func (e *mockK8sEnv) handleNodeSubpath(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	forbid := e.forbidProm
	e.mu.Unlock()

	if forbid {
		writeJSON(w, http.StatusForbidden, metav1.Status{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
			Status:   metav1.StatusFailure,
			Message:  "nodes/proxy is forbidden",
			Reason:   metav1.StatusReasonForbidden,
			Code:     http.StatusForbidden,
		})
		return
	}

	n := float64(e.scrapeCount.Add(1))
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	if strings.HasSuffix(r.URL.Path, "/proxy/metrics/cadvisor") {
		fmt.Fprintf(w,
			"# TYPE container_cpu_usage_seconds_total counter\n"+
				"container_cpu_usage_seconds_total{id=\"/\",cpu=\"total\"} %.4f\n"+
				"container_cpu_usage_seconds_total{id=\"/kubepods/pod1/web\",namespace=\"default\",pod=\"web-0\",container=\"web\"} %.4f\n"+
				"# TYPE container_memory_working_set_bytes gauge\n"+
				"container_memory_working_set_bytes{id=\"/\"} 536870912\n"+
				"container_memory_working_set_bytes{id=\"/kubepods/pod1/web\",namespace=\"default\",pod=\"web-0\",container=\"web\"} 134217728\n"+
				"# TYPE container_pressure_cpu_waiting_seconds_total counter\n"+
				"container_pressure_cpu_waiting_seconds_total{id=\"/\"} %.4f\n"+
				"container_pressure_cpu_waiting_seconds_total{id=\"/kubepods/pod1/web\",namespace=\"default\",pod=\"web-0\",container=\"web\"} %.4f\n",
			n*0.25, n*0.05, n*0.005, n*0.002,
		)
		return
	}

	// /proxy/metrics (kubelet)
	fmt.Fprint(w,
		"# TYPE kubelet_running_pods gauge\n"+
			"kubelet_running_pods 1\n"+
			"# TYPE container_count gauge\n"+
			"container_count 1\n",
	)
}

func (e *mockK8sEnv) handleListOrWatch(listBody func() any, extra func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if extra != nil {
			extra(w, r)
			return
		}
		if r.URL.Query().Get("watch") == "true" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			flusher, _ := w.(http.Flusher)
			if r.URL.Query().Get("sendInitialEvents") == "true" {
				raw, _ := json.Marshal(listBody())
				var decoded struct {
					APIVersion string `json:"apiVersion"`
					Kind       string `json:"kind"`
					Metadata   struct {
						ResourceVersion string `json:"resourceVersion"`
					} `json:"metadata"`
					Items []json.RawMessage `json:"items"`
				}
				if err := json.Unmarshal(raw, &decoded); err == nil {
					itemKind := strings.TrimSuffix(decoded.Kind, "List")
					enc := json.NewEncoder(w)
					for _, itemRaw := range decoded.Items {
						var obj map[string]any
						if json.Unmarshal(itemRaw, &obj) == nil {
							obj["apiVersion"] = decoded.APIVersion
							obj["kind"] = itemKind
							_ = enc.Encode(map[string]any{
								"type":   watch.Added,
								"object": obj,
							})
						}
					}
					rv := decoded.Metadata.ResourceVersion
					if rv == "" {
						rv = "1"
					}
					_ = enc.Encode(map[string]any{
						"type": watch.Bookmark,
						"object": map[string]any{
							"apiVersion": decoded.APIVersion,
							"kind":       itemKind,
							"metadata": map[string]any{
								"resourceVersion": rv,
								"annotations": map[string]string{
									metav1.InitialEventsAnnotationKey: "true",
								},
							},
						},
					})
				}
			}
			if flusher != nil {
				flusher.Flush()
			}
			<-r.Context().Done()
			return
		}
		writeJSON(w, http.StatusOK, listBody())
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func healthyPodFixture() coreV1.Pod {
	return coreV1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: "web-0", Namespace: "default", ResourceVersion: "1"},
		Spec:       coreV1.PodSpec{NodeName: "worker-1"},
		Status: coreV1.PodStatus{
			Phase: coreV1.PodRunning,
			ContainerStatuses: []coreV1.ContainerStatus{
				{Name: "web", Ready: true, State: coreV1.ContainerState{Running: &coreV1.ContainerStateRunning{}}},
			},
		},
	}
}

func runCLIForTest(t *testing.T, args ...string) (stdout string, stderr string, err error) {
	t.Helper()
	t.Setenv(userdir.EnvVar, t.TempDir())
	var outBuf, errBuf bytes.Buffer
	c := cmd.NewKtopCmd()
	c.SetOut(&outBuf)
	c.SetErr(&errBuf)
	c.SetArgs(args)
	err = c.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestHeadlessE2E_SingleShotPrometheusJSON(t *testing.T) {
	env := newMockK8sEnv(t, []coreV1.Pod{healthyPodFixture()})
	kc := env.kubeconfig(t)

	out, errOut, err := runCLIForTest(t,
		"--noui",
		"--kubeconfig="+kc,
		"--metrics-source=prometheus",
		"-i", "100ms",
	)
	if code := cmd.ExitCode(err); code != cmd.ExitOK {
		t.Fatalf("ExitCode = %d, err = %v, stderr = %s", code, err, errOut)
	}

	var agg collector.Aggregate
	if uErr := json.Unmarshal([]byte(out), &agg); uErr != nil {
		t.Fatalf("stdout is not clean JSON: %v\nstdout:\n%s", uErr, out)
	}
	if agg.Samples != nil {
		t.Errorf("single-shot JSON should omit samples, got %d", len(agg.Samples))
	}
	if agg.Metadata.SampleCount != 1 {
		t.Errorf("SampleCount = %d, want 1", agg.Metadata.SampleCount)
	}
	if !agg.Cluster.Ready || agg.Cluster.MetricsSource != "prometheus" {
		t.Errorf("unexpected cluster metadata: %+v", agg.Cluster)
	}
	if agg.Summary.HealthScore != 100 {
		t.Errorf("HealthScore = %d, want 100", agg.Summary.HealthScore)
	}
	if agg.Summary.UsageCPUMillis <= 0 || agg.Summary.UsageMemoryBytes <= 0 {
		t.Errorf("expected populated Prometheus usage metrics, got cpu=%dm mem=%d",
			agg.Summary.UsageCPUMillis, agg.Summary.UsageMemoryBytes)
	}
	if agg.HealthScoreStats != (collector.HealthScoreStats{Min: 100, Max: 100, Avg: 100, Latest: 100}) {
		t.Errorf("unexpected HealthScoreStats: %+v", agg.HealthScoreStats)
	}
}

func TestHeadlessE2E_HealthScorePenalties(t *testing.T) {
	oomAndCrashPod := coreV1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-0", Namespace: "default", ResourceVersion: "1"},
		Spec:       coreV1.PodSpec{NodeName: "worker-1"},
		Status: coreV1.PodStatus{
			Phase: coreV1.PodRunning,
			ContainerStatuses: []coreV1.ContainerStatus{
				{
					Name:         "worker",
					RestartCount: 3,
					State:        coreV1.ContainerState{Waiting: &coreV1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
					LastTerminationState: coreV1.ContainerState{
						Terminated: &coreV1.ContainerStateTerminated{Reason: "OOMKilled"},
					},
				},
			},
		},
	}

	env := newMockK8sEnv(t, []coreV1.Pod{healthyPodFixture(), oomAndCrashPod})
	kc := env.kubeconfig(t)

	out, errOut, err := runCLIForTest(t,
		"--noui",
		"--kubeconfig="+kc,
		"--metrics-source=none",
	)
	if code := cmd.ExitCode(err); code != cmd.ExitOK {
		t.Fatalf("ExitCode = %d, err = %v, stderr = %s", code, err, errOut)
	}

	var agg collector.Aggregate
	if uErr := json.Unmarshal([]byte(out), &agg); uErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", uErr, out)
	}
	if agg.Summary.OOMKillCount != 1 {
		t.Errorf("OOMKillCount = %d, want 1", agg.Summary.OOMKillCount)
	}
	if agg.Summary.CrashLoopPods != 1 {
		t.Errorf("CrashLoopPods = %d, want 1", agg.Summary.CrashLoopPods)
	}
	// 100 - 15 (OOM/CrashLoop) - 5 (metrics-source=none) = 80
	if agg.Summary.HealthScore != 80 {
		t.Errorf("HealthScore = %d, want 80", agg.Summary.HealthScore)
	}
}

func TestHeadlessE2E_WindowedJSONWithWatchTransition(t *testing.T) {
	pod := healthyPodFixture()
	env := newMockK8sEnv(t, []coreV1.Pod{pod})
	kc := env.kubeconfig(t)

	go func() {
		time.Sleep(240 * time.Millisecond)
		degraded := healthyPodFixture()
		degraded.Status.ContainerStatuses[0].RestartCount = 2
		degraded.Status.ContainerStatuses[0].State = coreV1.ContainerState{
			Waiting: &coreV1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
		}
		env.emitPodModified(degraded)
	}()

	out, errOut, err := runCLIForTest(t,
		"--noui",
		"--kubeconfig="+kc,
		"--metrics-source=none",
		"-d", "650ms",
		"-i", "100ms",
		"-o", "json",
	)
	if code := cmd.ExitCode(err); code != cmd.ExitOK {
		t.Fatalf("ExitCode = %d, err = %v, stderr = %s", code, err, errOut)
	}

	var agg collector.Aggregate
	if uErr := json.Unmarshal([]byte(out), &agg); uErr != nil {
		t.Fatalf("invalid JSON: %v\n%s", uErr, out)
	}
	if len(agg.Samples) < 2 || agg.Metadata.SampleCount != len(agg.Samples) {
		t.Fatalf("expected >= 2 window samples, got len=%d count=%d", len(agg.Samples), agg.Metadata.SampleCount)
	}
	if agg.HealthScoreStats.Max != 95 || agg.HealthScoreStats.Min != 80 || agg.HealthScoreStats.Latest != 80 {
		t.Errorf("expected transition from 95->80, got stats=%+v", agg.HealthScoreStats)
	}
}

func TestHeadlessE2E_StreamingJSONLAndQuiet(t *testing.T) {
	env := newMockK8sEnv(t, []coreV1.Pod{healthyPodFixture()})
	kc := env.kubeconfig(t)

	out, errOut, err := runCLIForTest(t,
		"--noui",
		"--kubeconfig="+kc,
		"--metrics-source=none",
		"-d", "250ms",
		"-i", "80ms",
		"-o", "jsonl",
		"--quiet",
	)
	if code := cmd.ExitCode(err); code != cmd.ExitOK {
		t.Fatalf("ExitCode = %d, err = %v, stderr = %s", code, err, errOut)
	}
	if errOut != "" {
		t.Errorf("expected empty stderr under --quiet, got %q", errOut)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected >= 2 JSONL lines, got %d:\n%s", len(lines), out)
	}
	for i, line := range lines {
		var snap snapshot.Snapshot
		if uErr := json.Unmarshal([]byte(line), &snap); uErr != nil {
			t.Fatalf("line %d invalid JSON: %v\nline: %s", i, uErr, line)
		}
		if snap.Summary.HealthScore != 95 {
			t.Errorf("line %d HealthScore = %d, want 95", i, snap.Summary.HealthScore)
		}
	}
}

func TestHeadlessE2E_OperationalExitCodes(t *testing.T) {
	t.Run("exit 2 on invalid flag", func(t *testing.T) {
		_, _, err := runCLIForTest(t, "--noui", "-o", "yaml")
		if got := cmd.ExitCode(err); got != cmd.ExitInvalidFlags {
			t.Fatalf("ExitCode = %d, want %d (err=%v)", got, cmd.ExitInvalidFlags, err)
		}
	})

	t.Run("exit 3 on unreachable cluster", func(t *testing.T) {
		env := newMockK8sEnv(t, nil)
		kc := env.kubeconfig(t)
		env.server.Close()

		_, _, err := runCLIForTest(t, "--noui", "--kubeconfig="+kc, "--metrics-source=none")
		if got := cmd.ExitCode(err); got != cmd.ExitK8sConnectError {
			t.Fatalf("ExitCode = %d, want %d (err=%v)", got, cmd.ExitK8sConnectError, err)
		}
	})

	t.Run("exit 3 on forbidden pods authz", func(t *testing.T) {
		env := newMockK8sEnv(t, nil)
		env.mu.Lock()
		env.forbidPods = true
		env.mu.Unlock()
		kc := env.kubeconfig(t)

		_, _, err := runCLIForTest(t, "--noui", "--kubeconfig="+kc, "--metrics-source=none")
		if got := cmd.ExitCode(err); got != cmd.ExitK8sConnectError {
			t.Fatalf("ExitCode = %d, want %d (err=%v)", got, cmd.ExitK8sConnectError, err)
		}
	})

	t.Run("exit 4 on required prometheus unavailable", func(t *testing.T) {
		env := newMockK8sEnv(t, []coreV1.Pod{healthyPodFixture()})
		env.mu.Lock()
		env.forbidProm = true
		env.mu.Unlock()
		kc := env.kubeconfig(t)

		_, _, err := runCLIForTest(t, "--noui", "--kubeconfig="+kc, "--metrics-source=prometheus")
		if got := cmd.ExitCode(err); got != cmd.ExitMetricsUnavailable {
			t.Fatalf("ExitCode = %d, want %d (err=%v)", got, cmd.ExitMetricsUnavailable, err)
		}
	})

	t.Run("exit 5 on collection timeout", func(t *testing.T) {
		env := newMockK8sEnv(t, []coreV1.Pod{healthyPodFixture()})
		env.mu.Lock()
		env.blockPodsList = true
		env.mu.Unlock()
		kc := env.kubeconfig(t)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ctx

		_, _, err := runCLIForTest(t, "--noui", "--kubeconfig="+kc, "--metrics-source=none", "--request-timeout=80ms")
		if got := cmd.ExitCode(err); got != cmd.ExitCollectionTimeout {
			t.Fatalf("ExitCode = %d, want %d (err=%v)", got, cmd.ExitCollectionTimeout, err)
		}
	})
}

func TestHeadlessSubprocess_NonTTY(t *testing.T) {
	if rawArgs := os.Getenv("KTOP_E2E_SUBPROCESS_ARGS"); rawArgs != "" {
		var args []string
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			os.Exit(99)
		}
		c := cmd.NewKtopCmd()
		c.SetArgs(args)
		if err := c.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(cmd.ExitCode(err))
		}
		os.Exit(0)
	}

	env := newMockK8sEnv(t, []coreV1.Pod{healthyPodFixture()})
	kc := env.kubeconfig(t)

	encodedArgs, err := json.Marshal([]string{
		"--noui",
		"--kubeconfig=" + kc,
		"--metrics-source=none",
		"--quiet",
	})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}

	proc := exec.Command(os.Args[0], "-test.run=^TestHeadlessSubprocess_NonTTY$")
	proc.Env = append(os.Environ(),
		userdir.EnvVar+"="+t.TempDir(),
		"KTOP_E2E_SUBPROCESS_ARGS="+string(encodedArgs),
	)
	proc.Stdin = nil // Verify execution with no TTY / closed stdin
	var stdout, stderr bytes.Buffer
	proc.Stdout = &stdout
	proc.Stderr = &stderr

	if err := proc.Run(); err != nil {
		t.Fatalf("subprocess failed: %v\nstderr:\n%s\nstdout:\n%s", err, stderr.String(), stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr with --quiet, got %q", stderr.String())
	}
	var agg collector.Aggregate
	if err := json.Unmarshal(stdout.Bytes(), &agg); err != nil {
		t.Fatalf("subprocess stdout invalid JSON: %v\n%s", err, stdout.String())
	}
	if agg.Summary.HealthScore != 95 {
		t.Errorf("subprocess HealthScore = %d, want 95", agg.Summary.HealthScore)
	}
}
