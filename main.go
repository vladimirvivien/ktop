package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"time"

	"github.com/vladimirvivien/ktop/kclient"
	"github.com/vladimirvivien/ktop/ui"
	"k8s.io/client-go/pkg/api/v1"
)

func main() {
	// create k8s config
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	fmt.Println("Using kubeconfig: ", kubeconfig)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatal(err)
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	k8s := kclient.New(clientset)
	tui := ui.New()
	if err := tui.Init(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	podsCh := make(chan []v1.Pod)
	tui.RenderPods(podsCh)
	go func() {
		for {
			pods, err := k8s.GetPodsByNS("")
			if err != nil {
				log.Fatal(err)
			}
			podsCh <- pods
			<-time.After(2 * time.Second)
		}
	}()

	tui.Open() // blocks
	defer tui.Close()

}
