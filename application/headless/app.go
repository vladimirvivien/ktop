package headless

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/collector"
	"github.com/vladimirvivien/ktop/internal/snapshot"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/output"
)

// Options configures the non-interactive headless application runner.
type Options struct {
	Duration      time.Duration
	Interval      time.Duration
	WarmupTimeout time.Duration
	OutputFormat  string
	Quiet         bool
	Version       string
	Out           io.Writer
	ErrOut        io.Writer
}

// App implements application.Runner without allocating a terminal screen.
type App struct {
	k8sC          *k8s.Client
	metricsSource metrics.MetricsSource
	formatter     output.Formatter
	opts          Options

	mu     sync.Mutex
	cancel context.CancelFunc
}

var _ application.Runner = (*App)(nil)

// NewRunner validates headless options, checks core Kubernetes authorization,
// and prepares the headless runner.
func NewRunner(ctx context.Context, k8sC *k8s.Client, metricsSource metrics.MetricsSource, opts Options) (*App, error) {
	if k8sC == nil {
		return nil, errors.New("kubernetes client cannot be nil")
	}

	formatter, err := output.NewFormatter(opts.OutputFormat)
	if err != nil {
		return nil, err
	}

	if err := k8sC.AssertCoreAuthz(ctx); err != nil {
		slog.Error("kubernetes authorization check failed", "error", err)
		return nil, fmt.Errorf("ktop: %w", err)
	}
	slog.Info("kubernetes authorization checks passed")

	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.ErrOut == nil {
		opts.ErrOut = os.Stderr
	}

	return &App{
		k8sC:          k8sC,
		metricsSource: metricsSource,
		formatter:     formatter,
		opts:          opts,
	}, nil
}

// Run collects cluster snapshots and emits structured output to Out until
// completion or context cancellation.
func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	runCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		a.cancel = nil
		a.mu.Unlock()
	}()

	provider := collector.NewK8sSnapshotProvider(a.k8sC, a.metricsSource, a.opts.WarmupTimeout)
	col := collector.New(provider, collector.Options{
		Version:        a.opts.Version,
		Duration:       a.opts.Duration,
		Interval:       a.opts.Interval,
		WarmupTimeout:  a.opts.WarmupTimeout,
		Quiet:          a.opts.Quiet,
		ProgressWriter: a.opts.ErrOut,
		OnSample: func(s *snapshot.Snapshot) error {
			return a.formatter.WriteSample(a.opts.Out, s)
		},
	})

	agg, err := col.Collect(runCtx)
	if err != nil {
		return err
	}
	return a.formatter.WriteFinal(a.opts.Out, agg)
}

// Stop cancels an in-progress headless collection run.
func (a *App) Stop() error {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}
