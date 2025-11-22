package prom

import (
	"context"
	"fmt"
	"log"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ExampleUsage demonstrates how to use the improved Prometheus integration
func ExampleUsage() {
	// 1. Load Kubernetes configuration
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		log.Fatalf("Error building kubeconfig: %v", err)
	}

	// 2. Create Prometheus integration
	integration := NewIntegration(kubeConfig, true)
	if !integration.IsEnabled() {
		log.Println("Prometheus integration is disabled")
		return
	}

	// 3. Start metrics collection
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := integration.Start(ctx); err != nil {
		log.Fatalf("Error starting Prometheus integration: %v", err)
	}

	// 4. Wait for initial metrics collection
	time.Sleep(5 * time.Second)

	// 5. Query some metrics
	demonstrateMetricQueries(integration)

	// 6. Show cluster summary
	demonstrateClusterSummary(integration)
}

func demonstrateMetricQueries(integration *Integration) {
	fmt.Println("=== Prometheus Metrics Queries ===")

	// Get node metrics for the first available node
	if nodes := integration.GetAvailableComponents(); len(nodes) > 0 {
		// This would require node discovery, simplified for example
		nodeMetrics, err := integration.GetNodeMetrics("minikube") // Example node name
		if err != nil {
			fmt.Printf("Error getting node metrics: %v\n", err)
		} else if nodeMetrics != nil {
			fmt.Printf("Node CPU Usage: %.2f%%\n", nodeMetrics.CPUUsagePercent)
			fmt.Printf("Node Memory Available: %d bytes\n", nodeMetrics.MemoryAvailableBytes)
			fmt.Printf("Node Network RX: %.2f bytes\n", nodeMetrics.NetworkRxBytesTotal)
		}
	}

	// Get pod metrics for a sample pod
	podMetrics, err := integration.GetPodMetrics("kube-system", "coredns-1234") // Example pod
	if err != nil {
		fmt.Printf("Error getting pod metrics: %v\n", err)
	} else if podMetrics != nil {
		fmt.Printf("Pod CPU Usage: %.2f%%\n", podMetrics.CPUUsagePercent)
		fmt.Printf("Pod Memory Usage: %d bytes\n", podMetrics.MemoryUsageBytes)
		fmt.Printf("Pod Network RX: %.2f bytes\n", podMetrics.NetworkRxBytes)
	}
}

func demonstrateClusterSummary(integration *Integration) {
	fmt.Println("\n=== Cluster Summary ===")

	summary := integration.GetClusterSummary()
	for key, value := range summary {
		fmt.Printf("%s: %v\n", key, value)
	}

	fmt.Printf("Available components: %v\n", integration.GetAvailableComponents())
	fmt.Printf("Integration healthy: %v\n", integration.IsHealthy())

	if err := integration.GetLastError(); err != nil {
		fmt.Printf("Last error: %v\n", err)
	}
}

// ExampleRESTClientUsage shows the internal RESTClient usage patterns
func ExampleRESTClientUsage(kubeConfig *rest.Config) {
	fmt.Println("=== RESTClient Usage Examples ===")

	// This demonstrates the patterns used internally by the scraper
	config := DefaultScrapeConfig()
	scraper, err := NewKubernetesScraper(kubeConfig, config)
	if err != nil {
		log.Fatalf("Error creating scraper: %v", err)
	}

	ctx := context.Background()

	// Discover targets using RESTClient-based discovery
	if err := scraper.discoverTargets(ctx); err != nil {
		log.Printf("Error discovering targets: %v", err)
		return
	}

	// Show discovered targets
	for component, targets := range scraper.targets {
		fmt.Printf("Component %s: %d targets\n", component, len(targets))
		for _, target := range targets {
			fmt.Printf("  - Path: %s", target.Path)
			if target.NodeName != "" {
				fmt.Printf(", Node: %s", target.NodeName)
			}
			if target.PodName != "" {
				fmt.Printf(", Pod: %s:%d", target.PodName, target.Port)
			}
			fmt.Println()
		}
	}
}

// ExampleIntegrationWithKtop shows how ktop controllers would use this
func ExampleIntegrationWithKtop(kubeConfig *rest.Config) {
	fmt.Println("=== Integration with ktop ===")

	// 1. Initialize Prometheus integration
	promIntegration := NewIntegration(kubeConfig, true)

	// 2. Start in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := promIntegration.Start(ctx); err != nil {
			log.Printf("Prometheus integration error: %v", err)
		}
	}()

	// 3. Use in existing ktop refresh functions
	// This would be called from k8s/client_controller.go
	refreshNodeViewWithPrometheus := func(nodeName string) {
		// Get enhanced metrics if available
		if promIntegration.IsEnabled() && promIntegration.IsHealthy() {
			nodeMetrics, err := promIntegration.GetNodeMetrics(nodeName)
			if err == nil && nodeMetrics != nil {
				fmt.Printf("Enhanced metrics for %s:\n", nodeName)
				fmt.Printf("  CPU Cores Usage: %v\n", nodeMetrics.CPUUsageByCore)
				fmt.Printf("  Load Average: %v\n", nodeMetrics.CPULoadAverage)
				fmt.Printf("  Network I/O: RX=%.0f, TX=%.0f\n",
					nodeMetrics.NetworkRxBytesTotal, nodeMetrics.NetworkTxBytesTotal)
				fmt.Printf("  Disk I/O: Read=%.0f, Write=%.0f\n",
					nodeMetrics.DiskReadBytesTotal, nodeMetrics.DiskWriteBytesTotal)
			}
		}

		// Fall back to regular metrics-server data if Prometheus not available
		fmt.Printf("Using fallback metrics for %s\n", nodeName)
	}

	// Example usage
	refreshNodeViewWithPrometheus("worker-node-1")
}
