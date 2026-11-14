package tui

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gdamore/tcell/v2"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/overview"
)

// Options configures the interactive TUI application runner.
type Options struct {
	Theme          string
	ColorMode      string
	ShowAllColumns bool
	NodeColumns    []string
	PodColumns     []string
	NodeSortCol    string
	NodeSortAsc    bool
	PodSortCol     string
	PodSortAsc     bool
}

// App wraps *application.Application to implement application.Runner for TUI mode.
type App struct {
	inner *application.Application
}

// Compile-time check that *App implements application.Runner.
var _ application.Runner = (*App)(nil)

// NewRunner wires the interactive TUI application, overview page, health
// tracker, and pre-flight checks.
func NewRunner(ctx context.Context, k8sC *k8s.Client, metricsSource metrics.MetricsSource, opts Options) (*App, error) {
	if _, err := ui.ResolveAndApplyTheme(opts.Theme, opts.ColorMode); err != nil {
		return nil, fmt.Errorf("resolve UI theme: %w", err)
	}

	app := application.New(k8sC, metricsSource)
	k8sC.Controller().SetHealthTracker(app.GetAPIHealthTracker())
	app.WelcomeBanner()

	overviewPage := overview.NewWithColumnOptions(app, "Overview", opts.ShowAllColumns, opts.NodeColumns, opts.PodColumns)
	if opts.NodeSortCol != "" || opts.PodSortCol != "" {
		overviewPage.SetInitialSort(opts.NodeSortCol, opts.NodeSortAsc, opts.PodSortCol, opts.PodSortAsc)
	}
	app.AddPage(overviewPage)

	if err := k8sC.AssertCoreAuthz(ctx); err != nil {
		slog.Error("kubernetes authorization check failed", "error", err)
		return nil, fmt.Errorf("ktop: %w", err)
	}
	slog.Info("kubernetes authorization checks passed")

	screen, err := tcell.NewScreen()
	if err == nil {
		if err := screen.Init(); err == nil {
			_, height := screen.Size()
			screen.Fini()
			if height < ui.MinTerminalHeight {
				slog.Error("terminal too small", "height", height, "min", ui.MinTerminalHeight)
				return nil, fmt.Errorf("terminal height too small (%d rows). Minimum required: %d rows", height, ui.MinTerminalHeight)
			}
		}
	}

	return &App{inner: app}, nil
}

// Run starts the TUI event loop and blocks until ctx is cancelled or the user quits.
func (a *App) Run(ctx context.Context) error {
	return a.inner.Run(ctx)
}

// Stop stops the TUI event loop.
func (a *App) Stop() error {
	return a.inner.Stop()
}
