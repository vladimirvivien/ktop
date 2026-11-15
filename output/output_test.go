package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vladimirvivien/ktop/collector"
	"github.com/vladimirvivien/ktop/internal/snapshot"
)

func sampleAggregate(includeSamples bool) *collector.Aggregate {
	snap1 := &snapshot.Snapshot{
		Timestamp: time.Unix(1700000000, 0).UTC(),
		Cluster: snapshot.ClusterInfo{
			Context:       "prod",
			MetricsSource: "prometheus",
			Ready:         true,
		},
		Summary: snapshot.SummaryData{
			HealthScore: 95,
			NodesCount:  1,
			NodesReady:  1,
		},
		Nodes: []snapshot.NodeData{{Name: "n1", Status: "Ready"}},
		Pods:  []snapshot.PodData{{Namespace: "default", Name: "api-0", Status: "Running"}},
	}
	agg := &collector.Aggregate{
		Metadata: collector.ReportMetadata{
			KtopVersion: "v0.7.1",
			SampleCount: 1,
		},
		Cluster:          snap1.Cluster,
		Summary:          snap1.Summary,
		HealthScoreStats: collector.HealthScoreStats{Min: 95, Max: 95, Avg: 95, Latest: 95},
		Nodes:            snap1.Nodes,
		Pods:             snap1.Pods,
	}
	if includeSamples {
		snap2 := snap1.Clone()
		snap2.Summary.HealthScore = 80
		agg.Metadata.SampleCount = 2
		agg.Metadata.DurationSeconds = 5
		agg.Metadata.IntervalSeconds = 5
		agg.HealthScoreStats = collector.HealthScoreStats{Min: 80, Max: 95, Avg: 87.5, Latest: 80}
		agg.Summary = snap2.Summary
		agg.Samples = []*snapshot.Snapshot{snap1, snap2}
	}
	return agg
}

func TestNewFormatter(t *testing.T) {
	if _, err := NewFormatter(""); err != nil {
		t.Fatalf("empty format should default to json: %v", err)
	}
	if _, err := NewFormatter("JSON"); err != nil {
		t.Fatalf("JSON format should succeed: %v", err)
	}
	if _, err := NewFormatter("jsonl"); err != nil {
		t.Fatalf("jsonl format should succeed: %v", err)
	}
	if _, err := NewFormatter("yaml"); err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestJSONFormatter(t *testing.T) {
	f := &JSONFormatter{}

	t.Run("single shot omits samples", func(t *testing.T) {
		var buf bytes.Buffer
		agg := sampleAggregate(false)
		if err := f.WriteSample(&buf, &snapshot.Snapshot{Summary: agg.Summary}); err != nil {
			t.Fatalf("WriteSample failed: %v", err)
		}
		if buf.Len() != 0 {
			t.Fatalf("expected WriteSample to be a no-op, wrote %q", buf.String())
		}
		if err := f.WriteFinal(&buf, agg); err != nil {
			t.Fatalf("WriteFinal failed: %v", err)
		}

		var raw map[string]any
		if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
			t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
		}
		if _, hasSamples := raw["samples"]; hasSamples {
			t.Error("expected samples field to be omitted on single-shot aggregate")
		}
		if _, hasStats := raw["health_score_stats"]; !hasStats {
			t.Error("expected health_score_stats field in JSON output")
		}
	})

	t.Run("windowed includes samples", func(t *testing.T) {
		var buf bytes.Buffer
		agg := sampleAggregate(true)
		if err := f.WriteFinal(&buf, agg); err != nil {
			t.Fatalf("WriteFinal failed: %v", err)
		}

		var decoded collector.Aggregate
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if len(decoded.Samples) != 2 || decoded.HealthScoreStats.Min != 80 || decoded.HealthScoreStats.Latest != 80 {
			t.Errorf("unexpected decoded aggregate: %+v", decoded)
		}
	})
}

type flushRecorder struct {
	bytes.Buffer
	flushes int
}

func (f *flushRecorder) Flush() error {
	f.flushes++
	return nil
}

func TestJSONLFormatter(t *testing.T) {
	f := &JSONLFormatter{}
	rec := &flushRecorder{}
	agg := sampleAggregate(true)

	for _, s := range agg.Samples {
		if err := f.WriteSample(rec, s); err != nil {
			t.Fatalf("WriteSample failed: %v", err)
		}
	}
	if err := f.WriteFinal(rec, agg); err != nil {
		t.Fatalf("WriteFinal failed: %v", err)
	}
	if rec.flushes != 2 {
		t.Errorf("Flush() count = %d, want 2", rec.flushes)
	}

	lines := strings.Split(strings.TrimSpace(rec.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d:\n%s", len(lines), rec.String())
	}
	for i, line := range lines {
		var snap snapshot.Snapshot
		if err := json.Unmarshal([]byte(line), &snap); err != nil {
			t.Fatalf("line %d is invalid JSON: %v", i, err)
		}
		if snap.Summary.HealthScore == 0 {
			t.Errorf("line %d missing health_score", i)
		}
	}
}
