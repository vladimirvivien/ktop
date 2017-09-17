package main

import (
	"fmt"
	"os"
	"path/filepath"
	"log"

	"k8s.io/client-go/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/vladimirvivien/ktop/kclient"
)

func main() {
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
	pods, err := k8s.GetPodsByNS("default")
	if err != nil {
		log.Fatal(err)
	}
	if len(pods) > 0 {
		fmt.Printf("There are %d pods in the cluster\n", len(pods))
		for _, pod := range pods {
			fmt.Printf("pod %s\n", pod.GetName())
		}
	} else {
		fmt.Println("No pods found!")
	}
}