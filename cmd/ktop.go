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
	configFile               string
	metricsSource            string
	enablePrometheus         bool
	prometheusScrapeInterval string
	prometheusRetention      string
	prometheusComponents     []string
	enhancedColumns          bool
	showTrends               bool
	showHealth               bool
	timeRange                string
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
	cmd.Flags().StringVar(&o.configFile, "config-file", "", "Path to metrics configuration file (default: $HOME/.ktop/config.yaml)")
	cmd.Flags().StringVar(&o.metricsSource, "metrics-source", "auto", "Metrics source: prometheus, metrics-server, hybrid, auto")
	cmd.Flags().BoolVar(&o.enablePrometheus, "enable-prometheus", true, "Enable Prometheus metrics collection")
	cmd.Flags().StringVar(&o.prometheusScrapeInterval, "prometheus-scrape-interval", "15s", "Prometheus scrape interval")
	cmd.Flags().StringVar(&o.prometheusRetention, "prometheus-retention", "1h", "Prometheus metrics retention time")
	cmd.Flags().StringSliceVar(&o.prometheusComponents, "prometheus-components", []string{"kubelet", "cadvisor", "apiserver"}, "Kubernetes components to scrape")
	
	// Display flags
	cmd.Flags().BoolVar(&o.enhancedColumns, "enhanced-columns", false, "Show enhanced columns with additional metrics")
	cmd.Flags().BoolVar(&o.showTrends, "show-trends", false, "Show trend indicators for metrics")
	cmd.Flags().BoolVar(&o.showHealth, "show-health", true, "Show metrics source health indicators")
	cmd.Flags().StringVar(&o.timeRange, "time-range", "15m", "Time range for trend calculation")
	
	o.kubeFlags.AddFlags(cmd.Flags())
	return cmd
}

func (o *ktopCmdOptions) runKtop(c *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if o.allNamespaces {
		o.namespace = k8s.AllNamespaces
	}

	// Load configuration
	cfg, err := config.LoadConfig(o.configFile)
	if err != nil {
		return fmt.Errorf("ktop: failed to load config: %s", err)
	}

	// Override config with command line flags if provided
	if c.Flags().Changed("metrics-source") {
		cfg.Source.Type = o.metricsSource
	}
	if c.Flags().Changed("enable-prometheus") {
		cfg.Prometheus.Enabled = o.enablePrometheus
	}
	if c.Flags().Changed("prometheus-scrape-interval") {
		cfg.Prometheus.ScrapeInterval = o.prometheusScrapeInterval
	}
	if c.Flags().Changed("prometheus-retention") {
		cfg.Prometheus.RetentionTime = o.prometheusRetention
	}
	if c.Flags().Changed("prometheus-components") {
		cfg.Prometheus.Components = o.prometheusComponents
	}
	if c.Flags().Changed("enhanced-columns") {
		cfg.Display.EnhancedColumns = o.enhancedColumns
	}
	if c.Flags().Changed("show-trends") {
		cfg.Display.ShowTrends = o.showTrends
	}
	if c.Flags().Changed("show-health") {
		cfg.Display.ShowHealth = o.showHealth
	}
	if c.Flags().Changed("time-range") {
		cfg.Display.TimeRange = o.timeRange
	}

	k8sC, err := k8s.New(o.kubeFlags)
	if err != nil {
		return fmt.Errorf("ktop: failed to create Kubernetes client: %s", err)
	}
	fmt.Printf("Connected to: %s\n", k8sC.RESTConfig().Host)

	// Create hybrid metrics controller
	fallbackTimeout, _ := time.ParseDuration(cfg.Source.FallbackTimeout)
	hybridConfig := &k8s.HybridConfig{
		PreferredSource:     cfg.Source.Type,
		FallbackEnabled:     cfg.Source.FallbackEnabled,
		FallbackTimeout:     fallbackTimeout,
		HealthCheckInterval: 30 * time.Second,
	}

	hybridController, err := k8s.NewHybridMetricsController(k8sC.RESTConfig(), hybridConfig)
	if err != nil {
		return fmt.Errorf("ktop: failed to create metrics controller: %s", err)
	}

	// Start the hybrid controller
	if err := hybridController.Start(ctx); err != nil {
		return fmt.Errorf("ktop: failed to start metrics controller: %s", err)
	}
	defer hybridController.Stop()

	// Store the controller and config in the client for access by other components
	k8sC.SetMetricsController(hybridController)
	k8sC.SetMetricsConfig(cfg)

	app := application.New(k8sC)
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
