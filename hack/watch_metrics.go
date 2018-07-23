package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/clientcmd"
	metricsV1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsclientset "k8s.io/metrics/pkg/client/clientset_generated/clientset"
)

func main() {
	var ns string
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	flag.StringVar(&ns, "namespace", "default", "namespace")
	flag.Parse()

	// bootstrap config
	fmt.Println()
	fmt.Println("Using kubeconfig: ", kubeconfig)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	// clientset, err := kubernetes.NewForConfig(config)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// api := clientset.CoreV1()

	metrics, err := metricsclientset.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	// watch future changes to PVCs
	watcher, err := metrics.MetricsV1beta1().NodeMetricses().Watch(metav1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}
	ch := watcher.ResultChan()

	fmt.Println("--- Node Metrics Watch ----")
	for event := range ch {
		metrics, ok := event.Object.(*metricsV1beta1.NodeMetrics)
		if !ok {
			log.Fatal("unexpected type")
		}

		switch event.Type {
		case watch.Added:
			log.Printf("NodeMetrics Added [%v]", metrics)

		case watch.Modified:
			log.Printf("NodeMetrics Modified [%v]", metrics)

		case watch.Deleted:
			log.Printf("NodeMetrics Deleted [%v]", metrics)
		case watch.Error:
			log.Printf("NodeMetrics Error [%v]", metrics)
		}
	}
}
