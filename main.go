package main

import (
	"flag"
	"log"
	"os"

	"github.com/spf13/pflag"
	"github.com/vladimirvivien/ktop/cmd"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)

	if err := flag.Set("logtostderr", "false"); err != nil {
		log.Fatalln(err)
	}
	if err := flag.Set("alsologtostderr", "false"); err != nil {
		log.Fatalln(err)
	}
	if err := flag.Set("stderrthreshold", "fatal"); err != nil {
		log.Fatalln(err)
	}
	if err := flag.Set("v", "0"); err != nil {
		log.Fatalln(err)
	}

	flags := pflag.NewFlagSet("ktop", pflag.ExitOnError)
	pflag.CommandLine = flags

	// run standalone
	if err := cmd.NewKtopCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
