package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
	"github.com/vladimirvivien/ktop/controllers"
)

type k8s struct {
	clientset *kubernetes.Clientset
	config    *restclient.Config
	factory   informers.SharedInformerFactory
}

type screen struct {
	app         *tview.Application
	root        *tview.Flex
	header      *tview.TextView
	nodeInfo    *tview.TextView
	deployments *tview.Table
}

func main() {
	var ns string
	flag.StringVar(&ns, "namespace", "default", "namespace")
	flag.Parse()

	// k8s connection setup
	k8sClient, err := k8sCreate(ns)
	if err != nil {
		log.Fatal(err)
	}

	//  ***************** Draw UI *****************

	scrn := drawScreen()
	scrn.drawHeader(k8sClient.config.Host, ns)

	// **************** Setup Controllers *********
	stopCh := make(chan struct{})
	defer close(stopCh)

	nodeCtrl := controllers.Nodes(k8sClient.clientset, 3*time.Second)
	nodeCtrl.SyncFunc = func(nodes []*v1.Node) {
		scrn.nodeInfo.Clear()
		fmt.Fprintf(scrn.nodeInfo, "%-6s%-10s%-10s\n", "CPU", "Memory", "Pods")
		if nodes != nil {
			for _, node := range nodes {
				cpu := node.Status.Capacity.Cpu().String()
				mem := node.Status.Capacity.Memory().String()
				pods := node.Status.Capacity.Pods().String()
				fmt.Fprintf(scrn.nodeInfo, "%-6s%-10s%-10s\n", cpu, mem, pods)
			}
		}
	}
	go nodeCtrl.Run(stopCh)

	// run the app
	scrn.run()
}

func k8sCreate(namespace string) (*k8s, error) {
	// create k8s config
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	factory := informers.NewFilteredSharedInformerFactory(clientset, time.Second*3, namespace, nil)

	return &k8s{clientset: clientset, config: config, factory: factory}, nil
}

func drawScreen() *screen {
	header := tview.NewTextView().
		SetDynamicColors(true)
	header.SetBorder(true)
	fmt.Fprint(header, "loading...")

	// nodeInfo := tview.NewTextView()
	// nodeInfo.SetTitle("Cluster info").SetBorder(true)

	// deps := tview.NewTable()
	// deps.SetBorder(true)
	// deps.SetTitle("Workload")

	// flex := tview.NewFlex().SetDirection(tview.FlexRow).
	// 	AddItem(header, 3, 1, false).
	// 	AddItem(nodeInfo, 0, 1, false).
	// 	AddItem(deps, 0, 3, false)

	app := tview.NewApplication()
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' {
			app.Stop()
		}
		return event
	})

	return &screen{
		app:         app,
		root:        flex,
		header:      header,
		nodeInfo:    nodeInfo,
		deployments: deps,
	}
}

func (s *screen) run() {
	if err := s.app.SetRoot(s.root, true).Run(); err != nil {
		panic(err)
	}
}

func (s *screen) drawHeader(host, ns string) {
	s.header.Clear()
	fmt.Fprintf(s.header, "[green]API server: [white]%s (namespace: %s)", host, ns)
}
