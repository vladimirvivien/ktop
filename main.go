package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/vladimirvivien/ktop/controllers"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

type k8s struct {
	clientset *kubernetes.Clientset
	config    *restclient.Config
}

type screen struct {
	root   *tview.Flex
	header *tview.TextView
}

func main() {
	var ns string
	flag.StringVar(&ns, "namespace", "default", "namespace")
	flag.Parse()

	// k8s connection setup
	k8sClient, err := k8sCreate()
	if err != nil {
		log.Fatal(err)
	}

	//  ***************** Draw UI *****************
	app := tview.NewApplication()
	scrn := drawScreen()

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' {
			app.Stop()
		}
		return event
	})

	scrn.header.Clear()
	fmt.Fprintf(scrn.header, "[green]API server:[white]%s", "123.456.789.999")

	// **************** Setup Controllers *********
	stopCh := make(chan struct{})
	defer close(stopCh)

	nodeCtrl := controllers.Nodes(k8sClient.clientset, time.Second)
	nodeCtrl.SyncFunc = func(nodes []*v1.Node) {
		if nodes != nil {
			fmt.Println(nodes)
		}
	}
	go nodeCtrl.Run(stopCh)

	if err := app.SetRoot(scrn.root, true).Run(); err != nil {
		panic(err)
	}
}

func k8sCreate() (*k8s, error) {
	// create k8s config
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	log.Println("Using kubeconfig: ", kubeconfig)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &k8s{clientset: clientset, config: config}, nil
}

func drawScreen() *screen {
	header := tview.NewTextView().
		SetDynamicColors(true)
	header.SetBorder(true)
	fmt.Fprint(header, "loading...")

	flex := tview.NewFlex().
		AddItem(header, 0, 1, false)

	return &screen{
		header: header,
		root:   flex,
	}
}
