package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/application/headless"
	"github.com/vladimirvivien/ktop/application/tui"
	"github.com/vladimirvivien/ktop/buildinfo"
	"github.com/vladimirvivien/ktop/config"
	"github.com/vladimirvivien/ktop/internal/logging"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	k8sMetrics "github.com/vladimirvivien/ktop/metrics/k8s"
	promMetrics "github.com/vladimirvivien/ktop/metrics/prom"
	"github.com/vladimirvivien/ktop/output"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

var (
	examples = `
# Start ktop using default configuration for the "default" namespace
%[1]s

# Start ktop with default configuration for all accessible namespaces
%[1]s -A

# Start ktop for a specific namespace in current context
%[1]s --namespace <namespace>

# Start ktop for a specific namespace and context
%[1]s --namespace <namespace> --context <context>

# Emit a single structured JSON snapshot in non-interactive mode
%[1]s --noui
`
)

type ktopCmdOptions struct {
	namespace      string
	allNamespaces  bool
	context        string
	kubeconfig     string
	kubeFlags      *genericclioptions.ConfigFlags
	page           string
	nodeColumns    string // comma-separated list of node columns to display
	podColumns     string // comma-separated list of pod columns to display
	showAllColumns bool   // show all columns
	sortNodes      string // initial sort column for nodes table (+NAME, -CPU)
	sortPods       string // initial sort column for pods table (+POD, -MEMORY)

	// Metrics configuration
	metricsSource            string
	prometheusScrapeInterval string
	prometheusRetention      string
	prometheusMaxSamples     int
	prometheusComponents     []string
	ksmMode                  string
	ksmEndpoint              string

	// UI appearance configuration
	theme     string
	colorMode string

	// Logging configuration
	logLevel  string
	logFormat string
	logDest   string

	// Non-interactive headless mode configuration
	noui         bool
	duration     time.Duration
	interval     time.Duration
	outputFormat string
	quiet        bool
}

// NewKtopCmd returns a command for ktop
func NewKtopCmd() *cobra.Command {
	o := &ktopCmdOptions{kubeFlags: genericclioptions.NewConfigFlags(false)}
	program := filepath.Base(os.Args[0])
	pluginMode := strings.HasPrefix(program, "kubectl-")
	usage := fmt.Sprintf("%s [flags]", program)
	shortDesc := fmt.Sprintf("Runs %s (standalone)", program)
	if pluginMode {
		shortDesc = fmt.Sprintf("Runs %s as kubectl plugin", program)
	}

	cmd := &cobra.Command{
		Use:           usage,
		Short:         shortDesc,
		Example:       fmt.Sprintf(examples, program),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(c *cobra.Command, args []string) error {
			if err := cobra.NoArgs(c, args); err != nil {
				return WrapExitError(ExitInvalidFlags, err)
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			return o.runKtop(c, args)
		},
	}
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return WrapExitError(ExitInvalidFlags, err)
	})

	cmd.Flags().BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "If true, display metrics for all accessible namespaces")
	cmd.Flags().StringVar(&o.nodeColumns, "node-columns", "", "Comma-separated list of node columns to display (e.g. 'NAME,CPU,MEM')")
	cmd.Flags().StringVar(&o.podColumns, "pod-columns", "", "Comma-separated list of pod columns to display (e.g. 'NAMESPACE,POD,STATUS')")
	cmd.Flags().BoolVar(&o.showAllColumns, "show-all-columns", false, "If true, show all columns")
	cmd.Flags().StringVar(&o.sortNodes, "sort-nodes", "", "Initial sort column for nodes table, prefix with + for asc, - for desc (e.g. '+NAME', '-CPU')")
	cmd.Flags().StringVar(&o.sortPods, "sort-pods", "", "Initial sort column for pods table, prefix with + for asc, - for desc (e.g. '-MEMORY', '+POD')")
	cmd.Flags().StringVar(&o.theme, "theme", "auto", "UI color theme (auto, dark, light, ansi, monochrome)")
	cmd.Flags().StringVar(&o.colorMode, "color-mode", "auto", "Terminal color depth override (auto, truecolor, 256, 16, none)")
	cmd.Flags().StringVar(&o.page, "page", "overview", "Initial TUI page to display (overview, workloads, events)")

	// Headless / non-interactive flags
	cmd.Flags().BoolVar(&o.noui, "noui", false, "Disable interactive terminal UI and write structured output to stdout")
	cmd.Flags().DurationVarP(&o.duration, "duration", "d", 0, "Collection window duration in --noui mode (e.g., 30s, 5m; 0 for single snapshot)")
	cmd.Flags().DurationVarP(&o.interval, "interval", "i", 5*time.Second, "Sampling interval between snapshots in --noui mode")
	cmd.Flags().StringVarP(&o.outputFormat, "output", "o", "json", "Output format in --noui mode: json or jsonl")
	cmd.Flags().BoolVar(&o.quiet, "quiet", false, "Suppress progress messages on stderr in --noui mode")

	// Metrics source flags
	cmd.Flags().StringVar(&o.metricsSource, "metrics-source", "prometheus",
		"Metrics source: 'prom'/'prometheus' (default), 'metrics-server', 'none'")
	cmd.Flags().StringVar(&o.prometheusScrapeInterval, "prometheus-scrape-interval", "5s",
		"Prometheus scrape interval (e.g., 10s, 30s, 1m)")
	cmd.Flags().StringVar(&o.prometheusRetention, "prometheus-retention", "1h",
		"Prometheus metrics retention time (e.g., 30m, 1h, 2h)")
	cmd.Flags().IntVar(&o.prometheusMaxSamples, "prometheus-max-samples", 10000,
		"Maximum samples per time series")
	cmd.Flags().StringSliceVar(&o.prometheusComponents, "prometheus-components",
		[]string{"kubelet", "cadvisor"},
		"Kubernetes components to scrape (comma-separated: kubelet,cadvisor,apiserver,etcd,scheduler,controller-manager,kube-proxy)")
	cmd.Flags().StringVar(&o.ksmMode, "ksm", "auto",
		"kube-state-metrics enrichment mode: auto (default) or off")
	cmd.Flags().StringVar(&o.ksmEndpoint, "ksm-endpoint", "",
		"Explicit kube-state-metrics endpoint (URL or namespace/service[:port])")

	// Logging flags
	cmd.Flags().StringVar(&o.logLevel, "log-level", "info",
		"Log verbosity: debug, info, warn, error")
	cmd.Flags().StringVar(&o.logFormat, "log-format", "text",
		"Log record format: text or json")
	cmd.Flags().StringVar(&o.logDest, "log", string(logging.DestFile),
		"Log destination: file (~/.ktop/ktop.log) or stderr")

	o.kubeFlags.AddFlags(cmd.Flags())
	return cmd
}

// tryPrometheus attempts to create, start, and verify a prometheus metrics source.
// It performs a connectivity test FIRST before starting the expensive collection.
func tryPrometheus(ctx context.Context, restConfig *rest.Config, cfg *promMetrics.PromConfig) (*promMetrics.PromMetricsSource, error) {
	source, err := promMetrics.NewPromMetricsSource(restConfig, cfg)
	if err != nil {
		return nil, err
	}
	// TEST FIRST - quick connectivity check before starting expensive collection
	// This prevents hanging on Start() if the cluster is unreachable
	if err := source.TestConnection(ctx); err != nil {
		return nil, err
	}
	// THEN start collection (now non-blocking)
	if err := source.Start(ctx); err != nil {
		source.Stop()
		return nil, err
	}
	return source, nil
}

// selectMetricsSource selects and initializes the metrics source.
// When enableFallback is true and prometheus fails, it falls back to metrics-server.
func selectMetricsSource(
	ctx context.Context,
	sourceType string,
	k8sC *k8s.Client,
	promConfig *promMetrics.PromConfig,
	enableFallback bool,
	requireAvailable bool,
) (metrics.MetricsSource, *promMetrics.PromMetricsSource, error) {
	switch sourceType {
	case "prometheus":
		slog.Info("connecting to metrics source", "source", "prometheus")
		promSource, err := tryPrometheus(ctx, k8sC.RESTConfig(), promConfig)
		if err == nil {
			slog.Info("metrics source ready", "source", "prometheus")
			return promSource, promSource, nil
		}
		if !enableFallback {
			slog.Error("prometheus required but unreachable", "error", err)
			return nil, nil, fmt.Errorf("prometheus not available: %w", err)
		}
		slog.Warn("prometheus unavailable, falling back to metrics-server", "error", err)
		source := k8sMetrics.NewMetricsServerSource(k8sC.Controller())
		slog.Info("metrics source ready", "source", "metrics-server", "reason", "prometheus-fallback")
		return source, nil, nil

	case "metrics-server":
		slog.Info("connecting to metrics source", "source", "metrics-server")
		if requireAvailable {
			if err := k8sC.AssertMetricsAvailable(); err != nil {
				slog.Error("metrics-server required but unavailable", "error", err)
				return nil, nil, fmt.Errorf("metrics-server not available: %w", err)
			}
		}
		source := k8sMetrics.NewMetricsServerSource(k8sC.Controller())
		slog.Info("metrics source ready", "source", "metrics-server")
		return source, nil, nil

	case "none":
		slog.Info("metrics source disabled", "source", "none")
		return nil, nil, nil

	default:
		return nil, nil, fmt.Errorf("unknown metrics source: %s", sourceType)
	}
}

func (o *ktopCmdOptions) runKtop(c *cobra.Command, _ []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration from ~/.ktop/config.yaml if present; create with defaults if absent
	appConfig, appConfigErr := config.LoadAppConfig()
	var saveConfigErr error
	if appConfig == nil && appConfigErr == nil {
		appConfig = config.DefaultAppConfig()
		saveConfigErr = config.SaveAppConfig(appConfig)
	}

	// Apply logging settings from config if not overridden by CLI flags
	if appConfig != nil {
		if !c.Flags().Changed("log-level") && appConfig.Logging.Level != "" {
			o.logLevel = appConfig.Logging.Level
		}
		if !c.Flags().Changed("log-format") && appConfig.Logging.Format != "" {
			o.logFormat = appConfig.Logging.Format
		}
	}

	logCfg := logging.Config{
		Level:  o.logLevel,
		Format: o.logFormat,
		Dest:   logging.Destination(strings.ToLower(strings.TrimSpace(o.logDest))),
	}
	if err := logging.Validate(logCfg); err != nil {
		return WrapExitError(ExitInvalidFlags, err)
	}

	// Initialize structured logging before any other work so subsequent
	// diagnostics land in ~/.ktop/ktop.log (or stderr with --log=stderr). A
	// sink creation failure is non-fatal: ktop continues with logging silenced
	// rather than letting slog fall back to stderr and corrupt TUI or JSON stdout.
	logCloser, err := logging.Init(logCfg)
	if err != nil {
		if !o.quiet {
			fmt.Fprintf(c.ErrOrStderr(), "ktop: logging disabled: %v\n", err)
		}
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	defer func() { _ = logCloser.Close() }()

	if saveConfigErr != nil {
		slog.Warn("failed to initialize default config file", "error", saveConfigErr)
	}
	if appConfigErr != nil {
		slog.Warn("failed to load ktop config file", "error", appConfigErr)
	}

	slog.Info("ktop starting",
		"version", buildinfo.Version,
		"log_level", o.logLevel,
		"log_dest", logCfg.Dest,
		"noui", o.noui,
		"metrics_source", o.metricsSource,
	)

	if o.allNamespaces {
		o.namespace = k8s.AllNamespaces
	}

	// Validate headless flags before contacting the cluster
	if o.duration < 0 {
		return WrapExitError(ExitInvalidFlags, fmt.Errorf("invalid duration %v: must be >= 0", o.duration))
	}
	if o.interval <= 0 {
		return WrapExitError(ExitInvalidFlags, fmt.Errorf("invalid interval %v: must be > 0", o.interval))
	}
	if _, err := output.NewFormatter(o.outputFormat); err != nil {
		return WrapExitError(ExitInvalidFlags, err)
	}

	warmupTimeout, err := parseWarmupTimeout(o.kubeFlags)
	if err != nil {
		return WrapExitError(ExitInvalidFlags, err)
	}

	// Apply metrics and UI appearance settings from config if not overridden by CLI flags
	if appConfig != nil {
		if !c.Flags().Changed("theme") && appConfig.UI.Theme != "" {
			o.theme = appConfig.UI.Theme
		}
		if !c.Flags().Changed("color-mode") && appConfig.UI.ColorMode != "" {
			o.colorMode = appConfig.UI.ColorMode
		}
		if !c.Flags().Changed("page") && appConfig.UI.DefaultPage != "" {
			o.page = appConfig.UI.DefaultPage
		}
		if !c.Flags().Changed("metrics-source") && appConfig.Metrics.Source != "" {
			o.metricsSource = appConfig.Metrics.Source
		}
		if !c.Flags().Changed("prometheus-scrape-interval") && appConfig.Metrics.ScrapeInterval != "" {
			o.prometheusScrapeInterval = appConfig.Metrics.ScrapeInterval
		}
		if !c.Flags().Changed("prometheus-retention") && appConfig.Metrics.Retention != "" {
			o.prometheusRetention = appConfig.Metrics.Retention
		}
		if !c.Flags().Changed("prometheus-max-samples") && appConfig.Metrics.MaxSamples > 0 {
			o.prometheusMaxSamples = appConfig.Metrics.MaxSamples
		}
		if !c.Flags().Changed("prometheus-components") && len(appConfig.Metrics.Components) > 0 {
			o.prometheusComponents = appConfig.Metrics.Components
		}
		if !c.Flags().Changed("ksm") && appConfig.Metrics.KSMMode != "" {
			o.ksmMode = appConfig.Metrics.KSMMode
		}
		if !c.Flags().Changed("ksm-endpoint") && appConfig.Metrics.KSMEndpoint != "" {
			o.ksmEndpoint = appConfig.Metrics.KSMEndpoint
		}
	}

	// Load configuration with defaults
	cfg := config.DefaultConfig()

	// Override with CLI flags
	cfg.Source.Type = o.metricsSource

	if c.Flags().Changed("prometheus-scrape-interval") {
		interval, err := time.ParseDuration(o.prometheusScrapeInterval)
		if err != nil {
			slog.Error("invalid prometheus-scrape-interval", "value", o.prometheusScrapeInterval, "error", err)
			return WrapExitError(ExitInvalidFlags, fmt.Errorf("invalid prometheus-scrape-interval: %w", err))
		}
		cfg.Prometheus.ScrapeInterval = interval
	}

	if c.Flags().Changed("prometheus-retention") {
		retention, err := time.ParseDuration(o.prometheusRetention)
		if err != nil {
			slog.Error("invalid prometheus-retention", "value", o.prometheusRetention, "error", err)
			return WrapExitError(ExitInvalidFlags, fmt.Errorf("invalid prometheus-retention: %w", err))
		}
		cfg.Prometheus.RetentionTime = retention
	}

	if c.Flags().Changed("prometheus-max-samples") {
		cfg.Prometheus.MaxSamples = o.prometheusMaxSamples
	}

	if c.Flags().Changed("prometheus-components") {
		components, err := config.ParseComponents(o.prometheusComponents)
		if err != nil {
			slog.Error("invalid prometheus-components", "value", o.prometheusComponents, "error", err)
			return WrapExitError(ExitInvalidFlags, fmt.Errorf("invalid prometheus-components: %w", err))
		}
		cfg.Prometheus.Components = components
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		return WrapExitError(ExitInvalidFlags, fmt.Errorf("invalid configuration: %w", err))
	}

	normalizedTheme, err := config.NormalizeTheme(o.theme)
	if err != nil {
		slog.Error("invalid theme", "value", o.theme, "error", err)
		return WrapExitError(ExitInvalidFlags, err)
	}
	o.theme = normalizedTheme

	normalizedColorMode, err := config.NormalizeColorMode(o.colorMode)
	if err != nil {
		slog.Error("invalid color-mode", "value", o.colorMode, "error", err)
		return WrapExitError(ExitInvalidFlags, err)
	}
	o.colorMode = normalizedColorMode

	normalizedPage, err := config.NormalizePage(o.page)
	if err != nil {
		slog.Error("invalid page", "value", o.page, "error", err)
		return WrapExitError(ExitInvalidFlags, err)
	}
	o.page = normalizedPage

	normalizedKSM, err := config.NormalizeKSMMode(o.ksmMode)
	if err != nil {
		slog.Error("invalid ksm mode", "value", o.ksmMode, "error", err)
		return WrapExitError(ExitInvalidFlags, err)
	}
	o.ksmMode = normalizedKSM

	k8sC, err := k8s.New(o.kubeFlags)
	if err != nil {
		slog.Error("kubernetes client creation failed", "error", err)
		wrapped := fmt.Errorf("ktop: failed to create Kubernetes client: %w", err)
		if isTimeoutError(err) {
			return WrapExitError(ExitCollectionTimeout, wrapped)
		}
		return WrapExitError(ExitK8sConnectError, wrapped)
	}
	slog.Info("cluster connected", "host", k8sC.RESTConfig().Host)

	// Initialize metrics source based on configuration
	// Fallback is enabled only when using default (not explicitly set)
	enableFallback := !c.Flags().Changed("metrics-source")

	promConfig := &promMetrics.PromConfig{
		Enabled:        true,
		ScrapeInterval: cfg.Prometheus.ScrapeInterval,
		RetentionTime:  cfg.Prometheus.RetentionTime,
		MaxSamples:     cfg.Prometheus.MaxSamples,
		Components:     cfg.Prometheus.Components,
	}
	if o.noui && !c.Flags().Changed("prometheus-scrape-interval") {
		if o.duration == 0 {
			if c.Flags().Changed("interval") && o.interval < promConfig.ScrapeInterval {
				promConfig.ScrapeInterval = o.interval
			} else if !c.Flags().Changed("interval") {
				// Keep single-shot headless warmup responsive while allowing two scrape cycles.
				promConfig.ScrapeInterval = 1500 * time.Millisecond
			}
		} else if o.interval < promConfig.ScrapeInterval {
			promConfig.ScrapeInterval = o.interval
		}
	}

	metricsSource, promSource, err := selectMetricsSource(ctx, cfg.Source.Type, k8sC, promConfig, enableFallback, o.noui && !enableFallback)
	if err != nil {
		if isTimeoutError(err) {
			return WrapExitError(ExitCollectionTimeout, err)
		}
		return WrapExitError(ExitMetricsUnavailable, err)
	}
	if promSource != nil {
		defer promSource.Stop()
	}

	var runner application.Runner
	runCtx := ctx
	if o.noui {
		var stopSignals context.CancelFunc
		runCtx, stopSignals = signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stopSignals()

		runner, err = headless.NewRunner(runCtx, k8sC, metricsSource, headless.Options{
			Duration:      o.duration,
			Interval:      o.interval,
			WarmupTimeout: warmupTimeout,
			OutputFormat:  o.outputFormat,
			Quiet:         o.quiet,
			Version:       buildinfo.Version,
			Out:           c.OutOrStdout(),
			ErrOut:        c.ErrOrStderr(),
		})
		if err != nil {
			if isTimeoutError(err) {
				return WrapExitError(ExitCollectionTimeout, err)
			}
			return WrapExitError(ExitK8sConnectError, err)
		}
	} else {
		// Process column options for TUI mode
		var nodeColumns []string
		var podColumns []string

		if o.showAllColumns {
			nodeColumns = slices.Clone(config.AllNodeColumns)
			podColumns = slices.Clone(config.AllPodColumns)
		} else {
			if o.nodeColumns != "" {
				nodeColumns = config.NormalizeColumns(strings.Split(o.nodeColumns, ","))
			} else if appConfig != nil && len(appConfig.UI.Overview.NodeCols) > 0 {
				nodeColumns = config.NormalizeColumns(appConfig.UI.Overview.NodeCols)
			} else {
				nodeColumns = slices.Clone(config.DefaultNodeColumns)
			}

			if o.podColumns != "" {
				podColumns = config.NormalizeColumns(strings.Split(o.podColumns, ","))
			} else if appConfig != nil && len(appConfig.UI.Overview.PodCols) > 0 {
				podColumns = config.NormalizeColumns(appConfig.UI.Overview.PodCols)
			} else {
				podColumns = slices.Clone(config.DefaultPodColumns)
			}
		}

		// Persist CLI flag overrides to ~/.ktop/config.yaml
		if c.Flags().Changed("node-columns") || c.Flags().Changed("pod-columns") {
			if err := config.SaveOverviewColumns(nodeColumns, podColumns); err != nil {
				slog.Warn("failed to persist column configuration from flags", "error", err)
			}
		}

		nodeSortCol, nodeSortAsc := parseSortFlag(o.sortNodes)
		if nodeSortCol == "MEMORY" {
			nodeSortCol = "MEM"
		}
		podSortCol, podSortAsc := parseSortFlag(o.sortPods)
		if podSortCol == "NAME" {
			podSortCol = "POD"
		}
		if podSortCol == "MEM" {
			podSortCol = "MEMORY"
		}

		runner, err = tui.NewRunner(runCtx, k8sC, metricsSource, tui.Options{
			Theme:          o.theme,
			ColorMode:      o.colorMode,
			InitialPage:    o.page,
			ShowAllColumns: o.showAllColumns,
			NodeColumns:    nodeColumns,
			PodColumns:     podColumns,
			NodeSortCol:    nodeSortCol,
			NodeSortAsc:    nodeSortAsc,
			PodSortCol:     podSortCol,
			PodSortAsc:     podSortAsc,
		})
		if err != nil {
			if isTimeoutError(err) {
				return WrapExitError(ExitCollectionTimeout, err)
			}
			if strings.Contains(err.Error(), "authorized") || strings.Contains(err.Error(), "failed to list") {
				return WrapExitError(ExitK8sConnectError, err)
			}
			return WrapExitError(ExitGeneralError, err)
		}
	}

	if err := runner.Run(runCtx); err != nil {
		slog.Error("application exited with error", "error", err)
		if isTimeoutError(err) {
			return WrapExitError(ExitCollectionTimeout, err)
		}
		return WrapExitError(ExitGeneralError, err)
	}

	return nil
}

func parseWarmupTimeout(flags *genericclioptions.ConfigFlags) (time.Duration, error) {
	const defaultTimeout = 20 * time.Second
	if flags == nil || flags.Timeout == nil {
		return defaultTimeout, nil
	}
	raw := strings.TrimSpace(*flags.Timeout)
	if raw == "" || raw == "0" {
		return defaultTimeout, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid request-timeout %q: %w", raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("invalid request-timeout %q: must be > 0", raw)
	}
	return d, nil
}

func parseSortFlag(val string) (string, bool) {
	if val == "" {
		return "", true
	}
	val = strings.TrimSpace(val)
	asc := true
	if strings.HasPrefix(val, "+") {
		val = strings.TrimPrefix(val, "+")
		asc = true
	} else if strings.HasPrefix(val, "-") {
		val = strings.TrimPrefix(val, "-")
		asc = false
	}
	col := strings.ToUpper(val)
	if col == "RESTARTS" {
		col = "RST"
	}
	return col, asc
}
