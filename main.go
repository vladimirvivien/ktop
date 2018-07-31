package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/vladimirvivien/ktop/client"
	"github.com/vladimirvivien/ktop/controllers/deployments"
	"github.com/vladimirvivien/ktop/controllers/overview"
	"github.com/vladimirvivien/ktop/ui"
)

func main() {
	var ns, pg string
	flag.StringVar(&ns, "namespace", "default", "namespace")
	flag.StringVar(&pg, "page", "overview", "the default UI page to show")
	flag.Parse()

	//k8s connection setup
	k8sClient, err := client.New(ns, time.Second*5)
	if err != nil {
		log.Fatal(err)
	}
	stopCh := make(chan struct{})
	defer close(stopCh)

	app := ui.New()

	overviewCtrl := overview.New(
		k8sClient,
		app,
		ui.PageNames[0],
	)
	go overviewCtrl.Run(stopCh)

	deployCtrl := deployments.New(
		k8sClient,
		app,
		ui.PageNames[1],
	)
	go deployCtrl.Run(stopCh)

	k8sClient.InformerFactory.Start(stopCh)

	app.ViewPage(0)

	if err := app.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
