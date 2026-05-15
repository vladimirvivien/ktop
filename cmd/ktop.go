package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/spf13/cobra"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/buildinfo"
	"github.com/vladimirvivien/ktop/config"
	"github.com/vladimirvivien/ktop/internal/logging"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	k8sMetrics "github.com/vladimirvivien/ktop/metrics/k8s"
	promMetrics "github.com/vladimirvivien/ktop/metrics/prom"
	"github.com/vladimirvivien/ktop/ui"
	"github.com/vladimirvivien/ktop/views/overview"
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
`
)

type ktopCmdOptions struct {
	namespace      string
	allNamespaces  bool
	context        string
	kubeconfig     string
	kubeFlags      *genericclioptions.ConfigFlags
	page           string // future use
	nodeColumns    string // comma-separated list of node columns to display
	podColumns     string // comma-separated list of pod columns to display
	showAllColumns bool   // show all columns

	// Metrics configuration
	metricsSource            string
	prometheusScrapeInterval string
	prometheusRetention      string
	prometheusMaxSamples     int
	prometheusComponents     []string

	// Logging configuration
	logLevel  string
	logFormat string
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
		Use:          usage,
		Short:        shortDesc,
		Example:      fmt.Sprintf(examples, program),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			return o.runKtop(c, args)
		},
	}
	cmd.Flags().BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "If true, display metrics for all accessible namespaces")
	cmd.Flags().StringVar(&o.nodeColumns, "node-columns", "", "Comma-separated list of node columns to display (e.g. 'NAME,CPU,MEM')")
	cmd.Flags().StringVar(&o.podColumns, "pod-columns", "", "Comma-separated list of pod columns to display (e.g. 'NAMESPACE,POD,STATUS')")
	cmd.Flags().BoolVar(&o.showAllColumns, "show-all-columns", true, "If true, show all columns (default)")

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

	// Logging flags
	cmd.Flags().StringVar(&o.logLevel, "log-level", "info",
		"Log verbosity: debug, info, warn, error")
	cmd.Flags().StringVar(&o.logFormat, "log-format", "text",
		"Log record format: text or json")

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
			return nil, nil, fmt.Errorf("prometheus not available: %v", err)
		}
		slog.Warn("prometheus unavailable, falling back to metrics-server", "error", err)
		source := k8sMetrics.NewMetricsServerSource(k8sC.Controller())
		slog.Info("metrics source ready", "source", "metrics-server", "reason", "prometheus-fallback")
		return source, nil, nil

	case "metrics-server":
		slog.Info("connecting to metrics source", "source", "metrics-server")
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

func (o *ktopCmdOptions) runKtop(c *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize structured logging before any other work so subsequent
	// diagnostics land in ~/.ktop/ktop.log. A failure here is non-fatal:
	// ktop continues with logging silenced rather than letting slog
	// fall back to stderr (which would corrupt the TUI).
	logCloser, err := logging.Init(logging.Config{
		Level:  o.logLevel,
		Format: o.logFormat,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ktop: logging disabled: %v\n", err)
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	defer func() { _ = logCloser.Close() }()

	slog.Info("ktop starting",
		"version", buildinfo.Version,
		"log_level", o.logLevel,
		"metrics_source", o.metricsSource,
	)

	if o.allNamespaces {
		o.namespace = k8s.AllNamespaces
	}

	// Load configuration with defaults
	cfg := config.DefaultConfig()

	// Override with CLI flags
	cfg.Source.Type = o.metricsSource

	if c.Flags().Changed("prometheus-scrape-interval") {
		interval, err := time.ParseDuration(o.prometheusScrapeInterval)
		if err != nil {
			slog.Error("invalid prometheus-scrape-interval", "value", o.prometheusScrapeInterval, "error", err)
			return fmt.Errorf("invalid prometheus-scrape-interval: %w", err)
		}
		cfg.Prometheus.ScrapeInterval = interval
	}

	if c.Flags().Changed("prometheus-retention") {
		retention, err := time.ParseDuration(o.prometheusRetention)
		if err != nil {
			slog.Error("invalid prometheus-retention", "value", o.prometheusRetention, "error", err)
			return fmt.Errorf("invalid prometheus-retention: %w", err)
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
			return fmt.Errorf("invalid prometheus-components: %w", err)
		}
		cfg.Prometheus.Components = components
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		return fmt.Errorf("invalid configuration: %w", err)
	}

	k8sC, err := k8s.New(o.kubeFlags)
	if err != nil {
		slog.Error("kubernetes client creation failed", "error", err)
		return fmt.Errorf("ktop: failed to create Kubernetes client: %s", err)
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

	metricsSource, promSource, err := selectMetricsSource(ctx, cfg.Source.Type, k8sC, promConfig, enableFallback)
	if err != nil {
		return err
	}
	if promSource != nil {
		defer promSource.Stop()
	}

	app := application.New(k8sC, metricsSource)

	// Connect API health tracker to the k8s controller
	k8sC.Controller().SetHealthTracker(app.GetAPIHealthTracker())

	app.WelcomeBanner()

	// Process column options
	nodeColumns := []string{}
	if o.nodeColumns != "" {
		nodeColumns = strings.Split(o.nodeColumns, ",")
		o.showAllColumns = false
	}

	podColumns := []string{}
	if o.podColumns != "" {
		podColumns = strings.Split(o.podColumns, ",")
		o.showAllColumns = false
	}

	// Create a new overview page with column options
	app.AddPage(overview.NewWithColumnOptions(app, "Overview", o.showAllColumns, nodeColumns, podColumns))

	if err := k8sC.AssertCoreAuthz(ctx); err != nil {
		slog.Error("kubernetes authorization check failed", "error", err)
		return fmt.Errorf("ktop: %s", err)
	}
	slog.Info("kubernetes authorization checks passed")

	// Check terminal height before starting TUI
	screen, err := tcell.NewScreen()
	if err == nil {
		if err := screen.Init(); err == nil {
			_, height := screen.Size()
			screen.Fini()
			if height < ui.MinTerminalHeight {
				slog.Error("terminal too small", "height", height, "min", ui.MinTerminalHeight)
				return fmt.Errorf("terminal height too small (%d rows). Minimum required: %d rows", height, ui.MinTerminalHeight)
			}
		}
	}

	// launch application
	appErr := make(chan error)
	go func() {
		appErr <- app.Run(ctx)
	}()

	select {
	case err := <-appErr:
		if err != nil {
			slog.Error("application exited with error", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
	}

	return nil
}
