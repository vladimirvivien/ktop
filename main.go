package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	"github.com/vladimirvivien/ktop/client"
	"github.com/vladimirvivien/ktop/controllers"
	"github.com/vladimirvivien/ktop/ui"
)

type k8s struct {
	clientset        kubernetes.Interface
	config           *restclient.Config
	factory          informers.SharedInformerFactory
	metricsAvailable bool
}

func main() {
	var ns string
	flag.StringVar(&ns, "namespace", "default", "namespace")
	flag.Parse()

	//k8s connection setup
	k8sClient, err := client.New(ns, time.Second*5)
	if err != nil {
		log.Fatal(err)
	}

	appUI := ui.New()
	overviewPg := ui.NewOverviewPage(appUI.Application())
	appUI.AddPage("test", overviewPg.Root())
	appUI.Focus(overviewPg.NodeList())

	overviewCtrl := controllers.NewOverview(
		k8sClient,
		overviewPg,
	)

	stopCh := make(chan struct{})
	defer close(stopCh)
	k8sClient.InformerFactory.Start(stopCh)

	go overviewCtrl.Run(stopCh)

	if err := appUI.Start(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// // k8s connection setup
	// k8sClient, err := k8sCreate(ns)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// //  ***************** Draw UI *****************

	// scrn := drawScreen()
	// scrn.drawHeader(k8sClient.config.Host, ns)

	// // **************** Setup Controllers *********
	// stopCh := make(chan struct{})
	// defer close(stopCh)

	// nodeCtrl := controllers.Nodes(k8sClient.clientset, 3*time.Second)
	// nodeCtrl.SyncFunc = func(nodes []*v1.Node) {
	// 	scrn.nodeInfo.Clear()
	// 	fmt.Fprintf(scrn.nodeInfo, "%-6s%-10s%-10s\n", "CPU", "Memory", "Pods")
	// 	if nodes != nil {
	// 		for _, node := range nodes {
	// 			cpu := node.Status.Capacity.Cpu().String()
	// 			mem := node.Status.Capacity.Memory().String()
	// 			pods := node.Status.Capacity.Pods().String()
	// 			fmt.Fprintf(scrn.nodeInfo, "%-6s%-10s%-10s\n", cpu, mem, pods)
	// 		}
	// 	}
	// }
	// go nodeCtrl.Run(stopCh)

	// // run the app
	// scrn.run()
}
