package collector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/vladimirvivien/ktop/internal/snapshot"
)

// ErrTimeout indicates readiness or collection warmup exceeded the allotted deadline.
var ErrTimeout = errors.New("collection timed out")

const (
	defaultInterval      = 5 * time.Second
	defaultWarmupTimeout = 20 * time.Second
)

// SnapshotProvider supplies readiness synchronization and point-in-time
// cluster snapshots for headless collection.
type SnapshotProvider interface {
	WaitReady(ctx context.Context) error
	Capture(ctx context.Context) (*snapshot.Snapshot, error)
}

// ReportMetadata summarizes the collection window for structured reports.
type ReportMetadata struct {
	KtopVersion     string    `json:"ktop_version"`
	CollectionStart time.Time `json:"collection_start"`
	CollectionEnd   time.Time `json:"collection_end"`
	DurationSeconds float64   `json:"duration_seconds"`
	IntervalSeconds float64   `json:"interval_seconds"`
	SampleCount     int       `json:"sample_count"`
}

// HealthScoreStats aggregates cluster health_score (0-100) across a window.
type HealthScoreStats struct {
	Min    int     `json:"min"`
	Max    int     `json:"max"`
	Avg    float64 `json:"avg"`
	Latest int     `json:"latest"`
}

// Aggregate is the top-level JSON report produced by headless mode.
type Aggregate struct {
	Metadata         ReportMetadata       `json:"metadata"`
	Cluster          snapshot.ClusterInfo `json:"cluster"`
	Summary          snapshot.SummaryData `json:"summary"`
	HealthScoreStats HealthScoreStats     `json:"health_score_stats"`
	Nodes            []snapshot.NodeData  `json:"nodes"`
	Pods             []snapshot.PodData   `json:"pods"`
	Samples          []*snapshot.Snapshot `json:"samples,omitempty"`
}

// Options configures a Collector run.
type Options struct {
	Version        string
	Duration       time.Duration
	Interval       time.Duration
	WarmupTimeout  time.Duration
	Quiet          bool
	ProgressWriter io.Writer
	OnSample       func(*snapshot.Snapshot) error
}

// Collector orchestrates readiness waiting and periodic snapshot capture.
type Collector struct {
	provider SnapshotProvider
	opts     Options
}

// New constructs a Collector with the given SnapshotProvider and Options.
func New(provider SnapshotProvider, opts Options) *Collector {
	return &Collector{
		provider: provider,
		opts:     opts,
	}
}

// Collect waits for cluster readiness, captures one snapshot (Duration <= 0) or
// periodic snapshots (Duration > 0), and builds the final Aggregate report.
func (c *Collector) Collect(ctx context.Context) (*Aggregate, error) {
	if c.provider == nil {
		return nil, errors.New("snapshot provider is nil")
	}

	warmupTimeout := c.opts.WarmupTimeout
	if warmupTimeout <= 0 {
		warmupTimeout = defaultWarmupTimeout
	}
	interval := c.opts.Interval
	if interval <= 0 {
		interval = defaultInterval
	}

	c.progress("waiting for cluster readiness...")
	warmupCtx, warmupCancel := context.WithTimeout(ctx, warmupTimeout)
	defer warmupCancel()

	if err := c.provider.WaitReady(warmupCtx); err != nil {
		if errors.Is(err, ErrTimeout) || (ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded)) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return nil, err
	}

	start := time.Now().UTC()
	first, err := c.captureSample(ctx)
	if err != nil {
		return nil, err
	}
	samples := []*snapshot.Snapshot{first}

	if c.opts.Duration <= 0 {
		end := time.Now().UTC()
		return c.buildAggregate(start, end, 0, samples, false), nil
	}

	c.progress("collecting samples for %s every %s...", c.opts.Duration, interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	windowTimer := time.NewTimer(c.opts.Duration)
	defer windowTimer.Stop()

collectLoop:
	for {
		select {
		case <-ctx.Done():
			break collectLoop
		case <-windowTimer.C:
			break collectLoop
		case <-ticker.C:
			// Check window expiration first when ticker and timer fire simultaneously.
			select {
			case <-ctx.Done():
				break collectLoop
			case <-windowTimer.C:
				break collectLoop
			default:
			}

			snap, err := c.captureSample(ctx)
			if err != nil {
				if ctx.Err() != nil {
					break collectLoop
				}
				return nil, err
			}
			samples = append(samples, snap)
		}
	}

	end := time.Now().UTC()
	return c.buildAggregate(start, end, interval.Seconds(), samples, true), nil
}

func (c *Collector) captureSample(ctx context.Context) (*snapshot.Snapshot, error) {
	snap, err := c.provider.Capture(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrTimeout) {
			return nil, fmt.Errorf("%w: %v", ErrTimeout, err)
		}
		return nil, err
	}
	if snap == nil {
		return nil, errors.New("captured nil snapshot")
	}
	snap.Cluster.Ready = true
	if c.opts.OnSample != nil {
		if err := c.opts.OnSample(snap); err != nil {
			return nil, err
		}
	}
	return snap, nil
}

func (c *Collector) buildAggregate(start, end time.Time, intervalSec float64, samples []*snapshot.Snapshot, includeSamples bool) *Aggregate {
	latest := samples[len(samples)-1]
	durationSec := 0.0
	if includeSamples {
		durationSec = math.Round(end.Sub(start).Seconds()*1000) / 1000
	}

	agg := &Aggregate{
		Metadata: ReportMetadata{
			KtopVersion:     c.opts.Version,
			CollectionStart: start,
			CollectionEnd:   end,
			DurationSeconds: durationSec,
			IntervalSeconds: intervalSec,
			SampleCount:     len(samples),
		},
		Cluster:          latest.Cluster,
		Summary:          latest.Summary,
		HealthScoreStats: ComputeHealthScoreStats(samples),
		Nodes:            latest.Nodes,
		Pods:             latest.Pods,
	}
	if includeSamples {
		agg.Samples = samples
	}
	return agg
}

// ComputeHealthScoreStats calculates min, max, rounded average, and latest
// health_score values across the collected snapshots.
func ComputeHealthScoreStats(samples []*snapshot.Snapshot) HealthScoreStats {
	if len(samples) == 0 {
		return HealthScoreStats{}
	}
	minScore := samples[0].Summary.HealthScore
	maxScore := minScore
	sum := 0
	for _, s := range samples {
		score := s.Summary.HealthScore
		if score < minScore {
			minScore = score
		}
		if score > maxScore {
			maxScore = score
		}
		sum += score
	}
	avg := math.Round((float64(sum)/float64(len(samples)))*100) / 100
	return HealthScoreStats{
		Min:    minScore,
		Max:    maxScore,
		Avg:    avg,
		Latest: samples[len(samples)-1].Summary.HealthScore,
	}
}

func (c *Collector) progress(format string, args ...any) {
	if c.opts.Quiet || c.opts.ProgressWriter == nil {
		return
	}
	_, _ = fmt.Fprintf(c.opts.ProgressWriter, "ktop: "+format+"\n", args...)
}
