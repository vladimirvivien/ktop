package main

import (
	"flag"
	"fmt"
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

	app := ui.New()
	app.WelcomeBanner()
	fmt.Println("Connecting to API server...")
	k8sClient, err := client.New(ns, time.Second*5)
	if err != nil {
		fmt.Println("Failed:", err)
		os.Exit(1)
	}
	fmt.Println("Connected to server: ", k8sClient.Config.Host)

	stopCh := make(chan struct{})
	defer close(stopCh)

	app.SetHeader(fmt.Sprintf(
		"[green]API server: [white]%s [green]namespace: [white]%s",
		k8sClient.Config.Host,
		ns,
	))

	overviewCtrl := overview.New(
		k8sClient,
		app,
		ui.PageNames[0],
	)
	go overviewCtrl.Run(app.StopChan())

	deployCtrl := deployments.New(
		k8sClient,
		app,
		ui.PageNames[1],
	)
	go deployCtrl.Run(app.StopChan())

	k8sClient.InformerFactory.Start(app.StopChan())

	app.ViewPage(0) // set default page

	if err := app.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
