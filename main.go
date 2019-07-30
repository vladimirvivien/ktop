package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/vladimirvivien/ktop/controllers/overview"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/vladimirvivien/ktop/client"
	topctx "github.com/vladimirvivien/ktop/context"
	"github.com/vladimirvivien/ktop/ui"
)

func main() {
	var ns, pg string
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	flag.StringVar(&kubeconfig, "kubeconfig", kubeconfig, "kubeconfig file")
	flag.StringVar(&ns, "namespace", "default", "namespace")
	flag.StringVar(&pg, "page", "overview", "the default UI page to show")
	flag.Parse()

	app := ui.New()
	app.WelcomeBanner()
	fmt.Println("Connecting to API server...")

	k8sClient, err := client.New(kubeconfig, ns, time.Second*5)
	if err != nil {
		fmt.Println("Failed:", err)
		os.Exit(1)
	}
	fmt.Println("Connected to server: ", k8sClient.Config.Host)

	stopCh := make(chan struct{})
	defer close(stopCh)

	ctx := topctx.WithK8sInterface(context.Background(), k8sClient.Clientset)
	ctx = topctx.WithNamespace(ctx, ns)
	ctx = topctx.WithIsMetricsAvailable(ctx, k8sClient.MetricsAPIAvailable)

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	dynclient := dynamic.NewForConfigOrDie(config)
	fac := dynamicinformer.NewDynamicSharedInformerFactory(dynclient, 0)
	overview.NewNodePanelCtrl(ctx, fac, app).Start()

	fac.Start(app.StopChan())
	if synced := fac.WaitForCacheSync(stopCh); !synced[client.NodesResource] {
		log.Fatalf("informer for %s hasn't synced", client.NodesResource)
	}

	app.SetHeader(fmt.Sprintf(
		"[green]API server: [white]%s [green]namespace: [white]%s",
		k8sClient.Config.Host,
		ns,
	))

	// overviewCtrl := overview.New(
	// 	k8sClient,
	// 	app,
	// 	ui.PageNames[0],
	// )
	// go overviewCtrl.Run(app.StopChan())

	// deployCtrl := deployments.New(
	// 	k8sClient,
	// 	app,
	// 	ui.PageNames[1],
	// )
	// go deployCtrl.Run(app.StopChan())

	// k8sClient.InformerFactory.Start(app.StopChan())

	app.ViewPage(0) // set default page

	if err := app.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
