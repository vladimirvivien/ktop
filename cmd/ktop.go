package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/config"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/metrics"
	k8sMetrics "github.com/vladimirvivien/ktop/metrics/k8s"
	promMetrics "github.com/vladimirvivien/ktop/metrics/prom"
	"github.com/vladimirvivien/ktop/views/overview"
	"k8s.io/cli-runtime/pkg/genericclioptions"
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
	namespace         string
	allNamespaces     bool
	context           string
	kubeconfig        string
	kubeFlags         *genericclioptions.ConfigFlags
	page              string // future use
	nodeColumns       string // comma-separated list of node columns to display
	podColumns        string // comma-separated list of pod columns to display
	showAllColumns    bool   // show all columns

	// Metrics configuration
	metricsSource            string
	prometheusScrapeInterval string
	prometheusRetention      string
	prometheusMaxSamples     int
	prometheusComponents     []string
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
	cmd.Flags().StringVar(&o.metricsSource, "metrics-source", "metrics-server",
		"Metrics source: 'metrics-server' (default) or 'prometheus'")
	cmd.Flags().StringVar(&o.prometheusScrapeInterval, "prometheus-scrape-interval", "15s",
		"Prometheus scrape interval (e.g., 10s, 30s, 1m)")
	cmd.Flags().StringVar(&o.prometheusRetention, "prometheus-retention", "1h",
		"Prometheus metrics retention time (e.g., 30m, 1h, 2h)")
	cmd.Flags().IntVar(&o.prometheusMaxSamples, "prometheus-max-samples", 10000,
		"Maximum samples per time series")
	cmd.Flags().StringSliceVar(&o.prometheusComponents, "prometheus-components",
		[]string{"kubelet", "cadvisor"},
		"Kubernetes components to scrape (comma-separated: kubelet,cadvisor,apiserver,etcd,scheduler,controller-manager,kube-proxy)")

	o.kubeFlags.AddFlags(cmd.Flags())
	return cmd
}

func (o *ktopCmdOptions) runKtop(c *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
			return fmt.Errorf("invalid prometheus-scrape-interval: %w", err)
		}
		cfg.Prometheus.ScrapeInterval = interval
	}

	if c.Flags().Changed("prometheus-retention") {
		retention, err := time.ParseDuration(o.prometheusRetention)
		if err != nil {
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
			return fmt.Errorf("invalid prometheus-components: %w", err)
		}
		cfg.Prometheus.Components = components
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	k8sC, err := k8s.New(o.kubeFlags)
	if err != nil {
		return fmt.Errorf("ktop: failed to create Kubernetes client: %s", err)
	}
	fmt.Printf("Connected to: %s\n", k8sC.RESTConfig().Host)

	// Initialize metrics source based on configuration
	var metricsSource metrics.MetricsSource

	switch cfg.Source.Type {
	case "metrics-server":
		fmt.Println("Using metrics source: Metrics Server")
		// MetricsServerSource uses the existing k8s.Controller
		// It already has graceful fallback to requests/limits built-in
		controller := k8sC.Controller()
		metricsSource = k8sMetrics.NewMetricsServerSource(controller)

	case "prometheus":
		fmt.Println("Using metrics source: Prometheus")

		// Create Prometheus configuration
		promConfig := &promMetrics.PromConfig{
			Enabled:        true,
			ScrapeInterval: cfg.Prometheus.ScrapeInterval,
			RetentionTime:  cfg.Prometheus.RetentionTime,
			MaxSamples:     cfg.Prometheus.MaxSamples,
			Components:     cfg.Prometheus.Components,
		}

		// Create Prometheus metrics source
		promSource, err := promMetrics.NewPromMetricsSource(k8sC.RESTConfig(), promConfig)
		if err != nil {
			return fmt.Errorf("failed to create prometheus source: %w", err)
		}

		// Start Prometheus collection
		if err := promSource.Start(ctx); err != nil {
			return fmt.Errorf("failed to start prometheus collection: %w", err)
		}
		defer promSource.Stop()

		metricsSource = promSource

		fmt.Println("Warning: Prometheus metrics source is not yet fully integrated with the UI")
		fmt.Println("         Currently using metrics-server fallback for display")

	default:
		return fmt.Errorf("unknown metrics source: %s", cfg.Source.Type)
	}

	app := application.New(k8sC, metricsSource)
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
		return fmt.Errorf("ktop: %s", err)
	}

	// launch application
	appErr := make(chan error)
	go func() {
		appErr <- app.Run(ctx)
	}()

	select {
	case err := <-appErr:
		if err != nil {
			fmt.Printf("app error: %s\n", err)
			os.Exit(1)
		}
	case <-ctx.Done():
	}

	return nil
}
