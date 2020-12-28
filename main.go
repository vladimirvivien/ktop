package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/vladimirvivien/ktop/application"
	"github.com/vladimirvivien/ktop/k8s"
	"github.com/vladimirvivien/ktop/views/overview"
)

func main() {
	var ns, pg string
	flag.StringVar(&ns, "namespace", "default", "namespace")
	flag.StringVar(&pg, "page", "overview", "the default UI page to show")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fmt.Println("Connecting ...")
	k8sClient, err := k8s.New(ctx, ns)
	if err != nil {
		fmt.Println("Failed:", err)
		os.Exit(1)
	}
	fmt.Println("Connected to server: ", k8sClient.Config().Host)


	app := application.New(k8sClient)
	app.WelcomeBanner()
	app.AddPanel(overview.New(k8sClient, "Overview", app.Refresh))
	app.Init()
	//app.ShowPanel(0) // set default page

	// start client
	if err := k8sClient.Start(); err != nil {
		fmt.Println("client: failed to start:", err)
		os.Exit(1)
	}

	if err := app.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
