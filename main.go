package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vladimirvivien/ktop/controllers/overview"

	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/client"
)

func main() {
	var ns, pg string
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	flag.StringVar(&kubeconfig, "kubeconfig", kubeconfig, "kubeconfig file")
	flag.StringVar(&ns, "namespace", "default", "namespace")
	flag.StringVar(&pg, "page", "overview", "the default UI page to show")
	flag.Parse()

	fmt.Println("Connecting ...")
	k8sClient, err := client.New(kubeconfig, ns)
	if err != nil {
		fmt.Println("Failed:", err)
		os.Exit(1)
	}
	fmt.Println("Connected to server: ", k8sClient.Config.Host)

	app := application.New(k8sClient)
	app.WelcomeBanner()
	fmt.Println("Connecting to API server...")
	overview.NewNodePanelCtrl(k8sClient, app).Run()
	app.ShowPage(0) // set default page

	if err := app.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
