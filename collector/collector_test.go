package collector

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/internal/snapshot"
	"github.com/vladimirvivien/ktop/k8s"
	coreV1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/fake"
)

type fakeProvider struct {
	waitDelay time.Duration
	waitErr   error
	scores    []int
	idx       atomic.Int32
}

func (f *fakeProvider) WaitReady(ctx context.Context) error {
	if f.waitDelay > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(f.waitDelay):
		}
	}
	return f.waitErr
}

func (f *fakeProvider) Capture(ctx context.Context) (*snapshot.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	n := int(f.idx.Add(1)) - 1
	score := 100
	if len(f.scores) > 0 {
		if n < len(f.scores) {
			score = f.scores[n]
		} else {
			score = f.scores[len(f.scores)-1]
		}
	}
	return &snapshot.Snapshot{
		Timestamp: time.Now().UTC(),
		Cluster: snapshot.ClusterInfo{
			Context:       "dev-ctx",
			MetricsSource: "none",
		},
		Summary: snapshot.SummaryData{
			HealthScore: score,
			NodesCount:  1,
			NodesReady:  1,
		},
	}, nil
}

func TestCollector_SingleShot(t *testing.T) {
	var progress bytes.Buffer
	c := New(&fakeProvider{scores: []int{95}}, Options{
		Version:        "v0.7.1",
		ProgressWriter: &progress,
	})

	agg, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if agg.Metadata.SampleCount != 1 {
		t.Errorf("SampleCount = %d, want 1", agg.Metadata.SampleCount)
	}
	if agg.Samples != nil {
		t.Errorf("expected nil Samples in single-shot mode, got %d", len(agg.Samples))
	}
	if !agg.Cluster.Ready {
		t.Error("expected Cluster.Ready=true")
	}
	if agg.Summary.HealthScore != 95 {
		t.Errorf("Summary.HealthScore = %d, want 95", agg.Summary.HealthScore)
	}
	if agg.HealthScoreStats != (HealthScoreStats{Min: 95, Max: 95, Avg: 95, Latest: 95}) {
		t.Errorf("unexpected HealthScoreStats: %+v", agg.HealthScoreStats)
	}
	if !strings.Contains(progress.String(), "waiting for cluster readiness") {
		t.Errorf("expected progress message, got %q", progress.String())
	}
}

func TestCollector_WindowAndHealthScoreStats(t *testing.T) {
	var streamed int
	var progress bytes.Buffer
	c := New(&fakeProvider{scores: []int{100, 80, 90}}, Options{
		Version:        "v0.7.1",
		Duration:       110 * time.Millisecond,
		Interval:       40 * time.Millisecond,
		Quiet:          true,
		ProgressWriter: &progress,
		OnSample: func(s *snapshot.Snapshot) error {
			streamed++
			return nil
		},
	})

	agg, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect() unexpected error: %v", err)
	}
	if streamed < 2 || len(agg.Samples) != streamed || agg.Metadata.SampleCount != streamed {
		t.Fatalf("expected >= 2 streamed samples, streamed=%d len=%d count=%d",
			streamed, len(agg.Samples), agg.Metadata.SampleCount)
	}
	if progress.Len() != 0 {
		t.Errorf("expected no progress output with Quiet=true, got %q", progress.String())
	}
	if agg.HealthScoreStats.Min > agg.HealthScoreStats.Max {
		t.Errorf("invalid HealthScoreStats min/max: %+v", agg.HealthScoreStats)
	}
	if agg.HealthScoreStats.Min != 80 {
		t.Errorf("HealthScoreStats.Min = %d, want 80 (stats=%+v)", agg.HealthScoreStats.Min, agg.HealthScoreStats)
	}
	if agg.HealthScoreStats.Max != 100 {
		t.Errorf("HealthScoreStats.Max = %d, want 100", agg.HealthScoreStats.Max)
	}
}

func TestCollector_CancelMidWindowPreservesSamples(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
	defer cancel()

	c := New(&fakeProvider{scores: []int{95, 85}}, Options{
		Duration: 2 * time.Second,
		Interval: 40 * time.Millisecond,
	})

	agg, err := c.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect() unexpected error on mid-window cancellation: %v", err)
	}
	if agg.Metadata.SampleCount < 1 {
		t.Fatalf("expected >= 1 sample on clean mid-window cancellation, got %d", agg.Metadata.SampleCount)
	}
}

func TestCollector_WarmupTimeout(t *testing.T) {
	c := New(&fakeProvider{waitDelay: 500 * time.Millisecond}, Options{
		WarmupTimeout: 30 * time.Millisecond,
	})

	_, err := c.Collect(context.Background())
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("Collect() error = %v, want ErrTimeout", err)
	}
}

func TestCollector_OnSampleErrorAborts(t *testing.T) {
	wantErr := errors.New("broken pipe")
	c := New(&fakeProvider{}, Options{
		OnSample: func(*snapshot.Snapshot) error { return wantErr },
	})
	_, err := c.Collect(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Collect() error = %v, want %v", err, wantErr)
	}
}

func TestK8sSnapshotProvider_WithFakeClientset(t *testing.T) {
	ns := &coreV1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	node := &coreV1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Status: coreV1.NodeStatus{
			Conditions: []coreV1.NodeCondition{{Type: coreV1.NodeReady, Status: coreV1.ConditionTrue}},
		},
	}
	pod := &coreV1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web-0", Namespace: "default"},
		Spec:       coreV1.PodSpec{NodeName: "worker-1"},
		Status:     coreV1.PodStatus{Phase: coreV1.PodRunning},
	}

	fakeKube := fake.NewSimpleClientset(ns, node, pod)
	client := k8s.NewWithClient(fakeKube, k8s.AllNamespaces, "kind-test", &version.Info{Major: "1", Minor: "36"})
	provider := NewK8sSnapshotProvider(client, nil, time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	agg, err := New(provider, Options{Version: "v0.7.1", WarmupTimeout: time.Second}).Collect(ctx)
	if err != nil {
		t.Fatalf("Collect() failed: %v", err)
	}
	if agg.Cluster.MetricsSource != "none" || agg.Cluster.Context != "kind-test" || !agg.Cluster.Ready {
		t.Errorf("unexpected cluster metadata: %+v", agg.Cluster)
	}
	if agg.Summary.NodesCount != 1 || agg.Summary.PodsRunning != 1 || agg.Summary.HealthScore != 95 {
		t.Errorf("unexpected summary: %+v", agg.Summary)
	}
}
