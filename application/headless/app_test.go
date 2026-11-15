package headless

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/collector"
	"github.com/vladimirvivien/ktop/internal/snapshot"
	"github.com/vladimirvivien/ktop/k8s"
	coreV1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func testFakeK8sClient() *k8s.Client {
	ns := &coreV1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	node := &coreV1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a"},
		Status: coreV1.NodeStatus{
			Conditions: []coreV1.NodeCondition{{Type: coreV1.NodeReady, Status: coreV1.ConditionTrue}},
		},
	}
	pod := &coreV1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-0", Namespace: "default"},
		Spec:       coreV1.PodSpec{NodeName: "node-a"},
		Status:     coreV1.PodStatus{Phase: coreV1.PodRunning},
	}
	fakeKube := fake.NewSimpleClientset(ns, node, pod)
	return k8s.NewWithClient(fakeKube, k8s.AllNamespaces, "kind-headless", &version.Info{Major: "1", Minor: "36"})
}

func TestNewRunner_ValidationAndAuthz(t *testing.T) {
	t.Run("invalid output format", func(t *testing.T) {
		_, err := NewRunner(context.Background(), testFakeK8sClient(), nil, Options{OutputFormat: "xml"})
		if err == nil || !strings.Contains(err.Error(), "invalid output format") {
			t.Fatalf("expected invalid output format error, got: %v", err)
		}
	})

	t.Run("authz failure propagated", func(t *testing.T) {
		fakeKube := fake.NewSimpleClientset()
		fakeKube.PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", nil)
		})
		client := k8s.NewWithClient(fakeKube, k8s.AllNamespaces, "ctx", nil)
		_, err := NewRunner(context.Background(), client, nil, Options{OutputFormat: "json"})
		if err == nil || !strings.Contains(err.Error(), "not authorized") {
			t.Fatalf("expected authz error, got: %v", err)
		}
	})
}

func TestAppRun_JSONAndJSONL(t *testing.T) {
	t.Run("single shot json writes only valid json to out", func(t *testing.T) {
		var out, errOut bytes.Buffer
		app, err := NewRunner(context.Background(), testFakeK8sClient(), nil, Options{
			Version:       "v0.7.1",
			OutputFormat:  "json",
			WarmupTimeout: time.Second,
			Out:           &out,
			ErrOut:        &errOut,
		})
		if err != nil {
			t.Fatalf("NewRunner failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := app.Run(ctx); err != nil {
			t.Fatalf("Run failed: %v", err)
		}

		var agg collector.Aggregate
		if err := json.Unmarshal(out.Bytes(), &agg); err != nil {
			t.Fatalf("out is not valid JSON: %v\n%s", err, out.String())
		}
		if agg.Metadata.KtopVersion != "v0.7.1" || agg.Metadata.SampleCount != 1 {
			t.Errorf("unexpected metadata: %+v", agg.Metadata)
		}
		if agg.Summary.HealthScore != 95 {
			t.Errorf("HealthScore = %d, want 95", agg.Summary.HealthScore)
		}
		if errOut.Len() == 0 {
			t.Error("expected progress output on ErrOut when Quiet=false")
		}
	})

	t.Run("windowed jsonl with quiet suppresses errout", func(t *testing.T) {
		var out, errOut bytes.Buffer
		app, err := NewRunner(context.Background(), testFakeK8sClient(), nil, Options{
			Duration:      120 * time.Millisecond,
			Interval:      45 * time.Millisecond,
			WarmupTimeout: time.Second,
			OutputFormat:  "jsonl",
			Quiet:         true,
			Out:           &out,
			ErrOut:        &errOut,
		})
		if err != nil {
			t.Fatalf("NewRunner failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := app.Run(ctx); err != nil {
			t.Fatalf("Run failed: %v", err)
		}

		if errOut.Len() != 0 {
			t.Errorf("expected empty ErrOut with Quiet=true, got %q", errOut.String())
		}
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		if len(lines) < 2 {
			t.Fatalf("expected >= 2 JSONL lines, got %d:\n%s", len(lines), out.String())
		}
		for i, line := range lines {
			var snap snapshot.Snapshot
			if err := json.Unmarshal([]byte(line), &snap); err != nil {
				t.Fatalf("line %d invalid JSON: %v", i, err)
			}
		}
	})
}
