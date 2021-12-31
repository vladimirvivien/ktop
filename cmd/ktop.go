package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/views/overview"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var(
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
	namespace string
	allNamespaces bool
	context string
	kubeconfig string
	kubeFlags *genericclioptions.ConfigFlags
	page string // future use
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
	o.kubeFlags.AddFlags(cmd.Flags())
	return cmd
}

func (o *ktopCmdOptions) runKtop(c *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if o.allNamespaces {
		o.namespace = k8s.AllNamespaces
	}

	k8sC, err := k8s.New(o.kubeFlags)
	if err != nil {
		fmt.Printf("main: failed to create Kubernetes client: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Connected to: %s\n", k8sC.Config().Host)

	app := application.New(k8sC)
	app.WelcomeBanner()
	app.AddPage(overview.New(app, "Overview"))

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